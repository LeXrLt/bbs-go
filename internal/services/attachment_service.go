package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/common/strs"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/attachmentreview"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/docpreview"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/pkg/uploader"
	"bbs-go/internal/repositories"
)

var AttachmentService = new(attachmentService)

var ErrAttachmentAccessForbidden = errors.New("attachment access forbidden")

type attachmentAccessError struct {
	cause   error
	message string
}

func (e *attachmentAccessError) Error() string {
	return e.message
}

func (e *attachmentAccessError) Unwrap() error {
	return e.cause
}

func attachmentAccessForbidden(messageKey string) error {
	return &attachmentAccessError{
		cause:   ErrAttachmentAccessForbidden,
		message: locales.Get(messageKey),
	}
}

type attachmentService struct{}

func (s *attachmentService) extAllowed(ext string, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return false
	}
	ext = strings.ToLower(ext)
	for _, a := range allowedTypes {
		if strings.ToLower(strings.TrimSpace(a)) == ext {
			return true
		}
	}
	return false
}

// Upload validates a spooled upload, generates its preview, and only persists
// the attachment after every required object has been stored successfully.
func (s *attachmentService) Upload(ctx context.Context, userId int64, filename, sourcePath string, contentLength int64, downloadScore int, categoryId int64) (*models.Attachment, error) {
	if downloadScore < 0 {
		downloadScore = 0
	}
	filename = filepath.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	if filename == "" || filename == "." || len(filename) > 256 {
		return nil, errors.New(locales.Get("attachment.invalid_document"))
	}
	for _, character := range filename {
		if character < 0x20 || character == 0x7f {
			return nil, errors.New(locales.Get("attachment.invalid_document"))
		}
	}
	reviewCategories, err := s.reviewCategories(categoryId)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(sourcePath)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() != contentLength {
		return nil, errors.New(locales.Get("attachment.invalid_document"))
	}

	cfg := SysConfigService.GetAttachmentConfig()
	ext := strings.ToLower(filepath.Ext(filename))
	if docpreview.IsMacroEnabledExtension(filename) {
		return nil, errors.New(locales.Get("attachment.invalid_document"))
	}
	if !s.extAllowed(ext, cfg.AllowedTypes) {
		return nil, errors.New(locales.Get("attachment.ext_not_allowed"))
	}

	contentType, previewStatus := detectAttachmentContentType(sourcePath, ext), constants.AttachmentPreviewUnsupported
	var (
		previewPDF        []byte
		normalizedPDFPath string
		normalizedPDFSize int64
	)
	if isPreviewDocumentExtension(ext) {
		info, validateErr := docpreview.Validate(sourcePath, filename)
		if ext == ".pdf" && errors.Is(validateErr, docpreview.ErrEncryptedDocument) {
			normalizedFile, tempErr := os.CreateTemp("", "bbsgo-pdf-preview-*.pdf")
			if tempErr != nil {
				slog.Error("create attachment PDF preview file", slog.Any("err", tempErr))
				return nil, errors.New(locales.Get("attachment.preview_unavailable"))
			}
			normalizedPDFPath = normalizedFile.Name()
			defer os.Remove(normalizedPDFPath)

			timeoutSeconds, maxOutputMB := config.DefaultDocumentConverterTimeoutSeconds, config.DefaultDocumentPreviewMaxOutputMB
			if config.Instance != nil {
				if config.Instance.DocumentPreview.TimeoutSeconds > 0 {
					timeoutSeconds = config.Instance.DocumentPreview.TimeoutSeconds
				}
				if config.Instance.DocumentPreview.MaxOutputMB > 0 {
					maxOutputMB = config.Instance.DocumentPreview.MaxOutputMB
				}
			}
			normalizeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
			normalizedPDFSize, err = docpreview.NormalizePDFWithEmptyPassword(
				normalizeCtx,
				sourcePath,
				normalizedFile,
				int64(maxOutputMB)*1024*1024,
			)
			cancel()
			closeErr := normalizedFile.Close()
			if err != nil || closeErr != nil {
				if err == nil {
					err = closeErr
				}
				slog.Warn("attachment PDF normalization failed", slog.Any("err", err))
				if errors.Is(err, docpreview.ErrPDFPasswordRequired) {
					return nil, errors.New(locales.Get("attachment.pdf_password_required"))
				}
				return nil, errors.New(locales.Get("attachment.preview_unavailable"))
			}
			info, validateErr = docpreview.Validate(normalizedPDFPath, filename)
		}
		if validateErr != nil {
			slog.Warn("attachment document validation failed", slog.String("extension", ext), slog.Int64("normalizedPDFSize", normalizedPDFSize), slog.Any("err", validateErr))
			return nil, errors.New(locales.Get("attachment.invalid_document"))
		}
		contentType = info.MIMEType
		previewStatus = constants.AttachmentPreviewReady
		if info.NeedsConversion {
			previewCfg := config.DocumentPreviewConfig{}
			if config.Instance != nil {
				previewCfg = config.Instance.DocumentPreview
			}
			if previewCfg.ConverterURL == "" || previewCfg.TimeoutSeconds <= 0 || previewCfg.MaxOutputMB <= 0 {
				return nil, errors.New(locales.Get("attachment.preview_unavailable"))
			}
			maxSourceBytes := int64(cfg.MaxSizeMB) * 1024 * 1024
			client, clientErr := docpreview.NewGotenbergClientWithLimits(
				previewCfg.ConverterURL,
				&http.Client{Timeout: time.Duration(previewCfg.TimeoutSeconds) * time.Second},
				maxSourceBytes,
				int64(previewCfg.MaxOutputMB)*1024*1024,
			)
			if clientErr != nil {
				slog.Error("configure document converter", slog.Any("err", clientErr))
				return nil, errors.New(locales.Get("attachment.preview_unavailable"))
			}
			convertCtx, cancel := context.WithTimeout(ctx, time.Duration(previewCfg.TimeoutSeconds)*time.Second)
			previewPDF, err = client.Convert(convertCtx, sourcePath, filename)
			cancel()
			if err != nil {
				slog.Warn("attachment document conversion failed", slog.String("extension", ext), slog.Bool("retryable", docpreview.IsRetryable(err)), slog.Any("err", err))
				if docpreview.IsPermanent(err) {
					return nil, errors.New(locales.Get("attachment.invalid_document"))
				}
				return nil, errors.New(locales.Get("attachment.preview_unavailable"))
			}
		}
	}

	var (
		attId       = strs.UUID()
		key         = uploader.GenerateAttachmentKey(attId, ext)
		disposition = mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	)
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	fileURL, storageMethod, err := UploadService.PutObjectTracked(key, source, &uploader.PutOptions{
		ContentType:        contentType,
		ContentDisposition: disposition,
		ContentLength:      contentLength,
		Private:            true,
	})
	closeErr := source.Close()
	if err != nil {
		s.cleanupAttachmentObjects(storageMethod, key)
		return nil, err
	}
	if closeErr != nil {
		s.cleanupAttachmentObjects(storageMethod, key)
		return nil, closeErr
	}

	previewKey, previewSize := "", int64(0)
	if previewStatus == constants.AttachmentPreviewReady {
		if ext == ".pdf" && normalizedPDFPath == "" {
			previewKey, previewSize = key, contentLength
		} else {
			var (
				previewReader io.Reader = bytes.NewReader(previewPDF)
				previewClose  func() error
			)
			previewSize = int64(len(previewPDF))
			if normalizedPDFPath != "" {
				normalizedFile, openErr := os.Open(normalizedPDFPath)
				if openErr != nil {
					s.cleanupAttachmentObjects(storageMethod, key)
					return nil, errors.New(locales.Get("attachment.preview_unavailable"))
				}
				previewReader, previewClose, previewSize = normalizedFile, normalizedFile.Close, normalizedPDFSize
			}
			previewKey = uploader.GenerateAttachmentPreviewKey(attId)
			_, previewErr := UploadService.PutObjectWithMethod(storageMethod, previewKey, previewReader, &uploader.PutOptions{
				ContentType:        "application/pdf",
				ContentDisposition: mime.FormatMediaType("inline", map[string]string{"filename": strings.TrimSuffix(filename, ext) + ".pdf"}),
				ContentLength:      previewSize,
				Private:            true,
			})
			var previewCloseErr error
			if previewClose != nil {
				previewCloseErr = previewClose()
			}
			if previewErr != nil || previewCloseErr != nil {
				s.cleanupAttachmentObjects(storageMethod, key, previewKey)
				if previewErr != nil {
					return nil, previewErr
				}
				return nil, previewCloseErr
			}
		}
	}
	if err := s.verifyAttachmentObjects(ctx, storageMethod, key, contentLength, previewKey, previewSize); err != nil {
		s.cleanupAttachmentObjects(storageMethod, key, previewKey)
		return nil, errors.New(locales.Get("attachment.preview_unavailable"))
	}

	att := &models.Attachment{
		Id:            attId,
		TopicId:       0,
		UserId:        userId,
		FileName:      filename,
		FileUrl:       fileURL,
		StorageMethod: string(storageMethod),
		FileKey:       key,
		FileSize:      contentLength,
		FileType:      contentType,
		PreviewKey:    previewKey,
		PreviewSize:   previewSize,
		PreviewStatus: previewStatus,
		DownloadScore: downloadScore,
		Status:        constants.StatusOk,
		CreateTime:    dates.NowTimestamp(),
		UpdateTime:    dates.NowTimestamp(),
	}
	if err := repositories.AttachmentRepository.Create(sqls.DB(), att); err != nil {
		s.cleanupAttachmentObjects(storageMethod, key, previewKey)
		return nil, err
	}
	if len(reviewCategories) > 0 {
		if _, err := attachmentreview.Write(config.Instance.AttachmentReview.Dir, reviewCategories, att.Id, att.FileName, sourcePath); err != nil {
			slog.Error("write attachment review copy", slog.String("attachmentId", att.Id), slog.Int64("categoryId", categoryId), slog.Any("err", err))
		}
	}
	return att, nil
}

