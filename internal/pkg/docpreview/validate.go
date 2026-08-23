// Package docpreview validates documents accepted for attachment previews and
// converts supported Office documents to PDF through Gotenberg.
package docpreview

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const (
	DefaultMaxZIPEntries           = 4096
	DefaultMaxZIPUncompressedBytes = int64(1 << 30)

	maxContentTypesBytes = int64(2 << 20)
)

var (
	ErrEmptyFile            = errors.New("document is empty")
	ErrUnsupportedFormat    = errors.New("unsupported document format")
	ErrMacroEnabled         = errors.New("macro-enabled documents are not allowed")
	ErrEncryptedDocument    = errors.New("encrypted or password-protected documents are not allowed")
	ErrContentMismatch      = errors.New("document content does not match its extension")
	ErrInvalidOLE           = errors.New("invalid OLE CFB document")
	ErrInvalidArchive       = errors.New("invalid OOXML archive")
	ErrArchiveLimitExceeded = errors.New("OOXML archive limit exceeded")
)

// Format identifies one of the document formats accepted for previews.
type Format string

const (
	FormatPDF  Format = "pdf"
	FormatDOC  Format = "doc"
	FormatDOCX Format = "docx"
	FormatXLS  Format = "xls"
	FormatXLSX Format = "xlsx"
	FormatPPT  Format = "ppt"
	FormatPPTX Format = "pptx"
)

// Info is the validated document metadata needed by the attachment service.
type Info struct {
	Format          Format
	Extension       string
	MIMEType        string
	NeedsConversion bool
}

// ZIPLimits caps work performed while validating an OOXML package. Both
// values must be positive when passed to ValidateWithLimits.
type ZIPLimits struct {
	MaxEntries           int
	MaxUncompressedBytes int64
}

var supportedFormats = map[string]Info{
	".pdf":  {Format: FormatPDF, Extension: ".pdf", MIMEType: "application/pdf"},
	".doc":  {Format: FormatDOC, Extension: ".doc", MIMEType: "application/msword", NeedsConversion: true},
	".docx": {Format: FormatDOCX, Extension: ".docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", NeedsConversion: true},
	".xls":  {Format: FormatXLS, Extension: ".xls", MIMEType: "application/vnd.ms-excel", NeedsConversion: true},
	".xlsx": {Format: FormatXLSX, Extension: ".xlsx", MIMEType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", NeedsConversion: true},
	".ppt":  {Format: FormatPPT, Extension: ".ppt", MIMEType: "application/vnd.ms-powerpoint", NeedsConversion: true},
	".pptx": {Format: FormatPPTX, Extension: ".pptx", MIMEType: "application/vnd.openxmlformats-officedocument.presentationml.presentation", NeedsConversion: true},
}

var macroExtensions = map[string]struct{}{
	".docb": {},
	".docm": {},
	".dotm": {},
	".xlsb": {},
	".xlam": {},
	".xlsm": {},
	".xltm": {},
	".potm": {},
	".ppam": {},
	".ppsm": {},
	".pptm": {},
	".sldm": {},
}

var oleCFBMagic = []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}

// SupportedExtensions returns a fresh slice containing the accepted filename
// extensions. Extension matching performed by Validate is case-insensitive.
func SupportedExtensions() []string {
	return []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx"}
}

// IsMacroEnabledExtension reports whether a filename or extension uses a known macro-capable
// Office extension. Upload handlers should apply this check before their
// configurable attachment allowlist so an administrator cannot enable macros
// accidentally.
func IsMacroEnabledExtension(originalName string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(originalName)))
	_, macro := macroExtensions[ext]
	return macro
}

// DefaultZIPLimits returns the limits used by Validate.
func DefaultZIPLimits() ZIPLimits {
	return ZIPLimits{
		MaxEntries:           DefaultMaxZIPEntries,
		MaxUncompressedBytes: DefaultMaxZIPUncompressedBytes,
	}
}

