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

func TestMigrateTopicReadsIsIdempotent(t *testing.T) {
	dsn := fmt.Sprintf("file:topic_reads_migration_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqls.SetDB(db)

	for i := 0; i < 2; i++ {
		if err := migrate_topic_reads(); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}
	if !db.Migrator().HasTable(&models.TopicRead{}) {
		t.Fatal("topic read table was not created")
	}
	if !db.Migrator().HasIndex(&models.TopicRead{}, "uk_topic_read_user_topic") {
		t.Fatal("topic read unique index was not created")
	}
}