func (s *attachmentService) reviewCategories(categoryId int64) ([]attachmentreview.Category, error) {
	if config.Instance == nil || strings.TrimSpace(config.Instance.AttachmentReview.Dir) == "" {
		return nil, nil
	}
	if categoryId <= 0 {
		return nil, errors.New(locales.Get("topic.category_required"))
	}
	category := repositories.CategoryRepository.Get(sqls.DB(), categoryId)
	if category == nil || category.Status != constants.StatusOk {
		return nil, errors.New(locales.Get("topic.category_not_found"))
	}
	if !category.Type.Supports(constants.TopicTypeTopic) {
		return nil, errors.New(locales.Get("topic.category_type_mismatch"))
	}

	path := []attachmentreview.Category{{ID: category.Id, Name: category.Name}}
	if category.ParentId == 0 {
		return path, nil
	}
	parent := repositories.CategoryRepository.Get(sqls.DB(), category.ParentId)
	if parent == nil || parent.Status != constants.StatusOk || parent.ParentId != 0 {
		return nil, errors.New(locales.Get("topic.category_not_found"))
	}
	if !parent.Type.Supports(constants.TopicTypeTopic) {
		return nil, errors.New(locales.Get("topic.category_type_mismatch"))
	}
	return []attachmentreview.Category{
		{ID: parent.Id, Name: parent.Name},
		{ID: category.Id, Name: category.Name},
	}, nil
}