// Validate checks that the filename extension is supported and that the file
// contents match that format. OOXML archives are fully read to verify their
// entry sizes, CRCs, and required package metadata.
func Validate(filePath, originalName string) (Info, error) {
	return ValidateWithLimits(filePath, originalName, DefaultZIPLimits())
}

// ValidateWithLimits is Validate with caller-supplied OOXML resource limits.
func ValidateWithLimits(filePath, originalName string, limits ZIPLimits) (Info, error) {
	if limits.MaxEntries <= 0 || limits.MaxUncompressedBytes <= 0 || limits.MaxUncompressedBytes == math.MaxInt64 {
		return Info{}, fmt.Errorf("invalid OOXML limits: entries and bytes must be positive")
	}

	info, err := infoForFilename(originalName)
	if err != nil {
		return Info{}, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return Info{}, fmt.Errorf("open document: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return Info{}, fmt.Errorf("stat document: %w", err)
	}
	if !stat.Mode().IsRegular() {
		return Info{}, fmt.Errorf("%w: source is not a regular file", ErrContentMismatch)
	}
	if stat.Size() == 0 {
		return Info{}, ErrEmptyFile
	}

	switch info.Format {
	case FormatPDF:
		err = validatePDFReader(file, stat.Size())
	case FormatDOC, FormatXLS, FormatPPT:
		err = validateOLEReader(file, stat.Size(), info.Format)
	case FormatDOCX, FormatXLSX, FormatPPTX:
		err = validateOOXMLReader(file, stat.Size(), info.Format, limits)
	default:
		err = ErrUnsupportedFormat
	}
	if err != nil {
		return Info{}, err
	}
	return info, nil
}

func infoForFilename(originalName string) (Info, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(originalName)))
	if IsMacroEnabledExtension(originalName) {
		return Info{}, fmt.Errorf("%w: %s", ErrMacroEnabled, ext)
	}
	info, ok := supportedFormats[ext]
	if !ok {
		return Info{}, fmt.Errorf("%w: %s", ErrUnsupportedFormat, ext)
	}
	return info, nil
}

func validatePDFReader(reader io.ReaderAt, size int64) error {
	if size < 8 {
		return fmt.Errorf("%w: missing PDF header", ErrContentMismatch)
	}
	header := make([]byte, 8)
	if _, err := reader.ReadAt(header, 0); err != nil {
		return fmt.Errorf("read PDF header: %w", err)
	}
	if !validPDFHeader(header) {
		return fmt.Errorf("%w: invalid PDF header", ErrContentMismatch)
	}

	tailSize := min(size, int64(4096))
	tail := make([]byte, tailSize)
	if _, err := reader.ReadAt(tail, size-tailSize); err != nil {
		return fmt.Errorf("read PDF trailer: %w", err)
	}
	if !bytes.HasSuffix(bytes.TrimSpace(tail), []byte("%%EOF")) {
		return fmt.Errorf("%w: missing PDF EOF marker", ErrContentMismatch)
	}
	return validatePDFEncryption(reader, size)
}

func validatePDFBytes(data []byte) error {
	if len(data) < 8 || !validPDFHeader(data[:8]) {
		return fmt.Errorf("%w: invalid PDF header", ErrContentMismatch)
	}
	if !bytes.HasSuffix(bytes.TrimSpace(data), []byte("%%EOF")) {
		return fmt.Errorf("%w: missing PDF EOF marker", ErrContentMismatch)
	}
	return validatePDFEncryption(bytes.NewReader(data), int64(len(data)))
}

func validPDFHeader(header []byte) bool {
	return len(header) >= 8 &&
		bytes.Equal(header[:5], []byte("%PDF-")) &&
		header[5] >= '0' && header[5] <= '9' &&
		header[6] == '.' &&
		header[7] >= '0' && header[7] <= '9'
}

