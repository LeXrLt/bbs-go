package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/resp"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/idcodec"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestCommentCreateAcceptsMarkdownFormAndReturnsSafeHTML(t *testing.T) {
	db := setupCommentHandlerTestDB(t)
	source := `## Heading

**bold**

<script>alert("script")</script>
<iframe src="https://example.com"></iframe>`
	form := url.Values{
		"entityType":  {constants.EntityArticle},
		"entityId":    {"1"},
		"content":     {source},
		"contentType": {string(constants.ContentTypeMarkdown)},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/comment/create", strings.NewReader(form.Encode()))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	common.SetCurrentUser(ctx, &models.User{
		Model:         models.Model{Id: 99},
		Status:        constants.StatusOk,
		EmailVerified: true,
		CreateTime:    time.Now().Add(-24 * time.Hour).UnixMilli(),
	})

	CommentCreate(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var result struct {
		Success bool                 `json:"success"`
		Message string               `json:"message"`
		Data    resp.CommentResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", recorder.Body.String())
	}
	if result.Data.ContentType != constants.ContentTypeMarkdown {
		t.Fatalf("expected markdown content type, got %q", result.Data.ContentType)
	}
	for _, expected := range []string{"<h2>Heading</h2>", "<strong>bold</strong>"} {
		if !strings.Contains(result.Data.Content, expected) {
			t.Fatalf("rendered response does not contain %q: %s", expected, result.Data.Content)
		}
	}
	for _, forbidden := range []string{"<script", "<iframe"} {
		if strings.Contains(strings.ToLower(result.Data.Content), forbidden) {
			t.Fatalf("rendered response must not contain %q: %s", forbidden, result.Data.Content)
		}
	}

	var stored models.Comment
	if err := db.First(&stored, result.Data.Id).Error; err != nil {
		t.Fatalf("load stored comment: %v", err)
	}
	if stored.Content != source || stored.ContentType != constants.ContentTypeMarkdown {
		t.Fatalf("unexpected stored comment: %#v", stored)
	}
}

func setupCommentHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousConfig := config.Instance
	previousCodec := idcodec.Instance
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	idcodec.Init(1)
	t.Cleanup(func() {
		config.Instance = previousConfig
		idcodec.Instance = previousCodec
	})

	dsn := fmt.Sprintf("file:comment_handler_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
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
	if err := db.AutoMigrate(&models.Comment{}, &models.User{}, &models.SysConfig{}, &models.LevelConfig{}); err != nil {
		t.Fatalf("auto migrate comment handler dependencies: %v", err)
	}
	return db
}