func isPreviewDocumentExtension(ext string) bool {
	for _, supported := range docpreview.SupportedExtensions() {
		if ext == supported {
			return true
		}
	}
	return false
}

func detectAttachmentContentType(filePath, ext string) string {
	file, err := os.Open(filePath)
	if err == nil {
		defer file.Close()
		header := make([]byte, 512)
		if count, readErr := file.Read(header); readErr == nil || errors.Is(readErr, io.EOF) {
			if count > 0 {
				return http.DetectContentType(header[:count])
			}
		}
	}
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func (s *attachmentService) verifyAttachmentObjects(ctx context.Context, method dto.UploadMethod, originalKey string, originalSize int64, previewKey string, previewSize int64) error {
	originalMeta, err := UploadService.HeadObject(ctx, method, originalKey)
	if err != nil || originalMeta.Size != originalSize {
		return errors.New("stored attachment size mismatch")
	}
	if previewKey != "" && previewKey != originalKey {
		previewMeta, previewErr := UploadService.HeadObject(ctx, method, previewKey)
		if previewErr != nil || previewMeta.Size != previewSize {
			return errors.New("stored preview size mismatch")
		}
	}
	return nil
}

func (s *attachmentService) cleanupAttachmentObjects(method dto.UploadMethod, keys ...string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if err := UploadService.DeleteObject(cleanupCtx, method, key); err != nil {
			slog.Error("cleanup attachment object failed", slog.String("key", key), slog.Any("err", err))
		}
	}
}

