package services

import (
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/req"
	"bbs-go/internal/pkg/idcodec"
	"bbs-go/internal/pkg/search"

	"github.com/mlogclub/simple/sqls"
)

func TestTopicWithdrawAndRepublishPreservesRecords(t *testing.T) {
	db := setupTestDB(t)
	initTopicNewStatusSearch(t)
	if err := db.AutoMigrate(&models.Topic{}, &models.Category{}, &models.Tag{}, &models.TopicTag{},
		&models.Comment{}, &models.UserLike{}, &models.Vote{}, &models.VoteRecord{},
		&models.Attachment{}, &models.AttachmentDownloadLog{}, &models.TopicVisibleEvent{}); err != nil {
		t.Fatal(err)
	}
	category := models.Category{Type: constants.CategoryTypeNormal, Status: constants.StatusOk}
	if err := db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	topic := models.Topic{Type: constants.TopicTypeTopic, UserId: 10, CategoryId: category.Id,
		Title: "original", Content: "original content", Status: constants.StatusOk,
		CreateTime: 100, EditTime: 100, LastCommentTime: 150, CommentCount: 1, LikeCount: 1}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatal(err)
	}
	related := []any{
		&models.Comment{UserId: 11, EntityType: constants.EntityTopic, EntityId: topic.Id, Status: constants.StatusOk},
		&models.UserLike{UserId: 11, EntityType: constants.EntityTopic, EntityId: topic.Id},
		&models.Attachment{Id: "attached", TopicId: topic.Id, UserId: 10, Status: constants.StatusOk},
		&models.AttachmentDownloadLog{UserId: 11, AttachmentId: "attached"},
		&models.TopicVisibleEvent{TopicId: topic.Id, CreateTime: 100},
	}
	for _, record := range related {
		if err := db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	vote := models.Vote{TopicId: topic.Id, UserId: 10}
	if err := db.Create(&vote).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.VoteRecord{UserId: 11, VoteId: vote.Id}).Error; err != nil {
		t.Fatal(err)
	}

	if err := TopicService.Withdraw(topic.Id, 11); err == nil {
		t.Fatal("another user withdrew post")
	}
	for _, postType := range []constants.TopicType{constants.TopicTypeTweet, constants.TopicTypeQA} {
		other := models.Topic{UserId: 10, Type: postType, Status: constants.StatusOk}
		if err := db.Create(&other).Error; err != nil {
			t.Fatal(err)
		}
		if err := TopicService.Withdraw(other.Id, 10); err == nil {
			t.Fatalf("withdrew type %d", postType)
		}
	}
	if err := TopicService.Withdraw(topic.Id, 10); err != nil {
		t.Fatal(err)
	}
	if err := TopicService.Withdraw(topic.Id, 10); err == nil {
		t.Fatal("withdrew twice")
	}
	if got := TopicService.Get(topic.Id); got.Status != constants.StatusWithdrawn || got.EditTime != 100 {
		t.Fatalf("unexpected withdrawn post: %+v", got)
	}
	if posts, _, _ := TopicService.GetTopics(nil, category.Id, 0, "", "", ""); len(posts) != 0 {
		t.Fatalf("withdrawn post in listing: %+v", posts)
	}
	if docs, _, err := search.SearchTopic("original", 0, nil, 0, 1, 20); err != nil || len(docs) != 0 {
		t.Fatalf("withdrawn post in search: %+v, %v", docs, err)
	}
	if err := TopicService.Edit(11, topic.Id, req.EditTopicReq{CategoryId: category.Id, Title: "hijacked"}); err == nil {
		t.Fatal("another user republished post")
	}
	if err := TopicService.Audit(topic.Id); err != nil {
		t.Fatal(err)
	}
	if got := TopicService.Get(topic.Id); got.Status != constants.StatusWithdrawn {
		t.Fatalf("audit republished withdrawn post: %+v", got)
	}
	if err := TopicService.Edit(10, topic.Id, req.EditTopicReq{CategoryId: category.Id, Title: "republished", Content: "edited"}); err != nil {
		t.Fatal(err)
	}
	got := TopicService.Get(topic.Id)
	if got.Status != constants.StatusOk || got.CreateTime != 100 || got.EditTime <= 100 || got.LastCommentTime != 150 || got.CommentCount != 1 || got.LikeCount != 1 {
		t.Fatalf("unexpected republished post: %+v", got)
	}
	for _, item := range []struct {
		table any
		where string
		args  []any
	}{
		{&models.Comment{}, "entity_type = ? AND entity_id = ?", []any{constants.EntityTopic, topic.Id}},
		{&models.UserLike{}, "entity_type = ? AND entity_id = ?", []any{constants.EntityTopic, topic.Id}},
		{&models.Vote{}, "topic_id = ?", []any{topic.Id}},
		{&models.VoteRecord{}, "vote_id = ?", []any{vote.Id}},
		{&models.Attachment{}, "topic_id = ? AND status = ?", []any{topic.Id, constants.StatusOk}},
		{&models.AttachmentDownloadLog{}, "attachment_id = ?", []any{"attached"}},
		{&models.TopicVisibleEvent{}, "topic_id = ?", []any{topic.Id}},
	} {
		var count int64
		if err := sqls.DB().Model(item.table).Where(item.where, item.args...).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("related record %T: count %d, %v", item.table, count, err)
		}
	}
	if docs, _, err := search.SearchTopic("republished", 0, nil, 0, 1, 20); err != nil || len(docs) != 1 {
		t.Fatalf("republished post not searchable: %+v, %v", docs, err)
	}
}

func TestWithdrawnTopicCommentsAreUnavailable(t *testing.T) {
	idcodec.Init(1)
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Topic{}, &models.Comment{}); err != nil {
		t.Fatal(err)
	}
	topic := models.Topic{UserId: 10, Type: constants.TopicTypeTopic, Status: constants.StatusWithdrawn}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{EntityType: constants.EntityTopic, EntityId: topic.Id, Status: constants.StatusOk}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	if CommentService.ParentAvailable(constants.EntityTopic, topic.Id) || CommentService.ParentAvailable(constants.EntityComment, comment.Id) {
		t.Fatal("comments accessible on withdrawn topic")
	}
	if _, err := CommentService.Publish(11, req.CreateCommentReq{EntityType: constants.EntityTopic, EntityId: idcodec.Encode(topic.Id), Content: "new"}); err == nil {
		t.Fatal("comment created on withdrawn topic")
	}
}
