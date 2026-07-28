package services

import (
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/locales"

	"gorm.io/gorm"
)

func TestThirdUserServiceLoginBoundUserReturnsExistingUser(t *testing.T) {
	db := setupThirdUserLoginTestDB(t)
	user := createThirdUserLoginUser(t, db, "bound-user")
	createThirdUserLoginBinding(t, db, user.Id, "google-subject", constants.ThirdTypeGoogle)

	got, err := ThirdUserService.loginBoundUser("google-subject", constants.ThirdTypeGoogle)
	if err != nil {
		t.Fatalf("login bound user: %v", err)
	}
	if got == nil || got.Id != user.Id {
		t.Fatalf("expected user %d, got %#v", user.Id, got)
	}

	// Google OAuth and Google One Tap both use the Google third-party namespace.
	gotOneTap, err := ThirdUserService.loginBoundUser("google-subject", constants.ThirdTypeGoogle)
	if err != nil {
		t.Fatalf("login bound Google One Tap user: %v", err)
	}
	if gotOneTap == nil || gotOneTap.Id != user.Id {
		t.Fatalf("expected Google One Tap to return user %d, got %#v", user.Id, gotOneTap)
	}
}

func TestThirdUserServiceLoginBoundUserRejectsUnknownIdentityWithoutCreatingData(t *testing.T) {
	db := setupThirdUserLoginTestDB(t)
	createThirdUserLoginUser(t, db, "existing-user")

	beforeUsers := countThirdUserLoginRows(t, db, &models.User{})
	beforeBindings := countThirdUserLoginRows(t, db, &models.ThirdUser{})

	for _, thirdType := range []constants.ThirdType{
		constants.ThirdTypeWeixin,
		constants.ThirdTypeGoogle,
		constants.ThirdTypeGithub,
	} {
		user, err := ThirdUserService.loginBoundUser("unknown-identity", thirdType)
		if err == nil {
			t.Fatalf("expected %s identity to be rejected", thirdType)
		}
		if user != nil {
			t.Fatalf("expected no user for %s identity, got %#v", thirdType, user)
		}
		if err.Error() != locales.Get("auth.registration_disabled") {
			t.Fatalf("unexpected error for %s identity: %q", thirdType, err)
		}
	}

	if got := countThirdUserLoginRows(t, db, &models.User{}); got != beforeUsers {
		t.Fatalf("expected user count to remain %d, got %d", beforeUsers, got)
	}
	if got := countThirdUserLoginRows(t, db, &models.ThirdUser{}); got != beforeBindings {
		t.Fatalf("expected binding count to remain %d, got %d", beforeBindings, got)
	}
}

func TestThirdUserServiceLoginBoundUserRejectsInvalidBindingWithoutCreatingData(t *testing.T) {
	db := setupThirdUserLoginTestDB(t)
	createThirdUserLoginBinding(t, db, 0, "invalid-binding", constants.ThirdTypeWeixin)

	beforeUsers := countThirdUserLoginRows(t, db, &models.User{})
	beforeBindings := countThirdUserLoginRows(t, db, &models.ThirdUser{})
	user, err := ThirdUserService.loginBoundUser("invalid-binding", constants.ThirdTypeWeixin)
	if err == nil || err.Error() != locales.Get("auth.registration_disabled") {
		t.Fatalf("expected registration-disabled error, got user=%#v err=%v", user, err)
	}

	if got := countThirdUserLoginRows(t, db, &models.User{}); got != beforeUsers {
		t.Fatalf("expected user count to remain %d, got %d", beforeUsers, got)
	}
	if got := countThirdUserLoginRows(t, db, &models.ThirdUser{}); got != beforeBindings {
		t.Fatalf("expected binding count to remain %d, got %d", beforeBindings, got)
	}
}

func TestThirdUserServiceLoginBoundUserRejectsDanglingBinding(t *testing.T) {
	db := setupThirdUserLoginTestDB(t)
	createThirdUserLoginBinding(t, db, 999, "dangling-binding", constants.ThirdTypeGithub)

	beforeUsers := countThirdUserLoginRows(t, db, &models.User{})
	beforeBindings := countThirdUserLoginRows(t, db, &models.ThirdUser{})
	user, err := ThirdUserService.loginBoundUser("dangling-binding", constants.ThirdTypeGithub)
	if err == nil || err.Error() != locales.Get("errors.user_not_found_or_disabled") {
		t.Fatalf("expected user-not-found error, got user=%#v err=%v", user, err)
	}

	if got := countThirdUserLoginRows(t, db, &models.User{}); got != beforeUsers {
		t.Fatalf("expected user count to remain %d, got %d", beforeUsers, got)
	}
	if got := countThirdUserLoginRows(t, db, &models.ThirdUser{}); got != beforeBindings {
		t.Fatalf("expected binding count to remain %d, got %d", beforeBindings, got)
	}
}

func setupThirdUserLoginTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousConfig := config.Instance
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	t.Cleanup(func() {
		config.Instance = previousConfig
	})

	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.ThirdUser{}); err != nil {
		t.Fatalf("auto migrate third users: %v", err)
	}
	return db
}

func createThirdUserLoginUser(t *testing.T, db *gorm.DB, nickname string) *models.User {
	t.Helper()
	user := &models.User{Nickname: nickname, Status: constants.StatusOk}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func createThirdUserLoginBinding(
	t *testing.T,
	db *gorm.DB,
	userID int64,
	openID string,
	thirdType constants.ThirdType,
) *models.ThirdUser {
	t.Helper()
	binding := &models.ThirdUser{
		UserId:    userID,
		OpenId:    openID,
		ThirdType: thirdType,
	}
	if err := db.Create(binding).Error; err != nil {
		t.Fatalf("create third user: %v", err)
	}
	return binding
}

func countThirdUserLoginRows(t *testing.T, db *gorm.DB, model interface{}) int64 {
	t.Helper()
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		t.Fatalf("count %T rows: %v", model, err)
	}
	return count
}
