package services

import (
	"fmt"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
)

func TestTopicAuthorFiltersApplyBeforePaginationAndToStickyTopics(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Category{}, &models.Topic{}, &models.UserFeed{}); err != nil {
		t.Fatal(err)
	}
	for _, category := range []models.Category{
		{Model: models.Model{Id: 1}, Name: "daily"},
		{Model: models.Model{Id: 2}, Name: "daily-child", ParentId: 1},
		{Model: models.Model{Id: 3}, Name: "other"},
	} {
		if err := db.Create(&category).Error; err != nil {
			t.Fatal(err)
		}
	}
	var selectedIds []int64
	for i := 0; i < 75; i++ {
		topic := models.Topic{
			CategoryId: int64(1 + i%2), UserId: int64(1 + i%3),
			Title: fmt.Sprintf("daily-%d", i), Status: constants.StatusOk,
			LastCommentTime: int64(i + 1), Sticky: true, StickyTime: int64(i + 1),
		}
		if err := db.Create(&topic).Error; err != nil {
			t.Fatal(err)
		}
		if topic.UserId != 3 {
			selectedIds = append(selectedIds, topic.Id)
		}
	}
	for _, topic := range []models.Topic{
		{CategoryId: 3, UserId: 1, Title: "other category", Sticky: true, StickyTime: 100, LastCommentTime: 100},
		{CategoryId: 1, UserId: 1, Title: "deleted", Status: constants.StatusDeleted, Sticky: true, StickyTime: 101, LastCommentTime: 101},
		{CategoryId: 1, UserId: 1, Title: "pending", Status: 2, Sticky: true, StickyTime: 102, LastCommentTime: 102},
	} {
		if err := db.Create(&topic).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, sort := range []string{"latestPublish", "latestReply"} {
		seen := map[int64]bool{}
		cursor := int64(0)
		for page := 0; page < 4; page++ {
			topics, next, more := TopicService.GetTopics(nil, 1, cursor, "", sort, "", 1, 2)
			for _, topic := range topics {
				if seen[topic.Id] || topic.UserId == 3 || topic.CategoryId == 3 || topic.Status != constants.StatusOk {
					t.Fatalf("%s returned duplicate or unselected topic: %#v", sort, topic)
				}
				seen[topic.Id] = true
			}
			if !more {
				break
			}
			if next == cursor {
				t.Fatal("pagination did not advance")
			}
			cursor = next
		}
		if len(seen) != len(selectedIds) {
			t.Fatalf("%s got %d topics, want %d", sort, len(seen), len(selectedIds))
		}
	}
	sticky := TopicService.GetStickyTopics(1, 3, "", "", 1)
	if len(sticky) != 3 {
		t.Fatalf("expected 3 sticky topics, got %d", len(sticky))
	}
	for _, topic := range sticky {
		if topic.UserId != 1 || topic.CategoryId == 3 {
			t.Fatalf("unexpected sticky topic: %#v", topic)
		}
	}
	all, _, _ := TopicService.GetTopics(nil, 1, 0, "", "latestPublish", "")
	foundThirdAuthor := false
	for _, topic := range all {
		foundThirdAuthor = foundThirdAuthor || topic.UserId == 3
	}
	if !foundThirdAuthor {
		t.Fatal("clearing users must restore all authors")
	}
	unknown, _, more := TopicService.GetTopics(nil, 1, 0, "", "latestPublish", "", 999)
	if len(unknown) != 0 || more {
		t.Fatal("unknown authors must produce an empty page")
	}
	for i, author := range []int64{1, 3} {
		if err := db.Create(&models.UserFeed{UserId: 9, AuthorId: author, DataId: author, DataType: constants.EntityTopic, CreateTime: int64(i + 1)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	followed, _, _ := TopicService.GetTopics(&models.User{Model: models.Model{Id: 9}}, constants.CategoryIdFollow, 0, "", "", "", 1)
	if len(followed) != 1 || followed[0].UserId != 1 {
		t.Fatalf("followed topics must respect authors: %#v", followed)
	}
}

func TestCategoryAuthorsOnlyIncludeVisibleCategoryParticipants(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Category{}, &models.Topic{}); err != nil {
		t.Fatal(err)
	}
	for _, category := range []models.Category{
		{Model: models.Model{Id: 1}, Name: "daily"},
		{Model: models.Model{Id: 2}, Name: "daily-child", ParentId: 1},
		{Model: models.Model{Id: 3}, Name: "other"},
	} {
		if err := db.Create(&category).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 6; i++ {
		user := models.User{Model: models.Model{Id: int64(i)}, Nickname: fmt.Sprintf("user-%d", i)}
		if i == 6 {
			user.Status = constants.StatusDeleted
		}
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, topic := range []models.Topic{
		{CategoryId: 1, UserId: 1}, {CategoryId: 1, UserId: 1}, {CategoryId: 2, UserId: 2},
		{CategoryId: 3, UserId: 3}, {CategoryId: 1, UserId: 4, Status: 2},
		{CategoryId: 1, UserId: 5, Status: constants.StatusDeleted}, {CategoryId: 1, UserId: 6},
	} {
		if err := db.Create(&topic).Error; err != nil {
			t.Fatal(err)
		}
	}
	authors, err := TopicService.GetCategoryAuthors(1)
	if err != nil || len(authors) != 2 || authors[0].Id != 1 || authors[1].Id != 2 {
		t.Fatalf("expected two distinct visible authors: %#v, %v", authors, err)
	}
	if err := db.Migrator().DropTable(&models.User{}); err != nil {
		t.Fatal(err)
	}
	if _, err := TopicService.GetCategoryAuthors(1); err == nil {
		t.Fatal("author query failures must propagate")
	}
}
