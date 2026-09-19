package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"
	"pki-certificate-rollover-impact/backend/internal/algorithm"
	"pki-certificate-rollover-impact/backend/internal/dto"
	"pki-certificate-rollover-impact/backend/internal/model"
	"pki-certificate-rollover-impact/backend/internal/repository"
	"pki-certificate-rollover-impact/backend/internal/util"
)

// gateTestScenario freezes a snapshot whose only critical service loses its
// trust path once the overlap window closes, then stores the simulated result
// so the review gate has exactly one critical affected service to demand.
func gateTestScenario(t *testing.T, db *gorm.DB, state string, createdBy uint, offset time.Duration) model.RolloverScenario {
	t.Helper()
	now := time.Date(2032, 6, 1, 12, 0, 0, 0, time.UTC).Add(offset)
	snapshot := algorithm.NewSnapshot(
		algorithm.ScenarioConfig{Name: "gate", OldAnchorID: 1, NewAnchorID: 2, OverlapStart: now.Add(time.Hour), OverlapEnd: now.Add(2 * time.Hour), CandidateChainIDs: []uint{1}, SimulationTime: now.Add(90 * time.Minute)},
		[]algorithm.AnchorSnapshot{{ID: 1, Code: "OLD", State: "valid", NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour)}, {ID: 2, Code: "NEW", State: "valid", NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour)}},
		[]algorithm.ChainSnapshot{{ID: 1, Code: "CHAIN", AnchorID: 1, LeafSubject: "CN=leaf", ValidFrom: now.Add(-time.Hour), ValidTo: now.Add(24 * time.Hour), State: "validated", ValidationValid: true}},
		[]algorithm.ServiceSnapshot{{ID: 1, Code: "SERVICE", ChainID: 1, TrustAnchorIDs: []uint{1}, Criticality: "critical", State: "active"}},
	)
	result, err := algorithm.Simulate(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AffectedServices) == 0 {
		t.Fatal("gate fixture must produce a critical affected service")
	}
	snapshotJSON, _ := snapshot.Canonical()
	hash, _ := snapshot.Hash()
	affectedJSON, _ := encode(result.AffectedServices)
	pathsJSON, _ := encode(result.BrokenPaths)
	evidenceJSON, _ := encode(result.Evidence)
	base := minimalScenario(t, db, "gate", hash, "", state, createdBy, offset)
	base.InputSnapshot = snapshotJSON
	base.AffectedServicesJSON = affectedJSON
	base.BrokenPathsJSON = pathsJSON
	base.PathEvidenceJSON = evidenceJSON
	base.Explanation = result.Explanation
	return persistScenario(t, db, base)
}

func newGateService(db *gorm.DB, audits repository.AuditRepository) *RolloverScenarioService {
	return NewRolloverScenarioService(repository.NewRolloverScenarioRepository(db), nil, nil, nil, repository.NewRolloverSignoffRepository(db), audits, repository.NewTransactionManager(db))
}

func gateReviewer() util.Actor {
	return util.Actor{UserID: 9, Username: "reviewer", Role: "security_reviewer"}
}

func TestReviewGateRejectsVerifyWhenSignoffMissing(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := gateTestScenario(t, db, "executing", 7, 0)
	service := newGateService(db, repository.NewAuditRepository(db))

	_, err := service.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: "verified"}, gateReviewer(), "request-gate-missing")
	var apiErr *util.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != util.CodeReviewGate {
		t.Fatalf("got %#v, want 409 %s", err, util.CodeReviewGate)
	}
	details, ok := apiErr.Details.(dto.RolloverReviewGateResponse)
	if !ok {
		t.Fatalf("gate rejection must carry review gate details, got %#v", apiErr.Details)
	}
	if details.Satisfied || len(details.Pending) != 1 {
		t.Fatalf("expected exactly one pending todo, got %+v", details.Pending)
	}
	todo := details.Pending[0]
	if todo.Kind != "signoff" || todo.ServiceCode != "SERVICE" || todo.ServiceID != 1 {
		t.Fatalf("unexpected pending todo %+v", todo)
	}
	if len(details.Required) != 1 || details.Required[0].ServiceCode != "SERVICE" {
		t.Fatalf("critical affected services missing from details %+v", details.Required)
	}
	var stored model.RolloverScenario
	if err := db.First(&stored, scenario.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ScenarioState != "executing" {
		t.Fatalf("rejected review must not change state, got %s", stored.ScenarioState)
	}
}

func TestReviewGateRejectsVerifyWhenReplayFails(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := gateTestScenario(t, db, "executing", 7, 0)
	service := newGateService(db, repository.NewAuditRepository(db))
	reviewer := gateReviewer()
	if _, err := service.RegisterSignoff(context.Background(), scenario.ID, 1, dto.UpsertRolloverSignoffRequest{Note: "已确认回退预案并通知值班"}, reviewer, "request-signoff-first"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.RolloverScenario{}).Where("id = ?", scenario.ID).Update("affected_services_json", "[]").Error; err != nil {
		t.Fatal(err)
	}

	_, err := service.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: "verified"}, reviewer, "request-gate-replay")
	var apiErr *util.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != util.CodeReviewGate {
		t.Fatalf("got %#v, want 409 %s", err, util.CodeReviewGate)
	}
	details, ok := apiErr.Details.(dto.RolloverReviewGateResponse)
	if !ok {
		t.Fatalf("gate rejection must carry review gate details, got %#v", apiErr.Details)
	}
	if len(details.Pending) != 1 || details.Pending[0].Kind != "replay" {
		t.Fatalf("expected a single replay todo, got %+v", details.Pending)
	}
	if details.ReplayVerified {
		t.Fatal("replay must be reported as failed")
	}
}

