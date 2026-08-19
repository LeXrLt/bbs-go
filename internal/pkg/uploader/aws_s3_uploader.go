package uploader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mlogclub/simple/common/strs"

	"bbs-go/internal/models/dto"
)

type AwsS3Uploader struct {
	m          sync.Mutex
	client     *s3.Client
	currentCfg dto.UploadConfig
}

func (u *AwsS3Uploader) PutObject(cfg dto.UploadConfig, key string, body io.Reader, opts *PutOptions) (string, error) {
	if err := u.initClient(cfg); err != nil {
		return "", err
	}
	putInput := &s3.PutObjectInput{
		Bucket: aws.String(cfg.AwsS3.Bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if opts != nil {
		if opts.ContentLength > 0 {
			putInput.ContentLength = aws.Int64(opts.ContentLength)
		}
		if opts.ContentType != "" {
			putInput.ContentType = aws.String(opts.ContentType)
		}
		if opts.ContentDisposition != "" {
			putInput.ContentDisposition = aws.String(opts.ContentDisposition)
		}
	}
	if _, err := u.client.PutObject(context.Background(), putInput); err != nil {
		slog.Error("AWS S3 PutObject failed", slog.Any("err", err), slog.String("bucket", cfg.AwsS3.Bucket), slog.String("key", key))
		return "", fmt.Errorf("failed to upload object to S3: %w", err)
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.AwsS3.Bucket, cfg.AwsS3.Region, key), nil
}

func (u *AwsS3Uploader) HeadObject(ctx context.Context, cfg dto.UploadConfig, key string) (ObjectMeta, error) {
	if err := u.initClient(cfg); err != nil {
		return ObjectMeta{}, err
	}
	out, err := u.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(cfg.AwsS3.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ObjectMeta{}, err
	}
	meta := ObjectMeta{Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType)}
	if out.LastModified != nil {
		meta.LastModified = *out.LastModified
	}
	return meta, nil
}

func (u *AwsS3Uploader) GetObject(ctx context.Context, cfg dto.UploadConfig, key string, opts GetOptions) (io.ReadCloser, error) {
	if err := u.initClient(cfg); err != nil {
		return nil, err
	}
	if opts.Offset < 0 || opts.Length < 0 {
		return nil, fmt.Errorf("invalid object range")
	}
	input := &s3.GetObjectInput{Bucket: aws.String(cfg.AwsS3.Bucket), Key: aws.String(key)}
	if opts.Offset > 0 || opts.Length > 0 {
		if opts.Length <= 0 {
			return nil, fmt.Errorf("invalid object range")
		}
		input.Range = aws.String(fmt.Sprintf("bytes=%d-%d", opts.Offset, opts.Offset+opts.Length-1))
	}
	out, err := u.client.GetObject(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (u *AwsS3Uploader) DeleteObject(ctx context.Context, cfg dto.UploadConfig, key string) error {
	if err := u.initClient(cfg); err != nil {
		return err
	}
	_, err := u.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(cfg.AwsS3.Bucket),
		Key:    aws.String(key),
	})
	return err
}

func (u *AwsS3Uploader) CopyImage(cfg dto.UploadConfig, originUrl string) (string, error) {
	data, ct, err := download(originUrl)
	if err != nil {
		return "", err
	}
	ct = NormalizeImageContentType(ct)
	key := GenerateImageKey(data, ct)
	opts := &PutOptions{ContentType: ct, ContentLength: int64(len(data))}
	return u.PutObject(cfg, key, bytes.NewReader(data), opts)
}

func (u *AwsS3Uploader) initClient(cfg dto.UploadConfig) error {
	if !u.isCfgChange(cfg) {
		return nil
	}

	u.m.Lock()
	defer u.m.Unlock()

	// 验证必要配置项不能为空
	if strs.IsAnyBlank(cfg.AwsS3.Region, cfg.AwsS3.Bucket, cfg.AwsS3.AccessKeyId, cfg.AwsS3.AccessKeySecret) {
		return fmt.Errorf("AWS S3 configuration is incomplete: Region, Bucket, AccessKeyId, and AccessKeySecret are required")
	}

	// 创建 AWS 配置（使用标准 AWS S3）
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(cfg.AwsS3.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AwsS3.AccessKeyId,
			cfg.AwsS3.AccessKeySecret,
			"",
		)),
	)
	if err != nil {
		slog.Error("Failed to load AWS config", slog.Any("err", err))
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// 创建 S3 客户端
	u.client = s3.NewFromConfig(awsCfg)

	u.currentCfg = cfg
	return nil
}

func (u *AwsS3Uploader) isCfgChange(cfg dto.UploadConfig) bool {
	if u.client == nil {
		return true
	}

	if u.currentCfg.AwsS3.Region != cfg.AwsS3.Region ||
		u.currentCfg.AwsS3.Bucket != cfg.AwsS3.Bucket ||
		u.currentCfg.AwsS3.AccessKeyId != cfg.AwsS3.AccessKeyId ||
		u.currentCfg.AwsS3.AccessKeySecret != cfg.AwsS3.AccessKeySecret {
		return true
	}

	return false
}
