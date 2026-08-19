package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/mlogclub/simple/common/strs"
	"github.com/mlogclub/simple/common/urls"

	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/bbsurls"
	"bbs-go/internal/pkg/respath"
	"bbs-go/internal/pkg/uploader"
)

var UploadService = newUploadService()

type uploadService struct {
	uploaderMap map[dto.UploadMethod]uploader.Uploader
	once        sync.Once
}

func newUploadService() *uploadService {
	return &uploadService{
		uploaderMap: make(map[dto.UploadMethod]uploader.Uploader),
	}
}

func (s *uploadService) putObject(key string, body io.Reader, opts *uploader.PutOptions) (string, error) {
	u, cfg, _, err := s.currentUploader()
	if err != nil {
		return "", err
	}
	return u.PutObject(cfg, key, body, opts)
}

// PutObject 按 key 流式上传；opts 可设置 ContentType、ContentDisposition、ContentLength。
func (s *uploadService) PutObject(key string, body io.Reader, opts *uploader.PutOptions) (string, error) {
	return s.putObject(key, body, opts)
}

// PutObjectTracked uploads with the currently configured backend and returns
// the backend identity that must be persisted alongside the object key.
func (s *uploadService) PutObjectTracked(key string, body io.Reader, opts *uploader.PutOptions) (string, dto.UploadMethod, error) {
	u, cfg, method, err := s.currentUploader()
	if err != nil {
		return "", "", err
	}
	objectURL, err := u.PutObject(cfg, key, body, opts)
	return objectURL, method, err
}

func (s *uploadService) PutObjectWithMethod(method dto.UploadMethod, key string, body io.Reader, opts *uploader.PutOptions) (string, error) {
	u, cfg, err := s.uploaderForStoredMethod(method)
	if err != nil {
		return "", err
	}
	return u.PutObject(cfg, key, body, opts)
}

func (s *uploadService) HeadObject(ctx context.Context, method dto.UploadMethod, key string) (uploader.ObjectMeta, error) {
	u, cfg, err := s.uploaderForStoredMethod(method)
	if err != nil {
		return uploader.ObjectMeta{}, err
	}
	return u.HeadObject(ctx, cfg, key)
}

func (s *uploadService) GetObject(ctx context.Context, method dto.UploadMethod, key string, opts uploader.GetOptions) (io.ReadCloser, error) {
	u, cfg, err := s.uploaderForStoredMethod(method)
	if err != nil {
		return nil, err
	}
	return u.GetObject(ctx, cfg, key, opts)
}

func (s *uploadService) DeleteObject(ctx context.Context, method dto.UploadMethod, key string) error {
	u, cfg, err := s.uploaderForStoredMethod(method)
	if err != nil {
		return err
	}
	return u.DeleteObject(ctx, cfg, key)
}

// ResolveStoredObject supports rows created before storage_method/file_key were
// introduced. New attachment rows always persist both fields directly.
func (s *uploadService) ResolveStoredObject(storageMethod, fileKey, fileURL string) (dto.UploadMethod, string, error) {
	method := dto.UploadMethod(strings.TrimSpace(storageMethod))
	key := strings.TrimSpace(fileKey)
	if key == "" {
		parsed, err := url.Parse(strings.TrimSpace(fileURL))
		if err != nil {
			return "", "", fmt.Errorf("invalid stored object URL: %w", err)
		}
		key = strings.TrimPrefix(parsed.Path, "/")
		if strings.HasPrefix(parsed.Path, respath.UploadsURLPrefix) {
			key = strings.TrimPrefix(parsed.Path, respath.UploadsURLPrefix)
			if method == "" {
				method = dto.Local
			}
		}
	}
	if key == "" {
		return "", "", fmt.Errorf("stored object key is missing")
	}
	if method == "" {
		method = s.inferLegacyUploadMethod(fileURL)
	}
	if _, err := s.getUploaderForMethod(method); err != nil {
		return "", "", err
	}
	return method, key, nil
}

func (s *uploadService) ObjectURL(key string) string {
	cfg := SysConfigService.GetUploadConfig()
	if strs.IsBlank(string(cfg.EnableUploadMethod)) {
		cfg.EnableUploadMethod = dto.Local
	}

	switch cfg.EnableUploadMethod {
	case dto.AliyunOss:
		return bbsurls.UrlJoin(cfg.AliyunOss.Host, key)
	case dto.TencentCos:
		return fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s", cfg.TencentCos.Bucket, cfg.TencentCos.Region, key)
	case dto.AwsS3:
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.AwsS3.Bucket, cfg.AwsS3.Region, key)
	default:
		return respath.UploadsURLPrefix + key
	}
}

