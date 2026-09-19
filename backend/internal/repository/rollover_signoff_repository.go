package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"pki-certificate-rollover-impact/backend/internal/model"
)

type RolloverSignoffRepository interface {
	ListByScenario(context.Context, uint) ([]model.RolloverSignoff, error)
	FindByScenarioAndService(context.Context, uint, uint) (model.RolloverSignoff, error)
	Upsert(context.Context, *model.RolloverSignoff) error
	LockByScenario(context.Context, uint, time.Time) error
}
type rolloverSignoffRepository struct{ db *gorm.DB }

func NewRolloverSignoffRepository(db *gorm.DB) RolloverSignoffRepository {
	return &rolloverSignoffRepository{db: db}
}
func (r *rolloverSignoffRepository) ListByScenario(ctx context.Context, scenarioID uint) ([]model.RolloverSignoff, error) {
	signoffs := []model.RolloverSignoff{}
	if err := scopedDB(ctx, r.db).Where("scenario_id = ?", scenarioID).Order("service_id ASC").Find(&signoffs).Error; err != nil {
		return nil, fmt.Errorf("list rollover sign-offs: %w", err)
	}
	return signoffs, nil
}
func (r *rolloverSignoffRepository) FindByScenarioAndService(ctx context.Context, scenarioID, serviceID uint) (model.RolloverSignoff, error) {
	var signoff model.RolloverSignoff
	if err := scopedDB(ctx, r.db).Where("scenario_id = ? AND service_id = ?", scenarioID, serviceID).First(&signoff).Error; err != nil {
		return model.RolloverSignoff{}, fmt.Errorf("find rollover sign-off: %w", err)
	}
	return signoff, nil
}
func (r *rolloverSignoffRepository) Upsert(ctx context.Context, signoff *model.RolloverSignoff) error {
	if err := scopedDB(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "scenario_id"}, {Name: "service_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"service_code", "criticality", "note", "signed_by", "signed_by_name", "updated_at"}),
	}).Create(signoff).Error; err != nil {
		return fmt.Errorf("upsert rollover sign-off: %w", err)
	}
	return nil
}
func (r *rolloverSignoffRepository) LockByScenario(ctx context.Context, scenarioID uint, lockedAt time.Time) error {
	if err := scopedDB(ctx, r.db).Model(&model.RolloverSignoff{}).Where("scenario_id = ? AND locked_at IS NULL", scenarioID).Updates(map[string]any{"locked_at": lockedAt, "updated_at": lockedAt}).Error; err != nil {
		return fmt.Errorf("lock rollover sign-offs: %w", err)
	}
	return nil
}
