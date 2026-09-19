package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"pki-certificate-rollover-impact/backend/internal/algorithm"
	"pki-certificate-rollover-impact/backend/internal/constants"
	"pki-certificate-rollover-impact/backend/internal/dto"
	"pki-certificate-rollover-impact/backend/internal/model"
	"pki-certificate-rollover-impact/backend/internal/repository"
	"pki-certificate-rollover-impact/backend/internal/util"
)

// signoffOpenStates are the scenario states in which reviewers may register
// impact dispositions: the gate guards the step before review concludes.
var signoffOpenStates = map[string]bool{
	string(constants.ScenarioSimulated): true,
	string(constants.ScenarioReady):     true,
	string(constants.ScenarioExecuting): true,
}

type ImpactSignoffService struct {
	scenarios    repository.RolloverScenarioRepository
	signoffs     repository.ImpactSignoffRepository
	audits       repository.AuditRepository
	transactions repository.TransactionManager
	now          func() time.Time
}

func NewImpactSignoffService(scenarios repository.RolloverScenarioRepository, signoffs repository.ImpactSignoffRepository, audits repository.AuditRepository, transactions repository.TransactionManager) *ImpactSignoffService {
	return &ImpactSignoffService{scenarios: scenarios, signoffs: signoffs, audits: audits, transactions: transactions, now: func() time.Time { return time.Now().UTC() }}
}

// criticalAffectedServices extracts the unique critical affected services the
// frozen simulation listed; each of them requires one disposition sign-off.
func criticalAffectedServices(scenario model.RolloverScenario) []dto.CriticalService {
	affected := []algorithm.AffectedService{}
	_ = json.Unmarshal([]byte(scenario.AffectedServicesJSON), &affected)
	seen := map[uint]bool{}
	required := []dto.CriticalService{}
	for _, item := range affected {
		if item.Criticality != "critical" || item.ServiceID == 0 || seen[item.ServiceID] {
			continue
		}
		seen[item.ServiceID] = true
		required = append(required, dto.CriticalService{ServiceID: item.ServiceID, ServiceCode: item.ServiceCode, Criticality: item.Criticality})
	}
	sort.Slice(required, func(i, j int) bool { return required[i].ServiceID < required[j].ServiceID })
	return required
}

// evaluateSignoffGate lists every outstanding todo before review may conclude:
// one disposition per critical affected service and a passing history replay.
func evaluateSignoffGate(scenario model.RolloverScenario, signoffs []model.ImpactSignoff) []dto.SignoffTodo {
	signed := map[uint]bool{}
	for _, signoff := range signoffs {
		signed[signoff.ServiceID] = true
	}
	todos := []dto.SignoffTodo{}
	for _, service := range criticalAffectedServices(scenario) {
		if !signed[service.ServiceID] {
			todos = append(todos, dto.SignoffTodo{Type: "signoff_missing", ServiceID: service.ServiceID, ServiceCode: service.ServiceCode, Message: "critical affected service " + service.ServiceCode + " is missing a disposition sign-off"})
		}
	}
	if !scenario.ReplayVerified {
		todos = append(todos, dto.SignoffTodo{Type: "replay_required", Message: "historical result replay has not passed for the frozen evidence"})
	}
	return todos
}

func buildGateResponse(scenario model.RolloverScenario, signoffs []model.ImpactSignoff) dto.ImpactSignoffGateResponse {
	responses := make([]dto.ImpactSignoffResponse, 0, len(signoffs))
	for _, signoff := range signoffs {
		responses = append(responses, dto.NewImpactSignoffResponse(signoff))
	}
	todos := evaluateSignoffGate(scenario, signoffs)
	return dto.ImpactSignoffGateResponse{ScenarioID: scenario.ID, ScenarioState: scenario.ScenarioState, RequiredServices: criticalAffectedServices(scenario), Signoffs: responses, Todos: todos, ReplayVerified: scenario.ReplayVerified, GateSatisfied: len(todos) == 0}
}

func (s *ImpactSignoffService) GateStatus(ctx context.Context, scenarioID uint) (dto.ImpactSignoffGateResponse, error) {
	scenario, err := s.scenarios.GetByID(ctx, scenarioID, false)
	if err != nil {
		return dto.ImpactSignoffGateResponse{}, util.NotFound("rollover scenario")
	}
	signoffs, err := s.signoffs.ListByScenario(ctx, scenarioID)
	if err != nil {
		return dto.ImpactSignoffGateResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load impact sign-offs", err)
	}
	return buildGateResponse(scenario, signoffs), nil
}

// CreatedBy exposes the scenario creator so the handler can answer a creator
// who lacks the verify permission with the precise separation conflict.
func (s *ImpactSignoffService) CreatedBy(ctx context.Context, scenarioID uint) (uint, error) {
	scenario, err := s.scenarios.GetByID(ctx, scenarioID, false)
	if err != nil {
		return 0, util.NotFound("rollover scenario")
	}
	return scenario.CreatedBy, nil
}

func (s *ImpactSignoffService) Register(ctx context.Context, scenarioID uint, request dto.CreateImpactSignoffRequest, actor util.Actor, requestID string) (dto.ImpactSignoffGateResponse, error) {
	if err := validateRequest(request); err != nil {
		return dto.ImpactSignoffGateResponse{}, err
	}
	scenario, err := s.scenarios.GetByID(ctx, scenarioID, false)
	if err != nil {
		return dto.ImpactSignoffGateResponse{}, util.NotFound("rollover scenario")
	}
	if !signoffOpenStates[scenario.ScenarioState] {
		return dto.ImpactSignoffGateResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "impact sign-off is only open while the scenario is simulated, ready, or executing")
	}
	if !scenario.ReviewerSeparated(actor.UserID) {
		return dto.ImpactSignoffGateResponse{}, util.NewError(http.StatusConflict, util.CodeReviewerConflict, "scenario creator cannot sign off impact dispositions")
	}
	var target *dto.CriticalService
	for _, service := range criticalAffectedServices(scenario) {
		if service.ServiceID == request.ServiceID {
			matched := service
			target = &matched
			break
		}
	}
	if target == nil {
		return dto.ImpactSignoffGateResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "service is not a critical affected service of this scenario")
	}
	now := s.now()
	signoff := model.ImpactSignoff{ScenarioID: scenarioID, ServiceID: target.ServiceID, ServiceCode: target.ServiceCode, Disposition: strings.TrimSpace(request.Disposition), SignedBy: actor.UserID, SignedByName: actor.Username, CreatedAt: now, UpdatedAt: now}
	err = s.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		previous, findErr := s.signoffs.FindByScenarioAndService(txCtx, scenarioID, target.ServiceID)
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to read existing impact sign-off", findErr)
		}
		if upsertErr := s.signoffs.Upsert(txCtx, &signoff); upsertErr != nil {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to register impact sign-off", upsertErr)
		}
		var before any
		if findErr == nil {
			before = previous
		}
		return recordAudit(txCtx, s.audits, actor, requestID, "rollover_scenario", scenarioID, "impact_signoff", before, signoff, scenario.InputHash, scenario.AlgorithmVersion, &scenario.SimulationTime, 0, "disposition signed for critical service "+target.ServiceCode)
	})
	if err != nil {
		return dto.ImpactSignoffGateResponse{}, err
	}
	return s.GateStatus(ctx, scenarioID)
}
