package dto

import (
	"time"

	"pki-certificate-rollover-impact/backend/internal/model"
)

type CreateImpactSignoffRequest struct {
	ServiceID   uint   `json:"service_id" validate:"required,gt=0"`
	Disposition string `json:"disposition" validate:"required,min=4,max=1000"`
}

type CriticalService struct {
	ServiceID   uint   `json:"service_id"`
	ServiceCode string `json:"service_code"`
	Criticality string `json:"criticality"`
}

type SignoffTodo struct {
	Type        string `json:"type"`
	ServiceID   uint   `json:"service_id,omitempty"`
	ServiceCode string `json:"service_code,omitempty"`
	Message     string `json:"message"`
}

type ImpactSignoffResponse struct {
	ID           uint       `json:"id"`
	ScenarioID   uint       `json:"scenario_id"`
	ServiceID    uint       `json:"service_id"`
	ServiceCode  string     `json:"service_code"`
	Disposition  string     `json:"disposition"`
	SignedBy     uint       `json:"signed_by"`
	SignedByName string     `json:"signed_by_name"`
	AppliedAt    *time.Time `json:"applied_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type ImpactSignoffGateResponse struct {
	ScenarioID       uint                    `json:"scenario_id"`
	ScenarioState    string                  `json:"scenario_state"`
	RequiredServices []CriticalService       `json:"required_services"`
	Signoffs         []ImpactSignoffResponse `json:"signoffs"`
	Todos            []SignoffTodo           `json:"todos"`
	ReplayVerified   bool                    `json:"replay_verified"`
	GateSatisfied    bool                    `json:"gate_satisfied"`
}

func NewImpactSignoffResponse(signoff model.ImpactSignoff) ImpactSignoffResponse {
	return ImpactSignoffResponse{ID: signoff.ID, ScenarioID: signoff.ScenarioID, ServiceID: signoff.ServiceID, ServiceCode: signoff.ServiceCode, Disposition: signoff.Disposition, SignedBy: signoff.SignedBy, SignedByName: signoff.SignedByName, AppliedAt: signoff.AppliedAt, CreatedAt: signoff.CreatedAt, UpdatedAt: signoff.UpdatedAt}
}
