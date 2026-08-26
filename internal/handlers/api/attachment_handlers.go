package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/pkg/params"
	"bbs-go/internal/pkg/uploader"

	"github.com/mlogclub/simple/common/strs"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/req"
	"bbs-go/internal/models/resp"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/services"
)

// PostUpload 上传附件（发帖前或发帖时）
func AttachmentUpload(ctx *gin.Context) {
	user, err := common.CheckLogin(ctx)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	if err := services.UserService.CheckPostStatus(user); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	cfg := services.SysConfigService.GetAttachmentConfig()
	if !cfg.Enabled {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("attachment.disabled")))
		return
	}

	const multipartOverheadBytes = int64(1 << 20)
	maxSizeMB := int64(cfg.MaxSizeMB)
	if maxSizeMB <= 0 || maxSizeMB > (int64(^uint64(0)>>1)-multipartOverheadBytes)/(1024*1024) {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("attachment.invalid_document")))
		return
	}
	maxBytes := maxSizeMB * 1024 * 1024
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxBytes+multipartOverheadBytes)

	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Getf("attachment.too_large", cfg.MaxSizeMB)))
		} else {
			ginx.WriteJSON(ctx, err)
		}
		return
	}
	defer file.Close()

	if header.Size > maxBytes {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Getf("attachment.too_large", cfg.MaxSizeMB)))
		return
	}

	tempFile, err := os.CreateTemp("", "bbsgo-attachment-*")
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	limited := &io.LimitedReader{R: file, N: maxBytes + 1}
	size, copyErr := io.Copy(tempFile, limited)
	closeErr := tempFile.Close()
	if copyErr != nil || closeErr != nil {
		if copyErr != nil {
			ginx.WriteJSON(ctx, copyErr)
		} else {
			ginx.WriteJSON(ctx, closeErr)
		}
		return
	}
	if size > maxBytes {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Getf("attachment.too_large", cfg.MaxSizeMB)))
		return
	}

	downloadScore, _ := params.GetInt(ctx, "downloadScore")
	categoryId, _ := params.GetInt64(ctx, "categoryId")
	att, err := services.AttachmentService.Upload(ctx.Request.Context(), user.Id, header.Filename, tempPath, size, downloadScore, categoryId)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	ginx.WriteJSON(ctx, buildAttachmentResponse(att, user))

}

func AttachmentAccess(ctx *gin.Context) {
	user, err := common.CheckLogin(ctx)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	att, err := services.AttachmentService.Access(ctx.Param("id"), user)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	ginx.WriteJSON(ctx, buildAttachmentResponse(att, user))
}

func buildAttachmentResponse(att *models.Attachment, currentUser *models.User) resp.AttachmentResponse {
	downloaded := currentUser != nil && services.AttachmentService.HasDownloaded(currentUser.Id, att.Id)
	accessGranted := currentUser != nil && services.AttachmentService.HasAccess(att, currentUser.Id)
	return resp.AttachmentResponse{
		Id:            att.Id,
		FileName:      att.FileName,
		FileSize:      att.FileSize,
		FileType:      att.FileType,
		Previewable:   att.PreviewStatus == constants.AttachmentPreviewReady && strs.IsNotBlank(att.PreviewKey),
		AccessGranted: accessGranted,
		DownloadScore: att.DownloadScore,
		DownloadCount: att.DownloadCount,
		Downloaded:    downloaded,
	}
}

type attachmentByteRange struct {
	start  int64
	length int64
	set    bool
}

