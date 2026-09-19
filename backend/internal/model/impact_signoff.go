package model

import "time"

// ImpactSignoff records a security reviewer's disposition for one critical
// affected service of a frozen rollover scenario. The unique index on
// (scenario_id, service_id) keeps only the latest registration per service.
type ImpactSignoff struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ScenarioID   uint       `gorm:"not null;uniqueIndex:idx_signoff_scenario_service,priority:1" json:"scenario_id"`
	ServiceID    uint       `gorm:"not null;uniqueIndex:idx_signoff_scenario_service,priority:2" json:"service_id"`
	ServiceCode  string     `gorm:"size:80;not null" json:"service_code"`
	Disposition  string     `gorm:"type:text;not null" json:"disposition"`
	SignedBy     uint       `gorm:"not null;index" json:"signed_by"`
	SignedByName string     `gorm:"size:80;not null" json:"signed_by_name"`
	AppliedAt    *time.Time `json:"applied_at,omitempty"`
	CreatedAt    time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"not null" json:"updated_at"`
}
