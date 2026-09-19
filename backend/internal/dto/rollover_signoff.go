package dto

import (
	"time"

	"pki-certificate-rollover-impact/backend/internal/algorithm"
	"pki-certificate-rollover-impact/backend/internal/model"
)

type UpsertRolloverSignoffRequest struct {
	Note string `json:"note" validate:"required,min=4,max=1000"`
}

type RolloverSignoffResponse struct {
	ServiceID    uint       `json:"service_id"`
	ServiceCode  string     `json:"service_code"`
	Criticality  string     `json:"criticality"`
	Note         string     `json:"note"`
	SignedBy     uint       `json:"signed_by"`
	SignedByName string     `json:"signed_by_name"`
	LockedAt     *time.Time `json:"locked_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type RolloverGateTodo struct {
	Kind        string `json:"kind"`
	ServiceID   uint   `json:"service_id,omitempty"`
	ServiceCode string `json:"service_code,omitempty"`
	Message     string `json:"message"`
}

type RolloverReviewGateResponse struct {
	ScenarioID     uint                        `json:"scenario_id"`
	ScenarioState  string                      `json:"scenario_state"`
	Required       []algorithm.AffectedService `json:"required"`
	Signoffs       []RolloverSignoffResponse   `json:"signoffs"`
	Pending        []RolloverGateTodo          `json:"pending"`
	ReplayVerified bool                        `json:"replay_verified"`
	Satisfied      bool                        `json:"satisfied"`
}

func NewRolloverSignoffResponse(signoff model.RolloverSignoff) RolloverSignoffResponse {
	return RolloverSignoffResponse{ServiceID: signoff.ServiceID, ServiceCode: signoff.ServiceCode, Criticality: signoff.Criticality, Note: signoff.Note, SignedBy: signoff.SignedBy, SignedByName: signoff.SignedByName, LockedAt: signoff.LockedAt, UpdatedAt: signoff.UpdatedAt}
}

func NewRolloverReviewGateResponse(scenario model.RolloverScenario, required []algorithm.AffectedService, signoffs []model.RolloverSignoff, pending []RolloverGateTodo, replayVerified bool) RolloverReviewGateResponse {
	responses := make([]RolloverSignoffResponse, 0, len(signoffs))
	for _, signoff := range signoffs {
		responses = append(responses, NewRolloverSignoffResponse(signoff))
	}
	if required == nil {
		required = []algorithm.AffectedService{}
	}
	if pending == nil {
		pending = []RolloverGateTodo{}
	}
	return RolloverReviewGateResponse{ScenarioID: scenario.ID, ScenarioState: scenario.ScenarioState, Required: required, Signoffs: responses, Pending: pending, ReplayVerified: replayVerified, Satisfied: len(pending) == 0}
}
