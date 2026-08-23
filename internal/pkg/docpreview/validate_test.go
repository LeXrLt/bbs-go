package docpreview

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

type archiveEntry struct {
	name   string
	data   []byte
	method uint16
	flags  uint16
}

func TestSupportedExtensions(t *testing.T) {
	want := []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx"}
	got := SupportedExtensions()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedExtensions() = %v, want %v", got, want)
	}
	got[0] = ".changed"
	if SupportedExtensions()[0] != ".pdf" {
		t.Fatal("SupportedExtensions must return a fresh slice")
	}
}

func TestIsMacroEnabledExtension(t *testing.T) {
	for _, filename := range []string{"report.docm", "REPORT.XLSM", "slides.pptm", "template.dotm", "addin.xlam", "binary.xlsb", "slide.sldm", ".DOCM"} {
		if !IsMacroEnabledExtension(filename) {
			t.Errorf("IsMacroEnabledExtension(%q) = false, want true", filename)
		}
	}
	for _, filename := range []string{"report.doc", "report.docx", "report.pdf", "archive.zip"} {
		if IsMacroEnabledExtension(filename) {
			t.Errorf("IsMacroEnabledExtension(%q) = true, want false", filename)
		}
	}
}

func TestValidateSupportedFormats(t *testing.T) {
	tests := []struct {
		name         string
		originalName string
		data         []byte
		wantFormat   Format
		wantMIME     string
		wantConvert  bool
	}{
		{
			name:         "PDF is case insensitive",
			originalName: "Report.PDF",
			data:         minimalTestPDF(),
			wantFormat:   FormatPDF,
			wantMIME:     "application/pdf",
		},
		{
			name:         "legacy Word",
			originalName: "report.doc",
			data:         validLegacyOffice(t, FormatDOC, legacyOptions{}),
			wantFormat:   FormatDOC,
			wantMIME:     "application/msword",
			wantConvert:  true,
		},
		{
			name:         "legacy Excel",
			originalName: "report.xls",
			data:         validLegacyOffice(t, FormatXLS, legacyOptions{}),
			wantFormat:   FormatXLS,
			wantMIME:     "application/vnd.ms-excel",
			wantConvert:  true,
		},
		{
			name:         "legacy PowerPoint",
			originalName: "report.ppt",
			data:         validLegacyOffice(t, FormatPPT, legacyOptions{}),
			wantFormat:   FormatPPT,
			wantMIME:     "application/vnd.ms-powerpoint",
			wantConvert:  true,
		},
		{
			name:         "OOXML Word",
			originalName: "report.docx",
			data:         validOOXML(t, FormatDOCX),
			wantFormat:   FormatDOCX,
			wantMIME:     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			wantConvert:  true,
		},
		{
			name:         "OOXML Excel",
			originalName: "report.xlsx",
			data:         validOOXML(t, FormatXLSX),
			wantFormat:   FormatXLSX,
			wantMIME:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			wantConvert:  true,
		},
		{
			name:         "OOXML PowerPoint",
			originalName: "report.pptx",
			data:         validOOXML(t, FormatPPTX),
			wantFormat:   FormatPPTX,
			wantMIME:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			wantConvert:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filePath := writeFixture(t, test.data)
			info, err := Validate(filePath, test.originalName)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if info.Format != test.wantFormat || info.MIMEType != test.wantMIME || info.NeedsConversion != test.wantConvert {
				t.Fatalf("Validate() = %+v, want format=%s MIME=%s conversion=%v", info, test.wantFormat, test.wantMIME, test.wantConvert)
			}
			if info.Extension != "."+string(test.wantFormat) {
				t.Fatalf("Validate() extension = %q, want %q", info.Extension, "."+string(test.wantFormat))
			}
		})
	}
}

func TestValidateLegacyOfficeTypeMacroAndEncryption(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		originalName string
		wantError    error
	}{
		{name: "Word renamed as Excel", data: validLegacyOffice(t, FormatDOC, legacyOptions{}), originalName: "report.xls", wantError: ErrContentMismatch},
		{name: "Excel renamed as PowerPoint", data: validLegacyOffice(t, FormatXLS, legacyOptions{}), originalName: "report.ppt", wantError: ErrContentMismatch},
		{name: "PowerPoint renamed as Word", data: validLegacyOffice(t, FormatPPT, legacyOptions{}), originalName: "report.doc", wantError: ErrContentMismatch},
		{name: "magic only is not a valid CFB file", data: append([]byte(nil), oleCFBMagic...), originalName: "report.doc", wantError: ErrInvalidOLE},
		{name: "VBA storage", data: validLegacyOffice(t, FormatDOC, legacyOptions{macroStorage: true}), originalName: "report.doc", wantError: ErrMacroEnabled},
		{name: "encrypted Word FIB", data: validLegacyOffice(t, FormatDOC, legacyOptions{formatEncryption: true}), originalName: "report.doc", wantError: ErrEncryptedDocument},
		{name: "Excel FILEPASS", data: validLegacyOffice(t, FormatXLS, legacyOptions{formatEncryption: true}), originalName: "report.xls", wantError: ErrEncryptedDocument},
		{name: "Excel macro sheet", data: validLegacyOffice(t, FormatXLS, legacyOptions{excelMacroSheet: true}), originalName: "report.xls", wantError: ErrMacroEnabled},
		{name: "encrypted PowerPoint stream", data: validLegacyOffice(t, FormatPPT, legacyOptions{formatEncryption: true}), originalName: "report.ppt", wantError: ErrEncryptedDocument},
		{name: "generic Office encryption stream", data: validLegacyOffice(t, FormatDOC, legacyOptions{genericEncryption: true}), originalName: "report.doc", wantError: ErrEncryptedDocument},
		{name: "encrypted OOXML OLE container", data: validLegacyOffice(t, FormatDOC, legacyOptions{genericEncryption: true}), originalName: "report.docx", wantError: ErrEncryptedDocument},
		{name: "invalid Word header", data: validLegacyOffice(t, FormatDOC, legacyOptions{invalidMainHeader: true}), originalName: "report.doc", wantError: ErrContentMismatch},
		{name: "invalid Excel header", data: validLegacyOffice(t, FormatXLS, legacyOptions{invalidMainHeader: true}), originalName: "report.xls", wantError: ErrContentMismatch},
		{name: "invalid PowerPoint header", data: validLegacyOffice(t, FormatPPT, legacyOptions{invalidMainHeader: true}), originalName: "report.ppt", wantError: ErrContentMismatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Validate(writeFixture(t, test.data), test.originalName)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Validate() error = %v, want errors.Is(_, %v)", err, test.wantError)
			}
		})
	}
}

