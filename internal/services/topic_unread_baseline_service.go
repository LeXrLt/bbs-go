package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/repositories"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

var TopicUnreadBaselineService = newTopicUnreadBaselineService()

func newTopicUnreadBaselineService() *topicUnreadBaselineService {
	return &topicUnreadBaselineService{}
}

type topicUnreadBaselineService struct{}

func (s *topicUnreadBaselineService) Get(userId int64, roleName string) (eventId int64, found bool, err error) {
	roleName = NormalizeTopicRoleName(roleName)
	if userId <= 0 || roleName == "" {
		return 0, false, nil
	}
	baseline, err := repositories.TopicUnreadBaselineRepository.Get(sqls.DB(), userId, roleName)
	if err != nil || baseline == nil {
		return 0, false, err
	}
	return baseline.EventId, true, nil
}

func (s *topicUnreadBaselineService) Ensure(userId int64, roleName string, browserMarker int64) (eventId int64, initialized bool, err error) {
	roleName = NormalizeTopicRoleName(roleName)
	if userId <= 0 || roleName == "" {
		return 0, false, nil
	}
	if existing, getErr := repositories.TopicUnreadBaselineRepository.Get(sqls.DB(), userId, roleName); getErr != nil {
		return 0, false, getErr
	} else if existing != nil {
		return existing.EventId, false, nil
	}

	latest, _, err := repositories.TopicVisibleEventRepository.GetRoleStatus(sqls.DB(), userId, roleName, -1)
	if err != nil {
		return 0, false, err
	}
	eventId = browserMarker
	if eventId < 0 || eventId > latest {
		eventId = latest
	}
	baseline := &models.TopicUnreadBaseline{
		UserId:     userId,
		RoleName:   roleName,
		EventId:    eventId,
		CreateTime: dates.NowTimestamp(),
	}
	created, err := repositories.TopicUnreadBaselineRepository.CreateIfMissing(sqls.DB(), baseline)
	if err != nil {
		return 0, false, err
	}
	if created {
		return eventId, true, nil
	}
	existing, err := repositories.TopicUnreadBaselineRepository.Get(sqls.DB(), userId, roleName)
	if err != nil || existing == nil {
		return 0, false, err
	}
	return existing.EventId, false, nil
}
