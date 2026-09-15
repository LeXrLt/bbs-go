package repositories

import (
	"testing"
	"time"

	"bbs-go/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTopicVisibleEventRepositoryGetLatestIDs(t *testing.T) {
	dsn := "file:topic_visible_event_repository_test_" + time.Now().Format("20060102150405.000000000") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&models.TopicVisibleEvent{}); err != nil {
		t.Fatalf("auto migrate visible events: %v", err)
	}
	if err := db.Create(&[]models.TopicVisibleEvent{
		{TopicId: 101, CreateTime: 1},
		{TopicId: 101, CreateTime: 2},
		{TopicId: 202, CreateTime: 3},
	}).Error; err != nil {
		t.Fatalf("create visible events: %v", err)
	}

	got := TopicVisibleEventRepository.GetLatestIDs(db, []int64{101, 202, 303})
	if got[101] != 2 || got[202] != 3 {
		t.Fatalf("latest event IDs = %#v, want topic 101=2 and topic 202=3", got)
	}
	if _, ok := got[303]; ok {
		t.Fatalf("unexpected event ID for topic without events: %#v", got)
	}
}
