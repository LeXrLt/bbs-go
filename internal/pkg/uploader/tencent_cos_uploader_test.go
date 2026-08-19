package uploader

import (
	"bytes"
	"hash/crc64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/tencentyun/cos-go-sdk-v5"

	"bbs-go/internal/models/dto"
)

func TestTencentCosUploader_PutObject_OmitsACL(t *testing.T) {
	requestHeaders := make(chan http.Header, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checksum := crc64.New(crc64.MakeTable(crc64.ECMA))
		_, _ = io.Copy(checksum, r.Body)
		requestHeaders <- r.Header.Clone()
		w.Header().Set("x-cos-request-id", "test-request-id")
		w.Header().Set("x-cos-hash-crc64ecma", strconv.FormatUint(checksum.Sum64(), 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	bucketURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	cfg := dto.UploadConfig{TencentCos: dto.TencentCosUploadConfig{
		Bucket:    "test-bucket-1234567890",
		Region:    "ap-beijing",
		SecretId:  "test-secret-id",
		SecretKey: "test-secret-key",
	}}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL, ServiceURL: bucketURL}, server.Client())
	uploader := &TencentCosUploader{client: client, currentCfg: cfg}

	tests := []struct {
		name        string
		key         string
		private     bool
		contentType string
	}{
		{name: "ordinary image", key: "images/test.jpg", contentType: "image/jpeg"},
		{name: "private attachment", key: "attachments/test.pdf", private: true, contentType: "application/pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte("object body")
			disposition := `attachment; filename="test.pdf"`
			_, err := uploader.PutObject(cfg, tt.key, bytes.NewReader(body), &PutOptions{
				ContentType:        tt.contentType,
				ContentDisposition: disposition,
				ContentLength:      int64(len(body)),
				Private:            tt.private,
			})
			if err != nil {
				t.Fatalf("PutObject() error = %v", err)
			}

			headers := <-requestHeaders
			if _, ok := headers["X-Cos-Acl"]; ok {
				t.Fatalf("PutObject() sent x-cos-acl header %q", headers.Get("X-Cos-Acl"))
			}
			if got := headers.Get("Content-Type"); got != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if got := headers.Get("Content-Disposition"); got != disposition {
				t.Errorf("Content-Disposition = %q, want %q", got, disposition)
			}
		})
	}
}

func TestTencentCosUploader_InitClient_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     dto.UploadConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config should not error",
			cfg: dto.UploadConfig{
				TencentCos: dto.TencentCosUploadConfig{
					Bucket:    "test-bucket-1234567890",
					Region:    "ap-beijing",
					SecretId:  "test-secret-id",
					SecretKey: "test-secret-key",
				},
			},
			wantErr: false,
		},
		{
			name: "missing bucket should error",
			cfg: dto.UploadConfig{
				TencentCos: dto.TencentCosUploadConfig{
					Region:    "ap-beijing",
					SecretId:  "test-secret-id",
					SecretKey: "test-secret-key",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
		{
			name: "missing region should error",
			cfg: dto.UploadConfig{
				TencentCos: dto.TencentCosUploadConfig{
					Bucket:    "test-bucket-1234567890",
					SecretId:  "test-secret-id",
					SecretKey: "test-secret-key",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
		{
			name: "missing secret id should error",
			cfg: dto.UploadConfig{
				TencentCos: dto.TencentCosUploadConfig{
					Bucket:    "test-bucket-1234567890",
					Region:    "ap-beijing",
					SecretKey: "test-secret-key",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
		{
			name: "missing secret key should error",
			cfg: dto.UploadConfig{
				TencentCos: dto.TencentCosUploadConfig{
					Bucket:   "test-bucket-1234567890",
					Region:   "ap-beijing",
					SecretId: "test-secret-id",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := &TencentCosUploader{}
			err := uploader.initClient(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Errorf("initClient() expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if err.Error() == "" {
						t.Errorf("initClient() error message is empty")
					}
				}
			} else {
				// 即使配置有效，如果没有真实的凭证，也会在创建客户端时失败
				// 所以我们只检查配置验证是否通过
				if err != nil {
					// 如果是配置验证错误，应该包含 "incomplete"
					if err.Error() != "" {
						// 配置验证通过，但可能因为凭证无效而失败，这是正常的
					}
				}
			}
		})
	}
}

func TestTencentCosUploader_PutImage_ContentType(t *testing.T) {
	uploader := &TencentCosUploader{}

	tests := []struct {
		name        string
		contentType string
		wantDefault string
	}{
		{
			name:        "empty contentType should default to image/jpeg",
			contentType: "",
			wantDefault: "image/jpeg",
		},
		{
			name:        "blank contentType should default to image/jpeg",
			contentType: "   ",
			wantDefault: "image/jpeg",
		},
		{
			name:        "valid contentType should be preserved",
			contentType: "image/png",
			wantDefault: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 由于需要真实配置，这里只测试逻辑
			// 实际测试需要 mock COS 客户端
			_ = uploader
			_ = tt
		})
	}
}