// PutImage 上传图片（已有完整字节）；key 使用内容 MD5，供 CopyImage 等场景。
func (s *uploadService) PutImage(data []byte, contentType string) (string, error) {
	contentType = uploader.NormalizeImageContentType(contentType)
	key := uploader.GenerateImageKey(data, contentType)
	opts := &uploader.PutOptions{ContentType: contentType, ContentLength: int64(len(data))}
	return s.putObject(key, bytes.NewReader(data), opts)
}

// PutImageStream 流式上传图片；key 使用 UUID，无需先读完整 body。
func (s *uploadService) PutImageStream(body io.Reader, contentLength int64, contentType string) (string, error) {
	contentType = uploader.NormalizeImageContentType(contentType)
	key := uploader.GenerateImageKeyByContentType(contentType)
	opts := &uploader.PutOptions{ContentType: contentType, ContentLength: contentLength}
	return s.putObject(key, body, opts)
}

func (s *uploadService) CopyImage(url string) (string, error) {
	u, cfg, _, err := s.currentUploader()
	if err != nil {
		return "", err
	}
	u1 := urls.ParseUrl(url).GetURL()
	u2 := urls.ParseUrl(SysConfigService.GetBaseURL()).GetURL()
	if u1.Host == u2.Host {
		return url, nil
	}
	return u.CopyImage(cfg, url)
}

func (s *uploadService) getUploader() (uploader.Uploader, error) {
	u, _, _, err := s.currentUploader()
	return u, err
}

func (s *uploadService) currentUploader() (uploader.Uploader, dto.UploadConfig, dto.UploadMethod, error) {
	cfg := SysConfigService.GetUploadConfig()
	method := cfg.EnableUploadMethod
	if strs.IsBlank(string(method)) {
		method = dto.Local
	}
	u, err := s.getUploaderForMethod(method)
	return u, cfg, method, err
}

func (s *uploadService) uploaderForStoredMethod(method dto.UploadMethod) (uploader.Uploader, dto.UploadConfig, error) {
	if strs.IsBlank(string(method)) {
		return nil, dto.UploadConfig{}, fmt.Errorf("stored upload method is missing")
	}
	u, err := s.getUploaderForMethod(method)
	if method == dto.Local {
		return u, dto.UploadConfig{EnableUploadMethod: dto.Local}, err
	}
	return u, SysConfigService.GetUploadConfig(), err
}

func (s *uploadService) getUploaderForMethod(method dto.UploadMethod) (uploader.Uploader, error) {
	s.once.Do(func() {
		s.uploaderMap[dto.Local] = &uploader.LocalUploader{}
		s.uploaderMap[dto.AliyunOss] = &uploader.AliyunOssUploader{}
		s.uploaderMap[dto.TencentCos] = &uploader.TencentCosUploader{}
		s.uploaderMap[dto.AwsS3] = &uploader.AwsS3Uploader{}
	})
	u, ok := s.uploaderMap[method]
	if !ok {
		return nil, fmt.Errorf("error: Upload method: %s not found", method)
	}
	return u, nil
}

func (s *uploadService) inferLegacyUploadMethod(fileURL string) dto.UploadMethod {
	parsed, _ := url.Parse(strings.TrimSpace(fileURL))
	if parsed == nil || parsed.Host == "" {
		return dto.Local
	}
	cfg := SysConfigService.GetUploadConfig()
	if host, _ := url.Parse(cfg.AliyunOss.Host); host != nil && strings.EqualFold(host.Host, parsed.Host) {
		return dto.AliyunOss
	}
	if strings.EqualFold(parsed.Host, fmt.Sprintf("%s.cos.%s.myqcloud.com", cfg.TencentCos.Bucket, cfg.TencentCos.Region)) {
		return dto.TencentCos
	}
	if strings.EqualFold(parsed.Host, fmt.Sprintf("%s.s3.%s.amazonaws.com", cfg.AwsS3.Bucket, cfg.AwsS3.Region)) {
		return dto.AwsS3
	}
	if strs.IsBlank(string(cfg.EnableUploadMethod)) {
		return dto.Local
	}
	return cfg.EnableUploadMethod
}
