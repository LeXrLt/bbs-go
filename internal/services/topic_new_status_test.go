package services

import (
	"path/filepath"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/req"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/search"
	"bbs-go/internal/repositories"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

func initTopicNewStatusSearch(t *testing.T) {
	t.Helper()
	previousConfig := config.Instance
	config.Instance = &config.Config{
		Search: config.SearchConfig{IndexPath: filepath.Join(t.TempDir(), "index")},
	}
	search.Init()
	t.Cleanup(func() { config.Instance = previousConfig })
}

func setupTopicNewStatusTestDB(t *testing.T) {
	t.Helper()
	db := setupTestDB(t)
	if err := db.AutoMigrate(
		&models.Role{},
		&models.UserRole{},
		&models.Topic{},
		&models.TopicVisibleEvent{},
		&models.TopicTag{},
	); err != nil {
		t.Fatalf("auto migrate new topic status models: %v", err)
	}
}

func mustCreateTopicForNewStatus(t *testing.T, userId int64, status int) *models.Topic {
	t.Helper()
	now := dates.NowTimestamp()
	topic := &models.Topic{
		UserId:          userId,
		Title:           "new topic status test",
		Status:          status,
		LastCommentTime: now,
		CreateTime:      now,
	}
	if err := repositories.TopicRepository.Create(sqls.DB(), topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	return topic
}

func mustCreateVisibleEventForNewStatus(t *testing.T, topicId int64) *models.TopicVisibleEvent {
	t.Helper()
	visibleEvent := &models.TopicVisibleEvent{
		TopicId:    topicId,
		CreateTime: dates.NowTimestamp(),
	}
	if err := repositories.TopicVisibleEventRepository.Create(sqls.DB(), visibleEvent); err != nil {
		t.Fatalf("create topic visible event: %v", err)
	}
	return visibleEvent
}

func mustCreateVisibleTopicForNewStatus(t *testing.T, userId int64) (*models.Topic, *models.TopicVisibleEvent) {
	t.Helper()
	topic := mustCreateTopicForNewStatus(t, userId, constants.StatusOk)
	return topic, mustCreateVisibleEventForNewStatus(t, topic.Id)
}

func mustCreateNamedRoleForNewStatus(t *testing.T, name, code string, status int) *models.Role {
	t.Helper()
	role := mustCreateRole(t, code, status)
	role.Name = name
	if err := repositories.RoleRepository.Update(sqls.DB(), role); err != nil {
		t.Fatalf("update role name: %v", err)
	}
	return role
}

func mustGetNewTopicStatus(t *testing.T, userId int64, roleName string, after int64) (marker, count int64) {
	t.Helper()
	marker, count, err := TopicService.GetNewTopicStatus(userId, roleName, after)
	if err != nil {
		t.Fatalf("get %s new topic status: %v", roleName, err)
	}
	return marker, count
}

func TestTopicService_GetNewTopicStatusSeparatesRoles(t *testing.T) {
	setupTopicNewStatusTestDB(t)

	now := dates.NowTimestamp()
	currentUser := mustCreateUser(t, now)
	agentUser := mustCreateUser(t, now)
	regularUser := mustCreateUser(t, now)
	rolelessUser := mustCreateUser(t, now)
	disabledRoleUser := mustCreateUser(t, now)
	agentRole := mustCreateNamedRoleForNewStatus(t, "agent", "new-status-agent", constants.StatusOk)
	regularRole := mustCreateNamedRoleForNewStatus(t, "用户", "new-status-user", constants.StatusOk)
	disabledAgentRole := mustCreateNamedRoleForNewStatus(t, "agent", "new-status-agent-disabled", constants.StatusDeleted)
	mustAssignRole(t, agentUser, agentRole)
	mustAssignRole(t, regularUser, regularRole)
	mustAssignRole(t, disabledRoleUser, disabledAgentRole)

	marker, count := mustGetNewTopicStatus(t, currentUser.Id, "agent", -1)
	if marker != 0 || count != 0 {
		t.Fatalf("empty agent status = (%d, %d), want (0, 0)", marker, count)
	}

	_, agentBaseline := mustCreateVisibleTopicForNewStatus(t, agentUser.Id)
	_, userBaseline := mustCreateVisibleTopicForNewStatus(t, regularUser.Id)

	marker, count = mustGetNewTopicStatus(t, currentUser.Id, "agent", -1)
	if marker != agentBaseline.Id || count != 0 {
		t.Fatalf("initial agent status = (%d, %d), want (%d, 0)", marker, count, agentBaseline.Id)
	}
	marker, count = mustGetNewTopicStatus(t, currentUser.Id, "用户", -1)
	if marker != userBaseline.Id || count != 0 {
		t.Fatalf("initial user status = (%d, %d), want (%d, 0)", marker, count, userBaseline.Id)
	}

	_, agentEvent := mustCreateVisibleTopicForNewStatus(t, agentUser.Id)
	_, userEvent := mustCreateVisibleTopicForNewStatus(t, regularUser.Id)
	_, _ = mustCreateVisibleTopicForNewStatus(t, rolelessUser.Id)
	_, _ = mustCreateVisibleTopicForNewStatus(t, disabledRoleUser.Id)
	deletedTopic := mustCreateTopicForNewStatus(t, agentUser.Id, constants.StatusDeleted)
	mustCreateVisibleEventForNewStatus(t, deletedTopic.Id)
	reviewTopic := mustCreateTopicForNewStatus(t, agentUser.Id, constants.StatusReview)
	mustCreateVisibleEventForNewStatus(t, reviewTopic.Id)

	marker, count = mustGetNewTopicStatus(t, currentUser.Id, "agent", agentBaseline.Id)
	if marker != agentEvent.Id || count != 1 {
		t.Fatalf("agent status = (%d, %d), want (%d, 1)", marker, count, agentEvent.Id)
	}
	marker, count = mustGetNewTopicStatus(t, currentUser.Id, "用户", userBaseline.Id)
	if marker != userEvent.Id || count != 1 {
		t.Fatalf("user status = (%d, %d), want (%d, 1)", marker, count, userEvent.Id)
	}
}

func TestTopicService_GetNewTopicStatusSupportsOverlappingRolesAndDistinctTopics(t *testing.T) {
	setupTopicNewStatusTestDB(t)

	now := dates.NowTimestamp()
	currentUser := mustCreateUser(t, now)
	multiRoleUser := mustCreateUser(t, now)
	agentRole := mustCreateNamedRoleForNewStatus(t, "agent", "overlap-agent", constants.StatusOk)
	regularRole := mustCreateNamedRoleForNewStatus(t, "用户", "overlap-user", constants.StatusOk)
	mustAssignRole(t, multiRoleUser, agentRole)
	mustAssignRole(t, multiRoleUser, regularRole)

	topic, firstEvent := mustCreateVisibleTopicForNewStatus(t, multiRoleUser.Id)
	secondEvent := mustCreateVisibleEventForNewStatus(t, topic.Id)

	for _, roleName := range []string{"agent", "用户"} {
		marker, count := mustGetNewTopicStatus(t, currentUser.Id, roleName, 0)
		if marker != secondEvent.Id || count != 1 {
			t.Fatalf("%s overlap status = (%d, %d), want (%d, 1); first event %d must not be double counted", roleName, marker, count, secondEvent.Id, firstEvent.Id)
		}
	}
}

func TestTopicService_GetNewTopicStatusExcludesOwnTopicFromCount(t *testing.T) {
	setupTopicNewStatusTestDB(t)

	now := dates.NowTimestamp()
	currentUser := mustCreateUser(t, now)
	agentRole := mustCreateNamedRoleForNewStatus(t, "agent", "own-agent", constants.StatusOk)
	mustAssignRole(t, currentUser, agentRole)
	_, ownEvent := mustCreateVisibleTopicForNewStatus(t, currentUser.Id)

	marker, count := mustGetNewTopicStatus(t, currentUser.Id, "agent", 0)
	if marker != ownEvent.Id || count != 0 {
		t.Fatalf("own-topic status = (%d, %d), want (%d, 0)", marker, count, ownEvent.Id)
	}
}

func TestTopicService_GetNewTopicStatusPropagatesQueryError(t *testing.T) {
	setupTopicNewStatusTestDB(t)
	sqlDB, err := sqls.DB().DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}

	marker, count, err := TopicService.GetNewTopicStatus(1, "agent", 0)
	if err == nil {
		t.Fatalf("closed database status = (%d, %d, nil), want query error", marker, count)
	}
	if marker != 0 || count != 0 {
		t.Fatalf("closed database status = (%d, %d), want zero values with error", marker, count)
	}
}

func TestTopicService_AuditAndUndeleteCreateVisibleEvents(t *testing.T) {
	setupTopicNewStatusTestDB(t)
	initTopicNewStatusSearch(t)

	now := dates.NowTimestamp()
	currentUser := mustCreateUser(t, now)
	agentUser := mustCreateUser(t, now)
	agentRole := mustCreateNamedRoleForNewStatus(t, "agent", "transition-agent", constants.StatusOk)
	mustAssignRole(t, agentUser, agentRole)

	topic := mustCreateTopicForNewStatus(t, agentUser.Id, constants.StatusReview)
	if err := TopicService.Audit(topic.Id); err != nil {
		t.Fatalf("audit topic: %v", err)
	}
	auditMarker, count := mustGetNewTopicStatus(t, currentUser.Id, "agent", 0)
	if auditMarker == 0 || count != 1 {
		t.Fatalf("audited topic status = (%d, %d), want non-zero marker and count 1", auditMarker, count)
	}

	if err := repositories.TopicRepository.UpdateColumn(sqls.DB(), topic.Id, "status", constants.StatusDeleted); err != nil {
		t.Fatalf("delete topic for restore test: %v", err)
	}
	if err := TopicService.Undelete(topic.Id); err != nil {
		t.Fatalf("undelete topic: %v", err)
	}
	restoreMarker, count := mustGetNewTopicStatus(t, currentUser.Id, "agent", auditMarker)
	if restoreMarker <= auditMarker || count != 1 {
		t.Fatalf("restored topic status = (%d, %d), want marker > %d and count 1", restoreMarker, count, auditMarker)
	}
	if err := TopicService.Undelete(topic.Id); err != nil {
		t.Fatalf("repeat undelete: %v", err)
	}

	if err := TopicService.Audit(topic.Id); err != nil {
		t.Fatalf("repeat audit: %v", err)
	}
	var eventCount int64
	if err := sqls.DB().Model(&models.TopicVisibleEvent{}).Where("topic_id = ?", topic.Id).Count(&eventCount).Error; err != nil {
		t.Fatalf("count visible events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("repeat audit created an event: got %d events, want 2", eventCount)
	}
}

func TestTopicService_UndeleteOnlyRestoresDeletedTopics(t *testing.T) {
	setupTopicNewStatusTestDB(t)

	user := mustCreateUser(t, dates.NowTimestamp())
	reviewTopic := mustCreateTopicForNewStatus(t, user.Id, constants.StatusReview)
	if err := TopicService.Undelete(reviewTopic.Id); err != nil {
		t.Fatalf("undelete review topic: %v", err)
	}

	saved := TopicService.Get(reviewTopic.Id)
	if saved == nil || saved.Status != constants.StatusReview {
		t.Fatalf("review topic status = %#v, want StatusReview", saved)
	}
	var eventCount int64
	if err := sqls.DB().Model(&models.TopicVisibleEvent{}).Where("topic_id = ?", reviewTopic.Id).Count(&eventCount).Error; err != nil {
		t.Fatalf("count review topic events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("undelete created %d events for review topic, want 0", eventCount)
	}
}

func TestTopicPublishService_PublishVisibleTopicCreatesEvent(t *testing.T) {
	setupTopicNewStatusTestDB(t)
	initTopicNewStatusSearch(t)
	db := sqls.DB()
	if err := db.AutoMigrate(
		&models.Category{},
		&models.Tag{},
		&models.SysConfig{},
		&models.ForbiddenWord{},
	); err != nil {
		t.Fatalf("auto migrate publish models: %v", err)
	}

	now := dates.NowTimestamp()
	user := mustCreateUser(t, now)
	category := &models.Category{
		Name:       "visible event category",
		Type:       constants.CategoryTypeNormal,
		Status:     constants.StatusOk,
		CreateTime: now,
	}
	if err := repositories.CategoryRepository.Create(db, category); err != nil {
		t.Fatalf("create category: %v", err)
	}

	topic, err := TopicPublishService.Publish(user.Id, req.CreateTopicReq{
		Type:        constants.TopicTypeTopic,
		CategoryId:  category.Id,
		Title:       "published visible topic",
		Content:     "content",
		ContentType: constants.ContentTypeMarkdown,
	})
	if err != nil {
		t.Fatalf("publish topic: %v", err)
	}
	var eventCount int64
	if err := db.Model(&models.TopicVisibleEvent{}).Where("topic_id = ?", topic.Id).Count(&eventCount).Error; err != nil {
		t.Fatalf("count publish visible events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("published topic has %d visible events, want 1", eventCount)
	}
}
