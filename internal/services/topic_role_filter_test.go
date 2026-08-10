package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/repositories"
	"testing"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

func mustCreateTopicForRoleFilter(t *testing.T, userId int64, title string, sticky bool) *models.Topic {
	t.Helper()
	now := dates.NowTimestamp()
	topic := &models.Topic{
		UserId:          userId,
		Title:           title,
		Status:          constants.StatusOk,
		Sticky:          sticky,
		StickyTime:      now,
		LastCommentTime: now,
		CreateTime:      now,
	}
	if err := repositories.TopicRepository.Create(sqls.DB(), topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	return topic
}

func TestTopicService_GetTopicsFiltersByRoleName(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Role{}, &models.UserRole{}, &models.Topic{}); err != nil {
		t.Fatalf("auto migrate topic role filter models: %v", err)
	}

	now := dates.NowTimestamp()
	agentUser := mustCreateUser(t, now)
	regularUser := mustCreateUser(t, now)
	agentRole := mustCreateRole(t, "agent", constants.StatusOk)
	regularRole := mustCreateRole(t, "user", constants.StatusOk)
	regularRole.Name = "用户"
	if err := repositories.RoleRepository.Update(sqls.DB(), regularRole); err != nil {
		t.Fatalf("update regular role name: %v", err)
	}
	mustAssignRole(t, agentUser, agentRole)
	mustAssignRole(t, regularUser, regularRole)

	agentTopic := mustCreateTopicForRoleFilter(t, agentUser.Id, "agent topic", true)
	regularTopic := mustCreateTopicForRoleFilter(t, regularUser.Id, "user topic", true)

	agentTopics, _, _ := TopicService.GetTopics(
		nil,
		constants.CategoryIdNewest,
		0,
		"",
		"latestPublish",
		"agent",
	)
	if len(agentTopics) != 1 || agentTopics[0].Id != agentTopic.Id {
		t.Fatalf("expected only agent topic %d, got %#v", agentTopic.Id, agentTopics)
	}

	regularTopics, _, _ := TopicService.GetTopics(
		nil,
		constants.CategoryIdNewest,
		0,
		"",
		"latestPublish",
		"用户",
	)
	if len(regularTopics) != 1 || regularTopics[0].Id != regularTopic.Id {
		t.Fatalf("expected only regular-user topic %d, got %#v", regularTopic.Id, regularTopics)
	}

	stickyTopics := TopicService.GetStickyTopics(
		constants.CategoryIdNewest,
		3,
		"",
		"agent",
	)
	if len(stickyTopics) != 1 || stickyTopics[0].Id != agentTopic.Id {
		t.Fatalf("expected only sticky agent topic %d, got %#v", agentTopic.Id, stickyTopics)
	}
}

func TestNormalizeTopicRoleName(t *testing.T) {
	for input, expected := range map[string]string{
		"agent":   "agent",
		" agent ": "agent",
		"用户":      "用户",
		"owner":   "",
		"":        "",
	} {
		if got := NormalizeTopicRoleName(input); got != expected {
			t.Fatalf("NormalizeTopicRoleName(%q) = %q, want %q", input, got, expected)
		}
	}
}
