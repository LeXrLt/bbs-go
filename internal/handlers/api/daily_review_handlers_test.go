package api

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/permissions"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/services"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestDailyReviewIngestRequiresLoginAndPermission(t *testing.T) {
	setupDailyReviewHandlerTestDB(t)

	unauthenticated := runDailyReviewIngest(t, nil, "2026-10-28", "focus", "# report")
	if errorCode(unauthenticated.Body.Bytes()) != 1 {
		t.Fatalf("unauthenticated response = %s", unauthenticated.Body.String())
	}

	regularUser := &models.User{Model: models.Model{Id: 92001}}
	forbidden := runDailyReviewIngest(t, regularUser, "2026-10-28", "focus", "# report")
	if errorCode(forbidden.Body.Bytes()) != 2 {
		t.Fatalf("forbidden response = %s", forbidden.Body.String())
	}
}

func TestDailyReviewIngestCreatesReportForOwner(t *testing.T) {
	db := setupDailyReviewHandlerTestDB(t)
	owner := &models.User{Model: models.Model{Id: 1}, Roles: constants.RoleOwner}
	recorder := runDailyReviewIngest(t, owner, "2026-10-28", "focus", "# 重点公司\n\n【公司A】")
	if recorder.Code != http.StatusOK || !responseSucceeded(recorder.Body.Bytes()) {
		t.Fatalf("ingest response = %d %s", recorder.Code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&models.DailyReviewReport{}).Count(&count).Error; err != nil {
		t.Fatalf("count reports: %v", err)
	}
	if count != 1 {
		t.Fatalf("report count = %d, want 1", count)
	}
}

func TestDailyReviewIngestCreatesReportForGrantedUser(t *testing.T) {
	db := setupDailyReviewHandlerTestDB(t)
	user := &models.User{Model: models.Model{Id: 92002}}
	grantDailyReviewIngestPermission(t, db, user.Id)

	recorder := runDailyReviewIngest(t, user, "2026-10-28", "focus", "# 重点公司\n\n【公司A】")
	if recorder.Code != http.StatusOK || !responseSucceeded(recorder.Body.Bytes()) {
		t.Fatalf("ingest response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDailyReviewIngestAcceptsEscapedMarkdownNearLimit(t *testing.T) {
	setupDailyReviewHandlerTestDB(t)
	owner := &models.User{Model: models.Model{Id: 1}, Roles: constants.RoleOwner}
	prefix := "# Escaped report\n\n"
	markdown := prefix + strings.Repeat(`\"`, 300_000) + strings.Repeat("\n", 10_000)
	markdown += strings.Repeat("a", services.MaxDailyReviewMarkdownLength-len(markdown)-1)
	body, err := json.Marshal(map[string]string{"markdown": markdown})
	if err != nil {
		t.Fatalf("marshal escaped upload: %v", err)
	}
	if len(body) <= services.MaxDailyReviewMarkdownLength+64*1024 {
		t.Fatalf("escaped body size = %d, want larger than previous request limit", len(body))
	}

	recorder := runDailyReviewIngest(t, owner, "2026-10-28", "focus", markdown)
	if recorder.Code != http.StatusOK || !responseSucceeded(recorder.Body.Bytes()) {
		t.Fatalf("near-limit ingest response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDailyReviewIngestRejectsDecodedMarkdownOverLimit(t *testing.T) {
	setupDailyReviewHandlerTestDB(t)
	owner := &models.User{Model: models.Model{Id: 1}, Roles: constants.RoleOwner}
	markdown := strings.Repeat("a", services.MaxDailyReviewMarkdownLength+1)

	recorder := runDailyReviewIngest(t, owner, "2026-10-28", "focus", markdown)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("over-limit ingest response = %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "markdown exceeds 1000000 bytes") {
		t.Fatalf("unexpected over-limit response: %s", recorder.Body.String())
	}
}

func TestDailyReviewIngestDoesNotExposeDatabaseErrors(t *testing.T) {
	db := setupDailyReviewHandlerTestDB(t)
	if err := db.Migrator().DropTable(&models.DailyReviewReport{}); err != nil {
		t.Fatalf("drop reports table: %v", err)
	}
	owner := &models.User{Model: models.Model{Id: 1}, Roles: constants.RoleOwner}
	recorder := runDailyReviewIngest(t, owner, "2026-10-28", "focus", "# 重点公司")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("ingest status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "failed to save daily review") || strings.Contains(strings.ToLower(body), "no such table") {
		t.Fatalf("unexpected internal error response: %s", body)
	}
}

func TestDailyReviewReportReturnsSanitizedHTML(t *testing.T) {
	setupDailyReviewHandlerTestDB(t)
	reportDate, _ := time.Parse(time.DateOnly, "2026-10-28")
	if _, err := services.DailyReviewService.Save(reportDate, "focus", "# 重点公司\n\n**bold**\n\n<script>alert(1)</script>\n<iframe src=\"https://example.com\"></iframe>"); err != nil {
		t.Fatalf("save report: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "kind", Value: "focus"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/daily-review/report/focus?date=2026-10-28", nil)
	ctx.Request.URL.RawQuery = "date=2026-10-28"
	DailyReviewReport(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("report status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ContentHTML string `json:"content_html"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !result.Success || !strings.Contains(result.Data.ContentHTML, "<strong>bold</strong>") {
		t.Fatalf("unexpected rendered report: %s", recorder.Body.String())
	}
	for _, forbidden := range []string{"<script", "<iframe"} {
		if strings.Contains(strings.ToLower(result.Data.ContentHTML), forbidden) {
			t.Fatalf("report contains forbidden markup %q: %s", forbidden, result.Data.ContentHTML)
		}
	}
}

func TestDailyReviewHandlersRejectInvalidDateAndKind(t *testing.T) {
	setupDailyReviewHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/daily-review/dashboard?date=2026-02-30", nil)
	ctx.Request.URL.RawQuery = "date=2026-02-30"
	DailyReviewDashboard(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid date status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "kind", Value: "unknown"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/daily-review/report/unknown?date=2026-10-28", nil)
	ctx.Request.URL.RawQuery = "date=2026-10-28"
	DailyReviewReport(ctx)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("invalid kind status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDailyReviewDashboardReturnsSixPendingSections(t *testing.T) {
	setupDailyReviewHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/daily-review/dashboard?date=2026-10-28", nil)
	ctx.Request.URL.RawQuery = "date=2026-10-28"
	DailyReviewDashboard(ctx)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Sections []services.DailyReviewSection `json:"sections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if !result.Success || len(result.Data.Sections) != 6 {
		t.Fatalf("unexpected dashboard: %s", recorder.Body.String())
	}
	for _, section := range result.Data.Sections {
		if section.Status != "pending" {
			t.Fatalf("section should be pending: %#v", section)
		}
	}
}

func setupDailyReviewHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousConfig := config.Instance
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	t.Cleanup(func() { config.Instance = previousConfig })

	dsn := fmt.Sprintf("file:daily_review_handler_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)
	if err := db.AutoMigrate(
		&models.DailyReviewReport{}, &models.DailyReviewReportRevision{},
		&models.Permission{}, &models.Role{}, &models.RolePermission{}, &models.UserRole{},
	); err != nil {
		t.Fatalf("migrate daily review handler models: %v", err)
	}
	return db
}

func grantDailyReviewIngestPermission(t *testing.T, db *gorm.DB, userId int64) {
	t.Helper()
	now := time.Now().UnixMilli()
	permission := &models.Permission{
		Type: permissions.TypeDashboard, Code: permissions.PermissionDailyReviewIngest.Code,
		Name: "Daily review ingest", GroupName: permissions.GroupContent,
		Status: constants.StatusOk, CreateTime: now, UpdateTime: now,
	}
	if err := db.Create(permission).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	role := &models.Role{
		Type: 1, Name: "Daily review sync", Code: "daily-review-sync",
		Status: constants.StatusOk, CreateTime: now, UpdateTime: now,
	}
	if err := db.Create(role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := db.Create(&models.RolePermission{RoleId: role.Id, PermissionId: permission.Id, CreateTime: now}).Error; err != nil {
		t.Fatalf("create role permission: %v", err)
	}
	if err := db.Create(&models.UserRole{UserId: userId, RoleId: role.Id, CreateTime: now}).Error; err != nil {
		t.Fatalf("create user role: %v", err)
	}
	services.PermissionService.InvalidateUser(userId)
}

func runDailyReviewIngest(t *testing.T, user *models.User, date, kind, markdown string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"markdown": markdown})
	if err != nil {
		t.Fatalf("marshal upload: %v", err)
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "kind", Value: kind}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/daily-review/ingest/"+kind+"?date="+date, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.URL.RawQuery = "date=" + date
	if user != nil {
		common.SetCurrentUser(ctx, user)
	}
	DailyReviewIngest(ctx)
	return recorder
}

func errorCode(body []byte) int {
	var result struct {
		ErrorCode int `json:"errorCode"`
	}
	_ = json.Unmarshal(body, &result)
	return result.ErrorCode
}

func responseSucceeded(body []byte) bool {
	var result struct {
		Success bool `json:"success"`
	}
	_ = json.Unmarshal(body, &result)
	return result.Success
}
