package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"pki-certificate-rollover-impact/backend/internal/model"
)

type ImpactSignoffRepository interface {
	Upsert(context.Context, *model.ImpactSignoff) error
	FindByScenarioAndService(context.Context, uint, uint) (model.ImpactSignoff, error)
	ListByScenario(context.Context, uint) ([]model.ImpactSignoff, error)
	MarkApplied(context.Context, uint, time.Time) error
}
type impactSignoffRepository struct{ db *gorm.DB }

func NewImpactSignoffRepository(db *gorm.DB) ImpactSignoffRepository {
	return &impactSignoffRepository{db: db}
}

// Upsert keeps only the latest registration per (scenario, service): the
// unique index rejects duplicates and the conflict clause rewrites the row.
func (r *impactSignoffRepository) Upsert(ctx context.Context, signoff *model.ImpactSignoff) error {
	err := scopedDB(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "scenario_id"}, {Name: "service_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"service_code", "disposition", "signed_by", "signed_by_name", "updated_at"}),
	}).Create(signoff).Error
	if err != nil {
		return fmt.Errorf("upsert impact sign-off: %w", err)
	}
	return nil
}
func (r *impactSignoffRepository) FindByScenarioAndService(ctx context.Context, scenarioID, serviceID uint) (model.ImpactSignoff, error) {
	var signoff model.ImpactSignoff
	if err := scopedDB(ctx, r.db).Where("scenario_id = ? AND service_id = ?", scenarioID, serviceID).First(&signoff).Error; err != nil {
		return model.ImpactSignoff{}, fmt.Errorf("find impact sign-off: %w", err)
	}
	return signoff, nil
}
func (r *impactSignoffRepository) ListByScenario(ctx context.Context, scenarioID uint) ([]model.ImpactSignoff, error) {
	var signoffs []model.ImpactSignoff
	if err := scopedDB(ctx, r.db).Where("scenario_id = ?", scenarioID).Order("service_id ASC").Find(&signoffs).Error; err != nil {
		return nil, fmt.Errorf("list impact sign-offs: %w", err)
	}
	return signoffs, nil
}

// MarkApplied stamps every pending sign-off of the scenario inside the same
// transaction that records the review conclusion, so both take effect at once.
func (r *impactSignoffRepository) MarkApplied(ctx context.Context, scenarioID uint, appliedAt time.Time) error {
	result := scopedDB(ctx, r.db).Model(&model.ImpactSignoff{}).Where("scenario_id = ? AND applied_at IS NULL", scenarioID).Updates(map[string]any{"applied_at": appliedAt, "updated_at": appliedAt})
	if result.Error != nil {
		return fmt.Errorf("apply impact sign-offs: %w", result.Error)
	}
	return nil
}
