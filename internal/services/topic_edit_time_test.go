package services

import (
	"reflect"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/req"

	"github.com/mlogclub/simple/common/dates"
)

func TestTopicService_LatestEditSortAndPagination(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Topic{}, &models.Category{}); err != nil {
		t.Fatal(err)
	}
	// IDs, publication times, edit times and reply times intentionally disagree.
	for _, topic := range []models.Topic{
		{Model: models.Model{Id: 1}, CreateTime: 100, EditTime: 400, LastCommentTime: 100},
		{Model: models.Model{Id: 2}, CreateTime: 300, EditTime: 300, LastCommentTime: 500},
		{Model: models.Model{Id: 3}, CreateTime: 200, EditTime: 400, LastCommentTime: 200},
		{Model: models.Model{Id: 4}, CreateTime: 150, EditTime: 150, LastCommentTime: 300},
		{Model: models.Model{Id: 5}, CreateTime: 500, EditTime: 500, Status: constants.StatusDeleted},
		{Model: models.Model{Id: 6}, CreateTime: 600, EditTime: 600, Status: constants.StatusReview},
	} {
		if err := db.Create(&topic).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, sort := range []string{"", "latestEdit", "latestPublish", "invalid", "latestReply"} {
		t.Run(sort, func(t *testing.T) {
			var ids []int64
			cursor := int64(0)
			// A one-item page forces equal edit times across page boundaries.
			for page := 0; page < 6; page++ {
				topics, next, more := TopicService._GetCategoryTopics(0, cursor, 1, "", sort, "")
				for _, topic := range topics {
					ids = append(ids, topic.Id)
				}
				if !more {
					break
				}
				if next == cursor {
					t.Fatal("pagination did not advance")
				}
				cursor = next
			}
			want := []int64{3, 1, 2, 4}
			if sort == "latestReply" {
				want = []int64{2, 4, 3, 1}
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("got topic IDs %v, want %v", ids, want)
			}
		})
	}
}

func TestTopicService_EditUpdatesTimeOnlyOnSuccess(t *testing.T) {
	db := setupTestDB(t)
	initTopicNewStatusSearch(t)
	if err := db.AutoMigrate(&models.Topic{}, &models.Category{}, &models.Tag{}, &models.TopicTag{}); err != nil {
		t.Fatal(err)
	}
	category := models.Category{Type: constants.CategoryTypeNormal, Status: constants.StatusOk}
	if err := db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	topic := models.Topic{
		CategoryId: category.Id, Type: constants.TopicTypeTopic, UserId: 1,
		Title: "original", Content: "original content", ContentType: constants.ContentTypeMarkdown,
		CreateTime: 100, EditTime: 100, LastCommentTime: 200,
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatal(err)
	}
	newer := models.Topic{CategoryId: category.Id, CreateTime: 300, EditTime: 300}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	before, _, _ := TopicService.GetTopics(nil, category.Id, 0, "", "latestEdit", "")
	if len(before) != 2 || before[0].Id != newer.Id {
		t.Fatalf("unexpected initial order: %#v", before)
	}
	form := req.EditTopicReq{CategoryId: category.Id, Title: "edited", Content: "edited content"}
	start := dates.NowTimestamp()
	if err := TopicService.Edit(topic.UserId, topic.Id, form); err != nil {
		t.Fatal(err)
	}
	saved := TopicService.Get(topic.Id)
	if saved.EditTime < start || saved.EditTime > dates.NowTimestamp() || saved.CreateTime != 100 || saved.LastCommentTime != 200 {
		t.Fatalf("unexpected timestamps after editing: %#v", saved)
	}
	after, _, _ := TopicService.GetTopics(nil, category.Id, 0, "", "latestEdit", "")
	if len(after) != 2 || after[0].Id != topic.Id {
		t.Fatalf("edited topic should move first: %#v", after)
	}
	editTime := saved.EditTime
	TopicService.IncrViewCount(topic.Id)
	if err := TopicService.SetRecommend(topic.Id, true); err != nil {
		t.Fatal(err)
	}
	if got := TopicService.Get(topic.Id); got.EditTime != editTime {
		t.Fatalf("non-edit operation changed edit time: got %d, want %d", got.EditTime, editTime)
	}

	// Force a failure after the topic update, inside the same transaction.
	if err := db.Model(&models.Topic{}).Where("id = ?", topic.Id).UpdateColumn("edit_time", 123).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&models.TopicTag{}); err != nil {
		t.Fatal(err)
	}
	form.Title = "must roll back"
	if err := TopicService.Edit(topic.UserId, topic.Id, form); err == nil {
		t.Fatal("expected editing to fail when updating tags")
	}
	if got := TopicService.Get(topic.Id); got.EditTime != 123 || got.Title != "edited" {
		t.Fatalf("failed edit changed stored topic: %#v", got)
	}
}