// UpdateDownloadScore 更新附件的下载积分（仅附件所属用户可更新）
func (s *attachmentService) UpdateDownloadScore(attachmentId string, userId int64, downloadScore int) (*models.Attachment, error) {
	if strs.IsBlank(attachmentId) {
		return nil, errors.New(locales.Get("attachment.not_found"))
	}
	att := repositories.AttachmentRepository.Get(sqls.DB(), attachmentId)
	if att == nil || att.Status != constants.StatusOk {
		return nil, errors.New(locales.Get("attachment.not_found"))
	}
	if att.UserId != userId {
		return nil, errors.New(locales.Get("attachment.no_permission"))
	}
	if downloadScore < 0 {
		downloadScore = 0
	}
	att.DownloadScore = downloadScore
	att.UpdateTime = dates.NowTimestamp()
	return att, repositories.AttachmentRepository.Updates(sqls.DB(), attachmentId, map[string]any{
		"download_score": downloadScore,
		"update_time":    dates.NowTimestamp(),
	})
}

// Get 根据 ID 获取附件（仅返回存在且正常的）
func (s *attachmentService) Get(id string) *models.Attachment {
	att := repositories.AttachmentRepository.Get(sqls.DB(), id)
	if att == nil || att.Status != constants.StatusOk {
		return nil
	}
	return att
}

// ListByTopicId 按帖子查询正常状态的附件
func (s *attachmentService) ListByTopicId(topicId int64) []models.Attachment {
	return repositories.AttachmentRepository.ListByTopicId(sqls.DB(), topicId)
}

// HasDownloaded 当前用户是否已购买该附件
func (s *attachmentService) HasDownloaded(userId int64, attachmentId string) bool {
	if userId <= 0 || strs.IsBlank(attachmentId) {
		return false
	}
	return repositories.AttachmentDownloadLogRepository.Exists(sqls.DB(), userId, attachmentId)
}

func (s *attachmentService) FindDownloadedAttachmentIds(userId int64, attachmentIds []string) []string {
	if userId <= 0 || len(attachmentIds) == 0 {
		return nil
	}

	filteredIds := make([]string, 0, len(attachmentIds))
	seen := make(map[string]bool, len(attachmentIds))
	for _, attachmentId := range attachmentIds {
		if strs.IsBlank(attachmentId) || seen[attachmentId] {
			continue
		}
		seen[attachmentId] = true
		filteredIds = append(filteredIds, attachmentId)
	}
	if len(filteredIds) == 0 {
		return nil
	}
	return repositories.AttachmentDownloadLogRepository.FindDownloadedAttachmentIds(sqls.DB(), userId, filteredIds)
}

type AttachmentObject struct {
	Attachment *models.Attachment
	Method     dto.UploadMethod
	Key        string
}

