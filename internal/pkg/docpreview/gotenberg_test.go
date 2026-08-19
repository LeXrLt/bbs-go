package docpreview

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

var validConvertedPDF = minimalTestPDF()

func TestNewGotenbergClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		maxSize int64
		wantURL string
		wantErr bool
	}{
		{name: "root URL", baseURL: "http://gotenberg:3000", maxSize: 1024, wantURL: "http://gotenberg:3000/forms/libreoffice/convert"},
		{name: "trailing slash", baseURL: "https://example.test/", maxSize: 1024, wantURL: "https://example.test/forms/libreoffice/convert"},
		{name: "path prefix", baseURL: "https://example.test/internal", maxSize: 1024, wantErr: true},
		{name: "missing scheme", baseURL: "gotenberg:3000", maxSize: 1024, wantErr: true},
		{name: "unsupported scheme", baseURL: "ftp://example.test", maxSize: 1024, wantErr: true},
		{name: "credentials", baseURL: "http://user:pass@example.test", maxSize: 1024, wantErr: true},
		{name: "query", baseURL: "http://example.test?q=1", maxSize: 1024, wantErr: true},
		{name: "zero response limit", baseURL: "http://example.test", maxSize: 0, wantErr: true},
		{name: "negative response limit", baseURL: "http://example.test", maxSize: -1, wantErr: true},
		{name: "overflowing response limit", baseURL: "http://example.test", maxSize: math.MaxInt64, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewGotenbergClient(test.baseURL, nil, test.maxSize)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewGotenbergClient() error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && client.endpoint != test.wantURL {
				t.Fatalf("endpoint = %q, want %q", client.endpoint, test.wantURL)
			}
		})
	}
}

func TestNewGotenbergClientWithLimitsRejectsInvalidSourceLimit(t *testing.T) {
	for _, maxSourceSize := range []int64{0, -1} {
		if _, err := NewGotenbergClientWithLimits("http://example.test", nil, maxSourceSize, 1024); err == nil {
			t.Fatalf("NewGotenbergClientWithLimits() accepted source limit %d", maxSourceSize)
		}
	}
}

func TestGotenbergClientConvert(t *testing.T) {
	sourceData := append(append([]byte(nil), oleCFBMagic...), []byte("source office document")...)
	sourcePath := writeFixture(t, sourceData)

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != gotenbergConvertPath {
			t.Errorf("path = %q, want %q", request.URL.Path, gotenbergConvertPath)
		}
		if request.Header.Get("Accept") != "application/pdf" {
			t.Errorf("Accept = %q, want application/pdf", request.Header.Get("Accept"))
		}

		multipartReader, err := request.MultipartReader()
		if err != nil {
			t.Errorf("MultipartReader() error = %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		part, err := multipartReader.NextPart()
		if err != nil {
			t.Errorf("NextPart() error = %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		if part.FormName() != "files" {
			t.Errorf("multipart field = %q, want files", part.FormName())
		}
		if part.FileName() != "Quarterly Report.DOC" {
			t.Errorf("multipart filename = %q, want %q", part.FileName(), "Quarterly Report.DOC")
		}
		gotSource, err := io.ReadAll(part)
		if err != nil {
			t.Errorf("read multipart file: %v", err)
		}
		if string(gotSource) != string(sourceData) {
			t.Errorf("multipart source differs: got %d bytes, want %d", len(gotSource), len(sourceData))
		}
		if next, err := multipartReader.NextPart(); !errors.Is(err, io.EOF) || next != nil {
			t.Errorf("unexpected extra multipart part: part=%v err=%v", next, err)
		}

		response.Header().Set("Content-Type", "application/pdf; version=1.7")
		_, _ = response.Write(validConvertedPDF)
	}))
	defer server.Close()

	client, err := NewGotenbergClient(server.URL, server.Client(), int64(len(validConvertedPDF)))
	if err != nil {
		t.Fatalf("NewGotenbergClient() error = %v", err)
	}
	converted, err := client.Convert(context.Background(), sourcePath, `C:\fakepath\Quarterly Report.DOC`)
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if string(converted) != string(validConvertedPDF) {
		t.Fatalf("Convert() returned %q, want %q", converted, validConvertedPDF)
	}
}

