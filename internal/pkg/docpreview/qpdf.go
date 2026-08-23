package docpreview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
)

const maxQPDFErrorBytes = 64 << 10

var (
	ErrPDFPasswordRequired      = errors.New("PDF requires a password or uses unsupported encryption")
	ErrPDFNormalizerUnavailable = errors.New("PDF normalizer is unavailable")
)

// NormalizePDFWithEmptyPassword creates an unencrypted PDF preview when the
// source can be opened with an empty user password. The caller retains the
// original encrypted file for downloads.
func NormalizePDFWithEmptyPassword(ctx context.Context, sourcePath string, destination io.Writer, maxOutputBytes int64) (int64, error) {
	return normalizePDFWithCommand(ctx, "qpdf", sourcePath, destination, maxOutputBytes)
}

func normalizePDFWithCommand(ctx context.Context, executable, sourcePath string, destination io.Writer, maxOutputBytes int64) (int64, error) {
	if ctx == nil {
		return 0, errors.New("PDF normalization context is nil")
	}
	if strings.TrimSpace(executable) == "" {
		return 0, ErrPDFNormalizerUnavailable
	}
	if destination == nil {
		return 0, errors.New("PDF normalization destination is nil")
	}
	if maxOutputBytes <= 0 || maxOutputBytes == math.MaxInt64 {
		return 0, errors.New("PDF normalization output limit must be positive")
	}
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("stat encrypted PDF: %w", err)
	}
	if !stat.Mode().IsRegular() || stat.Size() == 0 {
		return 0, fmt.Errorf("%w: encrypted PDF source is not a non-empty regular file", ErrContentMismatch)
	}

	command := exec.CommandContext(
		ctx,
		executable,
		"--password=",
		"--suppress-password-recovery",
		"--decrypt",
		"--warning-exit-0",
		"--object-streams=generate",
		"--",
		sourcePath,
		"-",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("create qpdf output pipe: %w", err)
	}
	stderr := &cappedBuffer{remaining: maxQPDFErrorBytes}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%w: %v", ErrPDFNormalizerUnavailable, err)
		}
		return 0, fmt.Errorf("start qpdf: %w", err)
	}

	written, copyErr := io.Copy(destination, &io.LimitedReader{R: stdout, N: maxOutputBytes + 1})
	if copyErr != nil || written > maxOutputBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		if copyErr != nil {
			return written, fmt.Errorf("write normalized PDF: %w", copyErr)
		}
		return written, ErrConvertedPDFTooLarge
	}
	waitErr := command.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return written, fmt.Errorf("normalize PDF: %w", ctxErr)
	}
	if waitErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = waitErr.Error()
		}
		return written, fmt.Errorf("%w: %s", ErrPDFPasswordRequired, detail)
	}
	if written == 0 {
		return 0, fmt.Errorf("%w: qpdf produced an empty file", ErrContentMismatch)
	}
	return written, nil
}

type cappedBuffer struct {
	buffer    bytes.Buffer
	remaining int
}

func (w *cappedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	if w.remaining > 0 {
		writeLength := min(len(data), w.remaining)
		_, _ = w.buffer.Write(data[:writeLength])
		w.remaining -= writeLength
	}
	return originalLength, nil
}

func (w *cappedBuffer) String() string {
	return w.buffer.String()
}