func TestValidateLegacyOfficeMiniStreams(t *testing.T) {
	for _, format := range []Format{FormatDOC, FormatXLS, FormatPPT} {
		t.Run(string(format), func(t *testing.T) {
			info, err := Validate(writeFixture(t, validLegacyOfficeMini(t, format)), "report."+string(format))
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if info.Format != format {
				t.Fatalf("Validate() format = %s, want %s", info.Format, format)
			}
		})
	}
}

func TestValidateRejectsMalformedLegacyOLE(t *testing.T) {
	valid := validLegacyOffice(t, FormatDOC, legacyOptions{})
	const (
		headerSize         = 512
		directorySector    = headerSize
		primaryDirectory   = directorySector + cfbDirectorySize
		fatSector          = headerSize + (1+8)*512
		primaryStartOffset = primaryDirectory + 116
		primarySizeOffset  = primaryDirectory + 120
		primaryRightOffset = primaryDirectory + 72
	)
	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{name: "invalid byte order", mutate: func(data []byte) { binary.LittleEndian.PutUint16(data[28:30], 0) }},
		{name: "FAT chain cycle", mutate: func(data []byte) { binary.LittleEndian.PutUint32(data[fatSector+8*4:], 1) }},
		{name: "FAT sector marker", mutate: func(data []byte) { binary.LittleEndian.PutUint32(data[fatSector+9*4:], cfbFreeSector) }},
		{name: "directory cycle", mutate: func(data []byte) { binary.LittleEndian.PutUint32(data[primaryRightOffset:], 1) }},
		{name: "stream overlaps directory", mutate: func(data []byte) { binary.LittleEndian.PutUint32(data[primaryStartOffset:], 0) }},
		{name: "stream size exceeds file", mutate: func(data []byte) { binary.LittleEndian.PutUint64(data[primarySizeOffset:], 1<<20) }},
		{name: "orphaned directory entry", mutate: func(data []byte) {
			writeCFBDirectoryEntry(data[directorySector+2*cfbDirectorySize:directorySector+3*cfbDirectorySize], "Orphan", 1, cfbNoStream, cfbNoStream, cfbNoStream, cfbEndOfChain, 0)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := append([]byte(nil), valid...)
			test.mutate(data)
			_, err := Validate(writeFixture(t, data), "report.doc")
			if !errors.Is(err, ErrInvalidOLE) {
				t.Fatalf("Validate() error = %v, want errors.Is(_, ErrInvalidOLE)", err)
			}
		})
	}
}

func TestValidateRejectsFilenameAndSignatureProblems(t *testing.T) {
	validPDF := writeFixture(t, minimalTestPDF())
	empty := writeFixture(t, nil)

	tests := []struct {
		name         string
		filePath     string
		originalName string
		wantError    error
	}{
		{name: "empty file", filePath: empty, originalName: "empty.pdf", wantError: ErrEmptyFile},
		{name: "no extension", filePath: validPDF, originalName: "report", wantError: ErrUnsupportedFormat},
		{name: "unsupported extension", filePath: validPDF, originalName: "report.txt", wantError: ErrUnsupportedFormat},
		{name: "macro Word extension", filePath: validPDF, originalName: "report.docm", wantError: ErrMacroEnabled},
		{name: "macro Excel extension", filePath: validPDF, originalName: "report.xlsm", wantError: ErrMacroEnabled},
		{name: "macro PowerPoint extension", filePath: validPDF, originalName: "report.pptm", wantError: ErrMacroEnabled},
		{name: "PDF renamed as Word", filePath: validPDF, originalName: "report.doc", wantError: ErrContentMismatch},
		{name: "truncated OLE magic", filePath: writeFixture(t, oleCFBMagic[:4]), originalName: "report.xls", wantError: ErrContentMismatch},
		{name: "bad PDF header", filePath: writeFixture(t, []byte("not-pdf\n%%EOF")), originalName: "report.pdf", wantError: ErrContentMismatch},
		{name: "missing PDF EOF", filePath: writeFixture(t, []byte("%PDF-1.7\nbody")), originalName: "report.pdf", wantError: ErrContentMismatch},
		{name: "data after PDF EOF", filePath: writeFixture(t, []byte("%PDF-1.7\n%%EOF\ndata")), originalName: "report.pdf", wantError: ErrContentMismatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Validate(test.filePath, test.originalName)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Validate() error = %v, want errors.Is(_, %v)", err, test.wantError)
			}
		})
	}
}

