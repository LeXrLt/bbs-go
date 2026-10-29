package repositories

import (
	"bbs-go/internal/models"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var DailyReviewRepository = newDailyReviewRepository()

func newDailyReviewRepository() *dailyReviewRepository {
	return &dailyReviewRepository{}
}

type dailyReviewRepository struct{}

func (r *dailyReviewRepository) GetByDateAndKind(db *gorm.DB, reportDate time.Time, kind string) (*models.DailyReviewReport, error) {
	report := &models.DailyReviewReport{}
	if err := db.Where("report_date = ? AND kind = ?", reportDate, kind).Take(report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return report, nil
}

func (r *dailyReviewRepository) FindByDate(db *gorm.DB, reportDate time.Time) ([]models.DailyReviewReport, error) {
	var reports []models.DailyReviewReport
	err := db.Where("report_date = ?", reportDate).Find(&reports).Error
	return reports, err
}

func (r *dailyReviewRepository) AvailableDates(db *gorm.DB) ([]time.Time, error) {
	var dates []time.Time
	err := db.Model(&models.DailyReviewReport{}).
		Distinct("report_date").
		Order("report_date DESC").
		Pluck("report_date", &dates).Error
	return dates, err
}

func (r *dailyReviewRepository) CreateIfAbsent(db *gorm.DB, report *models.DailyReviewReport) (bool, error) {
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(report)
	return result.RowsAffected > 0, result.Error
}

func (r *dailyReviewRepository) UpdateIfHash(db *gorm.DB, report *models.DailyReviewReport, previousHash string) (bool, error) {
	result := db.Model(&models.DailyReviewReport{}).
		Where("id = ? AND content_hash = ?", report.Id, previousHash).
		Updates(map[string]any{
			"title":        report.Title,
			"markdown":     report.Markdown,
			"summary_json": report.SummaryJson,
			"content_hash": report.ContentHash,
			"update_time":  report.UpdateTime,
		})
	return result.RowsAffected > 0, result.Error
}

func (r *dailyReviewRepository) CreateRevision(db *gorm.DB, revision *models.DailyReviewReportRevision) error {
	return db.Create(revision).Error
}