func TestSignoffUpsertKeepsLatestPerService(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := gateTestScenario(t, db, "executing", 7, 0)
	service := newGateService(db, repository.NewAuditRepository(db))

	if _, err := service.RegisterSignoff(context.Background(), scenario.ID, 1, dto.UpsertRolloverSignoffRequest{Note: "第一版处置说明"}, gateReviewer(), "request-signoff-one"); err != nil {
		t.Fatal(err)
	}
	second := util.Actor{UserID: 10, Username: "reviewer-two", Role: "security_reviewer"}
	gate, err := service.RegisterSignoff(context.Background(), scenario.ID, 1, dto.UpsertRolloverSignoffRequest{Note: "更新后的处置说明"}, second, "request-signoff-two")
	if err != nil {
		t.Fatal(err)
	}
	if len(gate.Signoffs) != 1 {
		t.Fatalf("same service must keep only the latest sign-off, got %d", len(gate.Signoffs))
	}
	if gate.Signoffs[0].Note != "更新后的处置说明" || gate.Signoffs[0].SignedBy != 10 || gate.Signoffs[0].SignedByName != "reviewer-two" {
		t.Fatalf("latest sign-off not retained: %+v", gate.Signoffs[0])
	}
	var count int64
	if err := db.Model(&model.RolloverSignoff{}).Where("scenario_id = ?", scenario.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("only one sign-off row may survive per service, got %d", count)
	}
}

func TestScenarioCreatorCannotSignoff(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := gateTestScenario(t, db, "executing", 7, 0)
	service := newGateService(db, repository.NewAuditRepository(db))

	_, err := service.RegisterSignoff(context.Background(), scenario.ID, 1, dto.UpsertRolloverSignoffRequest{Note: "创建人尝试自签"}, util.Actor{UserID: 7, Username: "operator", Role: "security_reviewer"}, "request-self-signoff")
	var apiErr *util.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != util.CodeReviewerConflict {
		t.Fatalf("got %#v, want 409 %s", err, util.CodeReviewerConflict)
	}
}

func TestRegisterSignoffRejectsServiceOutsideCriticalSet(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := gateTestScenario(t, db, "executing", 7, 0)
	service := newGateService(db, repository.NewAuditRepository(db))

	_, err := service.RegisterSignoff(context.Background(), scenario.ID, 999, dto.UpsertRolloverSignoffRequest{Note: "无关服务签收"}, gateReviewer(), "request-foreign-signoff")
	var apiErr *util.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity || apiErr.Code != util.CodeValidation {
		t.Fatalf("got %#v, want 422 %s", err, util.CodeValidation)
	}
}

func TestVerifyLocksSignoffsAndSucceedsOnlyOnce(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := gateTestScenario(t, db, "executing", 7, 0)
	service := newGateService(db, repository.NewAuditRepository(db))
	reviewer := gateReviewer()
	if _, err := service.RegisterSignoff(context.Background(), scenario.ID, 1, dto.UpsertRolloverSignoffRequest{Note: "已确认回退预案并通知值班"}, reviewer, "request-signoff-lock"); err != nil {
		t.Fatal(err)
	}

	updated, err := service.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: "verified"}, reviewer, "request-verify")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ScenarioState != "verified" || !updated.ReplayVerified {
		t.Fatalf("verify must set verified state and replay evidence, got %s replay=%v", updated.ScenarioState, updated.ReplayVerified)
	}
	var signoff model.RolloverSignoff
	if err := db.First(&signoff, "scenario_id = ?", scenario.ID).Error; err != nil {
		t.Fatal(err)
	}
	if signoff.LockedAt == nil {
		t.Fatal("sign-off must lock in the same transaction as the review conclusion")
	}

	if _, err = service.Transition(context.Background(), scenario.ID, dto.RolloverScenarioTransitionRequest{ToState: "verified"}, reviewer, "request-verify-again"); err == nil {
		t.Fatal("repeated verify submission must not succeed twice")
	} else {
		var apiErr *util.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
			t.Fatalf("got %#v, want 409", err)
		}
	}
	if _, err = service.RegisterSignoff(context.Background(), scenario.ID, 1, dto.UpsertRolloverSignoffRequest{Note: "复核后补签"}, reviewer, "request-signoff-late"); err == nil {
		t.Fatal("sign-off must close once the review concludes")
	} else {
		var apiErr *util.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != util.CodeStateTransition {
			t.Fatalf("got %#v, want 409 %s", err, util.CodeStateTransition)
		}
	}
}

func TestSignoffRollsBackWhenAuditAppendFails(t *testing.T) {
	db := newScenarioTestDB(t)
	scenario := gateTestScenario(t, db, "executing", 7, 0)
	service := newGateService(db, failingAuditRepository{err: errors.New("audit storage unavailable")})

	if _, err := service.RegisterSignoff(context.Background(), scenario.ID, 1, dto.UpsertRolloverSignoffRequest{Note: "审计故障时的签收"}, gateReviewer(), "request-signoff-audit"); err == nil {
		t.Fatal("sign-off should fail when audit append fails")
	}
	var count int64
	if err := db.Model(&model.RolloverSignoff{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("sign-off committed without its audit record: %d", count)
	}
}
