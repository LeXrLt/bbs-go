package repositories

import (
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTopicReadRepositoryPersistsUniqueReadsAndFindsThemInBulk(t *testing.T) {
	dsn := fmt.Sprintf("file:topic_read_repository_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&models.TopicRead{}); err != nil {
		t.Fatalf("auto migrate topic reads: %v", err)
	}

	for _, readTime := range []int64{100, 200} {
		if err := TopicReadRepository.MarkRead(db, &models.TopicRead{
			UserId: 1, TopicId: 10, ReadTime: readTime,
		}); err != nil {
			t.Fatalf("mark topic read: %v", err)
		}
	}
	if err := TopicReadRepository.MarkRead(db, &models.TopicRead{
		UserId: 1, TopicId: 20, ReadTime: 300,
	}); err != nil {
		t.Fatalf("mark second topic read: %v", err)
	}

	var count int64
	if err := db.Model(&models.TopicRead{}).Count(&count).Error; err != nil {
		t.Fatalf("count topic reads: %v", err)
	}
	if count != 2 {
		t.Fatalf("topic read count = %d, want 2", count)
	}

	ids := TopicReadRepository.FindReadTopicIDs(db, 1, []int64{10, 20, 30})
	if len(ids) != 2 || ids[0] != 10 || ids[1] != 20 {
		t.Fatalf("read topic IDs = %#v, want [10 20]", ids)
	}
	if ids := TopicReadRepository.FindReadTopicIDs(db, 2, []int64{10, 20}); len(ids) != 0 {
		t.Fatalf("other user's read topic IDs = %#v, want none", ids)
	}
}
