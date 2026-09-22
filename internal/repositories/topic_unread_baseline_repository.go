package repositories

import (
	"bbs-go/internal/models"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var TopicUnreadBaselineRepository = newTopicUnreadBaselineRepository()

func newTopicUnreadBaselineRepository() *topicUnreadBaselineRepository {
	return &topicUnreadBaselineRepository{}
}

type topicUnreadBaselineRepository struct{}

func (r *topicUnreadBaselineRepository) Get(db *gorm.DB, userId int64, roleName string) (*models.TopicUnreadBaseline, error) {
	var baseline models.TopicUnreadBaseline
	err := db.Where("user_id = ? AND role_name = ?", userId, roleName).Take(&baseline).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &baseline, nil
}

func (r *topicUnreadBaselineRepository) CreateIfMissing(db *gorm.DB, baseline *models.TopicUnreadBaseline) (bool, error) {
	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "role_name"}},
		DoNothing: true,
	}).Create(baseline)
	return result.RowsAffected > 0, result.Error
}