func TestValidateRejectsEncryptedPDF(t *testing.T) {
	plain, _ := buildClassicTestPDF("", "")
	encrypted, _ := buildClassicTestPDF(" /Encrypt 2 0 R", "")
	escaped, _ := buildClassicTestPDF(" /Encr#79pt 2 0 R", "")
	inlineEncrypted := bytes.Replace(append([]byte(nil), encrypted...), []byte("trailer\n<<"), []byte("trailer <<"), 1)
	xrefStreamEncrypted := buildXrefStreamTestPDF(" /Encr#79pt 2 0 R")
	hybridXrefEncrypted := buildHybridXrefTestPDF(" /Encr#79pt 1 0 R")
	incremental := buildIncrementalTestPDF(t)

	for _, test := range []struct {
		name      string
		data      []byte
		wantError error
	}{
		{name: "plain classic xref", data: plain},
		{name: "classic trailer Encrypt", data: encrypted, wantError: ErrEncryptedDocument},
		{name: "inline classic trailer Encrypt", data: inlineEncrypted, wantError: ErrEncryptedDocument},
		{name: "escaped Encrypt name", data: escaped, wantError: ErrEncryptedDocument},
		{name: "xref stream Encrypt", data: xrefStreamEncrypted, wantError: ErrEncryptedDocument},
		{name: "hybrid XRefStm Encrypt", data: hybridXrefEncrypted, wantError: ErrEncryptedDocument},
		{name: "Encrypt inherited through Prev", data: incremental, wantError: ErrEncryptedDocument},
		{name: "plain xref stream", data: buildXrefStreamTestPDF("")},
		{name: "plain hybrid XRefStm", data: buildHybridXrefTestPDF("")},
		{name: "two-revision hybrid with split Prev chains", data: buildTwoRevisionHybridXrefTestPDF()},
		{name: "incremental Root inherited through Prev", data: buildIncrementalWithoutLatestRootTestPDF(t)},
		{name: "Encrypt inside string is not a key", data: mustClassicTestPDF("", " /Note (/Encrypt 2 0 R)")},
		{name: "nested Encrypt is not trailer encryption", data: mustClassicTestPDF(" /Info << /Encrypt 2 0 R >>", "")},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Validate(writeFixture(t, test.data), "report.pdf")
			if test.wantError == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Validate() error = %v, want errors.Is(_, %v)", err, test.wantError)
			}
		})
	}
}