func parseAttachmentRange(value string, size int64) (attachmentByteRange, error) {
	if strings.TrimSpace(value) == "" {
		return attachmentByteRange{start: 0, length: size}, nil
	}
	if size <= 0 || !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return attachmentByteRange{}, errors.New("invalid range")
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 {
		return attachmentByteRange{}, errors.New("invalid range")
	}
	startText, endText := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if startText == "" {
		suffix, err := strconv.ParseInt(endText, 10, 64)
		if err != nil || suffix <= 0 {
			return attachmentByteRange{}, errors.New("invalid range")
		}
		if suffix > size {
			suffix = size
		}
		return attachmentByteRange{start: size - suffix, length: suffix, set: true}, nil
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 || start >= size {
		return attachmentByteRange{}, errors.New("invalid range")
	}
	end := size - 1
	if endText != "" {
		end, err = strconv.ParseInt(endText, 10, 64)
		if err != nil || end < start {
			return attachmentByteRange{}, errors.New("invalid range")
		}
		if end >= size {
			end = size - 1
		}
	}
	return attachmentByteRange{start: start, length: end - start + 1, set: true}, nil
}

func AttachmentPreview(ctx *gin.Context) {
	serveAttachmentObject(ctx, attachmentObjectPDFPreview)
}

func AttachmentSpreadsheetPreview(ctx *gin.Context) {
	serveAttachmentObject(ctx, attachmentObjectSpreadsheetPreview)
}

func AttachmentDownload(ctx *gin.Context) {
	serveAttachmentObject(ctx, attachmentObjectDownload)
}

type attachmentObjectMode int

const (
	attachmentObjectPDFPreview attachmentObjectMode = iota
	attachmentObjectSpreadsheetPreview
	attachmentObjectDownload
)

func serveAttachmentObject(ctx *gin.Context, mode attachmentObjectMode) {
	user, err := common.CheckLogin(ctx)
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	object, err := services.AttachmentService.AuthorizedObject(ctx.Param("id"), user, mode == attachmentObjectPDFPreview)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, services.ErrAttachmentAccessForbidden) {
			status = http.StatusForbidden
		}
		ctx.AbortWithStatus(status)
		return
	}
	if mode == attachmentObjectSpreadsheetPreview {
		ext := strings.ToLower(filepath.Ext(object.Attachment.FileName))
		if ext != ".xls" && ext != ".xlsx" {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}
	}
	meta, err := services.UploadService.HeadObject(ctx.Request.Context(), object.Method, object.Key)
	if err != nil || meta.Size <= 0 {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	requestedRange, err := parseAttachmentRange(ctx.GetHeader("Range"), meta.Size)
	if err != nil {
		ctx.Header("Content-Range", fmt.Sprintf("bytes */%d", meta.Size))
		ctx.AbortWithStatus(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	filename := filepath.Base(object.Attachment.FileName)
	contentType := object.Attachment.FileType
	disposition := "attachment"
	if mode == attachmentObjectPDFPreview {
		contentType = "application/pdf"
		disposition = "inline"
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".pdf"
	} else if mode == attachmentObjectSpreadsheetPreview {
		disposition = "inline"
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	var reader io.ReadCloser
	if ctx.Request.Method != http.MethodHead {
		reader, err = services.UploadService.GetObject(ctx.Request.Context(), object.Method, object.Key, uploader.GetOptions{
			Offset: requestedRange.start,
			Length: requestedRange.length,
		})
		if err != nil {
			slog.Error("open attachment object failed", slog.String("attachmentId", object.Attachment.Id), slog.Any("err", err))
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}
		defer reader.Close()
	}
	ctx.Header("Accept-Ranges", "bytes")
	ctx.Header("Cache-Control", "private, no-store")
	ctx.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	ctx.Header("Content-Length", strconv.FormatInt(requestedRange.length, 10))
	ctx.Header("Content-Type", contentType)
	ctx.Header("Cross-Origin-Resource-Policy", "same-origin")
	ctx.Header("Pragma", "no-cache")
	ctx.Header("X-Content-Type-Options", "nosniff")
	status := http.StatusOK
	if requestedRange.set {
		status = http.StatusPartialContent
		ctx.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", requestedRange.start, requestedRange.start+requestedRange.length-1, meta.Size))
	}
	ctx.Status(status)
	ctx.Writer.WriteHeaderNow()
	if ctx.Request.Method == http.MethodHead {
		return
	}
	if _, err := io.CopyN(ctx.Writer, reader, requestedRange.length); err != nil {
		slog.Warn("stream attachment object failed", slog.String("attachmentId", object.Attachment.Id), slog.Any("err", err))
		return
	}
	if mode == attachmentObjectDownload {
		if err := services.AttachmentService.IncrementDownloadCount(object.Attachment.Id); err != nil {
			slog.Error("increment attachment download count failed", slog.String("attachmentId", object.Attachment.Id), slog.Any("err", err))
		}
	}
}

// PostUpdateDownloadScore 更新附件下载积分
func AttachmentUpdateDownloadScore(ctx *gin.Context) {
	user, err := common.CheckLogin(ctx)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}

	var body req.PatchDownloadScoreReq
	if err := ginx.BindJSON(ctx, &body); err != nil {
		ginx.WriteJSON(ctx, ginx.ErrorMessage("invalid body"))
		return
	}
	att, err := services.AttachmentService.UpdateDownloadScore(body.Id, user.Id, body.DownloadScore)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	ginx.WriteJSON(ctx, buildAttachmentResponse(att, user))

}
