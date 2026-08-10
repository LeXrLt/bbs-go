package migrations

import (
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestMigrateTopicVisibleEventsBackfillsVisibleTopicsIdempotently(t *testing.T) {
	dsn := fmt.Sprintf("file:topic_visible_event_migration_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "t_",
			SingularTable: true,
		},
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)

	if err := db.AutoMigrate(&models.Topic{}, &models.TopicVisibleEvent{}); err != nil {
		t.Fatalf("auto migrate models: %v", err)
	}
	visibleTopic := &models.Topic{Title: "visible", Status: constants.StatusOk, CreateTime: 10}
	hiddenTopic := &models.Topic{Title: "hidden", Status: constants.StatusReview, CreateTime: 20}
	if err := db.Create(&[]*models.Topic{visibleTopic, hiddenTopic}).Error; err != nil {
		t.Fatalf("create topics: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := migrate_topic_visible_events(); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	var events []models.TopicVisibleEvent
	if err := db.Order("id").Find(&events).Error; err != nil {
		t.Fatalf("find visible events: %v", err)
	}
	if len(events) != 1 || events[0].TopicId != visibleTopic.Id || events[0].CreateTime != visibleTopic.CreateTime {
		t.Fatalf("unexpected backfill events: %#v", events)
	}
}
