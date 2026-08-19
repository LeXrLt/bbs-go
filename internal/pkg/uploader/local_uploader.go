package uploader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/respath"
)

type LocalUploader struct{}

func localObjectPath(key string) (string, error) {
	normalized := filepath.ToSlash(strings.TrimSpace(key))
	if normalized == "" || strings.HasPrefix(normalized, "/") || strings.Contains(normalized, "\\") {
		return "", fmt.Errorf("invalid object key")
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid object key")
		}
	}
	cleanKey := strings.TrimPrefix(path.Clean("/"+normalized), "/")
	return respath.UploadsPath(filepath.FromSlash(cleanKey)), nil
}

func (u *LocalUploader) PutObject(_ dto.UploadConfig, key string, body io.Reader, opts *PutOptions) (string, error) {
	fullPath, err := localObjectPath(key)
	if err != nil {
		return "", err
	}
	cleanKey := filepath.ToSlash(strings.TrimSpace(key))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}
	dest, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer dest.Close()
	src := body
	if opts != nil && opts.ContentLength > 0 {
		src = io.LimitReader(body, opts.ContentLength)
	}
	if _, err := io.Copy(dest, src); err != nil {
		_ = os.Remove(fullPath)
		return "", err
	}
	return respath.UploadsURLPrefix + cleanKey, nil
}

func (u *LocalUploader) HeadObject(_ context.Context, _ dto.UploadConfig, key string) (ObjectMeta, error) {
	fullPath, err := localObjectPath(key)
	if err != nil {
		return ObjectMeta{}, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return ObjectMeta{}, err
	}
	if !info.Mode().IsRegular() {
		return ObjectMeta{}, os.ErrNotExist
	}
	return ObjectMeta{
		Size:         info.Size(),
		ContentType:  mime.TypeByExtension(filepath.Ext(fullPath)),
		LastModified: info.ModTime(),
	}, nil
}

type sectionReadCloser struct {
	*io.SectionReader
	closer io.Closer
}

func (r *sectionReadCloser) Close() error {
	return r.closer.Close()
}

func (u *LocalUploader) GetObject(_ context.Context, _ dto.UploadConfig, key string, opts GetOptions) (io.ReadCloser, error) {
	fullPath, err := localObjectPath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	if opts.Offset < 0 || opts.Length < 0 {
		_ = file.Close()
		return nil, fmt.Errorf("invalid object range")
	}
	if opts.Offset == 0 && opts.Length == 0 {
		return file, nil
	}
	return &sectionReadCloser{
		SectionReader: io.NewSectionReader(file, opts.Offset, opts.Length),
		closer:        file,
	}, nil
}

func (u *LocalUploader) DeleteObject(_ context.Context, _ dto.UploadConfig, key string) error {
	fullPath, err := localObjectPath(key)
	if err != nil {
		return err
	}
	err = os.Remove(fullPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (u *LocalUploader) CopyImage(cfg dto.UploadConfig, originUrl string) (string, error) {
	data, ct, err := download(originUrl)
	if err != nil {
		return "", err
	}
	ct = NormalizeImageContentType(ct)
	key := GenerateImageKey(data, ct)
	opts := &PutOptions{ContentType: ct, ContentLength: int64(len(data))}
	return u.PutObject(cfg, key, bytes.NewReader(data), opts)
}
