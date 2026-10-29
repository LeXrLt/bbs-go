package services

import (
	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestUserTokenDisableInvalidatesCachedToken(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	tokenValue := createCachedUserToken(t, db, "disable-cache")

	if err := UserTokenService.Disable(tokenValue); err != nil {
		t.Fatalf("disable token: %v", err)
	}
	if cached := cache.UserTokenCache.Get(tokenValue); cached == nil || cached.Status != constants.StatusDeleted {
		t.Fatalf("cache still contains active token after disable: %#v", cached)
	}
}

func TestUserTokenSignoutInvalidatesCachedToken(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	tokenValue := createCachedUserToken(t, db, "signout-cache")
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/signout", nil)
	ctx.Request.Header.Set("X-User-Token", tokenValue)

	if err := UserTokenService.Signout(ctx); err != nil {
		t.Fatalf("sign out token: %v", err)
	}
	if cached := cache.UserTokenCache.Get(tokenValue); cached == nil || cached.Status != constants.StatusDeleted {
		t.Fatalf("cache still contains active token after signout: %#v", cached)
	}
}

func setupUserTokenServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:user_token_disable_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
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
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)
	if err := db.AutoMigrate(&models.UserToken{}); err != nil {
		t.Fatalf("migrate user tokens: %v", err)
	}
	return db
}

func createCachedUserToken(t *testing.T, db *gorm.DB, prefix string) string {
	t.Helper()
	tokenValue := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	token := &models.UserToken{
		Token: tokenValue, UserId: 1, Status: constants.StatusOk,
		ExpiredAt: time.Now().Add(time.Hour).UnixMilli(), CreateTime: time.Now().UnixMilli(),
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	if cached := cache.UserTokenCache.Get(tokenValue); cached == nil || cached.Status != constants.StatusOk {
		t.Fatalf("failed to warm token cache: %#v", cached)
	}
	return tokenValue
}
