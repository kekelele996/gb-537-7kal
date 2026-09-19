package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"
	"pki-certificate-rollover-impact/backend/internal/algorithm"
	"pki-certificate-rollover-impact/backend/internal/constants"
	"pki-certificate-rollover-impact/backend/internal/dto"
	"pki-certificate-rollover-impact/backend/internal/model"
	"pki-certificate-rollover-impact/backend/internal/repository"
	"pki-certificate-rollover-impact/backend/internal/util"
)

var (
	reviewerActor = util.Actor{UserID: 9, Username: "reviewer", Role: string(constants.RoleSecurityReviewer)}
	secondReviewer = util.Actor{UserID: 10, Username: "reviewer-two", Role: string(constants.RoleSecurityReviewer)}
	creatorActor  = util.Actor{UserID: 7, Username: "operator", Role: string(constants.RolePKIOperator)}
)

func gateScenario(t *testing.T, db *gorm.DB, name, state string, replayVerified bool, offset time.Duration) model.RolloverScenario {
	t.Helper()
	scenario := minimalScenario(t, db, name, "gate-hash-"+name, "gate-key-"+name, state, 7, offset)
	affected := []algorithm.AffectedService{
		{ServiceID: 11, ServiceCode: "PAYMENTS-API", Criticality: "critical", At: scenario.SimulationTime, Reason: "trust path broken after overlap"},
		{ServiceID: 11, ServiceCode: "PAYMENTS-API", Criticality: "critical", At: scenario.SimulationTime.Add(time.Hour), Reason: "trust path broken at simulation time"},
		{ServiceID: 12, ServiceCode: "ORDER-WORKER", Criticality: "high", At: scenario.SimulationTime, Reason: "upstream dependency unreachable"},
	}
	raw, err := json.Marshal(affected)
	if err != nil {
		t.Fatal(err)
	}
	scenario.AffectedServicesJSON = string(raw)
	scenario.ReplayVerified = replayVerified
	return scenario
}

func newGateServices(db *gorm.DB) (*RolloverScenarioService, *ImpactSignoffService) {
	scenarios := repository.NewRolloverScenarioRepository(db)
	signoffs := repository.NewImpactSignoffRepository(db)
	audits := repository.NewAuditRepository(db)
	transactions := repository.NewTransactionManager(db)
	return NewRolloverScenarioService(scenarios, signoffs, nil, nil, nil, audits, transactions), NewImpactSignoffService(scenarios, signoffs, audits, transactions)
}

func gateTodos(t *testing.T, err error) []dto.SignoffTodo {
	t.Helper()
	var apiErr *util.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != util.CodeSignoffGate {
		t.Fatalf("got %#v, want 409 %s", err, util.CodeSignoffGate)
	}
	details, ok := apiErr.Details.(map[string]any)
	if !ok {
		t.Fatalf("gate error carries no details: %#v", apiErr.Details)
	}
	todos, ok := details["todos"].([]dto.SignoffTodo)
	if !ok || len(todos) == 0 {
		t.Fatalf("gate error carries no todos: %#v", details)
	}
	return todos
}

func TestVerifyRejectedWithTodosWhenSignoffMissing(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := persistScenario(t, db, gateScenario(t, db, "missing-signoff", "executing", true, 0))
	scenarios, _ := newGateServices(db)

	_, err := scenarios.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: string(constants.ScenarioVerified)}, reviewerActor, "request-gate-missing")
	todos := gateTodos(t, err)
	if len(todos) != 1 || todos[0].Type != "signoff_missing" || todos[0].ServiceID != 11 || todos[0].ServiceCode != "PAYMENTS-API" {
		t.Fatalf("unexpected todos: %#v", todos)
	}
	stored, loadErr := repository.NewRolloverScenarioRepository(db).GetByID(context.Background(), scenario.ID, false)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if stored.ScenarioState != "executing" || stored.VerifiedBy != nil {
		t.Fatalf("rejected review must not take effect: state=%s verified_by=%v", stored.ScenarioState, stored.VerifiedBy)
	}
}

func TestVerifyRejectedWhenReplayNotPassed(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := persistScenario(t, db, gateScenario(t, db, "replay-missing", "executing", false, 0))
	scenarios, signoffs := newGateServices(db)
	if _, err := signoffs.Register(context.Background(), scenario.ID, dto.CreateImpactSignoffRequest{ServiceID: 11, Disposition: "payments cutover rehearsed"}, reviewerActor, "request-signoff"); err != nil {
		t.Fatal(err)
	}

	_, err := scenarios.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: string(constants.ScenarioVerified)}, reviewerActor, "request-gate-replay")
	todos := gateTodos(t, err)
	if len(todos) != 1 || todos[0].Type != "replay_required" {
		t.Fatalf("unexpected todos: %#v", todos)
	}
}

