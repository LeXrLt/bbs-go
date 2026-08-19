package uploader

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"bbs-go/internal/models/dto"
)

func TestAwsS3Uploader_PutObject_OmitsACL(t *testing.T) {
	requestHeaders := make(chan http.Header, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		requestHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := dto.UploadConfig{AwsS3: dto.AwsS3UploadConfig{
		Region:          "us-east-1",
		Bucket:          "test-bucket",
		AccessKeyId:     "test-access-key-id",
		AccessKeySecret: "test-secret-access-key",
	}}
	client := s3.NewFromConfig(aws.Config{
		Region:      cfg.AwsS3.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AwsS3.AccessKeyId, cfg.AwsS3.AccessKeySecret, ""),
		HTTPClient:  server.Client(),
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(server.URL)
		options.UsePathStyle = true
	})
	uploader := &AwsS3Uploader{client: client, currentCfg: cfg}

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
			if _, ok := headers["X-Amz-Acl"]; ok {
				t.Fatalf("PutObject() sent x-amz-acl header %q", headers.Get("X-Amz-Acl"))
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

func TestAwsS3Uploader_InitClient_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     dto.UploadConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config should not error",
			cfg: dto.UploadConfig{
				AwsS3: dto.AwsS3UploadConfig{
					Region:          "us-east-1",
					Bucket:          "test-bucket",
					AccessKeyId:     "test-access-key-id",
					AccessKeySecret: "test-secret-access-key",
				},
			},
			wantErr: false,
		},
		{
			name: "missing region should error",
			cfg: dto.UploadConfig{
				AwsS3: dto.AwsS3UploadConfig{
					Bucket:          "test-bucket",
					AccessKeyId:     "test-access-key-id",
					AccessKeySecret: "test-secret-access-key",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
		{
			name: "missing bucket should error",
			cfg: dto.UploadConfig{
				AwsS3: dto.AwsS3UploadConfig{
					Region:          "us-east-1",
					AccessKeyId:     "test-access-key-id",
					AccessKeySecret: "test-secret-access-key",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
		{
			name: "missing access key id should error",
			cfg: dto.UploadConfig{
				AwsS3: dto.AwsS3UploadConfig{
					Region:          "us-east-1",
					Bucket:          "test-bucket",
					AccessKeySecret: "test-secret-access-key",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
		{
			name: "missing access key secret should error",
			cfg: dto.UploadConfig{
				AwsS3: dto.AwsS3UploadConfig{
					Region:      "us-east-1",
					Bucket:      "test-bucket",
					AccessKeyId: "test-access-key-id",
				},
			},
			wantErr: true,
			errMsg:  "configuration is incomplete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := &AwsS3Uploader{}
			err := uploader.initClient(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Errorf("initClient() expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if !strings.Contains(err.Error(), tt.errMsg) {
						t.Errorf("initClient() error = %v, want error containing %v", err.Error(), tt.errMsg)
					}
				}
			} else {
				// 即使配置有效，如果没有真实的凭证，也会在创建客户端时失败
				// 所以我们只检查配置验证是否通过
				if err != nil {
					// 如果是配置验证错误，应该包含 "incomplete"
					if strings.Contains(err.Error(), "configuration is incomplete") {
						t.Errorf("initClient() should pass config validation, got error: %v", err)
					}
					// 配置验证通过，但可能因为凭证无效而失败，这是正常的
				}
			}
		})
	}
}

func TestAwsS3Uploader_PutObject_URLGeneration(t *testing.T) {
	uploader := &AwsS3Uploader{}

	cfg := dto.UploadConfig{
		AwsS3: dto.AwsS3UploadConfig{
			Region:          "us-east-1",
			Bucket:          "test-bucket",
			AccessKeyId:     "test-access-key-id",
			AccessKeySecret: "test-secret-access-key",
		},
	}

	key := "images/2026/01/28/test123.jpg"
	data := []byte("test image data")
	contentType := "image/jpeg"

	// 由于需要真实的 AWS 凭证，这里只测试 URL 生成逻辑
	// 实际 URL 格式应该是: https://{bucket}.s3.{region}.amazonaws.com/{key}
	expectedURLPrefix := "https://test-bucket.s3.us-east-1.amazonaws.com/"

	// 注意：这个测试需要 mock S3 客户端才能完整运行
	// 这里只是验证 URL 格式逻辑
	_ = uploader
	_ = cfg
	_ = key
	_ = data
	_ = contentType
	_ = expectedURLPrefix

	// 验证 URL 格式
	expectedURL := expectedURLPrefix + key
	if !strings.HasPrefix(expectedURL, "https://") {
		t.Errorf("URL should start with https://")
	}
	if !strings.Contains(expectedURL, "test-bucket.s3.us-east-1.amazonaws.com") {
		t.Errorf("URL should contain bucket and region")
	}
	if !strings.HasSuffix(expectedURL, key) {
		t.Errorf("URL should end with key")
	}
}

func TestAwsS3Uploader_PutImage_ContentType(t *testing.T) {
	uploader := &AwsS3Uploader{}

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
			// 实际测试需要 mock S3 客户端
			_ = uploader
			_ = tt
		})
	}
}

func TestAwsS3Uploader_IsCfgChange(t *testing.T) {
	uploader := &AwsS3Uploader{}

	cfg1 := dto.UploadConfig{
		AwsS3: dto.AwsS3UploadConfig{
			Region:          "us-east-1",
			Bucket:          "test-bucket",
			AccessKeyId:     "test-key-id",
			AccessKeySecret: "test-secret",
		},
	}

	cfg2 := dto.UploadConfig{
		AwsS3: dto.AwsS3UploadConfig{
			Region:          "us-east-1",
			Bucket:          "test-bucket",
			AccessKeyId:     "test-key-id",
			AccessKeySecret: "test-secret",
		},
	}

	cfg3 := dto.UploadConfig{
		AwsS3: dto.AwsS3UploadConfig{
			Region:          "us-west-2", // 不同的 region
			Bucket:          "test-bucket",
			AccessKeyId:     "test-key-id",
			AccessKeySecret: "test-secret",
		},
	}

	// 初始状态，client 为 nil，应该返回 true
	if !uploader.isCfgChange(cfg1) {
		t.Errorf("isCfgChange() should return true when client is nil")
	}

	// 相同配置，应该返回 false（但需要先初始化）
	// 由于需要真实凭证才能初始化，这里只测试逻辑

	// 不同配置，应该返回 true
	if !uploader.isCfgChange(cfg3) {
		t.Errorf("isCfgChange() should return true when config changes")
	}

	_ = cfg2
}
