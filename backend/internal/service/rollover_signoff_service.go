package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"gorm.io/gorm"
	"pki-certificate-rollover-impact/backend/internal/algorithm"
	"pki-certificate-rollover-impact/backend/internal/constants"
	"pki-certificate-rollover-impact/backend/internal/dto"
	"pki-certificate-rollover-impact/backend/internal/model"
	"pki-certificate-rollover-impact/backend/internal/util"
)

// reviewGate captures everything the review gate needs before a scenario may
// move from executing to verified: the critical affected services that demand
// a disposition note, the latest sign-off per service, and whether replaying
// the frozen input still reproduces the stored historical result.
type reviewGate struct {
	required      []algorithm.AffectedService
	signoffs      []model.RolloverSignoff
	replayPassed  bool
	replayMessage string
}

func (gate reviewGate) pending() []dto.RolloverGateTodo {
	signed := map[uint]bool{}
	for _, signoff := range gate.signoffs {
		signed[signoff.ServiceID] = true
	}
	todos := []dto.RolloverGateTodo{}
	for _, service := range gate.required {
		if !signed[service.ServiceID] {
			todos = append(todos, dto.RolloverGateTodo{Kind: "signoff", ServiceID: service.ServiceID, ServiceCode: service.ServiceCode, Message: "critical affected service " + service.ServiceCode + " is missing a disposition sign-off"})
		}
	}
	if !gate.replayPassed {
		message := gate.replayMessage
		if message == "" {
			message = "historical result replay did not pass"
		}
		todos = append(todos, dto.RolloverGateTodo{Kind: "replay", Message: message})
	}
	return todos
}

// criticalAffectedServices deduplicates the simulated affected services down
// to the critical ones that each require exactly one disposition sign-off.
func criticalAffectedServices(affectedJSON string) []algorithm.AffectedService {
	affected := []algorithm.AffectedService{}
	_ = json.Unmarshal([]byte(affectedJSON), &affected)
	seen := map[uint]bool{}
	required := []algorithm.AffectedService{}
	for _, item := range affected {
		if item.Criticality != "critical" || seen[item.ServiceID] {
			continue
		}
		seen[item.ServiceID] = true
		required = append(required, item)
	}
	sort.Slice(required, func(i, j int) bool { return required[i].ServiceID < required[j].ServiceID })
	return required
}

// replayMatches re-runs the deterministic simulation over the frozen input and
// reports whether it still reproduces the stored historical result.
func replayMatches(scenario model.RolloverScenario) (bool, error) {
	snapshot, err := algorithm.DecodeSnapshot(scenario.InputSnapshot)
	if err != nil {
		return false, err
	}
	result, err := algorithm.Simulate(snapshot)
	if err != nil {
		return false, err
	}
	affectedJSON, err := encode(result.AffectedServices)
	if err != nil {
		return false, err
	}
	pathsJSON, err := encode(result.BrokenPaths)
	if err != nil {
		return false, err
	}
	evidenceJSON, err := encode(result.Evidence)
	if err != nil {
		return false, err
	}
	return affectedJSON == scenario.AffectedServicesJSON && pathsJSON == scenario.BrokenPathsJSON && evidenceJSON == scenario.PathEvidenceJSON && result.Explanation == scenario.Explanation, nil
}

func (s *RolloverScenarioService) evaluateReviewGate(ctx context.Context, scenario model.RolloverScenario) (reviewGate, error) {
	gate := reviewGate{required: criticalAffectedServices(scenario.AffectedServicesJSON)}
	signoffs, err := s.signoffs.ListByScenario(ctx, scenario.ID)
	if err != nil {
		return gate, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load rollover sign-offs", err)
	}
	gate.signoffs = signoffs
	passed, replayErr := replayMatches(scenario)
	gate.replayPassed = passed
	if replayErr != nil {
		gate.replayMessage = "historical result replay could not be reproduced"
	}
	return gate, nil
}

func (s *RolloverScenarioService) reviewGateResponse(ctx context.Context, scenario model.RolloverScenario) (dto.RolloverReviewGateResponse, error) {
	gate, err := s.evaluateReviewGate(ctx, scenario)
	if err != nil {
		return dto.RolloverReviewGateResponse{}, err
	}
	return dto.NewRolloverReviewGateResponse(scenario, gate.required, gate.signoffs, gate.pending(), gate.replayPassed), nil
}

func (s *RolloverScenarioService) ReviewGate(ctx context.Context, id uint) (dto.RolloverReviewGateResponse, error) {
	scenario, err := s.scenarios.GetByID(ctx, id, false)
	if err != nil {
		return dto.RolloverReviewGateResponse{}, util.NotFound("rollover scenario")
	}
	return s.reviewGateResponse(ctx, scenario)
}

func (s *RolloverScenarioService) RegisterSignoff(ctx context.Context, id, serviceID uint, request dto.UpsertRolloverSignoffRequest, actor util.Actor, requestID string) (dto.RolloverReviewGateResponse, error) {
	if err := validateRequest(request); err != nil {
		return dto.RolloverReviewGateResponse{}, err
	}
	note := strings.TrimSpace(request.Note)
	if len(note) < 4 {
		return dto.RolloverReviewGateResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "disposition note must contain at least 4 non-blank characters")
	}
	scenario, err := s.scenarios.GetByID(ctx, id, false)
	if err != nil {
		return dto.RolloverReviewGateResponse{}, util.NotFound("rollover scenario")
	}
	switch constants.ScenarioState(scenario.ScenarioState) {
	case constants.ScenarioSimulated, constants.ScenarioReady, constants.ScenarioExecuting:
	default:
		return dto.RolloverReviewGateResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "sign-off is open only after simulation and before the review concludes")
	}
	if !scenario.ReviewerSeparated(actor.UserID) {
		return dto.RolloverReviewGateResponse{}, util.NewError(http.StatusConflict, util.CodeReviewerConflict, "scenario creator cannot sign off their own simulation")
	}
	required := criticalAffectedServices(scenario.AffectedServicesJSON)
	var target *algorithm.AffectedService
	for index := range required {
		if required[index].ServiceID == serviceID {
			target = &required[index]
			break
		}
	}
	if target == nil {
		return dto.RolloverReviewGateResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "service is not listed as a critical affected service of this scenario")
	}
	now := s.now()
	signoff := model.RolloverSignoff{ScenarioID: id, ServiceID: serviceID, ServiceCode: target.ServiceCode, Criticality: target.Criticality, Note: note, SignedBy: actor.UserID, SignedByName: actor.Username, CreatedAt: now, UpdatedAt: now}
	err = s.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		var before any
		previous, findErr := s.signoffs.FindByScenarioAndService(txCtx, id, serviceID)
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load existing rollover sign-off", findErr)
		}
		if findErr == nil {
			before = previous
		}
		if upsertErr := s.signoffs.Upsert(txCtx, &signoff); upsertErr != nil {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to register rollover sign-off", upsertErr)
		}
		return recordAudit(txCtx, s.audits, actor, requestID, "rollover_scenario", id, "signoff_register", before, signoff, scenario.InputHash, scenario.AlgorithmVersion, &scenario.SimulationTime, 0, fmt.Sprintf("disposition sign-off for %s", signoff.ServiceCode))
	})
	if err != nil {
		return dto.RolloverReviewGateResponse{}, err
	}
	return s.reviewGateResponse(ctx, scenario)
}
