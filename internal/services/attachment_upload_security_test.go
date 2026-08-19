package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/repositories"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

func TestAttachmentUploadRejectsMacroExtensionEvenWhenAllowlisted(t *testing.T) {
	setupAttachmentServiceTestDB(t)
	if err := sqls.DB().AutoMigrate(&models.SysConfig{}); err != nil {
		t.Fatalf("auto migrate system config: %v", err)
	}

	now := dates.NowTimestamp()
	if err := repositories.SysConfigRepository.Create(sqls.DB(), &models.SysConfig{
		Key:         constants.SysConfigAttachmentConfig,
		Value:       `{"enabled":true,"allowedTypes":[".docm"],"maxSizeMB":10,"maxCount":5}`,
		Name:        "attachment config",
		Description: "test",
		CreateTime:  now,
		UpdateTime:  now,
	}); err != nil {
		t.Fatalf("create attachment config: %v", err)
	}
	cache.SysConfigCache.Invalidate(constants.SysConfigAttachmentConfig)
	t.Cleanup(func() {
		cache.SysConfigCache.Invalidate(constants.SysConfigAttachmentConfig)
	})

	payload := []byte("macro-enabled document")
	sourcePath := filepath.Join(t.TempDir(), "source.docm")
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	_, err := AttachmentService.Upload(
		context.Background(),
		1,
		"report.docm",
		sourcePath,
		int64(len(payload)),
		0,
	)
	if err == nil || err.Error() != locales.Get("attachment.invalid_document") {
		t.Fatalf("macro-enabled upload error = %v", err)
	}
}
