package services

import (
	"errors"
	"sync"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/repositories"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

func setupAttachmentServiceTestDB(t *testing.T) {
	t.Helper()
	previousConfig := config.Instance
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	t.Cleanup(func() { config.Instance = previousConfig })

	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Topic{}, &models.Attachment{}, &models.AttachmentDownloadLog{}); err != nil {
		t.Fatalf("auto migrate attachment models: %v", err)
	}
}

func createAttachmentTestUser(t *testing.T, score int) *models.User {
	t.Helper()
	now := dates.NowTimestamp()
	user := &models.User{
		Nickname:   "attachment-user",
		Score:      score,
		Status:     constants.StatusOk,
		CreateTime: now,
		UpdateTime: now,
	}
	if err := repositories.UserRepository.Create(sqls.DB(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func createAttachmentTestTopic(t *testing.T, userId int64, topicType constants.TopicType, status int) *models.Topic {
	t.Helper()
	topic := &models.Topic{
		Type:            topicType,
		UserId:          userId,
		Title:           "attachment topic",
		Content:         "content",
		Status:          status,
		LastCommentTime: dates.NowTimestamp(),
		CreateTime:      dates.NowTimestamp(),
	}
	if err := repositories.TopicRepository.Create(sqls.DB(), topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	return topic
}

func createAttachmentTestAttachment(t *testing.T, id string, topicId, userId int64, score int) *models.Attachment {
	t.Helper()
	attachment := &models.Attachment{
		Id:            id,
		TopicId:       topicId,
		UserId:        userId,
		FileName:      "report.pdf",
		StorageMethod: string(dto.Local),
		FileKey:       "attachments/2026/08/18/" + id + ".pdf",
		FileSize:      32,
		FileType:      "application/pdf",
		PreviewKey:    "attachments/2026/08/18/" + id + ".pdf",
		PreviewSize:   32,
		PreviewStatus: constants.AttachmentPreviewReady,
		DownloadScore: score,
		Status:        constants.StatusOk,
		CreateTime:    dates.NowTimestamp(),
		UpdateTime:    dates.NowTimestamp(),
	}
	if err := repositories.AttachmentRepository.Create(sqls.DB(), attachment); err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	return attachment
}

func TestAttachmentAccessConcurrentRequestsDeductOnce(t *testing.T) {
	setupAttachmentServiceTestDB(t)
	owner := createAttachmentTestUser(t, 0)
	buyer := createAttachmentTestUser(t, 100)
	topic := createAttachmentTestTopic(t, owner.Id, constants.TopicTypeTopic, constants.StatusOk)
	attachment := createAttachmentTestAttachment(t, "paid-concurrent", topic.Id, owner.Id, 15)

	const requests = 12
	errorsByRequest := make(chan error, requests)
	var waitGroup sync.WaitGroup
	for range requests {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := AttachmentService.Access(attachment.Id, buyer)
			errorsByRequest <- err
		}()
	}
	waitGroup.Wait()
	close(errorsByRequest)
	for err := range errorsByRequest {
		if err != nil {
			t.Fatalf("concurrent access failed: %v", err)
		}
	}

	storedBuyer := repositories.UserRepository.Get(sqls.DB(), buyer.Id)
	if storedBuyer == nil || storedBuyer.Score != 85 {
		t.Fatalf("buyer score=%v want 85", storedBuyer)
	}
	var unlockCount int64
	if err := sqls.DB().Model(&models.AttachmentDownloadLog{}).
		Where("user_id = ? AND attachment_id = ?", buyer.Id, attachment.Id).
		Count(&unlockCount).Error; err != nil {
		t.Fatal(err)
	}
	if unlockCount != 1 {
		t.Fatalf("unlock logs=%d want 1", unlockCount)
	}
	var scoreLogCount int64
	if err := sqls.DB().Model(&models.UserScoreLog{}).
		Where("user_id = ? AND source_type = ? AND source_id = ?", buyer.Id, constants.SourceTypeAttachmentDownload, attachment.Id).
		Count(&scoreLogCount).Error; err != nil {
		t.Fatal(err)
	}
	if scoreLogCount != 1 {
		t.Fatalf("score logs=%d want 1", scoreLogCount)
	}
}

func TestAttachmentAccessOwnerAndFreeAttachmentDoNotCreatePurchase(t *testing.T) {
	setupAttachmentServiceTestDB(t)
	owner := createAttachmentTestUser(t, 20)
	reader := createAttachmentTestUser(t, 20)
	topic := createAttachmentTestTopic(t, owner.Id, constants.TopicTypeTopic, constants.StatusOk)
	paid := createAttachmentTestAttachment(t, "owner-paid", topic.Id, owner.Id, 10)
	free := createAttachmentTestAttachment(t, "reader-free", topic.Id, owner.Id, 0)

	if _, err := AttachmentService.Access(paid.Id, owner); err != nil {
		t.Fatalf("owner access: %v", err)
	}
	if _, err := AttachmentService.Access(free.Id, reader); err != nil {
		t.Fatalf("free access: %v", err)
	}
	var count int64
	if err := sqls.DB().Model(&models.AttachmentDownloadLog{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("purchase logs=%d want 0", count)
	}
}

func TestAttachmentAuthorizedObjectEnforcesTopicVisibility(t *testing.T) {
	setupAttachmentServiceTestDB(t)
	owner := createAttachmentTestUser(t, 0)
	other := createAttachmentTestUser(t, 0)

	reviewTopic := createAttachmentTestTopic(t, owner.Id, constants.TopicTypeTopic, constants.StatusReview)
	reviewAttachment := createAttachmentTestAttachment(t, "review", reviewTopic.Id, owner.Id, 0)
	if _, err := AttachmentService.AuthorizedObject(reviewAttachment.Id, other, true); err == nil || err.Error() != locales.Get("attachment.no_permission") {
		t.Fatalf("review access error=%v", err)
	} else if !errors.Is(err, ErrAttachmentAccessForbidden) {
		t.Fatalf("review access error must be classified as forbidden: %v", err)
	}
	object, err := AttachmentService.AuthorizedObject(reviewAttachment.Id, owner, true)
	if err != nil || object.Key != reviewAttachment.PreviewKey {
		t.Fatalf("review owner object=%+v err=%v", object, err)
	}

	deletedTopic := createAttachmentTestTopic(t, owner.Id, constants.TopicTypeTopic, constants.StatusDeleted)
	deletedAttachment := createAttachmentTestAttachment(t, "deleted-topic", deletedTopic.Id, owner.Id, 0)
	if _, err := AttachmentService.AuthorizedObject(deletedAttachment.Id, owner, false); err == nil {
		t.Fatal("deleted topic attachment must not be readable")
	}

	qaTopic := createAttachmentTestTopic(t, owner.Id, constants.TopicTypeQA, constants.StatusOk)
	qaAttachment := createAttachmentTestAttachment(t, "qa-topic", qaTopic.Id, owner.Id, 0)
	if _, err := AttachmentService.AuthorizedObject(qaAttachment.Id, owner, false); err == nil {
		t.Fatal("non-regular topic attachment must not be readable")
	}

	unbound := createAttachmentTestAttachment(t, "unbound", 0, owner.Id, 0)
	if _, err := AttachmentService.AuthorizedObject(unbound.Id, owner, false); err == nil {
		t.Fatal("unbound attachment must not be readable")
	}

	paidTopic := createAttachmentTestTopic(t, owner.Id, constants.TopicTypeTopic, constants.StatusOk)
	paidAttachment := createAttachmentTestAttachment(t, "locked", paidTopic.Id, owner.Id, 5)
	if _, err := AttachmentService.AuthorizedObject(paidAttachment.Id, other, true); err == nil || !errors.Is(err, ErrAttachmentAccessForbidden) {
		t.Fatalf("locked attachment error=%v, want classified forbidden error", err)
	} else if err.Error() != locales.Get("attachment.unlock_required") {
		t.Fatalf("locked attachment message=%q", err.Error())
	}
}