func TestVerifyAppliesSignoffAndConclusionAtOnce(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := persistScenario(t, db, gateScenario(t, db, "gate-satisfied", "executing", true, 0))
	scenarios, signoffs := newGateServices(db)
	gate, err := signoffs.Register(context.Background(), scenario.ID, dto.CreateImpactSignoffRequest{ServiceID: 11, Disposition: "payments cutover rehearsed"}, reviewerActor, "request-signoff")
	if err != nil {
		t.Fatal(err)
	}
	if !gate.GateSatisfied || len(gate.Todos) != 0 {
		t.Fatalf("gate should be satisfied after signing: %#v", gate.Todos)
	}

	verified, err := scenarios.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: string(constants.ScenarioVerified)}, reviewerActor, "request-verify")
	if err != nil {
		t.Fatal(err)
	}
	if verified.ScenarioState != "verified" || verified.VerifiedBy == nil || *verified.VerifiedBy != reviewerActor.UserID {
		t.Fatalf("unexpected verified scenario: %#v", verified)
	}
	stored, err := repository.NewImpactSignoffRepository(db).ListByScenario(context.Background(), scenario.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].AppliedAt == nil {
		t.Fatalf("sign-off must take effect with the review conclusion: %#v", stored)
	}
	gate, err = signoffs.GateStatus(context.Background(), scenario.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !gate.GateSatisfied || gate.Signoffs[0].AppliedAt == nil || gate.Signoffs[0].SignedByName != "reviewer" {
		t.Fatalf("gate status inconsistent after verify: %#v", gate)
	}

	_, err = scenarios.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: string(constants.ScenarioVerified)}, secondReviewer, "request-verify-again")
	var apiErr *util.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
		t.Fatalf("repeated verify must fail once concluded, got %#v", err)
	}
}

func TestScenarioCreatorCannotRegisterSignoff(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := persistScenario(t, db, gateScenario(t, db, "creator-signoff", "executing", true, 0))
	_, signoffs := newGateServices(db)

	_, err := signoffs.Register(context.Background(), scenario.ID, dto.CreateImpactSignoffRequest{ServiceID: 11, Disposition: "self approval attempt"}, creatorActor, "request-self-signoff")
	var apiErr *util.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != util.CodeReviewerConflict {
		t.Fatalf("got %#v, want 409 %s", err, util.CodeReviewerConflict)
	}
}

func TestSignoffKeepsOnlyLatestPerService(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := persistScenario(t, db, gateScenario(t, db, "latest-signoff", "ready", true, 0))
	_, signoffs := newGateServices(db)

	if _, err := signoffs.Register(context.Background(), scenario.ID, dto.CreateImpactSignoffRequest{ServiceID: 11, Disposition: "first disposition"}, reviewerActor, "request-first"); err != nil {
		t.Fatal(err)
	}
	gate, err := signoffs.Register(context.Background(), scenario.ID, dto.CreateImpactSignoffRequest{ServiceID: 11, Disposition: "updated disposition"}, secondReviewer, "request-second")
	if err != nil {
		t.Fatal(err)
	}
	if len(gate.Signoffs) != 1 {
		t.Fatalf("same service must keep one sign-off, got %d", len(gate.Signoffs))
	}
	latest := gate.Signoffs[0]
	if latest.Disposition != "updated disposition" || latest.SignedBy != secondReviewer.UserID || latest.SignedByName != "reviewer-two" {
		t.Fatalf("latest registration must win: %#v", latest)
	}
	reloaded, err := signoffs.GateStatus(context.Background(), scenario.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Signoffs) != 1 || reloaded.Signoffs[0].Disposition != "updated disposition" || reloaded.Signoffs[0].SignedByName != "reviewer-two" {
		t.Fatalf("reloaded gate must match latest registration: %#v", reloaded.Signoffs)
	}
}

func TestSignoffRejectsServiceOutsideCriticalImpact(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := persistScenario(t, db, gateScenario(t, db, "non-critical", "executing", true, 0))
	_, signoffs := newGateServices(db)

	for _, serviceID := range []uint{12, 999} {
		_, err := signoffs.Register(context.Background(), scenario.ID, dto.CreateImpactSignoffRequest{ServiceID: serviceID, Disposition: "not required"}, reviewerActor, fmt.Sprintf("request-%d", serviceID))
		var apiErr *util.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity {
			t.Fatalf("service %d: got %#v, want 422", serviceID, err)
		}
	}
}

func TestSignoffClosedOutsideReviewWindow(t *testing.T) {
	db := newScenarioTestDB(t)
	draft := persistScenario(t, db, gateScenario(t, db, "draft-gate", "draft", false, 0))
	verified := persistScenario(t, db, gateScenario(t, db, "verified-gate", "verified", true, time.Minute))
	_, signoffs := newGateServices(db)

	for _, scenario := range []model.RolloverScenario{draft, verified} {
		_, err := signoffs.Register(context.Background(), scenario.ID, dto.CreateImpactSignoffRequest{ServiceID: 11, Disposition: "too late"}, reviewerActor, "request-"+scenario.Name)
		var apiErr *util.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != util.CodeStateTransition {
			t.Fatalf("state %s: got %#v, want 409 %s", scenario.ScenarioState, err, util.CodeStateTransition)
		}
	}
}
