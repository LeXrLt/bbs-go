package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/resp"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/config"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestCategoryNavsReturnsOnlyRealTopLevelCategories(t *testing.T) {
	previousConfig := config.Instance
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	t.Cleanup(func() {
		config.Instance = previousConfig
	})

	db := setupTopicHandlerCategoryTestDB(t)
	mustCreateTopicHandlerCategory(t, db, &models.Category{
		Model:  models.Model{Id: 1},
		Name:   "real-root",
		Status: constants.StatusOk,
	})
	mustCreateTopicHandlerCategory(t, db, &models.Category{
		Model:    models.Model{Id: 2},
		Name:     "real-child",
		ParentId: 1,
		Status:   constants.StatusOk,
	})

	data := getCategoryNavs(t)
	if len(data) != 1 {
		t.Fatalf("expected one real top-level category, got %#v", data)
	}
	if data[0].Id != 1 {
		t.Fatalf("expected real category id 1, got %#v", data[0])
	}
	for _, item := range data {
		if item.Id <= 0 {
			t.Fatalf("category navs should not include built-in feed category %#v", item)
		}
	}
}

func TestTopicNewStatusReturnsIndependentRoleStatuses(t *testing.T) {
	db := setupTopicHandlerCategoryTestDB(t)
	if err := db.Create(&models.Role{
		Model:  models.Model{Id: 1},
		Name:   "agent",
		Code:   "handler-agent",
		Status: constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create agent role: %v", err)
	}
	if err := db.Create(&models.Role{
		Model:  models.Model{Id: 2},
		Name:   "用户",
		Code:   "handler-user",
		Status: constants.StatusOk,
	}).Error; err != nil {
		t.Fatalf("create user role: %v", err)
	}
	if err := db.Create(&[]models.UserRole{
		{UserId: 2, RoleId: 1},
		{UserId: 3, RoleId: 2},
	}).Error; err != nil {
		t.Fatalf("assign roles: %v", err)
	}
	if err := db.Create(&models.Topic{
		UserId:          2,
		Title:           "new agent topic",
		Status:          constants.StatusOk,
		LastCommentTime: 1,
		CreateTime:      1,
	}).Error; err != nil {
		t.Fatalf("create agent topic: %v", err)
	}
	if err := db.Create(&models.Topic{
		UserId:          3,
		Title:           "new user topic",
		Status:          constants.StatusOk,
		LastCommentTime: 2,
		CreateTime:      2,
	}).Error; err != nil {
		t.Fatalf("create user topic: %v", err)
	}
	if err := db.Create(&[]models.TopicVisibleEvent{
		{TopicId: 1, CreateTime: 1},
		{TopicId: 2, CreateTime: 2},
	}).Error; err != nil {
		t.Fatalf("create topic visible events: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic/new_status?agentAfter=0&userAfter=0", nil)
	common.SetCurrentUser(ctx, &models.User{Model: models.Model{Id: 1}})

	TopicNewStatus(ctx)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Roles []struct {
				RoleName string `json:"roleName"`
				Marker   string `json:"marker"`
				Count    int64  `json:"count"`
			} `json:"roles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success || len(result.Data.Roles) != 2 {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
	if result.Data.Roles[0].RoleName != "agent" || result.Data.Roles[0].Marker != "1" || result.Data.Roles[0].Count != 1 {
		t.Fatalf("unexpected agent response: %s", w.Body.String())
	}
	if result.Data.Roles[1].RoleName != "用户" || result.Data.Roles[1].Marker != "2" || result.Data.Roles[1].Count != 1 {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestTopicNewStatusReturnsFailureWhenStatusQueryFails(t *testing.T) {
	db := setupTopicHandlerCategoryTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic/new_status?agentAfter=0&userAfter=0", nil)
	common.SetCurrentUser(ctx, &models.User{Model: models.Model{Id: 1}})

	TopicNewStatus(ctx)

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if result.Success || result.Message == "" {
		t.Fatalf("query error must return API failure, got %s", w.Body.String())
	}
}

func setupTopicHandlerCategoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:topic_handler_category_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
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
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	sqls.SetDB(db)
	if err := db.AutoMigrate(&models.Category{}, &models.Topic{}, &models.TopicVisibleEvent{}, &models.Role{}, &models.UserRole{}); err != nil {
		t.Fatalf("auto migrate topic handler models: %v", err)
	}
	return db
}

func mustCreateTopicHandlerCategory(t *testing.T, db *gorm.DB, category *models.Category) {
	t.Helper()

	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
}

func getCategoryNavs(t *testing.T) []resp.CategoryResponse {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic/category_navs", nil)

	CategoryNavs(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result struct {
		Success bool                    `json:"success"`
		Data    []resp.CategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", w.Body.String())
	}
	return result.Data
}
