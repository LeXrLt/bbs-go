package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/repositories"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

var TopicReadService = newTopicReadService()

func newTopicReadService() *topicReadService {
	return &topicReadService{}
}

type topicReadService struct{}

func (s *topicReadService) MarkRead(userId, topicId int64) error {
	if userId <= 0 || topicId <= 0 {
		return nil
	}
	return repositories.TopicReadRepository.MarkRead(sqls.DB(), &models.TopicRead{
		UserId:   userId,
		TopicId:  topicId,
		ReadTime: dates.NowTimestamp(),
	})
}

func (s *topicReadService) FindReadTopicIDs(userId int64, topicIds []int64) []int64 {
	return repositories.TopicReadRepository.FindReadTopicIDs(sqls.DB(), userId, topicIds)
}
