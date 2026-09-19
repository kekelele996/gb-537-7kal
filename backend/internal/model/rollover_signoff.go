package model

import "time"

// RolloverSignoff records the latest disposition note a security reviewer
// registered for one critical affected service of a rollover scenario. The
// (scenario_id, service_id) unique index keeps only the newest entry per
// service; LockedAt is stamped in the same transaction as the review
// conclusion so the acknowledged set cannot change after verification.
type RolloverSignoff struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ScenarioID   uint       `gorm:"not null;index;uniqueIndex:idx_signoff_scenario_service,priority:1" json:"scenario_id"`
	ServiceID    uint       `gorm:"not null;uniqueIndex:idx_signoff_scenario_service,priority:2" json:"service_id"`
	ServiceCode  string     `gorm:"size:120;not null" json:"service_code"`
	Criticality  string     `gorm:"size:40;not null" json:"criticality"`
	Note         string     `gorm:"type:text;not null" json:"note"`
	SignedBy     uint       `gorm:"not null;index" json:"signed_by"`
	SignedByName string     `gorm:"size:80;not null" json:"signed_by_name"`
	LockedAt     *time.Time `json:"locked_at,omitempty"`
	CreatedAt    time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"not null" json:"updated_at"`
}
