package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/pkg/uploader"
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
		0,
	)
	if err == nil || err.Error() != locales.Get("attachment.invalid_document") {
		t.Fatalf("macro-enabled upload error = %v", err)
	}
}

func TestAttachmentUploadCreatesPreviewForPDFWithEmptyUserPassword(t *testing.T) {
	t.Chdir(t.TempDir())
	setupAttachmentServiceTestDB(t)
	configureAttachmentUploadTypes(t, []string{".pdf"})

	plainPDF := attachmentTestPDF("")
	normalizedPath := filepath.Join(t.TempDir(), "normalized.pdf")
	if err := os.WriteFile(normalizedPath, plainPDF, 0o600); err != nil {
		t.Fatalf("write normalized PDF: %v", err)
	}
	qpdfDirectory := t.TempDir()
	qpdfPath := filepath.Join(qpdfDirectory, "qpdf")
	if err := os.WriteFile(qpdfPath, []byte("#!/bin/sh\nset -eu\ncat \"$QPDF_TEST_OUTPUT\"\n"), 0o700); err != nil {
		t.Fatalf("write qpdf test command: %v", err)
	}
	t.Setenv("QPDF_TEST_OUTPUT", normalizedPath)
	t.Setenv("PATH", qpdfDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	encryptedPDF := attachmentTestPDF(" /Encrypt 2 0 R")
	sourcePath := filepath.Join(t.TempDir(), "restricted.pdf")
	if err := os.WriteFile(sourcePath, encryptedPDF, 0o600); err != nil {
		t.Fatalf("write encrypted PDF: %v", err)
	}
	attachment, err := AttachmentService.Upload(context.Background(), 1, "restricted.pdf", sourcePath, int64(len(encryptedPDF)), 0, 0)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if attachment.PreviewStatus != constants.AttachmentPreviewReady || attachment.PreviewKey == "" || attachment.PreviewKey == attachment.FileKey {
		t.Fatalf("Upload() attachment = %+v, want a separate ready preview", attachment)
	}
	if attachment.FileSize != int64(len(encryptedPDF)) || attachment.PreviewSize != int64(len(plainPDF)) {
		t.Fatalf("Upload() sizes = original %d preview %d", attachment.FileSize, attachment.PreviewSize)
	}

	method := dto.UploadMethod(attachment.StorageMethod)
	assertStoredAttachmentBytes(t, method, attachment.FileKey, encryptedPDF)
	assertStoredAttachmentBytes(t, method, attachment.PreviewKey, plainPDF)
}

func TestAttachmentUploadWritesReviewCopyByCategory(t *testing.T) {
	t.Chdir(t.TempDir())
	setupAttachmentServiceTestDB(t)
	configureAttachmentUploadTypes(t, []string{".txt"})
	if err := sqls.DB().AutoMigrate(&models.Category{}); err != nil {
		t.Fatalf("auto migrate categories: %v", err)
	}
	root := &models.Category{Name: "研究/报告", Type: constants.CategoryTypeNormal, Status: constants.StatusOk}
	if err := repositories.CategoryRepository.Create(sqls.DB(), root); err != nil {
		t.Fatalf("create root category: %v", err)
	}
	child := &models.Category{ParentId: root.Id, Name: "季度材料", Type: constants.CategoryTypeNormal, Status: constants.StatusOk}
	if err := repositories.CategoryRepository.Create(sqls.DB(), child); err != nil {
		t.Fatalf("create child category: %v", err)
	}
	reviewDir := t.TempDir()
	config.Instance.AttachmentReview.Dir = reviewDir
	contents := []byte("review source bytes")
	sourcePath := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(sourcePath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := AttachmentService.Upload(context.Background(), 1, "原始文件.txt", sourcePath, int64(len(contents)), 0, child.Id)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	reviewPath := filepath.Join(
		reviewDir,
		fmt.Sprintf("%d-研究_报告", root.Id),
		fmt.Sprintf("%d-季度材料", child.Id),
		"原始文件.txt",
	)
	got, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("read review copy: %v", err)
	}
	if !bytes.Equal(got, contents) {
		t.Fatalf("review copy = %q, want %q", got, contents)
	}
}

func TestAttachmentUploadReviewFailureDoesNotFailPrimaryUpload(t *testing.T) {
	t.Chdir(t.TempDir())
	setupAttachmentServiceTestDB(t)
	configureAttachmentUploadTypes(t, []string{".txt"})
	if err := sqls.DB().AutoMigrate(&models.Category{}); err != nil {
		t.Fatalf("auto migrate categories: %v", err)
	}
	category := &models.Category{Name: "Reports", Type: constants.CategoryTypeNormal, Status: constants.StatusOk}
	if err := repositories.CategoryRepository.Create(sqls.DB(), category); err != nil {
		t.Fatalf("create category: %v", err)
	}
	blockedPath := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(blockedPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.Instance.AttachmentReview.Dir = blockedPath
	contents := []byte("primary upload survives")
	sourcePath := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(sourcePath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	attachment, err := AttachmentService.Upload(context.Background(), 1, "report.txt", sourcePath, int64(len(contents)), 0, category.Id)
	if err != nil {
		t.Fatalf("Upload() must remain successful when the review copy fails: %v", err)
	}
	assertStoredAttachmentBytes(t, dto.UploadMethod(attachment.StorageMethod), attachment.FileKey, contents)
}

func configureAttachmentUploadTypes(t *testing.T, allowedTypes []string) {
	t.Helper()
	if err := sqls.DB().AutoMigrate(&models.SysConfig{}); err != nil {
		t.Fatalf("auto migrate system config: %v", err)
	}
	now := dates.NowTimestamp()
	value := fmt.Sprintf(`{"enabled":true,"allowedTypes":["%s"],"maxSizeMB":256,"maxCount":5}`, strings.Join(allowedTypes, `","`))
	if err := repositories.SysConfigRepository.Create(sqls.DB(), &models.SysConfig{
		Key:         constants.SysConfigAttachmentConfig,
		Value:       value,
		Name:        "attachment config",
		Description: "test",
		CreateTime:  now,
		UpdateTime:  now,
	}); err != nil {
		t.Fatalf("create attachment config: %v", err)
	}
	cache.SysConfigCache.Invalidate(constants.SysConfigAttachmentConfig)
	t.Cleanup(func() { cache.SysConfigCache.Invalidate(constants.SysConfigAttachmentConfig) })
}

func assertStoredAttachmentBytes(t *testing.T, method dto.UploadMethod, key string, want []byte) {
	t.Helper()
	object, err := UploadService.GetObject(context.Background(), method, key, uploader.GetOptions{})
	if err != nil {
		t.Fatalf("get stored attachment %q: %v", key, err)
	}
	defer object.Close()
	got, err := io.ReadAll(object)
	if err != nil {
		t.Fatalf("read stored attachment %q: %v", key, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored attachment %q differs: got %d bytes, want %d", key, len(got), len(want))
	}
}

func attachmentTestPDF(trailerExtra string) []byte {
	var document bytes.Buffer
	document.WriteString("%PDF-1.4\n")
	catalogOffset := document.Len()
	document.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xrefOffset := document.Len()
	document.WriteString("xref\n0 2\n0000000000 65535 f \n")
	fmt.Fprintf(&document, "%010d 00000 n \n", catalogOffset)
	fmt.Fprintf(&document, "trailer\n<< /Size 2 /Root 1 0 R%s >>\nstartxref\n%d\n%%%%EOF\n", trailerExtra, xrefOffset)
	return document.Bytes()
}