func validateOLEReader(reader io.ReaderAt, size int64, format Format) error {
	header := make([]byte, len(oleCFBMagic))
	if _, err := reader.ReadAt(header, 0); err != nil {
		return fmt.Errorf("%w: truncated OLE CFB header", ErrContentMismatch)
	}
	if !bytes.Equal(header, oleCFBMagic) {
		return fmt.Errorf("%w: invalid OLE CFB header", ErrContentMismatch)
	}
	document, err := parseCFB(reader, size)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOLE, err)
	}
	if err := document.validateOfficeDocument(format); err != nil {
		return err
	}
	return nil
}

type packageContentTypes struct {
	XMLName   xml.Name             `xml:"Types"`
	Defaults  []packageContentType `xml:"Default"`
	Overrides []packageContentType `xml:"Override"`
}

type packageContentType struct {
	PartName    string `xml:"PartName,attr"`
	ContentType string `xml:"ContentType,attr"`
}

type ooxmlExpectation struct {
	prefix          string
	mainContentType string
}

var ooxmlExpectations = map[Format]ooxmlExpectation{
	FormatDOCX: {
		prefix:          "word/",
		mainContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml",
	},
	FormatXLSX: {
		prefix:          "xl/",
		mainContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
	},
	FormatPPTX: {
		prefix:          "ppt/",
		mainContentType: "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml",
	},
}

func validateOOXMLReader(reader io.ReaderAt, size int64, format Format, limits ZIPLimits) error {
	if size >= int64(len(oleCFBMagic)) {
		header := make([]byte, len(oleCFBMagic))
		if _, err := reader.ReadAt(header, 0); err == nil && bytes.Equal(header, oleCFBMagic) {
			document, err := parseCFB(reader, size)
			if err != nil {
				return fmt.Errorf("%w: OOXML file contains an invalid OLE encryption container", ErrInvalidArchive)
			}
			if document.containsEncryptionStream() {
				return ErrEncryptedDocument
			}
			return fmt.Errorf("%w: OOXML extension contains an OLE document", ErrContentMismatch)
		}
	}
	archive, err := zip.NewReader(reader, size)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	if len(archive.File) > limits.MaxEntries {
		return fmt.Errorf("%w: %d entries exceeds %d", ErrArchiveLimitExceeded, len(archive.File), limits.MaxEntries)
	}

	names := make(map[string]struct{}, len(archive.File))
	var declaredTotal uint64
	for _, entry := range archive.File {
		if !validArchiveEntryName(entry.Name, entry.FileInfo().IsDir()) {
			return fmt.Errorf("%w: unsafe or non-canonical entry name %q", ErrInvalidArchive, entry.Name)
		}
		if _, duplicate := names[entry.Name]; duplicate {
			return fmt.Errorf("%w: duplicate entry %q", ErrInvalidArchive, entry.Name)
		}
		names[entry.Name] = struct{}{}
		if entry.Flags&0x1 != 0 {
			return fmt.Errorf("%w: encrypted entry %q", ErrInvalidArchive, entry.Name)
		}
		if !entry.FileInfo().IsDir() && entry.FileInfo().Mode()&os.ModeType != 0 {
			return fmt.Errorf("%w: special entry %q", ErrInvalidArchive, entry.Name)
		}
		remaining := uint64(limits.MaxUncompressedBytes) - declaredTotal
		if entry.UncompressedSize64 > remaining {
			return fmt.Errorf("%w: declared uncompressed size exceeds %d bytes", ErrArchiveLimitExceeded, limits.MaxUncompressedBytes)
		}
		declaredTotal += entry.UncompressedSize64
	}

	var contentTypesData []byte
	var actualTotal int64
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.Name == "[Content_Types].xml" && entry.UncompressedSize64 > uint64(maxContentTypesBytes) {
			return fmt.Errorf("%w: [Content_Types].xml exceeds %d bytes", ErrArchiveLimitExceeded, maxContentTypesBytes)
		}

		entryReader, err := entry.Open()
		if err != nil {
			return fmt.Errorf("%w: open %q: %v", ErrInvalidArchive, entry.Name, err)
		}
		remaining := limits.MaxUncompressedBytes - actualTotal
		limited := &io.LimitedReader{R: entryReader, N: remaining + 1}
		var destination io.Writer = io.Discard
		var contentTypes bytes.Buffer
		if entry.Name == "[Content_Types].xml" {
			destination = &contentTypes
		}
		readBytes, readErr := io.Copy(destination, limited)
		closeErr := entryReader.Close()
		if readBytes > remaining {
			return fmt.Errorf("%w: actual uncompressed size exceeds %d bytes", ErrArchiveLimitExceeded, limits.MaxUncompressedBytes)
		}
		if readErr != nil {
			return fmt.Errorf("%w: read %q: %v", ErrInvalidArchive, entry.Name, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("%w: close %q: %v", ErrInvalidArchive, entry.Name, closeErr)
		}
		if uint64(readBytes) != entry.UncompressedSize64 {
			return fmt.Errorf("%w: size mismatch for %q", ErrInvalidArchive, entry.Name)
		}
		actualTotal += readBytes
		if entry.Name == "[Content_Types].xml" {
			contentTypesData = append([]byte(nil), contentTypes.Bytes()...)
		}
	}

	if len(contentTypesData) == 0 {
		return fmt.Errorf("%w: missing [Content_Types].xml", ErrInvalidArchive)
	}
	return validateContentTypes(contentTypesData, names, format)
}

