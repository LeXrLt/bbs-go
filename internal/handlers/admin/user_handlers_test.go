package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/idcodec"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/pkg/search"
	"bbs-go/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/common/passwd"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestUserListFiltersForbiddenUsers(t *testing.T) {
	db := setupAdminUserTestDB(t)
	now := time.Now().UnixMilli()
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 1},
		Nickname:         "normal",
		ForbiddenEndTime: 0,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 2},
		Nickname:         "forbidden",
		ForbiddenEndTime: now + 3600_000,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 3},
		Nickname:         "forever",
		ForbiddenEndTime: -1,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 4},
		Nickname:         "expired",
		ForbiddenEndTime: now - 3600_000,
	})

	users := postUserList(t, "forbidden=true")

	gotIDs := make([]int64, 0, len(users))
	for _, user := range users {
		gotIDs = append(gotIDs, int64(user["id"].(float64)))
		if _, exists := user["password"]; exists {
			t.Fatalf("expected password hash to be excluded, got %#v", user)
		}
		if forbidden, ok := user["forbidden"].(bool); !ok || !forbidden {
			t.Fatalf("expected only forbidden users, got %#v", user)
		}
	}
	wantIDs := []int64{3, 2}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
		}
	}
}

func TestUserListFiltersNonForbiddenUsers(t *testing.T) {
	db := setupAdminUserTestDB(t)
	now := time.Now().UnixMilli()
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 1},
		Nickname:         "normal",
		ForbiddenEndTime: 0,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 2},
		Nickname:         "forbidden",
		ForbiddenEndTime: now + 3600_000,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 3},
		Nickname:         "forever",
		ForbiddenEndTime: -1,
	})
	mustCreateUser(t, db, &models.User{
		Model:            models.Model{Id: 4},
		Nickname:         "expired",
		ForbiddenEndTime: now - 3600_000,
	})

	users := postUserList(t, "forbidden=false")

	gotIDs := make([]int64, 0, len(users))
	for _, user := range users {
		gotIDs = append(gotIDs, int64(user["id"].(float64)))
		if forbidden, ok := user["forbidden"].(bool); !ok || forbidden {
			t.Fatalf("expected only non-forbidden users, got %#v", user)
		}
	}
	wantIDs := []int64{4, 1}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("expected ids %v, got %v", wantIDs, gotIDs)
		}
	}
}

func TestUserResetPasswordDisablesUserTokens(t *testing.T) {
	db := setupAdminUserTestDB(t)
	mustCreateUser(t, db, &models.User{
		Model:    models.Model{Id: 1},
		Nickname: "target",
		Status:   constants.StatusOk,
		Password: "old-password",
	})
	mustCreateUser(t, db, &models.User{
		Model:    models.Model{Id: 2},
		Nickname: "other",
		Status:   constants.StatusOk,
		Password: "old-password",
	})

	now := time.Now().UnixMilli()
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "target-active-1",
		UserId:     1,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusOk,
		CreateTime: now,
	})
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "target-active-2",
		UserId:     1,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusOk,
		CreateTime: now,
	})
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "target-deleted",
		UserId:     1,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusDeleted,
		CreateTime: now,
	})
	mustCreateUserToken(t, db, &models.UserToken{
		Token:      "other-active",
		UserId:     2,
		ExpiredAt:  now + 3600_000,
		Status:     constants.StatusOk,
		CreateTime: now,
	})

	postUserResetPassword(t, "userId=1")

	var targetActiveCount int64
	if err := db.Model(&models.UserToken{}).
		Where("user_id = ? AND status = ?", 1, constants.StatusOk).
		Count(&targetActiveCount).Error; err != nil {
		t.Fatalf("count target active tokens: %v", err)
	}
	if targetActiveCount != 0 {
		t.Fatalf("expected target user active tokens to be disabled, got %d", targetActiveCount)
	}

	var otherActiveCount int64
	if err := db.Model(&models.UserToken{}).
		Where("user_id = ? AND status = ?", 2, constants.StatusOk).
		Count(&otherActiveCount).Error; err != nil {
		t.Fatalf("count other active tokens: %v", err)
	}
	if otherActiveCount != 1 {
		t.Fatalf("expected other user active token to remain, got %d", otherActiveCount)
	}
}

