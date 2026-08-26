package attachmentreview

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWritePreservesFilenameAndSeparatesDuplicateNames(t *testing.T) {
	rootDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source")
	contents := []byte("quarterly attachment")
	if err := os.WriteFile(sourcePath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	categories := []Category{
		{ID: 12, Name: "研究/报告"},
		{ID: 34, Name: "季度\\材料"},
	}

	firstPath, err := Write(rootDir, categories, "attachment-one", "季度报告.pdf", sourcePath)
	if err != nil {
		t.Fatalf("Write() first copy: %v", err)
	}
	secondPath, err := Write(rootDir, categories, "attachment-two", "季度报告.pdf", sourcePath)
	if err != nil {
		t.Fatalf("Write() second copy: %v", err)
	}
	if firstPath == secondPath {
		t.Fatal("duplicate original names must have distinct review paths")
	}
	thirdPath, err := Write(rootDir, categories, "attachment-three", "季度报告.pdf", sourcePath)
	if err != nil {
		t.Fatalf("Write() third copy: %v", err)
	}
	wantFirst := filepath.Join(rootDir, "12-研究_报告", "34-季度_材料", "季度报告.pdf")
	if firstPath != wantFirst {
		t.Fatalf("first path = %q, want %q", firstPath, wantFirst)
	}
	wantSecond := filepath.Join(rootDir, "12-研究_报告", "34-季度_材料", "季度报告_1.pdf")
	if secondPath != wantSecond {
		t.Fatalf("second path = %q, want %q", secondPath, wantSecond)
	}
	wantThird := filepath.Join(rootDir, "12-研究_报告", "34-季度_材料", "季度报告_2.pdf")
	if thirdPath != wantThird {
		t.Fatalf("third path = %q, want %q", thirdPath, wantThird)
	}
	for _, path := range []string{firstPath, secondPath, thirdPath} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read review copy %q: %v", path, readErr)
		}
		if string(got) != string(contents) {
			t.Fatalf("review copy %q = %q, want %q", path, got, contents)
		}
	}
}

func TestWriteRejectsUnsafeIdentifiersAndFilenames(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(sourcePath, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		attachmentID string
		filename     string
	}{
		{name: "attachment traversal", attachmentID: "../outside", filename: "report.pdf"},
		{name: "slash in filename", attachmentID: "safe-id", filename: "../report.pdf"},
		{name: "backslash in filename", attachmentID: "safe-id", filename: `folder\\report.pdf`},
		{name: "control in filename", attachmentID: "safe-id", filename: "report\n.pdf"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Write(t.TempDir(), []Category{{ID: 1, Name: "Reports"}}, test.attachmentID, test.filename, sourcePath)
			if err == nil {
				t.Fatal("Write() must reject unsafe path input")
			}
		})
	}
}

func TestWriteAllocatesConcurrentDuplicateSuffixes(t *testing.T) {
	rootDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(sourcePath, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	const copies = 12
	paths := make(chan string, copies)
	errorsByCopy := make(chan error, copies)
	var waitGroup sync.WaitGroup
	for index := range copies {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			path, err := Write(
				rootDir,
				[]Category{{ID: 1, Name: "Reports"}},
				fmt.Sprintf("attachment-%d", index),
				"report.pdf",
				sourcePath,
			)
			if err != nil {
				errorsByCopy <- err
				return
			}
			paths <- filepath.Base(path)
		}()
	}
	waitGroup.Wait()
	close(paths)
	close(errorsByCopy)
	for err := range errorsByCopy {
		t.Fatalf("concurrent Write(): %v", err)
	}

	seen := make(map[string]bool, copies)
	for path := range paths {
		if seen[path] {
			t.Fatalf("duplicate concurrent destination %q", path)
		}
		seen[path] = true
	}
	for sequence := range copies {
		want := duplicateFilename("report.pdf", sequence)
		if !seen[want] {
			t.Fatalf("missing concurrent destination %q; got %v", want, seen)
		}
	}
}

func TestWriteDoesNotFollowCategorySymlinkOutsideRoot(t *testing.T) {
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(rootDir, "1-Reports")); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(sourcePath, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(rootDir, []Category{{ID: 1, Name: "Reports"}}, "safe-id", "report.pdf", sourcePath); err == nil {
		t.Fatal("Write() must reject a category symlink that escapes the root")
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "report.pdf")); !os.IsNotExist(err) {
		t.Fatalf("file was written outside the review root: %v", err)
	}
}
