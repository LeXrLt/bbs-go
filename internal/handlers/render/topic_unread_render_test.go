package render

import (
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/idcodec"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestBuildSimpleTopicsWithUnreadExcludesReadAndOwnTopics(t *testing.T) {
	dsn := fmt.Sprintf("file:topic_unread_render_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.LevelConfig{},
		&models.TopicVisibleEvent{},
		&models.TopicRead{},
		&models.TopicTag{},
		&models.Tag{},
		&models.UserLike{},
		&models.Vote{},
	); err != nil {
		t.Fatalf("auto migrate unread render models: %v", err)
	}
	sqls.SetDB(db)
	idcodec.Init(1)
	cache.UserCache.Invalidate(1)
	cache.UserCache.Invalidate(2)

	topics := []models.Topic{
		{Model: models.Model{Id: 101}, UserId: 2, Title: "unread"},
		{Model: models.Model{Id: 102}, UserId: 2, Title: "already read"},
		{Model: models.Model{Id: 103}, UserId: 1, Title: "own topic"},
	}
	if err := db.Create(&[]models.TopicVisibleEvent{
		{TopicId: 101, CreateTime: 1},
		{TopicId: 102, CreateTime: 2},
		{TopicId: 103, CreateTime: 3},
	}).Error; err != nil {
		t.Fatalf("create visible events: %v", err)
	}
	if err := db.Create(&models.TopicRead{UserId: 1, TopicId: 102, ReadTime: 4}).Error; err != nil {
		t.Fatalf("create topic read: %v", err)
	}

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetCurrentUser(ctx, &models.User{Model: models.Model{Id: 1}})
	responses := BuildSimpleTopicsWithUnread(ctx, topics, 0)

	if len(responses) != 3 {
		t.Fatalf("response count = %d, want 3", len(responses))
	}
	if !responses[0].Unread {
		t.Fatal("new candidate topic should be unread")
	}
	if responses[1].Unread {
		t.Fatal("persisted read topic should not be unread")
	}
	if responses[2].Unread {
		t.Fatal("current user's own topic should not be unread")
	}
}

func TestBuildSimpleTopicsWithRoleUnreadUsesEachAuthorsBaseline(t *testing.T) {
	dsn := fmt.Sprintf("file:topic_role_unread_render_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.LevelConfig{},
		&models.Role{},
		&models.UserRole{},
		&models.TopicVisibleEvent{},
		&models.TopicRead{},
		&models.TopicTag{},
		&models.Tag{},
		&models.UserLike{},
		&models.Vote{},
	); err != nil {
		t.Fatalf("auto migrate role unread render models: %v", err)
	}
	sqls.SetDB(db)
	idcodec.Init(1)

	roles := []models.Role{
		{Model: models.Model{Id: 1}, Name: "agent", Code: "render-agent", Status: constants.StatusOk},
		{Model: models.Model{Id: 2}, Name: "用户", Code: "render-user", Status: constants.StatusOk},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := db.Create(&[]models.UserRole{
		{UserId: 2, RoleId: 1},
		{UserId: 3, RoleId: 2},
		{UserId: 4, RoleId: 1},
		{UserId: 4, RoleId: 2},
		{UserId: 6, RoleId: 2},
	}).Error; err != nil {
		t.Fatalf("assign roles: %v", err)
	}

	topics := []models.Topic{
		{Model: models.Model{Id: 201}, UserId: 2, Title: "agent at baseline"},
		{Model: models.Model{Id: 202}, UserId: 3, Title: "new user topic"},
		{Model: models.Model{Id: 203}, UserId: 4, Title: "overlapping roles"},
		{Model: models.Model{Id: 204}, UserId: 5, Title: "roleless topic"},
		{Model: models.Model{Id: 205}, UserId: 6, Title: "read user topic"},
		{Model: models.Model{Id: 206}, UserId: 1, Title: "own topic"},
	}
	for index, topic := range topics {
		if err := db.Create(&models.TopicVisibleEvent{TopicId: topic.Id, CreateTime: int64(index + 1)}).Error; err != nil {
			t.Fatalf("create visible event: %v", err)
		}
	}
	if err := db.Create(&models.TopicRead{UserId: 1, TopicId: 205, ReadTime: 7}).Error; err != nil {
		t.Fatalf("create topic read: %v", err)
	}

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetCurrentUser(ctx, &models.User{Model: models.Model{Id: 1}})
	responses := BuildSimpleTopicsWithRoleUnread(ctx, topics, map[string]int64{
		"agent": 1,
		"用户":    0,
	})

	wantUnread := []bool{false, true, true, false, false, false}
	for index, want := range wantUnread {
		if responses[index].Unread != want {
			t.Fatalf("topic %d unread = %t, want %t", topics[index].Id, responses[index].Unread, want)
		}
	}
}