func TestValidateRejectsMalformedPDFXref(t *testing.T) {
	validEntry := "0000000000 65535 f \n"
	inlineTrailer := bytes.Replace(
		buildCustomClassicTestPDF("0 1\n"+validEntry+"1 1\n{OBJECT_OFFSET} 00000 n \n", "/Size 2 /Root 1 0 R"),
		[]byte("trailer\n<<"),
		[]byte("trailer <<"),
		1,
	)
	longInlineTrailer, _ := buildClassicTestPDF(" /Padding ("+strings.Repeat("a", 256)+")", "")
	longInlineTrailer = bytes.Replace(longInlineTrailer, []byte("trailer\n<<"), []byte("trailer <<"), 1)
	badStreamEntry := buildXrefStreamTestPDF("")
	streamStart := bytes.Index(badStreamEntry, []byte("stream\n")) + len("stream\n")
	if streamStart < len("stream\n") {
		t.Fatal("xref stream fixture has no stream data")
	}
	badStreamEntry[streamStart+7] = 3
	badStreamOffset := append([]byte(nil), buildXrefStreamTestPDF("")...)
	streamStart = bytes.Index(badStreamOffset, []byte("stream\n")) + len("stream\n")
	for index := 8; index < 12; index++ {
		badStreamOffset[streamStart+index] = 0
	}
	validWithMalformedLatestStartXref := append(append([]byte(nil), minimalTestPDF()...), []byte("\nstartxref\nnot-a-number\n%%EOF\n")...)
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "multiple valid subsections", data: buildCustomClassicTestPDF("0 1\n"+validEntry+"1 1\n{OBJECT_OFFSET} 00000 n \n", "/Size 2 /Root 1 0 R")},
		{name: "valid CRLF entries", data: buildCustomClassicTestPDF("0 2\r\n0000000000 65535 f \r\n{OBJECT_OFFSET} 00000 n \r\n", "/Size 2 /Root 1 0 R")},
		{name: "valid inline trailer dictionary", data: inlineTrailer},
		{name: "valid long inline trailer dictionary", data: longInlineTrailer},
		{name: "arbitrary token before trailer", data: buildCustomClassicTestPDF("garbage\n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "missing subsection count", data: buildCustomClassicTestPDF("0\n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "zero subsection count", data: buildCustomClassicTestPDF("0 0\n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "truncated subsection", data: buildCustomClassicTestPDF("0 2\n"+validEntry, "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "short entry offset", data: buildCustomClassicTestPDF("0 1\n000000000 65535 f \n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "invalid object zero entry", data: buildCustomClassicTestPDF("0 1\n0000000000 00000 n \n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "bad entry status", data: buildCustomClassicTestPDF("0 1\n0000000000 65535 x \n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "in-use offset outside file", data: buildCustomClassicTestPDF("1 1\n9999999999 00000 n \n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "in-use offset points to wrong object", data: buildCustomClassicTestPDF("0 1\n"+validEntry+"1 1\n0000000000 00000 n \n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "in-use generation mismatch", data: buildCustomClassicTestPDF("0 1\n"+validEntry+"1 1\n{OBJECT_OFFSET} 00001 n \n", "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "overlapping subsections", data: buildCustomClassicTestPDF("0 1\n"+validEntry+"0 1\n"+validEntry, "/Size 2 /Root 1 0 R"), wantErr: true},
		{name: "subsection exceeds Size", data: buildCustomClassicTestPDF("0 2\n"+validEntry+"{OBJECT_OFFSET} 00000 n \n", "/Size 1 /Root 1 0 R"), wantErr: true},
		{name: "missing Size", data: buildCustomClassicTestPDF("0 1\n"+validEntry, "/Root 1 0 R"), wantErr: true},
		{name: "missing Root", data: buildCustomClassicTestPDF("0 1\n"+validEntry, "/Size 2"), wantErr: true},
		{name: "direct Root dictionary", data: buildCustomClassicTestPDF("0 1\n"+validEntry, "/Size 2 /Root << /Type /Catalog >>"), wantErr: true},
		{name: "Root outside Size", data: buildCustomClassicTestPDF("0 1\n"+validEntry, "/Size 2 /Root 2 0 R"), wantErr: true},
		{name: "classic Root is not Catalog", data: bytes.Replace(minimalTestPDF(), []byte("/Type /Catalog"), []byte("/Type /Pages"), 1), wantErr: true},
		{name: "malformed final startxref does not fall back", data: validWithMalformedLatestStartXref, wantErr: true},
		{name: "xref stream missing Size", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte(" /Size 3"), nil, 1), wantErr: true},
		{name: "xref stream missing Root", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte(" /Root 2 0 R"), nil, 1), wantErr: true},
		{name: "xref dictionary without stream", data: []byte("%PDF-1.7\n1 0 obj\n<< /Type /XRef /Size 2 /Root 1 0 R >>\nstartxref\n9\n%%EOF\n"), wantErr: true},
		{name: "xref stream missing W", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte(" /W [1 4 2]"), nil, 1), wantErr: true},
		{name: "xref stream bad W width", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("/W [1 4 2]"), []byte("/W [1 9 2]"), 1), wantErr: true},
		{name: "xref stream bad Length", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("/Length 21"), []byte("/Length 20"), 1), wantErr: true},
		{name: "xref stream indirect Length", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("/Length 21"), []byte("/Length 21 0 R"), 1), wantErr: true},
		{name: "xref stream bad entry type", data: badStreamEntry, wantErr: true},
		{name: "xref stream entry offset mismatch", data: badStreamOffset, wantErr: true},
		{name: "xref stream Root self reference", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("/Root 2 0 R"), []byte("/Root 1 0 R"), 1), wantErr: true},
		{name: "xref stream Root is not Catalog", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("/Type /Catalog"), []byte("/Type /Pages"), 1), wantErr: true},
		{name: "xref stream missing endstream", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("endstream"), []byte("notstream"), 1), wantErr: true},
		{name: "xref stream missing endobj", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("endobj\nstartxref"), []byte("broken\nstartxref"), 1), wantErr: true},
		{name: "valid direct endstream boundary", data: bytes.Replace(buildXrefStreamTestPDF(""), []byte("\nendstream"), []byte("endstream"), 1)},
		{name: "valid Flate predictor xref stream", data: buildFlateXrefStreamTestPDF(t)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Validate(writeFixture(t, test.data), "report.pdf")
			if test.wantErr {
				if !errors.Is(err, ErrContentMismatch) {
					t.Fatalf("Validate() error = %v, want ErrContentMismatch", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidOOXML(t *testing.T) {
	docxExpectation := ooxmlExpectations[FormatDOCX]
	validTypes := contentTypesXML("/word/document.xml", docxExpectation.mainContentType)

	tests := []struct {
		name      string
		data      []byte
		wantError error
	}{
		{
			name:      "not a ZIP archive",
			data:      []byte("PK but not a zip"),
			wantError: ErrInvalidArchive,
		},
		{
			name:      "missing content types",
			data:      makeArchive(t, archiveEntry{name: "word/document.xml", data: []byte("document")}),
			wantError: ErrInvalidArchive,
		},
		{
			name: "malformed content types XML",
			data: makeArchive(t,
				archiveEntry{name: "[Content_Types].xml", data: []byte("<Types>")},
				archiveEntry{name: "word/document.xml", data: []byte("document")},
			),
			wantError: ErrInvalidArchive,
		},
		{
			name: "wrong content types namespace",
			data: makeArchive(t,
				archiveEntry{name: "[Content_Types].xml", data: []byte(strings.Replace(validTypes, "http://schemas.openxmlformats.org/package/2006/content-types", "urn:wrong", 1))},
				archiveEntry{name: "word/document.xml", data: []byte("document")},
			),
			wantError: ErrInvalidArchive,
		},
		{
			name:      "main part is absent",
			data:      makeArchive(t, archiveEntry{name: "[Content_Types].xml", data: []byte(validTypes)}),
			wantError: ErrContentMismatch,
		},
		{
			name:      "spreadsheet renamed as docx",
			data:      validOOXML(t, FormatXLSX),
			wantError: ErrContentMismatch,
		},
		{
			name: "macro main content type",
			data: makeArchive(t,
				archiveEntry{name: "[Content_Types].xml", data: []byte(contentTypesXML("/word/document.xml", "application/vnd.ms-word.document.macroEnabled.main+xml"))},
				archiveEntry{name: "word/document.xml", data: []byte("document")},
			),
			wantError: ErrMacroEnabled,
		},
		{
			name: "embedded VBA project",
			data: makeArchive(t,
				archiveEntry{name: "[Content_Types].xml", data: []byte(validTypes)},
				archiveEntry{name: "word/document.xml", data: []byte("document")},
				archiveEntry{name: "word/vbaProject.bin", data: []byte("macro")},
			),
			wantError: ErrMacroEnabled,
		},
		{
			name: "path traversal entry",
			data: makeArchive(t,
				archiveEntry{name: "[Content_Types].xml", data: []byte(validTypes)},
				archiveEntry{name: "word/document.xml", data: []byte("document")},
				archiveEntry{name: "../outside", data: []byte("bad")},
			),
			wantError: ErrInvalidArchive,
		},
		{
			name: "duplicate entry",
			data: makeArchive(t,
				archiveEntry{name: "[Content_Types].xml", data: []byte(validTypes)},
				archiveEntry{name: "word/document.xml", data: []byte("first")},
				archiveEntry{name: "word/document.xml", data: []byte("second")},
			),
			wantError: ErrInvalidArchive,
		},
		{
			name: "encrypted entry flag",
			data: makeArchive(t,
				archiveEntry{name: "[Content_Types].xml", data: []byte(validTypes), flags: 0x1},
				archiveEntry{name: "word/document.xml", data: []byte("document")},
			),
			wantError: ErrInvalidArchive,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Validate(writeFixture(t, test.data), "report.docx")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Validate() error = %v, want errors.Is(_, %v)", err, test.wantError)
			}
		})
	}
}

func TestValidateOOXMLLimits(t *testing.T) {
	valid := validOOXML(t, FormatDOCX)

	tests := []struct {
		name   string
		limits ZIPLimits
		want   error
	}{
		{
			name:   "entry count",
			limits: ZIPLimits{MaxEntries: 1, MaxUncompressedBytes: 1 << 20},
			want:   ErrArchiveLimitExceeded,
		},
		{
			name:   "declared uncompressed bytes",
			limits: ZIPLimits{MaxEntries: 10, MaxUncompressedBytes: 8},
			want:   ErrArchiveLimitExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateWithLimits(writeFixture(t, valid), "report.docx", test.limits)
			if !errors.Is(err, test.want) {
				t.Fatalf("ValidateWithLimits() error = %v, want errors.Is(_, %v)", err, test.want)
			}
		})
	}
}

func TestValidateOOXMLChecksCRC(t *testing.T) {
	data := makeArchive(t,
		archiveEntry{name: "[Content_Types].xml", data: []byte(contentTypesXML("/word/document.xml", ooxmlExpectations[FormatDOCX].mainContentType)), method: zip.Store},
		archiveEntry{name: "word/document.xml", data: []byte("unique-document-payload"), method: zip.Store},
	)
	index := bytes.Index(data, []byte("unique-document-payload"))
	if index < 0 {
		t.Fatal("test fixture payload was not stored verbatim")
	}
	data[index] ^= 0xff

	_, err := Validate(writeFixture(t, data), "report.docx")
	if !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("Validate() error = %v, want CRC failure wrapped by ErrInvalidArchive", err)
	}
}

