package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/pkg/idcodec"
	"bbs-go/internal/pkg/msg"
	"bbs-go/internal/repositories"
	"testing"

	"github.com/mlogclub/simple/sqls"
)

func TestReplyMessageCountAndMarkReadIgnoreOtherMessageTypes(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Message{}); err != nil {
		t.Fatalf("auto migrate messages: %v", err)
	}

	const userID int64 = 42
	messages := []*models.Message{
		{UserId: userID, Type: int(msg.TypeTopicComment), Status: msg.StatusUnread},
		{UserId: userID, Type: int(msg.TypeCommentReply), Status: msg.StatusHaveRead},
		{UserId: userID, Type: int(msg.TypeArticleComment), Status: msg.StatusUnread},
		{UserId: userID, Type: int(msg.TypeTopicLike), Status: msg.StatusUnread},
		{UserId: userID, Type: int(msg.TypeUserLevelUp), Status: msg.StatusUnread},
		{UserId: userID + 1, Type: int(msg.TypeCommentReply), Status: msg.StatusUnread},
	}
	for _, message := range messages {
		if err := repositories.MessageRepository.Create(sqls.DB(), message); err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	if got := MessageService.GetUnreadReplyCount(userID); got != 2 {
		t.Fatalf("unread reply count = %d, want 2", got)
	}

	MessageService.MarkRepliesRead(userID)

	var unreadReplyCount int64
	if err := db.Model(&models.Message{}).
		Where("user_id = ? and status = ? and type in ?", userID, msg.StatusUnread, msg.ReplyTypes).
		Count(&unreadReplyCount).Error; err != nil {
		t.Fatal(err)
	}
	if unreadReplyCount != 0 {
		t.Fatalf("unread reply count after mark read = %d, want 0", unreadReplyCount)
	}

	var unreadOtherCount int64
	if err := db.Model(&models.Message{}).
		Where("user_id = ? and status = ? and type not in ?", userID, msg.StatusUnread, msg.ReplyTypes).
		Count(&unreadOtherCount).Error; err != nil {
		t.Fatal(err)
	}
	if unreadOtherCount != 2 {
		t.Fatalf("unread non-reply count after mark read = %d, want 2", unreadOtherCount)
	}
}

func TestBuildEmailNoticeSubjectAvoidsBlankSitePrefix(t *testing.T) {
	got := MessageService.buildEmailNoticeSubject("", "你的话题被设为推荐")
	if got != "你的话题被设为推荐" {
		t.Fatalf("expected subject without blank site prefix, got %q", got)
	}
}

func TestBuildEmailNoticeSubjectIncludesSiteTitle(t *testing.T) {
	got := MessageService.buildEmailNoticeSubject("BBS-GO", "你的话题被设为推荐")
	if got != "BBS-GO - 你的话题被设为推荐" {
		t.Fatalf("expected subject with site title, got %q", got)
	}
}

func TestBuildEmailNoticeContentFallsBackToNoticeTitle(t *testing.T) {
	got := MessageService.buildEmailNoticeContent("", "你的话题被设为推荐")
	if got != "你的话题被设为推荐" {
		t.Fatalf("expected notice title fallback, got %q", got)
	}
}

func TestBuildEmailNoticeDetailURLUsesTopicForRecommend(t *testing.T) {
	idcodec.Init(1)

	got := MessageService.buildEmailNoticeDetailURL(&models.Message{
		Type:      int(msg.TypeTopicRecommend),
		ExtraData: `{"topicId":123}`,
	})

	if got != "/topic/"+idcodec.Encode(123) {
		t.Fatalf("expected topic detail url, got %q", got)
	}
}
