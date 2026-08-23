package docpreview

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizePDFWithEmptyPassword(t *testing.T) {
	sourcePath := writeFixture(t, []byte("encrypted PDF placeholder"))
	normalized := minimalTestPDF()
	normalizedPath := filepath.Join(t.TempDir(), "normalized.pdf")
	if err := os.WriteFile(normalizedPath, normalized, 0o600); err != nil {
		t.Fatalf("write normalized fixture: %v", err)
	}
	argumentsPath := filepath.Join(t.TempDir(), "arguments")
	executable := writeQPDFTestCommand(t, `
printf '%s\n' "$@" > "$QPDF_TEST_ARGUMENTS"
cat "$QPDF_TEST_OUTPUT"
`)
	t.Setenv("QPDF_TEST_ARGUMENTS", argumentsPath)
	t.Setenv("QPDF_TEST_OUTPUT", normalizedPath)

	var destination bytes.Buffer
	written, err := normalizePDFWithCommand(context.Background(), executable, sourcePath, &destination, int64(len(normalized)))
	if err != nil {
		t.Fatalf("normalizePDFWithCommand() error = %v", err)
	}
	if written != int64(len(normalized)) || !bytes.Equal(destination.Bytes(), normalized) {
		t.Fatalf("normalized output length = %d, want %d", written, len(normalized))
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatalf("read qpdf arguments: %v", err)
	}
	for _, expected := range []string{"--password=\n", "--suppress-password-recovery\n", "--decrypt\n", "--warning-exit-0\n", "--object-streams=generate\n", sourcePath + "\n", "-\n"} {
		if !bytes.Contains(arguments, []byte(expected)) {
			t.Errorf("qpdf arguments %q do not contain %q", arguments, expected)
		}
	}
}

func TestNormalizePDFWithEmptyPasswordRejectsPasswordAndOversizedOutput(t *testing.T) {
	sourcePath := writeFixture(t, []byte("encrypted PDF placeholder"))

	t.Run("password required", func(t *testing.T) {
		executable := writeQPDFTestCommand(t, `
echo 'invalid password' >&2
exit 2
`)
		_, err := normalizePDFWithCommand(context.Background(), executable, sourcePath, &bytes.Buffer{}, 1024)
		if !errors.Is(err, ErrPDFPasswordRequired) {
			t.Fatalf("normalizePDFWithCommand() error = %v, want ErrPDFPasswordRequired", err)
		}
	})

	t.Run("output limit", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "large.pdf")
		if err := os.WriteFile(outputPath, bytes.Repeat([]byte{'x'}, 33), 0o600); err != nil {
			t.Fatalf("write oversized fixture: %v", err)
		}
		executable := writeQPDFTestCommand(t, `cat "$QPDF_TEST_OUTPUT"`)
		t.Setenv("QPDF_TEST_OUTPUT", outputPath)
		written, err := normalizePDFWithCommand(context.Background(), executable, sourcePath, &bytes.Buffer{}, 32)
		if written != 33 || !errors.Is(err, ErrConvertedPDFTooLarge) {
			t.Fatalf("normalizePDFWithCommand() = (%d, %v), want (33, ErrConvertedPDFTooLarge)", written, err)
		}
	})
}

func TestNormalizePDFWithEmptyPasswordReportsMissingTool(t *testing.T) {
	sourcePath := writeFixture(t, []byte("encrypted PDF placeholder"))
	_, err := normalizePDFWithCommand(context.Background(), filepath.Join(t.TempDir(), "missing-qpdf"), sourcePath, &bytes.Buffer{}, 1024)
	if !errors.Is(err, ErrPDFNormalizerUnavailable) {
		t.Fatalf("normalizePDFWithCommand() error = %v, want ErrPDFNormalizerUnavailable", err)
	}
}

func writeQPDFTestCommand(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "qpdf-test")
	contents := []byte("#!/bin/sh\nset -eu\n" + body + "\n")
	if err := os.WriteFile(path, contents, 0o700); err != nil {
		t.Fatalf("write qpdf test command: %v", err)
	}
	return path
}
