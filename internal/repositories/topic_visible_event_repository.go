package repositories

import (
	"math"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"

	"gorm.io/gorm"
)

var TopicVisibleEventRepository = newTopicVisibleEventRepository()

func newTopicVisibleEventRepository() *topicVisibleEventRepository {
	return &topicVisibleEventRepository{}
}

type topicVisibleEventRepository struct{}

type topicVisibleEventTopicID struct {
	TopicID int64 `gorm:"column:topic_id"`
	EventID int64 `gorm:"column:event_id"`
}

func (r *topicVisibleEventRepository) Create(db *gorm.DB, visibleEvent *models.TopicVisibleEvent) error {
	return db.Create(visibleEvent).Error
}

func (r *topicVisibleEventRepository) GetLatestIDs(db *gorm.DB, topicIDs []int64) map[int64]int64 {
	if len(topicIDs) == 0 {
		return nil
	}

	var rows []topicVisibleEventTopicID
	if err := db.Model(&models.TopicVisibleEvent{}).
		Select("topic_id, MAX(id) AS event_id").
		Where("topic_id IN ?", topicIDs).
		Group("topic_id").
		Find(&rows).Error; err != nil {
		return nil
	}

	result := make(map[int64]int64, len(rows))
	for _, row := range rows {
		result[row.TopicID] = row.EventID
	}
	return result
}

func (r *topicVisibleEventRepository) GetRoleStatus(db *gorm.DB, userId int64, roleName string, after int64) (marker, count int64, err error) {
	countAfter := after
	if countAfter < 0 {
		countAfter = math.MaxInt64
	}

	var result struct {
		Marker int64 `gorm:"column:marker"`
		Count  int64 `gorm:"column:count"`
	}
	err = db.Table("t_topic_visible_event AS visible_event").
		Select(`COALESCE(MAX(visible_event.id), 0) AS marker,
			COUNT(DISTINCT CASE
				WHEN visible_event.id > ? AND topic.user_id <> ?
					AND NOT EXISTS (
						SELECT 1
						FROM t_topic_read
						WHERE t_topic_read.user_id = ?
							AND t_topic_read.topic_id = visible_event.topic_id
					)
				THEN visible_event.topic_id
			END) AS count`, countAfter, userId, userId).
		Joins("JOIN t_topic AS topic ON topic.id = visible_event.topic_id").
		Where("topic.status = ?", constants.StatusOk).
		Where(`EXISTS (
			SELECT 1
			FROM t_user_role
			JOIN t_role ON t_role.id = t_user_role.role_id
			WHERE t_user_role.user_id = topic.user_id
				AND t_role.name = ?
				AND t_role.status = ?
		)`, roleName, constants.StatusOk).
		Scan(&result).Error
	if err != nil {
		return 0, 0, err
	}

	if after < 0 {
		result.Count = 0
	}
	return result.Marker, result.Count, nil
}
