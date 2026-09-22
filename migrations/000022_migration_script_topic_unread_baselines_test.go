package migrations

import (
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestMigrateTopicUnreadBaselinesIsIdempotent(t *testing.T) {
	dsn := fmt.Sprintf("file:topic_unread_baselines_migration_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqls.SetDB(db)

	for i := 0; i < 2; i++ {
		if err := migrate_topic_unread_baselines(); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}
	if !db.Migrator().HasTable(&models.TopicUnreadBaseline{}) {
		t.Fatal("topic unread baseline table was not created")
	}
	if !db.Migrator().HasIndex(&models.TopicUnreadBaseline{}, "uk_topic_unread_baseline_user_role") {
		t.Fatal("topic unread baseline unique index was not created")
	}
}