func (s *attachmentService) visibleAttachment(db *gorm.DB, attachmentId string, user *models.User) (*models.Attachment, error) {
	if user == nil || strs.IsBlank(attachmentId) {
		return nil, errors.New(locales.Get("attachment.not_found"))
	}
	att := repositories.AttachmentRepository.Get(db, attachmentId)
	if att == nil || att.Status != constants.StatusOk || att.TopicId <= 0 {
		return nil, errors.New(locales.Get("attachment.not_found"))
	}
	topic := repositories.TopicRepository.Get(db, att.TopicId)
	if topic == nil || topic.Type != constants.TopicTypeTopic || topic.Status == constants.StatusDeleted {
		return nil, errors.New(locales.Get("attachment.not_found"))
	}
	if topic.Status == constants.StatusReview {
		if topic.UserId != user.Id && !user.IsOwner() {
			return nil, attachmentAccessForbidden("attachment.no_permission")
		}
	} else if topic.Status != constants.StatusOk {
		return nil, errors.New(locales.Get("attachment.not_found"))
	}
	return att, nil
}

func (s *attachmentService) hasAccess(db *gorm.DB, att *models.Attachment, userId int64) bool {
	return att != nil && userId > 0 && (att.UserId == userId || att.DownloadScore <= 0 || repositories.AttachmentDownloadLogRepository.Exists(db, userId, att.Id))
}

func (s *attachmentService) HasAccess(att *models.Attachment, userId int64) bool {
	return s.hasAccess(sqls.DB(), att, userId)
}

// Access explicitly and idempotently unlocks a paid attachment. Merely
// opening a preview or download URL never deducts score.
func (s *attachmentService) Access(attachmentId string, currentUser *models.User) (*models.Attachment, error) {
	att, err := s.visibleAttachment(sqls.DB(), attachmentId, currentUser)
	if err != nil {
		return nil, err
	}
	if s.hasAccess(sqls.DB(), att, currentUser.Id) {
		return att, nil
	}

	err = sqls.WithTransaction(func(txCtx *sqls.TxContext) error {
		var lockedUser models.User
		if err := txCtx.Tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedUser, "id = ?", currentUser.Id).Error; err != nil {
			return errors.New(locales.Get("common.not_found"))
		}

		var lockedAttachment models.Attachment
		if err := txCtx.Tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedAttachment, "id = ?", attachmentId).Error; err != nil {
			return errors.New(locales.Get("attachment.not_found"))
		}
		if _, err := s.visibleAttachment(txCtx.Tx, attachmentId, currentUser); err != nil {
			return err
		}
		if s.hasAccess(txCtx.Tx, &lockedAttachment, currentUser.Id) {
			return nil
		}
		if lockedUser.Score < lockedAttachment.DownloadScore {
			return errors.New(locales.Get("attachment.insufficient_score"))
		}
		if err := UserService.DecrScoreTx(txCtx, currentUser.Id, lockedAttachment.DownloadScore, constants.SourceTypeAttachmentDownload, attachmentId, locales.Get("attachment.download_deduct")); err != nil {
			return err
		}
		return repositories.AttachmentDownloadLogRepository.Create(txCtx.Tx, &models.AttachmentDownloadLog{
			UserId:       currentUser.Id,
			AttachmentId: attachmentId,
			CreateTime:   dates.NowTimestamp(),
		})
	})
	if err != nil {
		return nil, err
	}
	return repositories.AttachmentRepository.Get(sqls.DB(), attachmentId), nil
}

func (s *attachmentService) AuthorizedObject(attachmentId string, currentUser *models.User, preview bool) (*AttachmentObject, error) {
	att, err := s.visibleAttachment(sqls.DB(), attachmentId, currentUser)
	if err != nil {
		return nil, err
	}
	if !s.hasAccess(sqls.DB(), att, currentUser.Id) {
		return nil, attachmentAccessForbidden("attachment.unlock_required")
	}
	key := att.FileKey
	if preview {
		if att.PreviewStatus != constants.AttachmentPreviewReady || strs.IsBlank(att.PreviewKey) {
			return nil, errors.New(locales.Get("attachment.preview_unavailable"))
		}
		key = att.PreviewKey
	}
	method, resolvedKey, err := UploadService.ResolveStoredObject(att.StorageMethod, key, att.FileUrl)
	if err != nil {
		slog.Error("resolve attachment object failed", slog.String("attachmentId", att.Id), slog.Any("err", err))
		return nil, errors.New(locales.Get("attachment.file_missing"))
	}
	return &AttachmentObject{Attachment: att, Method: method, Key: resolvedKey}, nil
}