func TestUserCreateGeneratesLoginPasswordAndAllowsEmptyNickname(t *testing.T) {
	db := setupAdminUserTestDB(t)
	result := postUserCreate(t, "username=staff01&email=staff01%40example.com")

	if !result.Success {
		t.Fatalf("expected success response, got %#v", result)
	}
	if result.Data.Password == "" {
		t.Fatal("expected generated password in response")
	}

	var user models.User
	if err := db.First(&user, "username = ?", "staff01").Error; err != nil {
		t.Fatalf("find created user: %v", err)
	}
	if user.Email.String != "staff01@example.com" {
		t.Fatalf("expected saved email, got %q", user.Email.String)
	}
	if user.Nickname != "" {
		t.Fatalf("expected empty nickname to be allowed, got %q", user.Nickname)
	}
	if user.Password == result.Data.Password {
		t.Fatal("expected database to contain a password hash, not plaintext")
	}
	if !passwd.ValidatePassword(user.Password, result.Data.Password) {
		t.Fatal("expected generated password to match the stored hash")
	}
	if _, err := services.UserService.SignIn("staff01", result.Data.Password); err != nil {
		t.Fatalf("sign in with generated password: %v", err)
	}
}

func TestUserCreateValidatesRequiredAndUniqueFields(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		existing *models.User
		message  func() string
	}{
		{
			name: "username is required",
			body: "email=staff01%40example.com",
			message: func() string {
				return locales.Get("user.username_required")
			},
		},
		{
			name: "username must be valid",
			body: "username=bad&email=staff01%40example.com",
			message: func() string {
				return locales.Get("user.username_invalid")
			},
		},
		{
			name: "email is required",
			body: "username=staff01",
			message: func() string {
				return locales.Get("user.email_required")
			},
		},
		{
			name: "email must be valid",
			body: "username=staff01&email=invalid",
			message: func() string {
				return locales.Get("user.email_invalid")
			},
		},
		{
			name: "username must be unique",
			body: "username=staff01&email=new%40example.com",
			existing: &models.User{
				Username: sqls.SqlNullString("staff01"),
				Email:    sqls.SqlNullString("existing@example.com"),
			},
			message: func() string {
				return locales.Getf("user.username_occupied", "staff01")
			},
		},
		{
			name: "email must be unique",
			body: "username=staff02&email=existing%40example.com",
			existing: &models.User{
				Username: sqls.SqlNullString("existing"),
				Email:    sqls.SqlNullString("existing@example.com"),
			},
			message: func() string {
				return locales.Getf("user.email_occupied", "existing@example.com")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupAdminUserTestDB(t)
			if tt.existing != nil {
				mustCreateUser(t, db, tt.existing)
			}
			var before int64
			if err := db.Model(&models.User{}).Count(&before).Error; err != nil {
				t.Fatalf("count users before request: %v", err)
			}

			result := postUserCreate(t, tt.body)
			if result.Success {
				t.Fatalf("expected validation failure, got %#v", result)
			}
			if result.Message != tt.message() {
				t.Fatalf("expected message %q, got %q", tt.message(), result.Message)
			}

			var after int64
			if err := db.Model(&models.User{}).Count(&after).Error; err != nil {
				t.Fatalf("count users after request: %v", err)
			}
			if after != before {
				t.Fatalf("expected user count to remain %d, got %d", before, after)
			}
		})
	}
}

func setupAdminUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	idcodec.Init(1)
	config.Instance = &config.Config{
		Search: config.SearchConfig{
			IndexPath: filepath.Join(t.TempDir(), "index"),
		},
	}
	search.Init()

	dsn := fmt.Sprintf("file:admin_user_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
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
	if err := db.AutoMigrate(&models.User{}, &models.UserToken{}); err != nil {
		t.Fatalf("auto migrate users: %v", err)
	}
	return db
}

func mustCreateUser(t *testing.T, db *gorm.DB, user *models.User) {
	t.Helper()

	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func mustCreateUserToken(t *testing.T, db *gorm.DB, userToken *models.UserToken) {
	t.Helper()

	if err := db.Create(userToken).Error; err != nil {
		t.Fatalf("create user token: %v", err)
	}
}

func postUserResetPassword(t *testing.T, body string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/reset_password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserResetPassword(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Password string `json:"password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", w.Body.String())
	}
	if result.Data.Password == "" {
		t.Fatalf("expected reset password in response, got %s", w.Body.String())
	}
}

type userCreateResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Password string `json:"password"`
	} `json:"data"`
}

func postUserCreate(t *testing.T, body string) userCreateResult {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserCreate(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result userCreateResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return result
}

func postUserList(t *testing.T, body string) []map[string]interface{} {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/user/list", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	UserList(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Results []map[string]interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !result.Success {
		t.Fatalf("expected success response, got %s", w.Body.String())
	}
	return result.Data.Results
}