func TestValidateWithLimitsRejectsInvalidConfiguration(t *testing.T) {
	filePath := writeFixture(t, validOOXML(t, FormatDOCX))
	for _, limits := range []ZIPLimits{
		{},
		{MaxEntries: -1, MaxUncompressedBytes: 1},
		{MaxEntries: 1, MaxUncompressedBytes: -1},
	} {
		if _, err := ValidateWithLimits(filePath, "report.docx", limits); err == nil {
			t.Fatalf("ValidateWithLimits(%+v) expected an error", limits)
		}
	}
}

func validOOXML(t *testing.T, format Format) []byte {
	t.Helper()
	expectation := ooxmlExpectations[format]
	mainPart := expectation.prefix + map[Format]string{
		FormatDOCX: "document.xml",
		FormatXLSX: "workbook.xml",
		FormatPPTX: "presentation.xml",
	}[format]
	return makeArchive(t,
		archiveEntry{name: "[Content_Types].xml", data: []byte(contentTypesXML("/"+mainPart, expectation.mainContentType))},
		archiveEntry{name: mainPart, data: []byte("document")},
	)
}

func contentTypesXML(partName, contentType string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Override PartName="%s" ContentType="%s"/>
</Types>`, partName, contentType)
}

func makeArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: entry.method, Flags: entry.flags}
		part, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create ZIP entry %q: %v", entry.name, err)
		}
		if _, err := part.Write(entry.data); err != nil {
			t.Fatalf("write ZIP entry %q: %v", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP: %v", err)
	}
	return data.Bytes()
}

func writeFixture(t *testing.T, data []byte) string {
	t.Helper()
	filePath := t.TempDir() + "/document"
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return filePath
}

type legacyOptions struct {
	macroStorage      bool
	formatEncryption  bool
	excelMacroSheet   bool
	genericEncryption bool
	invalidMainHeader bool
}

func validLegacyOffice(t *testing.T, format Format, options legacyOptions) []byte {
	t.Helper()
	const sectorSize = 512
	const streamSectorCount = 8
	const fatSectorID = 1 + streamSectorCount

	header := make([]byte, sectorSize)
	copy(header, oleCFBMagic)
	binary.LittleEndian.PutUint16(header[24:26], 0x003e)
	binary.LittleEndian.PutUint16(header[26:28], 3)
	binary.LittleEndian.PutUint16(header[28:30], 0xfffe)
	binary.LittleEndian.PutUint16(header[30:32], 9)
	binary.LittleEndian.PutUint16(header[32:34], 6)
	binary.LittleEndian.PutUint32(header[44:48], 1)
	binary.LittleEndian.PutUint32(header[48:52], 0)
	binary.LittleEndian.PutUint32(header[56:60], uint32(cfbMiniStreamCutoff))
	binary.LittleEndian.PutUint32(header[60:64], cfbEndOfChain)
	binary.LittleEndian.PutUint32(header[68:72], cfbEndOfChain)
	for offset := 76; offset < len(header); offset += 4 {
		binary.LittleEndian.PutUint32(header[offset:], cfbFreeSector)
	}
	binary.LittleEndian.PutUint32(header[76:80], fatSectorID)

	directory := make([]byte, sectorSize)
	writeCFBDirectoryEntry(directory[:cfbDirectorySize], "Root Entry", 5, cfbNoStream, cfbNoStream, 1, cfbEndOfChain, 0)
	mainName := map[Format]string{
		FormatDOC: "WordDocument",
		FormatXLS: "Workbook",
		FormatPPT: "PowerPoint Document",
	}[format]
	extraName := ""
	extraType := byte(0)
	if options.macroStorage {
		extraName, extraType = "VBA", 1
	} else if options.genericEncryption {
		extraName, extraType = "EncryptionInfo", 2
	} else if format == FormatPPT && options.formatEncryption {
		extraName, extraType = "EncryptedSummary", 2
	}
	rightSibling := cfbNoStream
	if extraType != 0 {
		rightSibling = 2
	}
	writeCFBDirectoryEntry(directory[cfbDirectorySize:2*cfbDirectorySize], mainName, 2, cfbNoStream, rightSibling, cfbNoStream, 1, cfbMiniStreamCutoff)
	if extraType != 0 {
		writeCFBDirectoryEntry(directory[2*cfbDirectorySize:3*cfbDirectorySize], extraName, extraType, cfbNoStream, cfbNoStream, cfbNoStream, cfbEndOfChain, 0)
	}

	mainStream := legacyMainStreamData(format, options)

	fat := make([]byte, sectorSize)
	for offset := 0; offset < len(fat); offset += 4 {
		binary.LittleEndian.PutUint32(fat[offset:], cfbFreeSector)
	}
	binary.LittleEndian.PutUint32(fat[0:4], cfbEndOfChain)
	for sector := uint32(1); sector < 1+streamSectorCount; sector++ {
		next := sector + 1
		if sector == streamSectorCount {
			next = cfbEndOfChain
		}
		binary.LittleEndian.PutUint32(fat[sector*4:], next)
	}
	binary.LittleEndian.PutUint32(fat[fatSectorID*4:], cfbFATSector)

	var file bytes.Buffer
	file.Write(header)
	file.Write(directory)
	file.Write(mainStream)
	file.Write(fat)
	return file.Bytes()
}

func validLegacyOfficeMini(t *testing.T, format Format) []byte {
	t.Helper()
	const sectorSize = 512
	const directorySectorID = 0
	const rootMiniStreamSectorID = 1
	const miniFATSectorID = 2
	const fatSectorID = 3

	mainData := legacyMainStreamData(format, legacyOptions{})
	mainData = mainData[:map[Format]int{FormatDOC: 12, FormatXLS: 12, FormatPPT: 8}[format]]

	header := make([]byte, sectorSize)
	copy(header, oleCFBMagic)
	binary.LittleEndian.PutUint16(header[24:26], 0x003e)
	binary.LittleEndian.PutUint16(header[26:28], 3)
	binary.LittleEndian.PutUint16(header[28:30], 0xfffe)
	binary.LittleEndian.PutUint16(header[30:32], 9)
	binary.LittleEndian.PutUint16(header[32:34], 6)
	binary.LittleEndian.PutUint32(header[44:48], 1)
	binary.LittleEndian.PutUint32(header[48:52], directorySectorID)
	binary.LittleEndian.PutUint32(header[56:60], uint32(cfbMiniStreamCutoff))
	binary.LittleEndian.PutUint32(header[60:64], miniFATSectorID)
	binary.LittleEndian.PutUint32(header[64:68], 1)
	binary.LittleEndian.PutUint32(header[68:72], cfbEndOfChain)
	for offset := 76; offset < len(header); offset += 4 {
		binary.LittleEndian.PutUint32(header[offset:], cfbFreeSector)
	}
	binary.LittleEndian.PutUint32(header[76:80], fatSectorID)

	directory := make([]byte, sectorSize)
	writeCFBDirectoryEntry(directory[:cfbDirectorySize], "Root Entry", 5, cfbNoStream, cfbNoStream, 1, rootMiniStreamSectorID, sectorSize)
	mainName := map[Format]string{FormatDOC: "WordDocument", FormatXLS: "Workbook", FormatPPT: "PowerPoint Document"}[format]
	writeCFBDirectoryEntry(directory[cfbDirectorySize:2*cfbDirectorySize], mainName, 2, cfbNoStream, cfbNoStream, cfbNoStream, 0, uint64(len(mainData)))

	rootMiniStream := make([]byte, sectorSize)
	copy(rootMiniStream, mainData)
	miniFAT := make([]byte, sectorSize)
	for offset := 0; offset < len(miniFAT); offset += 4 {
		binary.LittleEndian.PutUint32(miniFAT[offset:], cfbFreeSector)
	}
	binary.LittleEndian.PutUint32(miniFAT[:4], cfbEndOfChain)
	fat := make([]byte, sectorSize)
	for offset := 0; offset < len(fat); offset += 4 {
		binary.LittleEndian.PutUint32(fat[offset:], cfbFreeSector)
	}
	binary.LittleEndian.PutUint32(fat[directorySectorID*4:], cfbEndOfChain)
	binary.LittleEndian.PutUint32(fat[rootMiniStreamSectorID*4:], cfbEndOfChain)
	binary.LittleEndian.PutUint32(fat[miniFATSectorID*4:], cfbEndOfChain)
	binary.LittleEndian.PutUint32(fat[fatSectorID*4:], cfbFATSector)

	var file bytes.Buffer
	file.Write(header)
	file.Write(directory)
	file.Write(rootMiniStream)
	file.Write(miniFAT)
	file.Write(fat)
	return file.Bytes()
}

func legacyMainStreamData(format Format, options legacyOptions) []byte {
	mainStream := make([]byte, cfbMiniStreamCutoff)
	switch format {
	case FormatDOC:
		if !options.invalidMainHeader {
			binary.LittleEndian.PutUint16(mainStream[:2], 0xa5ec)
		}
		if options.formatEncryption {
			binary.LittleEndian.PutUint16(mainStream[10:12], 1<<8)
		}
	case FormatXLS:
		position := 0
		if !options.invalidMainHeader {
			position = appendBIFFRecord(mainStream, position, 0x0809, []byte{0x00, 0x06, 0x05, 0x00})
		}
		if options.formatEncryption {
			position = appendBIFFRecord(mainStream, position, 0x002f, []byte{0x01, 0x00})
		}
		if options.excelMacroSheet {
			position = appendBIFFRecord(mainStream, position, 0x0809, []byte{0x00, 0x06, 0x40, 0x00})
		}
		appendBIFFRecord(mainStream, position, 0x000a, nil)
	case FormatPPT:
		if !options.invalidMainHeader {
			binary.LittleEndian.PutUint16(mainStream[:2], 0x000f)
			binary.LittleEndian.PutUint16(mainStream[2:4], 0x03e8)
		}
	}
	return mainStream
}

func writeCFBDirectoryEntry(destination []byte, name string, typ byte, left, right, child, start uint32, size uint64) {
	encoded := utf16.Encode([]rune(name + "\x00"))
	for index, unit := range encoded {
		binary.LittleEndian.PutUint16(destination[index*2:], unit)
	}
	binary.LittleEndian.PutUint16(destination[64:66], uint16(len(encoded)*2))
	destination[66] = typ
	destination[67] = 1
	binary.LittleEndian.PutUint32(destination[68:72], left)
	binary.LittleEndian.PutUint32(destination[72:76], right)
	binary.LittleEndian.PutUint32(destination[76:80], child)
	binary.LittleEndian.PutUint32(destination[116:120], start)
	binary.LittleEndian.PutUint64(destination[120:128], size)
}

func appendBIFFRecord(destination []byte, position int, recordType uint16, payload []byte) int {
	binary.LittleEndian.PutUint16(destination[position:position+2], recordType)
	binary.LittleEndian.PutUint16(destination[position+2:position+4], uint16(len(payload)))
	copy(destination[position+4:], payload)
	return position + 4 + len(payload)
}

func minimalTestPDF() []byte {
	data, _ := buildClassicTestPDF("", "")
	return data
}

func mustClassicTestPDF(trailerExtra, catalogExtra string) []byte {
	data, _ := buildClassicTestPDF(trailerExtra, catalogExtra)
	return data
}

func buildClassicTestPDF(trailerExtra, catalogExtra string) ([]byte, int64) {
	var document bytes.Buffer
	document.WriteString("%PDF-1.7\n")
	objectOffset := document.Len()
	fmt.Fprintf(&document, "1 0 obj\n<< /Type /Catalog%s >>\nendobj\n", catalogExtra)
	xrefOffset := document.Len()
	document.WriteString("xref\n0 2\n")
	document.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&document, "%010d 00000 n \n", objectOffset)
	fmt.Fprintf(&document, "trailer\n<< /Size 2 /Root 1 0 R%s >>\n", trailerExtra)
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return document.Bytes(), int64(xrefOffset)
}

func buildCustomClassicTestPDF(xrefBody, trailerBody string) []byte {
	var document bytes.Buffer
	document.WriteString("%PDF-1.7\n")
	objectOffset := document.Len()
	document.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xrefOffset := document.Len()
	document.WriteString("xref\n")
	xrefBody = strings.ReplaceAll(xrefBody, "{OBJECT_OFFSET}", fmt.Sprintf("%010d", objectOffset))
	document.WriteString(xrefBody)
	document.WriteString("trailer\n<< ")
	document.WriteString(trailerBody)
	document.WriteString(" >>\n")
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return document.Bytes()
}

func buildXrefStreamTestPDF(dictionaryExtra string) []byte {
	var document bytes.Buffer
	document.WriteString("%PDF-1.7\n")
	catalogOffset := document.Len()
	document.WriteString("2 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xrefOffset := document.Len()
	stream := make([]byte, 21)
	stream[0] = 0
	binary.BigEndian.PutUint16(stream[5:7], 65535)
	stream[7] = 1
	binary.BigEndian.PutUint32(stream[8:12], uint32(xrefOffset))
	stream[14] = 1
	binary.BigEndian.PutUint32(stream[15:19], uint32(catalogOffset))
	fmt.Fprintf(&document, "1 0 obj\n<< /Ty#70e /X#52ef /Size 3 /Root 2 0 R /W [1 4 2] /Length %d%s >>\n", len(stream), dictionaryExtra)
	document.WriteString("stream\n")
	document.Write(stream)
	document.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return document.Bytes()
}

func buildFlateXrefStreamTestPDF(t *testing.T) []byte {
	t.Helper()
	var document bytes.Buffer
	document.WriteString("%PDF-1.7\n")
	catalogOffset := document.Len()
	document.WriteString("2 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xrefOffset := document.Len()
	raw := make([]byte, 28)
	raw[0] = 0
	binary.BigEndian.PutUint16(raw[5:7], 65535)
	raw[7] = 1
	binary.BigEndian.PutUint32(raw[8:12], uint32(xrefOffset))
	raw[14] = 1
	binary.BigEndian.PutUint32(raw[15:19], uint32(catalogOffset))

	predicted := make([]byte, 0, len(raw)+2)
	previous := make([]byte, 14)
	for rowStart := 0; rowStart < len(raw); rowStart += 14 {
		row := raw[rowStart : rowStart+14]
		predicted = append(predicted, 2)
		for index, value := range row {
			predicted = append(predicted, value-previous[index])
		}
		copy(previous, row)
	}
	var compressed bytes.Buffer
	compressor := zlib.NewWriter(&compressed)
	if _, err := compressor.Write(predicted); err != nil {
		t.Fatalf("compress xref stream: %v", err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatalf("close xref compressor: %v", err)
	}

	fmt.Fprintf(&document, "1 0 obj\n<< /Type /XRef /Size 4 /Root 2 0 R /W [1 4 2] /Index [0 4] /Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 14 >> /Length %d >>\n", compressed.Len())
	document.WriteString("stream\n")
	document.Write(compressed.Bytes())
	document.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return document.Bytes()
}

func buildHybridXrefTestPDF(streamDictionaryExtra string) []byte {
	var document bytes.Buffer
	document.WriteString("%PDF-1.7\n")
	catalogOffset := document.Len()
	document.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xrefStreamOffset := document.Len()
	stream := make([]byte, 21)
	stream[0] = 0
	binary.BigEndian.PutUint16(stream[5:7], 65535)
	stream[7] = 1
	binary.BigEndian.PutUint32(stream[8:12], uint32(catalogOffset))
	stream[14] = 1
	binary.BigEndian.PutUint32(stream[15:19], uint32(xrefStreamOffset))
	fmt.Fprintf(&document, "2 0 obj\n<< /Type /XRef /Size 3 /W [1 4 2] /Length %d%s >>\n", len(stream), streamDictionaryExtra)
	document.WriteString("stream\n")
	document.Write(stream)
	document.WriteString("\nendstream\nendobj\n")

	classicXrefOffset := document.Len()
	document.WriteString("xref\n0 2\n0000000000 65535 f \n")
	fmt.Fprintf(&document, "%010d 00000 n \n", catalogOffset)
	fmt.Fprintf(&document, "trailer\n<< /Size 3 /Root 1 0 R /XRefStm %d >>\n", xrefStreamOffset)
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", classicXrefOffset)
	return document.Bytes()
}

func buildTwoRevisionHybridXrefTestPDF() []byte {
	var document bytes.Buffer
	document.WriteString("%PDF-1.7\n")
	catalogOffset := document.Len()
	document.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")

	baseStreamOffset := document.Len()
	baseEntry := make([]byte, 7)
	baseEntry[0] = 1
	binary.BigEndian.PutUint32(baseEntry[1:5], uint32(baseStreamOffset))
	document.WriteString("2 0 obj\n<< /Type /XRef /Size 3 /W [1 4 2] /Index [2 1] /Length 7 >>\nstream\n")
	document.Write(baseEntry)
	document.WriteString("\nendstream\nendobj\n")
	baseClassicOffset := document.Len()
	document.WriteString("xref\n0 2\n0000000000 65535 f \n")
	fmt.Fprintf(&document, "%010d 00000 n \n", catalogOffset)
	fmt.Fprintf(&document, "trailer\n<< /Size 3 /Root 1 0 R /XRefStm %d >>\n", baseStreamOffset)
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n\n", baseClassicOffset)

	currentStreamOffset := document.Len()
	currentEntry := make([]byte, 7)
	currentEntry[0] = 1
	binary.BigEndian.PutUint32(currentEntry[1:5], uint32(currentStreamOffset))
	fmt.Fprintf(&document, "3 0 obj\n<< /Type /XRef /Size 4 /W [1 4 2] /Index [3 1] /Prev %d /Length 7 >>\nstream\n", baseStreamOffset)
	document.Write(currentEntry)
	document.WriteString("\nendstream\nendobj\n")
	currentClassicOffset := document.Len()
	document.WriteString("xref\n0 1\n0000000000 65535 f \n")
	fmt.Fprintf(&document, "trailer\n<< /Size 4 /Root 1 0 R /Prev %d /XRefStm %d >>\n", baseClassicOffset, currentStreamOffset)
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", currentClassicOffset)
	return document.Bytes()
}

func buildIncrementalTestPDF(t *testing.T) []byte {
	t.Helper()
	base, previousXref := buildClassicTestPDF(" /Encrypt 2 0 R", "")
	var document bytes.Buffer
	document.Write(base)
	document.WriteByte('\n')
	currentXref := document.Len()
	document.WriteString("xref\n0 1\n0000000000 65535 f \n")
	fmt.Fprintf(&document, "trailer\n<< /Size 2 /Root 1 0 R /Prev %d >>\n", previousXref)
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", currentXref)
	return document.Bytes()
}

func buildIncrementalWithoutLatestRootTestPDF(t *testing.T) []byte {
	t.Helper()
	base, previousXref := buildClassicTestPDF("", "")
	var document bytes.Buffer
	document.Write(base)
	document.WriteByte('\n')
	currentXref := document.Len()
	document.WriteString("xref\n0 1\n0000000000 65535 f \n")
	fmt.Fprintf(&document, "trailer\n<< /Size 2 /Prev %d >>\n", previousXref)
	fmt.Fprintf(&document, "startxref\n%d\n%%%%EOF\n", currentXref)
	return document.Bytes()
}
