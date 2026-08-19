package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/uploader"
	"bbs-go/internal/repositories"

	"github.com/gin-gonic/gin"
	"github.com/mlogclub/simple/sqls"
)

func TestParseAttachmentRange(t *testing.T) {
	tests := []struct {
		value      string
		size       int64
		wantStart  int64
		wantLength int64
		wantSet    bool
		wantError  bool
	}{
		{value: "", size: 10, wantLength: 10},
		{value: "bytes=2-5", size: 10, wantStart: 2, wantLength: 4, wantSet: true},
		{value: "bytes=7-", size: 10, wantStart: 7, wantLength: 3, wantSet: true},
		{value: "bytes=-4", size: 10, wantStart: 6, wantLength: 4, wantSet: true},
		{value: "bytes=2-99", size: 10, wantStart: 2, wantLength: 8, wantSet: true},
		{value: "bytes=10-", size: 10, wantError: true},
		{value: "bytes=4-2", size: 10, wantError: true},
		{value: "bytes=-0", size: 10, wantError: true},
		{value: "bytes=0-1,4-5", size: 10, wantError: true},
		{value: "items=0-1", size: 10, wantError: true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/%d", test.value, test.size), func(t *testing.T) {
			got, err := parseAttachmentRange(test.value, test.size)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.start != test.wantStart || got.length != test.wantLength || got.set != test.wantSet {
				t.Fatalf("range=%+v", got)
			}
		})
	}
}

func TestAttachmentPreviewAndDownloadStreaming(t *testing.T) {
	t.Chdir(t.TempDir())
	db := setupTopicHandlerCategoryTestDB(t)
	if err := db.AutoMigrate(&models.Attachment{}, &models.AttachmentDownloadLog{}); err != nil {
		t.Fatalf("auto migrate attachments: %v", err)
	}

	user := &models.User{Model: models.Model{Id: 100}}
	topic := &models.Topic{
		Type:       constants.TopicTypeTopic,
		UserId:     user.Id,
		Title:      "stream attachment",
		Content:    "content",
		Status:     constants.StatusOk,
		CreateTime: 1,
	}
	if err := repositories.TopicRepository.Create(db, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	const key = "attachments/tests/stream.pdf"
	const contents = "0123456789"
	local := &uploader.LocalUploader{}
	if _, err := local.PutObject(dto.UploadConfig{}, key, bytes.NewBufferString(contents), &uploader.PutOptions{
		ContentType:   "application/pdf",
		ContentLength: int64(len(contents)),
		Private:       true,
	}); err != nil {
		t.Fatalf("put test object: %v", err)
	}
	attachment := &models.Attachment{
		Id:            "stream-pdf",
		TopicId:       topic.Id,
		UserId:        user.Id,
		FileName:      "报告.pdf",
		StorageMethod: string(dto.Local),
		FileKey:       key,
		FileSize:      int64(len(contents)),
		FileType:      "application/pdf",
		PreviewKey:    key,
		PreviewSize:   int64(len(contents)),
		PreviewStatus: constants.AttachmentPreviewReady,
		Status:        constants.StatusOk,
	}
	if err := repositories.AttachmentRepository.Create(db, attachment); err != nil {
		t.Fatalf("create attachment: %v", err)
	}

	preview := performAttachmentObjectRequest(http.MethodGet, "/api/attachment/preview/stream-pdf", "bytes=2-5", attachment.Id, user, AttachmentPreview)
	if preview.Code != http.StatusPartialContent || preview.Body.String() != "2345" {
		t.Fatalf("preview status=%d body=%q", preview.Code, preview.Body.String())
	}
	if got := preview.Header().Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("content-range=%q", got)
	}
	if got := preview.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("content-type=%q", got)
	}
	assertAttachmentDownloadCount(t, attachment.Id, 0)

	head := performAttachmentObjectRequest(http.MethodHead, "/api/attachment/download/stream-pdf", "", attachment.Id, user, AttachmentDownload)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != "10" {
		t.Fatalf("HEAD status=%d length=%q body=%q", head.Code, head.Header().Get("Content-Length"), head.Body.String())
	}
	assertAttachmentDownloadCount(t, attachment.Id, 0)

	invalid := performAttachmentObjectRequest(http.MethodGet, "/api/attachment/download/stream-pdf", "bytes=0-1,3-4", attachment.Id, user, AttachmentDownload)
	if invalid.Code != http.StatusRequestedRangeNotSatisfiable || invalid.Header().Get("Content-Range") != "bytes */10" {
		t.Fatalf("invalid range status=%d header=%q", invalid.Code, invalid.Header().Get("Content-Range"))
	}
	assertAttachmentDownloadCount(t, attachment.Id, 0)

	download := performAttachmentObjectRequest(http.MethodGet, "/api/attachment/download/stream-pdf", "", attachment.Id, user, AttachmentDownload)
	if download.Code != http.StatusOK || download.Body.String() != contents {
		t.Fatalf("download status=%d body=%q", download.Code, download.Body.String())
	}
	if got := download.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff=%q", got)
	}
	assertAttachmentDownloadCount(t, attachment.Id, 1)
}

func performAttachmentObjectRequest(method, target, rangeHeader, id string, user *models.User, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	ctx.Params = []gin.Param{{Key: "id", Value: id}}
	if rangeHeader != "" {
		ctx.Request.Header.Set("Range", rangeHeader)
	}
	common.SetCurrentUser(ctx, user)
	handler(ctx)
	return recorder
}

func assertAttachmentDownloadCount(t *testing.T, id string, want int) {
	t.Helper()
	attachment := repositories.AttachmentRepository.Get(sqls.DB(), id)
	if attachment == nil || attachment.DownloadCount != want {
		t.Fatalf("download count=%v want %d", attachment, want)
	}
}
