package api

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/locales"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestLoginSmsUserReturnsExistingUser(t *testing.T) {
	db := setupLoginHandlerTestDB(t)
	user := &models.User{
		Phone:    sql.NullString{String: "13800138000", Valid: true},
		Nickname: "existing-user",
		Status:   constants.StatusOk,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	got, err := loginSmsUser("13800138000")
	if err != nil {
		t.Fatalf("find SMS login user: %v", err)
	}
	if got == nil || got.Id != user.Id {
		t.Fatalf("expected user %d, got %#v", user.Id, got)
	}
}

func TestLoginSmsUserRejectsUnknownPhoneWithoutCreatingUser(t *testing.T) {
	db := setupLoginHandlerTestDB(t)
	user := &models.User{
		Phone:    sql.NullString{String: "13800138000", Valid: true},
		Nickname: "existing-user",
		Status:   constants.StatusOk,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	before := countLoginHandlerUsers(t, db)
	got, err := loginSmsUser("13900139000")
	if err == nil || err.Error() != locales.Get("auth.registration_disabled") {
		t.Fatalf("expected registration-disabled error, got user=%#v err=%v", got, err)
	}
	if after := countLoginHandlerUsers(t, db); after != before {
		t.Fatalf("expected user count to remain %d, got %d", before, after)
	}
}

func setupLoginHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousConfig := config.Instance
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	t.Cleanup(func() {
		config.Instance = previousConfig
	})

	dsn := fmt.Sprintf("file:login_handler_test_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
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
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("auto migrate users: %v", err)
	}
	return db
}

func countLoginHandlerUsers(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	return count
}