func TestGotenbergClientClassifiesHTTPFailures(t *testing.T) {
	tests := []struct {
		status        int
		wantRetryable bool
	}{
		{status: http.StatusBadRequest},
		{status: http.StatusUnprocessableEntity},
		{status: http.StatusTooManyRequests, wantRetryable: true},
		{status: http.StatusInternalServerError, wantRetryable: true},
		{status: http.StatusBadGateway, wantRetryable: true},
		{status: http.StatusServiceUnavailable, wantRetryable: true},
	}

	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
			}))
			defer server.Close()
			client, err := NewGotenbergClient(server.URL, server.Client(), DefaultMaxConvertedPDFBytes)
			if err != nil {
				t.Fatalf("NewGotenbergClient() error = %v", err)
			}

			_, err = client.Convert(context.Background(), officeFixture(t), "report.doc")
			if err == nil {
				t.Fatal("Convert() expected an error")
			}
			if IsRetryable(err) != test.wantRetryable {
				t.Fatalf("IsRetryable(%v) = %v, want %v", err, IsRetryable(err), test.wantRetryable)
			}
			if IsPermanent(err) == test.wantRetryable {
				t.Fatalf("IsPermanent(%v) = %v, want %v", err, IsPermanent(err), !test.wantRetryable)
			}
			var conversionErr *ConversionError
			if !errors.As(err, &conversionErr) || conversionErr.StatusCode != test.status {
				t.Fatalf("conversion error = %#v, want status %d", conversionErr, test.status)
			}
		})
	}
}

func TestGotenbergClientDoesNotFollowRedirects(t *testing.T) {
	redirectTargetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		redirectTargetCalled = true
		response.Header().Set("Content-Type", "application/pdf")
		_, _ = response.Write(validConvertedPDF)
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Location", target.URL)
		response.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := NewGotenbergClient(server.URL, server.Client(), DefaultMaxConvertedPDFBytes)
	if err != nil {
		t.Fatalf("NewGotenbergClient() error = %v", err)
	}

	_, err = client.Convert(context.Background(), officeFixture(t), "report.doc")
	if err == nil || !IsPermanent(err) {
		t.Fatalf("Convert() error = %v, want permanent redirect error", err)
	}
	var conversionErr *ConversionError
	if !errors.As(err, &conversionErr) || conversionErr.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("conversion error = %#v, want HTTP %d", conversionErr, http.StatusTemporaryRedirect)
	}
	if redirectTargetCalled {
		t.Fatal("Gotenberg client followed a redirect and disclosed the source request")
	}
}

func TestGotenbergClientClassifiesTransportFailures(t *testing.T) {
	tests := []struct {
		name          string
		transportErr  error
		wantRetryable bool
	}{
		{name: "network error", transportErr: errors.New("connection refused"), wantRetryable: true},
		{name: "deadline", transportErr: context.DeadlineExceeded, wantRetryable: true},
		{name: "caller cancellation", transportErr: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, test.transportErr
			})}
			client, err := NewGotenbergClient("http://gotenberg.test", httpClient, DefaultMaxConvertedPDFBytes)
			if err != nil {
				t.Fatalf("NewGotenbergClient() error = %v", err)
			}
			_, err = client.Convert(context.Background(), officeFixture(t), "report.doc")
			if !errors.Is(err, test.transportErr) {
				t.Fatalf("Convert() error = %v, want wrapped %v", err, test.transportErr)
			}
			if IsRetryable(err) != test.wantRetryable {
				t.Fatalf("IsRetryable(%v) = %v, want %v", err, IsRetryable(err), test.wantRetryable)
			}
		})
	}
}

func TestGotenbergClientRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name          string
		contentType   string
		body          []byte
		maxSize       int64
		flushHeaders  bool
		wantError     error
		wantRetryable bool
	}{
		{
			name:          "wrong content type",
			contentType:   "text/plain",
			body:          validConvertedPDF,
			maxSize:       DefaultMaxConvertedPDFBytes,
			wantRetryable: true,
		},
		{
			name:          "invalid PDF signature",
			contentType:   "application/pdf",
			body:          []byte("not a PDF"),
			maxSize:       DefaultMaxConvertedPDFBytes,
			wantError:     ErrContentMismatch,
			wantRetryable: true,
		},
		{
			name:          "missing PDF EOF marker",
			contentType:   "application/pdf",
			body:          []byte("%PDF-1.7\nwithout trailer"),
			maxSize:       DefaultMaxConvertedPDFBytes,
			wantError:     ErrContentMismatch,
			wantRetryable: true,
		},
		{
			name:        "known oversized response",
			contentType: "application/pdf",
			body:        validConvertedPDF,
			maxSize:     8,
			wantError:   ErrConvertedPDFTooLarge,
		},
		{
			name:         "streamed oversized response",
			contentType:  "application/pdf",
			body:         validConvertedPDF,
			maxSize:      8,
			flushHeaders: true,
			wantError:    ErrConvertedPDFTooLarge,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", test.contentType)
				if test.flushHeaders {
					response.(http.Flusher).Flush()
				}
				_, _ = response.Write(test.body)
			}))
			defer server.Close()
			client, err := NewGotenbergClient(server.URL, server.Client(), test.maxSize)
			if err != nil {
				t.Fatalf("NewGotenbergClient() error = %v", err)
			}

			_, err = client.Convert(context.Background(), officeFixture(t), "report.doc")
			if err == nil {
				t.Fatal("Convert() expected an error")
			}
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("Convert() error = %v, want errors.Is(_, %v)", err, test.wantError)
			}
			if IsRetryable(err) != test.wantRetryable {
				t.Fatalf("IsRetryable(%v) = %v, want %v", err, IsRetryable(err), test.wantRetryable)
			}
		})
	}
}

func TestGotenbergClientRejectsInvalidInput(t *testing.T) {
	emptyPath := writeFixture(t, nil)
	oversizedPath := t.TempDir() + "/oversized"
	oversized, err := os.Create(oversizedPath)
	if err != nil {
		t.Fatalf("create oversized fixture: %v", err)
	}
	if err := oversized.Truncate(DefaultMaxSourceDocumentBytes + 1); err != nil {
		t.Fatalf("truncate oversized fixture: %v", err)
	}
	if err := oversized.Close(); err != nil {
		t.Fatalf("close oversized fixture: %v", err)
	}
	tests := []struct {
		name         string
		sourcePath   string
		originalName string
		wantError    error
	}{
		{name: "PDF needs no conversion", sourcePath: officeFixture(t), originalName: "report.pdf"},
		{name: "macro extension", sourcePath: officeFixture(t), originalName: "report.docm", wantError: ErrMacroEnabled},
		{name: "unsupported extension", sourcePath: officeFixture(t), originalName: "report.txt", wantError: ErrUnsupportedFormat},
		{name: "missing source", sourcePath: t.TempDir() + "/missing", originalName: "report.doc"},
		{name: "empty source", sourcePath: emptyPath, originalName: "report.doc", wantError: ErrEmptyFile},
		{name: "oversized source", sourcePath: oversizedPath, originalName: "report.doc", wantError: ErrSourceDocumentTooLarge},
		{name: "oversized filename", sourcePath: officeFixture(t), originalName: strings.Repeat("a", 252) + ".doc"},
	}

	client, err := NewGotenbergClient("http://gotenberg.test", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport must not be called for invalid input")
		return nil, nil
	})}, DefaultMaxConvertedPDFBytes)
	if err != nil {
		t.Fatalf("NewGotenbergClient() error = %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.Convert(context.Background(), test.sourcePath, test.originalName)
			if err == nil || !IsPermanent(err) {
				t.Fatalf("Convert() error = %v, want permanent error", err)
			}
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("Convert() error = %v, want errors.Is(_, %v)", err, test.wantError)
			}
		})
	}
}

func TestSafeMultipartFilename(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain", input: "report.docx", want: "report.docx"},
		{name: "browser fake path", input: `C:\fakepath\report.docx`, want: "report.docx"},
		{name: "Unix path", input: "/tmp/report.docx", want: "report.docx"},
		{name: "control character", input: "report\n.docx", wantErr: true},
		{name: "missing filename", input: "/", wantErr: true},
		{name: "missing extension", input: "report", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := safeMultipartFilename(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("safeMultipartFilename() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("safeMultipartFilename() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGotenbergClientUnconfigured(t *testing.T) {
	var client *GotenbergClient
	_, err := client.Convert(context.Background(), officeFixture(t), "report.doc")
	if err == nil || !IsPermanent(err) || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("Convert() error = %v, want permanent configuration error", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func officeFixture(t *testing.T) string {
	t.Helper()
	data := append(append([]byte(nil), oleCFBMagic...), []byte("office document")...)
	filePath := t.TempDir() + "/office"
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatalf("write office fixture: %v", err)
	}
	return filePath
}
