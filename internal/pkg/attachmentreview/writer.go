package attachmentreview

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type Category struct {
	ID   int64
	Name string
}

func EnsureRoot(rootDir string) error {
	if strings.TrimSpace(rootDir) == "" {
		return nil
	}
	if !filepath.IsAbs(rootDir) {
		return errors.New("attachment review directory must be absolute")
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create attachment review directory: %w", err)
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return fmt.Errorf("open attachment review directory: %w", err)
	}
	return root.Close()
}

func Write(rootDir string, categories []Category, attachmentID, filename, sourcePath string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", nil
	}
	if len(categories) == 0 {
		return "", errors.New("attachment review category path is empty")
	}
	if err := validateAttachmentID(attachmentID); err != nil {
		return "", err
	}
	if err := validateFilename(filename); err != nil {
		return "", err
	}
	if err := EnsureRoot(rootDir); err != nil {
		return "", err
	}

	parts := make([]string, 0, len(categories))
	for _, category := range categories {
		if category.ID <= 0 {
			return "", errors.New("attachment review category ID must be positive")
		}
		parts = append(parts, categoryDirectoryName(category))
	}
	directory := filepath.Join(parts...)
	temporary := filepath.Join(directory, "."+attachmentID+".copying")

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return "", fmt.Errorf("open attachment review directory: %w", err)
	}
	defer root.Close()
	if err := root.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create attachment review category directory: %w", err)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open attachment review source: %w", err)
	}
	defer source.Close()

	destinationFile, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("create attachment review temporary file: %w", err)
	}
	removeTemporary := true
	defer func() {
		_ = destinationFile.Close()
		if removeTemporary {
			_ = root.Remove(temporary)
		}
	}()
	if _, err := io.Copy(destinationFile, source); err != nil {
		return "", fmt.Errorf("copy attachment review file: %w", err)
	}
	if err := destinationFile.Sync(); err != nil {
		return "", fmt.Errorf("sync attachment review file: %w", err)
	}
	if err := destinationFile.Close(); err != nil {
		return "", fmt.Errorf("close attachment review file: %w", err)
	}
	for sequence := 0; ; sequence++ {
		destination := filepath.Join(directory, duplicateFilename(filename, sequence))
		if err := root.Link(temporary, destination); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("publish attachment review file: %w", err)
		}
		if err := root.Remove(temporary); err != nil {
			return filepath.Join(rootDir, destination), fmt.Errorf("remove attachment review temporary file: %w", err)
		}
		removeTemporary = false
		return filepath.Join(rootDir, destination), nil
	}
}

func duplicateFilename(filename string, sequence int) string {
	if sequence <= 0 {
		return filename
	}
	extension := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, extension)
	return fmt.Sprintf("%s_%d%s", name, sequence, extension)
}

func categoryDirectoryName(category Category) string {
	name := strings.TrimSpace(category.Name)
	var builder strings.Builder
	for _, character := range name {
		switch {
		case character == '/' || character == '\\' || unicode.IsControl(character):
			builder.WriteByte('_')
		default:
			builder.WriteRune(character)
		}
	}
	name = strings.Trim(builder.String(), " .")
	if name == "" {
		return fmt.Sprintf("%d", category.ID)
	}
	return fmt.Sprintf("%d-%s", category.ID, name)
}

func validateAttachmentID(attachmentID string) error {
	if attachmentID == "" {
		return errors.New("attachment review attachment ID is empty")
	}
	for _, character := range attachmentID {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
			return errors.New("attachment review attachment ID is invalid")
		}
	}
	return nil
}

func validateFilename(filename string) error {
	if filename == "" || filename == "." || filename == ".." || filepath.Base(filename) != filename || strings.Contains(filename, "\\") {
		return errors.New("attachment review filename is invalid")
	}
	for _, character := range filename {
		if unicode.IsControl(character) {
			return errors.New("attachment review filename is invalid")
		}
	}
	return nil
}
