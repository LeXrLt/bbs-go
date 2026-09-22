package repositories

import (
	"bbs-go/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var TopicReadRepository = newTopicReadRepository()

func newTopicReadRepository() *topicReadRepository {
	return &topicReadRepository{}
}

type topicReadRepository struct{}

func (r *topicReadRepository) MarkRead(db *gorm.DB, topicRead *models.TopicRead) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "topic_id"}},
		DoNothing: true,
	}).Create(topicRead).Error
}

func (r *topicReadRepository) FindReadTopicIDs(db *gorm.DB, userId int64, topicIds []int64) []int64 {
	if userId <= 0 || len(topicIds) == 0 {
		return nil
	}

	var ids []int64
	if err := db.Model(&models.TopicRead{}).
		Where("user_id = ? AND topic_id IN ?", userId, topicIds).
		Pluck("topic_id", &ids).Error; err != nil {
		return nil
	}
	return ids
}