func (s *attachmentService) IncrementDownloadCount(attachmentId string) error {
	return repositories.AttachmentRepository.IncrDownloadCount(sqls.DB(), attachmentId)
}

// SoftDeleteByTopicId 帖子删除时软删除其下所有附件
func (s *attachmentService) SoftDeleteByTopicId(ctx *sqls.TxContext, topicId int64) error {
	return repositories.AttachmentRepository.UpdateColumns(ctx.Tx, topicId, map[string]interface{}{
		"status":      constants.StatusDeleted,
		"update_time": dates.NowTimestamp(),
	})
}

// ReplaceTopicAttachments 编辑帖时全量替换附件
func (s *attachmentService) ReplaceTopicAttachments(ctx *sqls.TxContext, topicId, userId int64, attachmentIds []string) error {
	topic := repositories.TopicRepository.Get(ctx.Tx, topicId)
	if topic == nil || topic.Type != constants.TopicTypeTopic {
		return errors.New(locales.Get("attachment.topic_type_not_supported"))
	}
	newSet := make(map[string]bool)
	for _, id := range attachmentIds {
		if strs.IsNotBlank(id) {
			newSet[id] = true
		}
	}

	// 从当前中移除的：解绑 + 软删除
	current := repositories.AttachmentRepository.ListByTopicId(ctx.Tx, topicId)
	for _, att := range current {
		if !newSet[att.Id] {
			if err := repositories.AttachmentRepository.Updates(ctx.Tx, att.Id, map[string]interface{}{
				"topic_id": 0, "status": constants.StatusDeleted, "update_time": dates.NowTimestamp(),
			}); err != nil {
				return err
			}
		}
	}

	// 新列表中的：校验归属且未绑其他帖，再绑定
	for _, aid := range attachmentIds {
		if strs.IsBlank(aid) {
			continue
		}
		att := repositories.AttachmentRepository.Get(ctx.Tx, aid)
		if att == nil || att.Status != constants.StatusOk || att.UserId != userId {
			return errors.New(locales.Get("attachment.no_permission"))
		}
		if att.TopicId != 0 && att.TopicId != topicId {
			return errors.New(locales.Get("attachment.already_bound"))
		}
		if err := repositories.AttachmentRepository.Updates(ctx.Tx, aid, map[string]interface{}{
			"topic_id": topicId, "status": constants.StatusOk, "update_time": dates.NowTimestamp(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// CheckAttachmentsExistAndOwned 检查 attachmentIds 是否存在且均属于 userId，且未绑定其他帖子（或仅绑定 topicId）
func (s *attachmentService) CheckAttachmentsExistAndOwned(ctx *sqls.TxContext, userId int64, attachmentIds []string, topicId int64) error {
	topic := repositories.TopicRepository.Get(ctx.Tx, topicId)
	if topic == nil || topic.Type != constants.TopicTypeTopic {
		return errors.New(locales.Get("attachment.topic_type_not_supported"))
	}
	for _, aid := range attachmentIds {
		if strs.IsBlank(aid) {
			continue
		}
		att := repositories.AttachmentRepository.Get(ctx.Tx, aid)
		if att == nil || att.Status != constants.StatusOk || att.UserId != userId {
			return errors.New(locales.Get("attachment.no_permission"))
		}
		if att.TopicId != 0 && att.TopicId != topicId {
			return errors.New(locales.Get("attachment.already_bound"))
		}
	}
	return nil
}
