package repositories

import (
	"testing"

	"bbs-go/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTopicUnreadBaselineRepositoryCreateIfMissingDoesNotMoveBaseline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:topic_unread_baseline_repository?mode=memory&cache=shared"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&models.TopicUnreadBaseline{}); err != nil {
		t.Fatalf("auto migrate topic unread baseline: %v", err)
	}

	created, err := TopicUnreadBaselineRepository.CreateIfMissing(db, &models.TopicUnreadBaseline{
		UserId: 1, RoleName: "用户", EventId: 6, CreateTime: 1,
	})
	if err != nil || !created {
		t.Fatalf("first create = (%t, %v), want (true, nil)", created, err)
	}
	created, err = TopicUnreadBaselineRepository.CreateIfMissing(db, &models.TopicUnreadBaseline{
		UserId: 1, RoleName: "用户", EventId: 9, CreateTime: 2,
	})
	if err != nil || created {
		t.Fatalf("duplicate create = (%t, %v), want (false, nil)", created, err)
	}

	baseline, err := TopicUnreadBaselineRepository.Get(db, 1, "用户")
	if err != nil || baseline == nil || baseline.EventId != 6 {
		t.Fatalf("stored baseline = (%#v, %v), want event 6", baseline, err)
	}
}