func validArchiveEntryName(name string, directory bool) bool {
	if name == "" || strings.Contains(name, "\\") || strings.ContainsRune(name, '\x00') || strings.HasPrefix(name, "/") {
		return false
	}
	trimmed := name
	if directory {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	if trimmed == "" || pathpkg.Clean(trimmed) != trimmed || trimmed == ".." || strings.HasPrefix(trimmed, "../") {
		return false
	}
	return true
}

func validateContentTypes(data []byte, names map[string]struct{}, format Format) error {
	var contentTypes packageContentTypes
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&contentTypes); err != nil {
		return fmt.Errorf("%w: parse [Content_Types].xml: %v", ErrInvalidArchive, err)
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: parse [Content_Types].xml trailer: %v", ErrInvalidArchive, err)
		}
		if text, ok := token.(xml.CharData); !ok || strings.TrimSpace(string(text)) != "" {
			return fmt.Errorf("%w: unexpected data after [Content_Types].xml root", ErrInvalidArchive)
		}
	}
	if contentTypes.XMLName.Space != "http://schemas.openxmlformats.org/package/2006/content-types" {
		return fmt.Errorf("%w: invalid [Content_Types].xml namespace", ErrInvalidArchive)
	}

	for _, contentType := range append(contentTypes.Defaults, contentTypes.Overrides...) {
		lower := strings.ToLower(contentType.ContentType)
		if strings.Contains(lower, "macroenabled") || strings.Contains(lower, "vbaproject") {
			return fmt.Errorf("%w: macro content type %q", ErrMacroEnabled, contentType.ContentType)
		}
	}
	for name := range names {
		if strings.HasSuffix(strings.ToLower(name), "/vbaproject.bin") {
			return fmt.Errorf("%w: macro project entry %q", ErrMacroEnabled, name)
		}
	}

	expectation, ok := ooxmlExpectations[format]
	if !ok {
		return ErrUnsupportedFormat
	}
	for _, override := range contentTypes.Overrides {
		if override.ContentType != expectation.mainContentType {
			continue
		}
		partName := strings.TrimPrefix(override.PartName, "/")
		if !strings.HasPrefix(partName, expectation.prefix) {
			continue
		}
		if _, exists := names[partName]; exists {
			return nil
		}
	}
	return fmt.Errorf("%w: missing %s main document part", ErrContentMismatch, format)
}
