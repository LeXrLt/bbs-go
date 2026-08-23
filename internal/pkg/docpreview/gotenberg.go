package docpreview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const (
	DefaultMaxSourceDocumentBytes = int64(256 << 20)
	DefaultMaxConvertedPDFBytes   = int64(256 << 20)
	gotenbergConvertPath          = "/forms/libreoffice/convert"
)

var (
	ErrSourceDocumentTooLarge = errors.New("source document exceeds upload limit")
	ErrConvertedPDFTooLarge   = errors.New("converted PDF exceeds response limit")
)

// ConversionFailureKind tells callers whether retrying a conversion can be
// useful. Input errors and HTTP 4xx responses other than 429 are permanent;
// transport errors, HTTP 429, and HTTP 5xx responses are retryable.
type ConversionFailureKind string

const (
	ConversionFailurePermanent ConversionFailureKind = "permanent"
	ConversionFailureRetryable ConversionFailureKind = "retryable"
)

// ConversionError describes a failed Gotenberg conversion.
type ConversionError struct {
	Kind       ConversionFailureKind
	StatusCode int
	Operation  string
	Err        error
}

func (e *ConversionError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("document conversion %s failed with HTTP %d: %v", e.Operation, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("document conversion %s failed: %v", e.Operation, e.Err)
}

func (e *ConversionError) Unwrap() error {
	return e.Err
}

// IsRetryable reports whether err is a conversion failure that can be retried.
func IsRetryable(err error) bool {
	var conversionErr *ConversionError
	return errors.As(err, &conversionErr) && conversionErr.Kind == ConversionFailureRetryable
}

// IsPermanent reports whether err is a non-retryable conversion failure.
func IsPermanent(err error) bool {
	var conversionErr *ConversionError
	return errors.As(err, &conversionErr) && conversionErr.Kind == ConversionFailurePermanent
}

// PDFConverter is implemented by GotenbergClient and can be replaced by a
// test double in the attachment service.
type PDFConverter interface {
	Convert(ctx context.Context, sourcePath, originalName string) ([]byte, error)
}

// GotenbergClient converts Office files through Gotenberg's LibreOffice route.
// It does not set a timeout; callers control cancellation through ctx or the
// supplied http.Client.
type GotenbergClient struct {
	endpoint            string
	httpClient          *http.Client
	maxSourceSize       int64
	maxConvertedPDFSize int64
}

// NewGotenbergClient constructs a converter. maxConvertedPDFSize must be
// positive; DefaultMaxConvertedPDFBytes is suitable for the default setup.
func NewGotenbergClient(baseURL string, httpClient *http.Client, maxConvertedPDFSize int64) (*GotenbergClient, error) {
	return NewGotenbergClientWithLimits(baseURL, httpClient, DefaultMaxSourceDocumentBytes, maxConvertedPDFSize)
}

// NewGotenbergClientWithLimits constructs a converter with explicit source
// and response byte limits. The source is streamed, but its on-disk size must
// not exceed maxSourceSize.
func NewGotenbergClientWithLimits(baseURL string, httpClient *http.Client, maxSourceSize, maxConvertedPDFSize int64) (*GotenbergClient, error) {
	endpoint, err := gotenbergEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if maxSourceSize <= 0 {
		return nil, fmt.Errorf("max source document size must be positive")
	}
	if maxConvertedPDFSize <= 0 || maxConvertedPDFSize == int64(^uint64(0)>>1) {
		return nil, fmt.Errorf("max converted PDF size must be positive")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &GotenbergClient{
		endpoint:            endpoint,
		httpClient:          &clientCopy,
		maxSourceSize:       maxSourceSize,
		maxConvertedPDFSize: maxConvertedPDFSize,
	}, nil
}

// Convert sends one supported Office document using the multipart field
// "files" and returns a validated PDF. Call Validate before Convert when the
// source originates from an untrusted upload.
func (c *GotenbergClient) Convert(ctx context.Context, sourcePath, originalName string) ([]byte, error) {
	if c == nil || c.httpClient == nil || c.endpoint == "" || c.maxSourceSize <= 0 || c.maxConvertedPDFSize <= 0 {
		return nil, permanentConversionError("configure client", 0, errors.New("Gotenberg client is not configured"))
	}
	info, err := infoForFilename(originalName)
	if err != nil {
		return nil, permanentConversionError("validate input", 0, err)
	}
	if !info.NeedsConversion {
		return nil, permanentConversionError("validate input", 0, errors.New("PDF input does not require LibreOffice conversion"))
	}

	if ctx == nil {
		return nil, permanentConversionError("create request", 0, errors.New("context is nil"))
	}
	body, contentType, writeResult, err := buildMultipartBody(sourcePath, originalName, c.maxSourceSize)
	if err != nil {
		return nil, permanentConversionError("read input", 0, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		_ = body.Close()
		<-writeResult
		return nil, permanentConversionError("create request", 0, err)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", "application/pdf")

	response, requestErr := c.httpClient.Do(request)
	_ = body.Close()
	bodyErr := <-writeResult
	if bodyErr != nil && !errors.Is(bodyErr, io.ErrClosedPipe) {
		if response != nil {
			response.Body.Close()
		}
		return nil, permanentConversionError("read input", 0, bodyErr)
	}
	if requestErr != nil {
		kind := ConversionFailureRetryable
		if errors.Is(requestErr, context.Canceled) {
			kind = ConversionFailurePermanent
		}
		return nil, conversionError(kind, "request", 0, requestErr)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		kind := ConversionFailurePermanent
		if response.StatusCode == http.StatusTooManyRequests ||
			(response.StatusCode >= http.StatusInternalServerError && response.StatusCode <= 599) {
			kind = ConversionFailureRetryable
		}
		return nil, conversionError(kind, "request", response.StatusCode, errors.New(http.StatusText(response.StatusCode)))
	}

	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/pdf") {
		return nil, retryableConversionError("validate response", response.StatusCode, fmt.Errorf("unexpected Content-Type %q", response.Header.Get("Content-Type")))
	}
	if response.ContentLength > c.maxConvertedPDFSize {
		return nil, permanentConversionError("read response", response.StatusCode, ErrConvertedPDFTooLarge)
	}

	limited := &io.LimitedReader{R: response.Body, N: c.maxConvertedPDFSize + 1}
	converted, err := io.ReadAll(limited)
	if err != nil {
		return nil, retryableConversionError("read response", response.StatusCode, err)
	}
	if int64(len(converted)) > c.maxConvertedPDFSize {
		return nil, permanentConversionError("read response", response.StatusCode, ErrConvertedPDFTooLarge)
	}
	if err := validatePDFBytes(converted); err != nil {
		return nil, retryableConversionError("validate response", response.StatusCode, err)
	}
	return converted, nil
}

func gotenbergEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse Gotenberg URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("Gotenberg URL must use http or https and include a host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("Gotenberg URL must not contain credentials, query parameters, or a fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("Gotenberg URL must not contain a path")
	}
	parsed.Path = gotenbergConvertPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

func buildMultipartBody(sourcePath, originalName string, maxSourceSize int64) (*io.PipeReader, string, <-chan error, error) {
	filename, err := safeMultipartFilename(originalName)
	if err != nil {
		return nil, "", nil, err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("open source: %w", err)
	}

	stat, err := source.Stat()
	if err != nil {
		source.Close()
		return nil, "", nil, fmt.Errorf("stat source: %w", err)
	}
	if !stat.Mode().IsRegular() {
		source.Close()
		return nil, "", nil, errors.New("source is not a regular file")
	}
	if stat.Size() == 0 {
		source.Close()
		return nil, "", nil, ErrEmptyFile
	}
	if stat.Size() > maxSourceSize {
		source.Close()
		return nil, "", nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrSourceDocumentTooLarge, stat.Size(), maxSourceSize)
	}

	reader, writerPipe := io.Pipe()
	writer := multipart.NewWriter(writerPipe)
	contentType := writer.FormDataContentType()
	writeResult := make(chan error, 1)
	go func() {
		defer source.Close()
		part, writeErr := writer.CreateFormFile("files", filename)
		if writeErr == nil {
			_, writeErr = io.CopyN(part, source, stat.Size())
		}
		if closeErr := writer.Close(); writeErr == nil {
			writeErr = closeErr
		}
		_ = writerPipe.CloseWithError(writeErr)
		writeResult <- writeErr
		close(writeResult)
	}()
	return reader, contentType, writeResult, nil
}

func safeMultipartFilename(originalName string) (string, error) {
	filename := pathpkg.Base(strings.ReplaceAll(strings.TrimSpace(originalName), "\\", "/"))
	if filename == "" || filename == "." || filename == "/" {
		return "", errors.New("invalid original filename")
	}
	if len(filename) > 255 {
		return "", errors.New("original filename is too long")
	}
	for _, character := range filename {
		if character < 0x20 || character == 0x7f {
			return "", errors.New("original filename contains control characters")
		}
	}
	if filepath.Ext(filename) == "" {
		return "", errors.New("original filename has no extension")
	}
	return filename, nil
}

func conversionError(kind ConversionFailureKind, operation string, statusCode int, err error) error {
	return &ConversionError{Kind: kind, StatusCode: statusCode, Operation: operation, Err: err}
}

func permanentConversionError(operation string, statusCode int, err error) error {
	return conversionError(ConversionFailurePermanent, operation, statusCode, err)
}

func retryableConversionError(operation string, statusCode int, err error) error {
	return conversionError(ConversionFailureRetryable, operation, statusCode, err)
}
