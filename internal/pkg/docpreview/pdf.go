package docpreview

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
)

const (
	maxPDFStartXrefSearchBytes  = int64(64 << 10)
	maxPDFXrefScanBytes         = int64(64 << 20)
	maxPDFXrefDecodedBytes      = int64(64 << 20)
	maxPDFObjectStreamBytes     = int64(64 << 20)
	maxPDFObjectHeaderScanBytes = int64(4096)
	maxPDFXrefSections          = 128
	maxPDFXrefSubsections       = 65536
	maxPDFXrefEntries           = 1 << 20
	maxPDFXrefLineBytes         = 128
	maxPDFTokenBytes            = 1 << 20
	maxPDFContainerDepth        = 64
	maxPDFContainerValues       = 1 << 20
	maxPDFXrefFieldWidth        = 8
)

type pdfTokenKind byte

const (
	pdfTokenRegular pdfTokenKind = iota
	pdfTokenName
	pdfTokenDictionaryStart
	pdfTokenDictionaryEnd
	pdfTokenArrayStart
	pdfTokenArrayEnd
	pdfTokenString
)

type pdfToken struct {
	kind  pdfTokenKind
	value string
}

type pdfValueKind byte

const (
	pdfValueRegular pdfValueKind = iota
	pdfValueName
	pdfValueString
	pdfValueArray
	pdfValueDictionary
	pdfValueReference
)

type pdfReference struct {
	object     uint64
	generation uint16
}

type pdfValue struct {
	kind       pdfValueKind
	text       string
	array      []pdfValue
	dictionary pdfDictionary
	reference  pdfReference
}

type pdfDictionary map[string]pdfValue

type pdfXrefEntryKind byte

const (
	pdfXrefEntryFree pdfXrefEntryKind = iota
	pdfXrefEntryUncompressed
	pdfXrefEntryCompressed
)

type pdfXrefEntry struct {
	object       uint64
	kind         pdfXrefEntryKind
	generation   uint16
	offset       int64
	nextFree     uint64
	objectStream uint64
	objectIndex  uint64
}

type pdfXrefRange struct {
	first uint64
	count uint64
	end   uint64
}

type pdfXrefSection struct {
	offset       int64
	classic      bool
	size         uint64
	root         *pdfReference
	prev         *int64
	xrefStm      *int64
	encrypted    bool
	streamObject *pdfReference
	entries      map[uint64]pdfXrefEntry
}

type pdfStreamSettings struct {
	length       uint64
	filters      []string
	decodeParams pdfDecodeParams
}

type pdfDecodeParams struct {
	present   bool
	predictor uint64
	colors    uint64
	bits      uint64
	columns   uint64
}

// validatePDFEncryption validates the effective cross-reference graph and its
// catalog while detecting encryption. It intentionally rejects unsupported
// stream encodings instead of guessing where xref data ends.
func validatePDFEncryption(reader io.ReaderAt, size int64) error {
	start, err := findPDFStartXref(reader, size)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrContentMismatch, err)
	}

	entries := make(map[uint64]pdfXrefEntry)
	xrefStreamObjects := make(map[pdfReference]struct{})
	sections := make(map[int64]*pdfXrefSection)
	processed := make(map[int64]struct{})
	edges := make(map[int64][]int64)
	var effectiveRoot *pdfReference
	pending := []int64{start}

	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if _, done := processed[current]; done {
			continue
		}
		primary, err := loadPDFXrefSection(reader, size, current, sections)
		if err != nil {
			return fmt.Errorf("%w: xref at %d: %v", ErrContentMismatch, current, err)
		}
		processed[current] = struct{}{}
		if primary.encrypted {
			return ErrEncryptedDocument
		}

		var companion *pdfXrefSection
		if primary.xrefStm != nil {
			if !primary.classic {
				return fmt.Errorf("%w: xref stream contains /XRefStm", ErrContentMismatch)
			}
			if _, alreadyProcessed := processed[*primary.xrefStm]; alreadyProcessed {
				return fmt.Errorf("%w: /XRefStm reuses an already processed xref section", ErrContentMismatch)
			}
			companion, err = loadPDFXrefSection(reader, size, *primary.xrefStm, sections)
			if err != nil {
				return fmt.Errorf("%w: /XRefStm at %d: %v", ErrContentMismatch, *primary.xrefStm, err)
			}
			processed[*primary.xrefStm] = struct{}{}
			if companion.classic || companion.streamObject == nil {
				return fmt.Errorf("%w: /XRefStm does not point to an xref stream", ErrContentMismatch)
			}
			if companion.xrefStm != nil {
				return fmt.Errorf("%w: hybrid xref stream contains /XRefStm", ErrContentMismatch)
			}
			if companion.size != primary.size {
				return fmt.Errorf("%w: hybrid xref sections have inconsistent /Size", ErrContentMismatch)
			}
			if companion.encrypted {
				return ErrEncryptedDocument
			}
		}

		if effectiveRoot == nil {
			revisionRoot, rootErr := effectivePDFRevisionRoot(primary, companion)
			if rootErr != nil {
				return fmt.Errorf("%w: %v", ErrContentMismatch, rootErr)
			}
			if revisionRoot != nil {
				rootCopy := *revisionRoot
				effectiveRoot = &rootCopy
			}
		}

		revisionEntries := make(map[uint64]pdfXrefEntry, len(primary.entries))
		if companion != nil {
			for object, entry := range companion.entries {
				revisionEntries[object] = entry
			}
			if companion.streamObject != nil {
				xrefStreamObjects[*companion.streamObject] = struct{}{}
			}
		}
		for object, entry := range primary.entries {
			if _, fromStream := revisionEntries[object]; !fromStream {
				revisionEntries[object] = entry
			}
		}
		if primary.streamObject != nil {
			xrefStreamObjects[*primary.streamObject] = struct{}{}
		}
		for object, entry := range revisionEntries {
			if _, newer := entries[object]; !newer {
				entries[object] = entry
			}
		}

		if primary.xrefStm != nil {
			edges[current] = append(edges[current], *primary.xrefStm)
		}
		if primary.prev != nil {
			edges[current] = append(edges[current], *primary.prev)
			pending = append(pending, *primary.prev)
		}
		if companion != nil && companion.prev != nil {
			edges[companion.offset] = append(edges[companion.offset], *companion.prev)
			if primary.prev == nil || *primary.prev != *companion.prev {
				pending = append(pending, *companion.prev)
			}
		}
	}
	if err := validatePDFXrefGraphAcyclic(start, edges); err != nil {
		return fmt.Errorf("%w: %v", ErrContentMismatch, err)
	}

	if effectiveRoot == nil {
		return fmt.Errorf("%w: PDF trailer chain has no valid /Root reference", ErrContentMismatch)
	}
	if _, selfReference := xrefStreamObjects[*effectiveRoot]; selfReference {
		return fmt.Errorf("%w: PDF /Root points to an xref stream object", ErrContentMismatch)
	}
	if err := validatePDFCatalogReference(reader, size, *effectiveRoot, entries); err != nil {
		return fmt.Errorf("%w: invalid PDF /Root: %v", ErrContentMismatch, err)
	}
	return nil
}

func loadPDFXrefSection(reader io.ReaderAt, size, offset int64, sections map[int64]*pdfXrefSection) (*pdfXrefSection, error) {
	if offset < 0 || offset >= size {
		return nil, fmt.Errorf("PDF xref offset %d is outside the file", offset)
	}
	if section, loaded := sections[offset]; loaded {
		return section, nil
	}
	if len(sections) >= maxPDFXrefSections {
		return nil, fmt.Errorf("PDF xref chain exceeds %d sections", maxPDFXrefSections)
	}
	section, err := parsePDFXrefSection(reader, size, offset)
	if err != nil {
		return nil, err
	}
	sections[offset] = section
	return section, nil
}

func effectivePDFRevisionRoot(primary, companion *pdfXrefSection) (*pdfReference, error) {
	if companion == nil || companion.root == nil {
		return primary.root, nil
	}
	if primary.root == nil {
		return companion.root, nil
	}
	if *primary.root != *companion.root {
		return nil, errors.New("hybrid xref sections have inconsistent /Root references")
	}
	return primary.root, nil
}

func validatePDFXrefGraphAcyclic(start int64, edges map[int64][]int64) error {
	states := make(map[int64]byte)
	var visit func(int64) error
	visit = func(offset int64) error {
		switch states[offset] {
		case 1:
			return errors.New("cycle in PDF xref graph")
		case 2:
			return nil
		}
		states[offset] = 1
		for _, next := range edges[offset] {
			if err := visit(next); err != nil {
				return err
			}
		}
		states[offset] = 2
		return nil
	}
	return visit(start)
}

func findPDFStartXref(reader io.ReaderAt, size int64) (int64, error) {
	searchSize := min(size, maxPDFStartXrefSearchBytes)
	tail := make([]byte, searchSize)
	if _, err := reader.ReadAt(tail, size-searchSize); err != nil {
		return 0, fmt.Errorf("read PDF startxref trailer: %w", err)
	}
	searchEnd := len(tail)
	for searchEnd > 0 {
		index := bytes.LastIndex(tail[:searchEnd], []byte("startxref"))
		if index < 0 {
			break
		}
		if !pdfKeywordStartsLine(tail, index) {
			searchEnd = index
			continue
		}
		position := index + len("startxref")
		for position < len(tail) && isPDFWhitespace(tail[position]) {
			position++
		}
		startDigits := position
		for position < len(tail) && tail[position] >= '0' && tail[position] <= '9' {
			position++
		}
		if position == startDigits || (position < len(tail) && !isPDFWhitespace(tail[position])) {
			return 0, errors.New("final startxref has no valid offset")
		}
		offset, err := strconv.ParseInt(string(tail[startDigits:position]), 10, 64)
		if err != nil {
			return 0, errors.New("final startxref offset is out of range")
		}
		return offset, nil
	}
	return 0, errors.New("missing or invalid startxref")
}

func pdfKeywordStartsLine(data []byte, index int) bool {
	for position := index - 1; position >= 0; position-- {
		if data[position] == '\n' || data[position] == '\r' {
			return true
		}
		if !isPDFWhitespace(data[position]) {
			return false
		}
	}
	return true
}

func parsePDFXrefSection(reader io.ReaderAt, size, offset int64) (*pdfXrefSection, error) {
	firstByte := []byte{0}
	if _, err := reader.ReadAt(firstByte, offset); err != nil {
		return nil, err
	}
	if isPDFWhitespace(firstByte[0]) || firstByte[0] == '%' {
		return nil, errors.New("xref offset does not point to the beginning of an object")
	}

	length := min(size-offset, maxPDFXrefScanBytes)
	lexer := newPDFLexer(io.NewSectionReader(reader, offset, length))
	first, err := lexer.next()
	if err != nil {
		return nil, err
	}
	if first.kind == pdfTokenRegular && first.value == "xref" {
		return parseClassicPDFXref(reader, size, offset, lexer, length)
	}
	if first.kind != pdfTokenRegular || !isPDFUnsignedInteger(first.value) {
		return nil, errors.New("xref offset does not point to an xref table or stream object")
	}
	objectNumber, err := strconv.ParseUint(first.value, 10, 64)
	if err != nil || objectNumber == 0 {
		return nil, errors.New("invalid xref stream object number")
	}
	generation, err := lexer.next()
	if err != nil || generation.kind != pdfTokenRegular || !isPDFUnsignedInteger(generation.value) {
		return nil, errors.New("invalid xref stream object generation")
	}
	generationNumber, err := strconv.ParseUint(generation.value, 10, 16)
	if err != nil {
		return nil, errors.New("invalid xref stream object generation")
	}
	objectKeyword, err := lexer.next()
	if err != nil || objectKeyword.kind != pdfTokenRegular || objectKeyword.value != "obj" {
		return nil, errors.New("invalid xref stream object header")
	}
	dictionary, err := parsePDFDictionary(lexer)
	if err != nil {
		return nil, err
	}
	section, settings, widths, ranges, err := parsePDFXrefStreamDictionary(dictionary, offset)
	if err != nil {
		return nil, err
	}
	streamReference := pdfReference{object: objectNumber, generation: uint16(generationNumber)}
	section.streamObject = &streamReference
	if objectNumber >= section.size {
		return nil, errors.New("xref stream object number is outside /Size")
	}

	raw, err := readPDFStreamObject(lexer, settings.length)
	if err != nil {
		return nil, err
	}
	entryCount, entryWidth, err := validatePDFXrefLayout(ranges, widths)
	if err != nil {
		return nil, err
	}
	expectedBytes := int64(entryCount * entryWidth)
	decoded, err := decodePDFXrefStream(raw, settings, expectedBytes)
	if err != nil {
		return nil, err
	}
	section.entries, err = parsePDFXrefStreamEntries(reader, size, section.size, decoded, widths, ranges)
	if err != nil {
		return nil, err
	}
	selfEntry, ok := section.entries[objectNumber]
	if !ok || selfEntry.kind != pdfXrefEntryUncompressed || selfEntry.offset != offset || selfEntry.generation != uint16(generationNumber) {
		return nil, errors.New("xref stream does not contain a matching entry for itself")
	}
	return section, nil
}

func parseClassicPDFXref(reader io.ReaderAt, fileSize, sectionOffset int64, lexer *pdfLexer, scanLength int64) (*pdfXrefSection, error) {
	if remainder, err := lexer.readPhysicalLine(); err != nil || len(trimPDFLineWhitespace(remainder)) != 0 {
		return nil, errors.New("xref keyword must be followed by a line ending")
	}
	maxEntries := uint64(scanLength/18 + 1)
	var totalEntries uint64
	var ranges []pdfXrefRange
	entries := make(map[uint64]pdfXrefEntry)

	for {
		line, err := lexer.readPhysicalLine()
		if err != nil {
			return nil, fmt.Errorf("classic xref has no trailer: %w", err)
		}
		line = trimPDFLineWhitespace(line)
		if len(line) == 0 {
			continue
		}
		if bytes.Equal(line, []byte("trailer")) {
			if len(ranges) == 0 {
				return nil, errors.New("classic xref has no subsections")
			}
			dictionary, err := parsePDFDictionary(lexer)
			if err != nil {
				return nil, err
			}
			section, err := parsePDFClassicTrailer(dictionary, sectionOffset, entries)
			if err != nil {
				return nil, err
			}
			sort.Slice(ranges, func(left, right int) bool { return ranges[left].first < ranges[right].first })
			for index, current := range ranges {
				if current.end > section.size {
					return nil, errors.New("xref subsection exceeds trailer /Size")
				}
				if index > 0 && current.first < ranges[index-1].end {
					return nil, errors.New("overlapping xref subsections")
				}
			}
			for _, entry := range entries {
				if entry.kind == pdfXrefEntryFree && entry.nextFree >= section.size {
					return nil, errors.New("free xref entry points outside trailer /Size")
				}
			}
			return section, nil
		}

		fields := splitPDFWhitespaceFields(line)
		if len(fields) != 2 || !isPDFUnsignedInteger(string(fields[0])) || !isPDFUnsignedInteger(string(fields[1])) {
			return nil, fmt.Errorf("invalid xref subsection header %q", line)
		}
		firstObject, err := strconv.ParseUint(string(fields[0]), 10, 32)
		if err != nil {
			return nil, errors.New("xref subsection first object is out of range")
		}
		entryCount, err := strconv.ParseUint(string(fields[1]), 10, 32)
		if err != nil || entryCount == 0 || firstObject > uint64(^uint32(0))-entryCount {
			return nil, errors.New("xref subsection entry count is invalid")
		}
		if len(ranges) >= maxPDFXrefSubsections {
			return nil, fmt.Errorf("classic xref exceeds %d subsections", maxPDFXrefSubsections)
		}
		if entryCount > maxEntries-totalEntries || totalEntries+entryCount > maxPDFXrefEntries {
			return nil, errors.New("xref entry count exceeds bounded section size")
		}
		ranges = append(ranges, pdfXrefRange{first: firstObject, count: entryCount, end: firstObject + entryCount})
		totalEntries += entryCount

		for entryIndex := uint64(0); entryIndex < entryCount; entryIndex++ {
			entryLine, err := lexer.readPhysicalLine()
			if err != nil {
				return nil, fmt.Errorf("truncated xref subsection: %w", err)
			}
			entryFields := splitPDFWhitespaceFields(trimPDFLineWhitespace(entryLine))
			if len(entryFields) != 3 || len(entryFields[0]) != 10 || len(entryFields[1]) != 5 || len(entryFields[2]) != 1 {
				return nil, fmt.Errorf("invalid xref entry %q", entryLine)
			}
			if !isPDFUnsignedInteger(string(entryFields[0])) || !isPDFUnsignedInteger(string(entryFields[1])) {
				return nil, fmt.Errorf("invalid numeric xref entry %q", entryLine)
			}
			field2, offsetErr := strconv.ParseUint(string(entryFields[0]), 10, 64)
			generation, generationErr := strconv.ParseUint(string(entryFields[1]), 10, 16)
			status := entryFields[2][0]
			if offsetErr != nil || generationErr != nil || (status != 'n' && status != 'f') {
				return nil, fmt.Errorf("invalid xref entry %q", entryLine)
			}
			objectNumber := firstObject + entryIndex
			if _, duplicate := entries[objectNumber]; duplicate {
				return nil, errors.New("overlapping xref subsections")
			}
			if objectNumber == 0 && (status != 'f' || generation != 65535) {
				return nil, errors.New("xref object zero entry is invalid")
			}
			if status == 'f' {
				entries[objectNumber] = pdfXrefEntry{
					object: objectNumber, kind: pdfXrefEntryFree,
					generation: uint16(generation), nextFree: field2,
				}
				continue
			}
			if generation == 65535 || field2 >= uint64(fileSize) || field2 > uint64(^uint64(0)>>1) {
				return nil, errors.New("in-use xref entry offset or generation is invalid")
			}
			entry := pdfXrefEntry{
				object: objectNumber, kind: pdfXrefEntryUncompressed,
				generation: uint16(generation), offset: int64(field2),
			}
			if err := validatePDFObjectHeader(reader, fileSize, entry); err != nil {
				return nil, fmt.Errorf("xref entry for object %d: %w", objectNumber, err)
			}
			entries[objectNumber] = entry
		}
	}
}

func parsePDFClassicTrailer(dictionary pdfDictionary, offset int64, entries map[uint64]pdfXrefEntry) (*pdfXrefSection, error) {
	metadata, err := parsePDFXrefMetadata(dictionary)
	if err != nil {
		return nil, err
	}
	if !metadata.hasSize {
		return nil, errors.New("classic trailer has no valid /Size")
	}
	return &pdfXrefSection{
		offset: offset, classic: true, size: metadata.size, root: metadata.root,
		prev: metadata.prev, xrefStm: metadata.xrefStm, encrypted: metadata.encrypted,
		entries: entries,
	}, nil
}

type pdfXrefMetadata struct {
	hasSize   bool
	size      uint64
	root      *pdfReference
	prev      *int64
	xrefStm   *int64
	encrypted bool
}

func parsePDFXrefMetadata(dictionary pdfDictionary) (pdfXrefMetadata, error) {
	var metadata pdfXrefMetadata
	_, metadata.encrypted = dictionary["Encrypt"]
	if value, ok := dictionary["Size"]; ok {
		size, err := pdfDirectUnsignedInteger(value, "/Size")
		if err != nil || size == 0 {
			return metadata, errors.New("invalid PDF trailer /Size")
		}
		metadata.hasSize = true
		metadata.size = size
	}
	if value, ok := dictionary["Root"]; ok {
		if value.kind != pdfValueReference || value.reference.object == 0 {
			return metadata, errors.New("PDF trailer /Root is not a valid indirect reference")
		}
		root := value.reference
		metadata.root = &root
	}
	var err error
	metadata.prev, err = pdfOptionalDirectOffset(dictionary, "Prev")
	if err != nil {
		return metadata, err
	}
	metadata.xrefStm, err = pdfOptionalDirectOffset(dictionary, "XRefStm")
	if err != nil {
		return metadata, err
	}
	if metadata.root != nil && metadata.hasSize && metadata.root.object >= metadata.size {
		return metadata, errors.New("PDF trailer /Root is outside /Size")
	}
	return metadata, nil
}

func parsePDFXrefStreamDictionary(dictionary pdfDictionary, offset int64) (*pdfXrefSection, pdfStreamSettings, [3]uint64, []pdfXrefRange, error) {
	metadata, err := parsePDFXrefMetadata(dictionary)
	if err != nil {
		return nil, pdfStreamSettings{}, [3]uint64{}, nil, err
	}
	if !metadata.hasSize {
		return nil, pdfStreamSettings{}, [3]uint64{}, nil, errors.New("xref stream dictionary has no valid /Size")
	}
	typeName, err := pdfRequiredName(dictionary, "Type")
	if err != nil || typeName != "XRef" {
		return nil, pdfStreamSettings{}, [3]uint64{}, nil, errors.New("object at xref offset is not a /Type /XRef stream")
	}
	settings, err := parsePDFStreamSettings(dictionary)
	if err != nil {
		return nil, pdfStreamSettings{}, [3]uint64{}, nil, err
	}
	widths, err := parsePDFXrefWidths(dictionary)
	if err != nil {
		return nil, pdfStreamSettings{}, [3]uint64{}, nil, err
	}
	ranges, err := parsePDFXrefIndex(dictionary, metadata.size)
	if err != nil {
		return nil, pdfStreamSettings{}, [3]uint64{}, nil, err
	}
	section := &pdfXrefSection{
		offset: offset, size: metadata.size, root: metadata.root, prev: metadata.prev,
		xrefStm: metadata.xrefStm, encrypted: metadata.encrypted,
	}
	return section, settings, widths, ranges, nil
}

func parsePDFXrefWidths(dictionary pdfDictionary) ([3]uint64, error) {
	var widths [3]uint64
	value, ok := dictionary["W"]
	if !ok || value.kind != pdfValueArray || len(value.array) != len(widths) {
		return widths, errors.New("xref stream has no valid /W array")
	}
	var total uint64
	for index := range widths {
		width, err := pdfDirectUnsignedInteger(value.array[index], "/W")
		if err != nil || width > maxPDFXrefFieldWidth {
			return widths, errors.New("xref stream /W field width is invalid")
		}
		widths[index] = width
		total += width
	}
	if total == 0 {
		return widths, errors.New("xref stream /W entry width is zero")
	}
	return widths, nil
}

func parsePDFXrefIndex(dictionary pdfDictionary, size uint64) ([]pdfXrefRange, error) {
	value, ok := dictionary["Index"]
	if !ok {
		if size == 0 {
			return nil, errors.New("xref stream default /Index is empty")
		}
		return []pdfXrefRange{{first: 0, count: size, end: size}}, nil
	}
	if value.kind != pdfValueArray || len(value.array) == 0 || len(value.array)%2 != 0 {
		return nil, errors.New("xref stream /Index must contain object/count pairs")
	}
	if len(value.array)/2 > maxPDFXrefSubsections {
		return nil, errors.New("xref stream /Index has too many ranges")
	}
	ranges := make([]pdfXrefRange, 0, len(value.array)/2)
	var previousEnd uint64
	for index := 0; index < len(value.array); index += 2 {
		first, firstErr := pdfDirectUnsignedInteger(value.array[index], "/Index")
		count, countErr := pdfDirectUnsignedInteger(value.array[index+1], "/Index")
		if firstErr != nil || countErr != nil || count == 0 || first > ^uint64(0)-count {
			return nil, errors.New("xref stream /Index range is invalid")
		}
		end := first + count
		if end > size || (len(ranges) > 0 && first < previousEnd) {
			return nil, errors.New("xref stream /Index ranges overlap or exceed /Size")
		}
		ranges = append(ranges, pdfXrefRange{first: first, count: count, end: end})
		previousEnd = end
	}
	return ranges, nil
}

func validatePDFXrefLayout(ranges []pdfXrefRange, widths [3]uint64) (uint64, uint64, error) {
	entryWidth := widths[0] + widths[1] + widths[2]
	if entryWidth == 0 {
		return 0, 0, errors.New("xref stream entry width is zero")
	}
	var entryCount uint64
	for _, current := range ranges {
		if current.count > maxPDFXrefEntries-entryCount {
			return 0, 0, errors.New("xref stream has too many entries")
		}
		entryCount += current.count
	}
	if entryCount == 0 || entryCount > uint64(maxPDFXrefDecodedBytes)/entryWidth {
		return 0, 0, errors.New("decoded xref stream exceeds size limit")
	}
	return entryCount, entryWidth, nil
}

func parsePDFXrefStreamEntries(reader io.ReaderAt, fileSize int64, trailerSize uint64, decoded []byte, widths [3]uint64, ranges []pdfXrefRange) (map[uint64]pdfXrefEntry, error) {
	entries := make(map[uint64]pdfXrefEntry)
	position := 0
	for _, current := range ranges {
		for entryIndex := uint64(0); entryIndex < current.count; entryIndex++ {
			fields := [3]uint64{}
			for field := range fields {
				width := int(widths[field])
				if position+width > len(decoded) {
					return nil, errors.New("truncated decoded xref stream entry")
				}
				fields[field] = readPDFBigEndianInteger(decoded[position : position+width])
				position += width
			}
			entryType := fields[0]
			if widths[0] == 0 {
				entryType = 1
			}
			objectNumber := current.first + entryIndex
			entry := pdfXrefEntry{object: objectNumber}
			switch entryType {
			case 0:
				if fields[1] >= trailerSize || fields[2] > 65535 {
					return nil, errors.New("free xref stream entry is invalid")
				}
				entry.kind = pdfXrefEntryFree
				entry.nextFree = fields[1]
				entry.generation = uint16(fields[2])
			case 1:
				if objectNumber == 0 || fields[1] > uint64(^uint64(0)>>1) || fields[1] >= uint64(fileSize) || fields[2] >= 65535 {
					return nil, errors.New("in-use xref stream entry is invalid")
				}
				entry.kind = pdfXrefEntryUncompressed
				entry.offset = int64(fields[1])
				entry.generation = uint16(fields[2])
				if err := validatePDFObjectHeader(reader, fileSize, entry); err != nil {
					return nil, fmt.Errorf("xref stream entry for object %d: %w", objectNumber, err)
				}
			case 2:
				if objectNumber == 0 || fields[1] == 0 || fields[1] >= trailerSize || fields[2] >= maxPDFXrefEntries {
					return nil, errors.New("compressed xref stream entry is invalid")
				}
				entry.kind = pdfXrefEntryCompressed
				entry.objectStream = fields[1]
				entry.objectIndex = fields[2]
			default:
				return nil, fmt.Errorf("unsupported xref stream entry type %d", entryType)
			}
			if objectNumber == 0 && entry.kind != pdfXrefEntryFree {
				return nil, errors.New("xref stream object zero entry is invalid")
			}
			entries[objectNumber] = entry
		}
	}
	if position != len(decoded) {
		return nil, errors.New("decoded xref stream contains trailing entry data")
	}
	return entries, nil
}

func readPDFBigEndianInteger(data []byte) uint64 {
	var value uint64
	for _, current := range data {
		value = value<<8 | uint64(current)
	}
	return value
}

func validatePDFObjectHeader(reader io.ReaderAt, fileSize int64, entry pdfXrefEntry) error {
	if entry.kind != pdfXrefEntryUncompressed || entry.offset < 0 || entry.offset >= fileSize {
		return errors.New("object offset is outside the file")
	}
	first := []byte{0}
	if _, err := reader.ReadAt(first, entry.offset); err != nil {
		return err
	}
	if first[0] < '0' || first[0] > '9' {
		return errors.New("object offset does not point to an object header")
	}
	length := min(fileSize-entry.offset, maxPDFObjectHeaderScanBytes)
	lexer := newPDFLexer(io.NewSectionReader(reader, entry.offset, length))
	objectToken, err := lexer.next()
	if err != nil || objectToken.kind != pdfTokenRegular || !isPDFUnsignedInteger(objectToken.value) {
		return errors.New("invalid indirect object number")
	}
	objectNumber, err := strconv.ParseUint(objectToken.value, 10, 64)
	if err != nil || objectNumber != entry.object {
		return errors.New("indirect object number does not match xref entry")
	}
	generationToken, err := lexer.next()
	if err != nil || generationToken.kind != pdfTokenRegular || !isPDFUnsignedInteger(generationToken.value) {
		return errors.New("invalid indirect object generation")
	}
	generation, err := strconv.ParseUint(generationToken.value, 10, 16)
	if err != nil || uint16(generation) != entry.generation {
		return errors.New("indirect object generation does not match xref entry")
	}
	keyword, err := lexer.next()
	if err != nil || keyword.kind != pdfTokenRegular || keyword.value != "obj" {
		return errors.New("xref entry does not point to an indirect object")
	}
	return nil
}

func validatePDFCatalogReference(reader io.ReaderAt, fileSize int64, root pdfReference, entries map[uint64]pdfXrefEntry) error {
	entry, ok := entries[root.object]
	if !ok || entry.kind == pdfXrefEntryFree {
		return errors.New("catalog object has no effective in-use xref entry")
	}
	switch entry.kind {
	case pdfXrefEntryUncompressed:
		if entry.generation != root.generation {
			return errors.New("catalog generation does not match its xref entry")
		}
		dictionary, err := readPDFIndirectDictionaryObject(reader, fileSize, entry)
		if err != nil {
			return err
		}
		return requirePDFCatalogDictionary(dictionary)
	case pdfXrefEntryCompressed:
		if root.generation != 0 {
			return errors.New("compressed catalog must use generation zero")
		}
		return validatePDFCompressedCatalog(reader, fileSize, root, entry, entries)
	default:
		return errors.New("catalog xref entry type is invalid")
	}
}

func readPDFIndirectDictionaryObject(reader io.ReaderAt, fileSize int64, entry pdfXrefEntry) (pdfDictionary, error) {
	if err := validatePDFObjectHeader(reader, fileSize, entry); err != nil {
		return nil, err
	}
	length := min(fileSize-entry.offset, maxPDFXrefScanBytes)
	lexer := newPDFLexer(io.NewSectionReader(reader, entry.offset, length))
	if err := consumePDFIndirectObjectHeader(lexer, entry.object, entry.generation); err != nil {
		return nil, err
	}
	dictionary, err := parsePDFDictionary(lexer)
	if err != nil {
		return nil, err
	}
	end, err := lexer.next()
	if err != nil || end.kind != pdfTokenRegular || end.value != "endobj" {
		return nil, errors.New("catalog object has no valid endobj boundary")
	}
	return dictionary, nil
}

func consumePDFIndirectObjectHeader(lexer *pdfLexer, object uint64, generation uint16) error {
	objectToken, err := lexer.next()
	if err != nil || objectToken.kind != pdfTokenRegular || !isPDFUnsignedInteger(objectToken.value) {
		return errors.New("invalid indirect object number")
	}
	objectNumber, err := strconv.ParseUint(objectToken.value, 10, 64)
	if err != nil || objectNumber != object {
		return errors.New("invalid indirect object number")
	}
	generationToken, err := lexer.next()
	if err != nil || generationToken.kind != pdfTokenRegular || !isPDFUnsignedInteger(generationToken.value) {
		return errors.New("invalid indirect object generation")
	}
	generationNumber, err := strconv.ParseUint(generationToken.value, 10, 16)
	if err != nil || uint16(generationNumber) != generation {
		return errors.New("invalid indirect object generation")
	}
	keyword, err := lexer.next()
	if err != nil || keyword.kind != pdfTokenRegular || keyword.value != "obj" {
		return errors.New("invalid indirect object header")
	}
	return nil
}

func requirePDFCatalogDictionary(dictionary pdfDictionary) error {
	typeName, err := pdfRequiredName(dictionary, "Type")
	if err != nil || typeName != "Catalog" {
		return errors.New("root object is not a /Type /Catalog dictionary")
	}
	return nil
}

func validatePDFCompressedCatalog(reader io.ReaderAt, fileSize int64, root pdfReference, rootEntry pdfXrefEntry, entries map[uint64]pdfXrefEntry) error {
	if root.object == rootEntry.objectStream {
		return errors.New("compressed catalog points to itself as an object stream")
	}
	streamEntry, ok := entries[rootEntry.objectStream]
	if !ok || streamEntry.kind != pdfXrefEntryUncompressed {
		return errors.New("catalog object stream has no uncompressed xref entry")
	}
	dictionary, decoded, err := readPDFObjectStream(reader, fileSize, streamEntry)
	if err != nil {
		return err
	}
	typeName, err := pdfRequiredName(dictionary, "Type")
	if err != nil || typeName != "ObjStm" {
		return errors.New("catalog container is not a /Type /ObjStm stream")
	}
	n, err := pdfRequiredDirectUnsignedInteger(dictionary, "N")
	if err != nil || n == 0 || n > maxPDFXrefEntries {
		return errors.New("object stream has invalid /N")
	}
	first, err := pdfRequiredDirectUnsignedInteger(dictionary, "First")
	if err != nil || first > uint64(len(decoded)) {
		return errors.New("object stream has invalid /First")
	}
	if rootEntry.objectIndex >= n {
		return errors.New("catalog object index exceeds object stream /N")
	}

	objectNumbers, offsets, err := parsePDFObjectStreamHeader(decoded[:first], n, uint64(len(decoded))-first)
	if err != nil {
		return err
	}
	index := int(rootEntry.objectIndex)
	if objectNumbers[index] != root.object {
		return errors.New("catalog object number does not match object stream index")
	}
	start := first + offsets[index]
	end := uint64(len(decoded))
	if index+1 < len(offsets) {
		end = first + offsets[index+1]
	}
	if start >= end || end > uint64(len(decoded)) {
		return errors.New("catalog object stream boundaries are invalid")
	}
	lexer := newPDFLexer(bytes.NewReader(decoded[start:end]))
	dictionaryValue, err := lexer.next()
	if err != nil || dictionaryValue.kind != pdfTokenDictionaryStart {
		return errors.New("compressed catalog is not a dictionary")
	}
	budget := maxPDFContainerValues
	catalog, err := parsePDFDictionaryAfterStart(lexer, 1, &budget)
	if err != nil {
		return err
	}
	if token, trailingErr := lexer.next(); trailingErr == nil || !errors.Is(trailingErr, io.EOF) {
		return fmt.Errorf("compressed catalog has trailing data near %q", token.value)
	}
	return requirePDFCatalogDictionary(catalog)
}

func readPDFObjectStream(reader io.ReaderAt, fileSize int64, entry pdfXrefEntry) (pdfDictionary, []byte, error) {
	if err := validatePDFObjectHeader(reader, fileSize, entry); err != nil {
		return nil, nil, err
	}
	length := min(fileSize-entry.offset, maxPDFXrefScanBytes)
	lexer := newPDFLexer(io.NewSectionReader(reader, entry.offset, length))
	if err := consumePDFIndirectObjectHeader(lexer, entry.object, entry.generation); err != nil {
		return nil, nil, err
	}
	dictionary, err := parsePDFDictionary(lexer)
	if err != nil {
		return nil, nil, err
	}
	settings, err := parsePDFStreamSettings(dictionary)
	if err != nil {
		return nil, nil, err
	}
	raw, err := readPDFStreamObject(lexer, settings.length)
	if err != nil {
		return nil, nil, err
	}
	decoded, err := decodePDFObjectStream(raw, settings)
	if err != nil {
		return nil, nil, err
	}
	return dictionary, decoded, nil
}

func parsePDFObjectStreamHeader(header []byte, count, objectDataLength uint64) ([]uint64, []uint64, error) {
	lexer := newPDFLexer(bytes.NewReader(header))
	objectNumbers := make([]uint64, count)
	offsets := make([]uint64, count)
	seen := make(map[uint64]struct{}, count)
	for index := uint64(0); index < count; index++ {
		objectToken, err := lexer.next()
		if err != nil || objectToken.kind != pdfTokenRegular || !isPDFUnsignedInteger(objectToken.value) {
			return nil, nil, errors.New("invalid object stream object number")
		}
		objectNumber, err := strconv.ParseUint(objectToken.value, 10, 64)
		if err != nil || objectNumber == 0 {
			return nil, nil, errors.New("invalid object stream object number")
		}
		if _, duplicate := seen[objectNumber]; duplicate {
			return nil, nil, errors.New("duplicate object number in object stream")
		}
		seen[objectNumber] = struct{}{}

		offsetToken, err := lexer.next()
		if err != nil || offsetToken.kind != pdfTokenRegular || !isPDFUnsignedInteger(offsetToken.value) {
			return nil, nil, errors.New("invalid object stream object offset")
		}
		relativeOffset, err := strconv.ParseUint(offsetToken.value, 10, 64)
		if err != nil || relativeOffset >= objectDataLength || (index > 0 && relativeOffset <= offsets[index-1]) {
			return nil, nil, errors.New("object stream offsets are invalid")
		}
		objectNumbers[index] = objectNumber
		offsets[index] = relativeOffset
	}
	if token, err := lexer.next(); err == nil || !errors.Is(err, io.EOF) {
		return nil, nil, fmt.Errorf("object stream header has trailing token %q", token.value)
	}
	return objectNumbers, offsets, nil
}

func parsePDFStreamSettings(dictionary pdfDictionary) (pdfStreamSettings, error) {
	length, err := pdfRequiredDirectUnsignedInteger(dictionary, "Length")
	if err != nil || length == 0 || length > uint64(maxPDFXrefScanBytes) {
		return pdfStreamSettings{}, errors.New("PDF stream has invalid or unsupported /Length")
	}
	filters, params, err := parsePDFFilters(dictionary)
	if err != nil {
		return pdfStreamSettings{}, err
	}
	return pdfStreamSettings{length: length, filters: filters, decodeParams: params}, nil
}

func parsePDFFilters(dictionary pdfDictionary) ([]string, pdfDecodeParams, error) {
	params := pdfDecodeParams{predictor: 1, colors: 1, bits: 8, columns: 1}
	filterValue, hasFilter := dictionary["Filter"]
	var filters []string
	if hasFilter {
		switch filterValue.kind {
		case pdfValueName:
			filters = []string{filterValue.text}
		case pdfValueArray:
			if len(filterValue.array) == 0 {
				return nil, params, errors.New("PDF stream /Filter array is empty")
			}
			for _, value := range filterValue.array {
				if value.kind != pdfValueName {
					return nil, params, errors.New("PDF stream /Filter array contains a non-name value")
				}
				filters = append(filters, value.text)
			}
		default:
			return nil, params, errors.New("PDF stream /Filter is invalid")
		}
	}

	decodeValue, hasDecodeParams := dictionary["DecodeParms"]
	if !hasDecodeParams {
		return filters, params, nil
	}
	params.present = true
	if !hasFilter {
		return nil, params, errors.New("PDF stream has /DecodeParms without /Filter")
	}
	if decodeValue.kind == pdfValueRegular && decodeValue.text == "null" {
		return filters, params, nil
	}
	if decodeValue.kind == pdfValueArray {
		if len(filters) != 1 || len(decodeValue.array) != 1 {
			return nil, params, errors.New("multiple PDF stream filter parameters are unsupported")
		}
		decodeValue = decodeValue.array[0]
		if decodeValue.kind == pdfValueRegular && decodeValue.text == "null" {
			return filters, params, nil
		}
	}
	if decodeValue.kind != pdfValueDictionary {
		return nil, params, errors.New("PDF stream /DecodeParms is invalid")
	}
	for key, destination := range map[string]*uint64{
		"Predictor":        &params.predictor,
		"Colors":           &params.colors,
		"BitsPerComponent": &params.bits,
		"Columns":          &params.columns,
	} {
		if value, ok := decodeValue.dictionary[key]; ok {
			parsed, err := pdfDirectUnsignedInteger(value, "/DecodeParms "+key)
			if err != nil {
				return nil, params, err
			}
			*destination = parsed
		}
	}
	return filters, params, nil
}

func readPDFStreamObject(lexer *pdfLexer, length uint64) ([]byte, error) {
	if length > uint64(maxPDFXrefScanBytes) || length > uint64(^uint(0)>>1) {
		return nil, errors.New("PDF stream length exceeds parser limit")
	}
	streamKeyword, err := lexer.next()
	if err != nil || streamKeyword.kind != pdfTokenRegular || streamKeyword.value != "stream" {
		return nil, errors.New("PDF stream dictionary is not followed by stream")
	}
	if err := lexer.readRequiredLineEnding(); err != nil {
		return nil, errors.New("PDF stream keyword is not followed by a line ending")
	}
	raw := make([]byte, int(length))
	if err := lexer.readFull(raw); err != nil {
		return nil, fmt.Errorf("truncated PDF stream data: %w", err)
	}
	if err := lexer.readOptionalLineEnding(); err != nil {
		return nil, err
	}
	if err := lexer.readRequiredKeyword("endstream"); err != nil {
		return nil, errors.New("PDF stream has no valid endstream boundary")
	}
	endObject, err := lexer.next()
	if err != nil || endObject.kind != pdfTokenRegular || endObject.value != "endobj" {
		return nil, errors.New("PDF stream has no valid endobj boundary")
	}
	return raw, nil
}

func decodePDFXrefStream(raw []byte, settings pdfStreamSettings, expectedBytes int64) ([]byte, error) {
	if expectedBytes <= 0 || expectedBytes > maxPDFXrefDecodedBytes {
		return nil, errors.New("decoded xref stream size is invalid")
	}
	if len(settings.filters) == 0 {
		if settings.decodeParams.present {
			return nil, errors.New("unfiltered xref stream has /DecodeParms")
		}
		if int64(len(raw)) != expectedBytes {
			return nil, errors.New("xref stream length does not match /W and /Index")
		}
		return raw, nil
	}
	if len(settings.filters) != 1 || (settings.filters[0] != "FlateDecode" && settings.filters[0] != "Fl") {
		return nil, fmt.Errorf("unsupported xref stream /Filter %v", settings.filters)
	}
	inflatedLimit, err := pdfPredictorEncodedLimit(settings.decodeParams, expectedBytes)
	if err != nil {
		return nil, err
	}
	inflated, err := inflatePDFData(raw, inflatedLimit)
	if err != nil {
		return nil, fmt.Errorf("decode xref Flate stream: %w", err)
	}
	decoded, err := applyPDFPredictor(inflated, settings.decodeParams, expectedBytes)
	if err != nil {
		return nil, err
	}
	if int64(len(decoded)) != expectedBytes {
		return nil, errors.New("decoded xref stream length does not match /W and /Index")
	}
	return decoded, nil
}

func decodePDFObjectStream(raw []byte, settings pdfStreamSettings) ([]byte, error) {
	if settings.decodeParams.predictor != 1 {
		return nil, errors.New("predictor-compressed object streams are unsupported")
	}
	if len(settings.filters) == 0 {
		if settings.decodeParams.present {
			return nil, errors.New("unfiltered object stream has /DecodeParms")
		}
		if int64(len(raw)) > maxPDFObjectStreamBytes {
			return nil, errors.New("object stream exceeds decoded size limit")
		}
		return raw, nil
	}
	if len(settings.filters) != 1 || (settings.filters[0] != "FlateDecode" && settings.filters[0] != "Fl") {
		return nil, fmt.Errorf("unsupported object stream /Filter %v", settings.filters)
	}
	decoded, err := inflatePDFData(raw, maxPDFObjectStreamBytes)
	if err != nil {
		return nil, fmt.Errorf("decode object Flate stream: %w", err)
	}
	return decoded, nil
}

func pdfPredictorEncodedLimit(params pdfDecodeParams, expectedBytes int64) (int64, error) {
	switch params.predictor {
	case 1, 2:
		return expectedBytes, nil
	case 10, 11, 12, 13, 14, 15:
		rowBytes, _, err := pdfPredictorRowLayout(params)
		if err != nil {
			return 0, err
		}
		if uint64(expectedBytes)%rowBytes != 0 {
			return 0, errors.New("xref predictor rows do not cover complete entries")
		}
		rows := uint64(expectedBytes) / rowBytes
		if rows > uint64(maxPDFXrefDecodedBytes)-uint64(expectedBytes) {
			return 0, errors.New("predicted xref stream exceeds size limit")
		}
		return expectedBytes + int64(rows), nil
	default:
		return 0, fmt.Errorf("unsupported PDF predictor %d", params.predictor)
	}
}

func inflatePDFData(raw []byte, limit int64) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	decoded, readErr := io.ReadAll(limited)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(decoded)) > limit {
		return nil, errors.New("decoded PDF stream exceeds size limit")
	}
	return decoded, nil
}

func applyPDFPredictor(data []byte, params pdfDecodeParams, outputLimit int64) ([]byte, error) {
	if params.predictor == 1 {
		return data, nil
	}
	rowBytes, bytesPerPixel, err := pdfPredictorRowLayout(params)
	if err != nil {
		return nil, err
	}
	if params.predictor == 2 {
		if params.bits != 8 || len(data)%int(rowBytes) != 0 {
			return nil, errors.New("unsupported TIFF predictor layout")
		}
		decoded := append([]byte(nil), data...)
		for rowStart := 0; rowStart < len(decoded); rowStart += int(rowBytes) {
			for index := int(bytesPerPixel); index < int(rowBytes); index++ {
				decoded[rowStart+index] += decoded[rowStart+index-int(bytesPerPixel)]
			}
		}
		return decoded, nil
	}

	encodedRowBytes := rowBytes + 1
	if len(data)%int(encodedRowBytes) != 0 {
		return nil, errors.New("PNG predictor data has a truncated row")
	}
	rows := len(data) / int(encodedRowBytes)
	if uint64(rows)*rowBytes > uint64(outputLimit) {
		return nil, errors.New("predicted PDF stream exceeds size limit")
	}
	decoded := make([]byte, rows*int(rowBytes))
	for row := 0; row < rows; row++ {
		encoded := data[row*int(encodedRowBytes) : (row+1)*int(encodedRowBytes)]
		filter := encoded[0]
		if filter > 4 || (params.predictor != 15 && filter != byte(params.predictor-10)) {
			return nil, errors.New("PNG predictor row uses an invalid filter")
		}
		current := decoded[row*int(rowBytes) : (row+1)*int(rowBytes)]
		var previous []byte
		if row > 0 {
			previous = decoded[(row-1)*int(rowBytes) : row*int(rowBytes)]
		}
		for index, encodedByte := range encoded[1:] {
			var left, up, upperLeft byte
			if index >= int(bytesPerPixel) {
				left = current[index-int(bytesPerPixel)]
				if previous != nil {
					upperLeft = previous[index-int(bytesPerPixel)]
				}
			}
			if previous != nil {
				up = previous[index]
			}
			switch filter {
			case 0:
				current[index] = encodedByte
			case 1:
				current[index] = encodedByte + left
			case 2:
				current[index] = encodedByte + up
			case 3:
				current[index] = encodedByte + byte((uint16(left)+uint16(up))/2)
			case 4:
				current[index] = encodedByte + pdfPaethPredictor(left, up, upperLeft)
			}
		}
	}
	return decoded, nil
}

func pdfPredictorRowLayout(params pdfDecodeParams) (uint64, uint64, error) {
	if params.colors == 0 || params.columns == 0 {
		return 0, 0, errors.New("PDF predictor has zero Colors or Columns")
	}
	switch params.bits {
	case 1, 2, 4, 8, 16:
	default:
		return 0, 0, errors.New("PDF predictor BitsPerComponent is unsupported")
	}
	if params.colors > ^uint64(0)/params.columns || params.colors*params.columns > ^uint64(0)/params.bits {
		return 0, 0, errors.New("PDF predictor row size overflows")
	}
	rowBits := params.colors * params.columns * params.bits
	rowBytes := (rowBits + 7) / 8
	pixelBits := params.colors * params.bits
	bytesPerPixel := (pixelBits + 7) / 8
	if rowBytes == 0 || bytesPerPixel == 0 || rowBytes > uint64(maxPDFXrefDecodedBytes) {
		return 0, 0, errors.New("PDF predictor row exceeds size limit")
	}
	return rowBytes, bytesPerPixel, nil
}

func pdfPaethPredictor(left, up, upperLeft byte) byte {
	prediction := int(left) + int(up) - int(upperLeft)
	leftDistance := absPDFInt(prediction - int(left))
	upDistance := absPDFInt(prediction - int(up))
	upperLeftDistance := absPDFInt(prediction - int(upperLeft))
	if leftDistance <= upDistance && leftDistance <= upperLeftDistance {
		return left
	}
	if upDistance <= upperLeftDistance {
		return up
	}
	return upperLeft
}

func absPDFInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func parsePDFDictionary(lexer *pdfLexer) (pdfDictionary, error) {
	start, err := lexer.next()
	if err != nil || start.kind != pdfTokenDictionaryStart {
		return nil, errors.New("missing PDF dictionary")
	}
	budget := maxPDFContainerValues
	return parsePDFDictionaryAfterStart(lexer, 1, &budget)
}

func parsePDFDictionaryAfterStart(lexer *pdfLexer, depth int, budget *int) (pdfDictionary, error) {
	if depth > maxPDFContainerDepth {
		return nil, errors.New("PDF container nesting exceeds limit")
	}
	dictionary := make(pdfDictionary)
	for {
		key, err := lexer.next()
		if err != nil {
			return nil, fmt.Errorf("read PDF dictionary key: %w", err)
		}
		if key.kind == pdfTokenDictionaryEnd {
			return dictionary, nil
		}
		if key.kind != pdfTokenName {
			return nil, fmt.Errorf("invalid PDF dictionary key %q", key.value)
		}
		if _, duplicate := dictionary[key.value]; duplicate {
			return nil, fmt.Errorf("duplicate PDF dictionary key /%s", key.value)
		}
		valueToken, err := lexer.next()
		if err != nil {
			return nil, fmt.Errorf("read PDF dictionary value for /%s: %w", key.value, err)
		}
		value, err := parsePDFValueFromToken(lexer, valueToken, depth+1, budget)
		if err != nil {
			return nil, fmt.Errorf("read PDF dictionary value for /%s: %w", key.value, err)
		}
		dictionary[key.value] = value
	}
}

func parsePDFValueFromToken(lexer *pdfLexer, token pdfToken, depth int, budget *int) (pdfValue, error) {
	if *budget <= 0 {
		return pdfValue{}, errors.New("PDF container value count exceeds limit")
	}
	*budget--
	switch token.kind {
	case pdfTokenName:
		return pdfValue{kind: pdfValueName, text: token.value}, nil
	case pdfTokenString:
		return pdfValue{kind: pdfValueString}, nil
	case pdfTokenDictionaryStart:
		dictionary, err := parsePDFDictionaryAfterStart(lexer, depth, budget)
		return pdfValue{kind: pdfValueDictionary, dictionary: dictionary}, err
	case pdfTokenArrayStart:
		if depth > maxPDFContainerDepth {
			return pdfValue{}, errors.New("PDF container nesting exceeds limit")
		}
		var values []pdfValue
		for {
			current, err := lexer.next()
			if err != nil {
				return pdfValue{}, fmt.Errorf("read PDF array value: %w", err)
			}
			if current.kind == pdfTokenArrayEnd {
				return pdfValue{kind: pdfValueArray, array: values}, nil
			}
			value, err := parsePDFValueFromToken(lexer, current, depth+1, budget)
			if err != nil {
				return pdfValue{}, err
			}
			values = append(values, value)
		}
	case pdfTokenDictionaryEnd, pdfTokenArrayEnd:
		return pdfValue{}, errors.New("unexpected PDF container terminator")
	case pdfTokenRegular:
		if isPDFUnsignedInteger(token.value) {
			second, secondErr := lexer.next()
			if secondErr == nil && second.kind == pdfTokenRegular && isPDFUnsignedInteger(second.value) {
				third, thirdErr := lexer.next()
				if thirdErr == nil && third.kind == pdfTokenRegular && third.value == "R" {
					object, objectErr := strconv.ParseUint(token.value, 10, 64)
					generation, generationErr := strconv.ParseUint(second.value, 10, 16)
					if objectErr != nil || generationErr != nil {
						return pdfValue{}, errors.New("invalid PDF indirect reference")
					}
					return pdfValue{kind: pdfValueReference, reference: pdfReference{object: object, generation: uint16(generation)}}, nil
				}
				if thirdErr == nil {
					lexer.unreadTokens(second, third)
				} else {
					lexer.unreadTokens(second)
				}
			} else if secondErr == nil {
				lexer.unreadTokens(second)
			}
		}
		return pdfValue{kind: pdfValueRegular, text: token.value}, nil
	default:
		return pdfValue{}, errors.New("unsupported PDF value")
	}
}

func pdfDirectUnsignedInteger(value pdfValue, key string) (uint64, error) {
	if value.kind != pdfValueRegular || !isPDFUnsignedInteger(value.text) {
		return 0, fmt.Errorf("%s must be a direct unsigned integer", key)
	}
	parsed, err := strconv.ParseUint(value.text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s integer is out of range", key)
	}
	return parsed, nil
}

func pdfRequiredDirectUnsignedInteger(dictionary pdfDictionary, key string) (uint64, error) {
	value, ok := dictionary[key]
	if !ok {
		return 0, fmt.Errorf("PDF dictionary has no /%s", key)
	}
	return pdfDirectUnsignedInteger(value, "/"+key)
}

func pdfRequiredName(dictionary pdfDictionary, key string) (string, error) {
	value, ok := dictionary[key]
	if !ok || value.kind != pdfValueName {
		return "", fmt.Errorf("PDF dictionary has no valid /%s name", key)
	}
	return value.text, nil
}

func pdfOptionalDirectOffset(dictionary pdfDictionary, key string) (*int64, error) {
	value, ok := dictionary[key]
	if !ok {
		return nil, nil
	}
	parsed, err := pdfDirectUnsignedInteger(value, "/"+key)
	if err != nil || parsed > uint64(^uint64(0)>>1) {
		return nil, fmt.Errorf("invalid /%s offset", key)
	}
	offset := int64(parsed)
	return &offset, nil
}

type pdfLexer struct {
	reader      *bufio.Reader
	position    int64
	tokenBuffer []pdfToken
}

func (lexer *pdfLexer) readPhysicalLine() ([]byte, error) {
	if len(lexer.tokenBuffer) != 0 {
		return nil, errors.New("cannot read an xref line with buffered PDF tokens")
	}
	line := make([]byte, 0, 32)
	for len(line) <= maxPDFXrefLineBytes {
		value, err := lexer.readByte()
		if err != nil {
			return nil, err
		}
		switch value {
		case '\n':
			return line, nil
		case '\r':
			if next, peekErr := lexer.reader.Peek(1); peekErr == nil && next[0] == '\n' {
				_, _ = lexer.readByte()
			}
			return line, nil
		default:
			line = append(line, value)
		}
	}
	return nil, fmt.Errorf("PDF xref line exceeds %d bytes", maxPDFXrefLineBytes)
}

func (lexer *pdfLexer) readRequiredLineEnding() error {
	if len(lexer.tokenBuffer) != 0 {
		return errors.New("buffered PDF tokens precede a stream boundary")
	}
	value, err := lexer.readByte()
	if err != nil {
		return err
	}
	switch value {
	case '\n':
		return nil
	case '\r':
		if next, peekErr := lexer.reader.Peek(1); peekErr == nil && next[0] == '\n' {
			_, _ = lexer.readByte()
		}
		return nil
	default:
		return errors.New("missing line ending")
	}
}

func (lexer *pdfLexer) readOptionalLineEnding() error {
	if len(lexer.tokenBuffer) != 0 {
		return errors.New("buffered PDF tokens precede a stream boundary")
	}
	value, err := lexer.readByte()
	if err != nil {
		return err
	}
	switch value {
	case '\n':
		return nil
	case '\r':
		if next, peekErr := lexer.reader.Peek(1); peekErr == nil && next[0] == '\n' {
			_, _ = lexer.readByte()
		}
		return nil
	default:
		return lexer.unreadByte()
	}
}

func (lexer *pdfLexer) readRequiredKeyword(keyword string) error {
	if len(lexer.tokenBuffer) != 0 {
		return errors.New("buffered PDF tokens precede a keyword")
	}
	data := make([]byte, len(keyword))
	if err := lexer.readFull(data); err != nil {
		return err
	}
	if string(data) != keyword {
		return errors.New("unexpected PDF keyword")
	}
	if next, err := lexer.reader.Peek(1); err == nil && !isPDFWhitespace(next[0]) && !isPDFDelimiter(next[0]) {
		return errors.New("PDF keyword has no token boundary")
	}
	return nil
}

func (lexer *pdfLexer) readFull(destination []byte) error {
	if len(lexer.tokenBuffer) != 0 {
		return errors.New("buffered PDF tokens precede stream data")
	}
	count, err := io.ReadFull(lexer.reader, destination)
	lexer.position += int64(count)
	return err
}

func (lexer *pdfLexer) readByte() (byte, error) {
	value, err := lexer.reader.ReadByte()
	if err == nil {
		lexer.position++
	}
	return value, err
}

func (lexer *pdfLexer) unreadByte() error {
	if err := lexer.reader.UnreadByte(); err != nil {
		return err
	}
	lexer.position--
	return nil
}

func newPDFLexer(reader io.Reader) *pdfLexer {
	return &pdfLexer{reader: bufio.NewReaderSize(reader, 64)}
}

func (lexer *pdfLexer) unreadTokens(tokens ...pdfToken) {
	for index := len(tokens) - 1; index >= 0; index-- {
		lexer.tokenBuffer = append(lexer.tokenBuffer, tokens[index])
	}
}

func (lexer *pdfLexer) next() (pdfToken, error) {
	if last := len(lexer.tokenBuffer) - 1; last >= 0 {
		token := lexer.tokenBuffer[last]
		lexer.tokenBuffer = lexer.tokenBuffer[:last]
		return token, nil
	}
	first, err := lexer.nextNonWhitespace()
	if err != nil {
		return pdfToken{}, err
	}
	switch first {
	case '/':
		name, err := lexer.readPDFName()
		return pdfToken{kind: pdfTokenName, value: name}, err
	case '(':
		return pdfToken{kind: pdfTokenString}, lexer.skipLiteralString()
	case '[':
		return pdfToken{kind: pdfTokenArrayStart}, nil
	case ']':
		return pdfToken{kind: pdfTokenArrayEnd}, nil
	case '<':
		next, err := lexer.readByte()
		if err != nil {
			return pdfToken{}, errors.New("truncated PDF angle delimiter")
		}
		if next == '<' {
			return pdfToken{kind: pdfTokenDictionaryStart}, nil
		}
		if err := lexer.skipHexString(next); err != nil {
			return pdfToken{}, err
		}
		return pdfToken{kind: pdfTokenString}, nil
	case '>':
		next, err := lexer.readByte()
		if err != nil || next != '>' {
			return pdfToken{}, errors.New("invalid PDF dictionary terminator")
		}
		return pdfToken{kind: pdfTokenDictionaryEnd}, nil
	default:
		value, err := lexer.readRegularToken(first)
		return pdfToken{kind: pdfTokenRegular, value: value}, err
	}
}

func (lexer *pdfLexer) nextNonWhitespace() (byte, error) {
	for {
		value, err := lexer.readByte()
		if err != nil {
			return 0, err
		}
		if isPDFWhitespace(value) {
			continue
		}
		if value != '%' {
			return value, nil
		}
		for {
			value, err = lexer.readByte()
			if err != nil {
				return 0, err
			}
			if value == '\n' || value == '\r' {
				break
			}
		}
	}
}

func (lexer *pdfLexer) readPDFName() (string, error) {
	var encoded []byte
	for len(encoded) <= maxPDFTokenBytes {
		value, err := lexer.readByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		if isPDFWhitespace(value) || isPDFDelimiter(value) {
			if err := lexer.unreadByte(); err != nil {
				return "", err
			}
			break
		}
		encoded = append(encoded, value)
	}
	if len(encoded) > maxPDFTokenBytes {
		return "", errors.New("PDF name exceeds size limit")
	}
	decoded := make([]byte, 0, len(encoded))
	for index := 0; index < len(encoded); index++ {
		if encoded[index] != '#' {
			decoded = append(decoded, encoded[index])
			continue
		}
		if index+2 >= len(encoded) {
			return "", errors.New("truncated PDF name escape")
		}
		high, okHigh := pdfHexValue(encoded[index+1])
		low, okLow := pdfHexValue(encoded[index+2])
		if !okHigh || !okLow {
			return "", errors.New("invalid PDF name escape")
		}
		decoded = append(decoded, high<<4|low)
		index += 2
	}
	return string(decoded), nil
}

func (lexer *pdfLexer) skipLiteralString() error {
	depth := 1
	escaped := false
	consumed := 0
	for depth > 0 {
		if consumed >= maxPDFTokenBytes {
			return errors.New("PDF literal string exceeds size limit")
		}
		value, err := lexer.readByte()
		if err != nil {
			return errors.New("unterminated PDF literal string")
		}
		consumed++
		if escaped {
			escaped = false
			if value == '\r' {
				if next, peekErr := lexer.reader.Peek(1); peekErr == nil && next[0] == '\n' {
					_, _ = lexer.readByte()
					consumed++
				}
			}
			continue
		}
		if value == '\\' {
			escaped = true
		} else if value == '(' {
			depth++
		} else if value == ')' {
			depth--
		}
	}
	return nil
}

func (lexer *pdfLexer) skipHexString(first byte) error {
	value := first
	for consumed := 0; consumed <= maxPDFTokenBytes; consumed++ {
		if value == '>' {
			return nil
		}
		if !isPDFWhitespace(value) {
			if _, ok := pdfHexValue(value); !ok {
				return errors.New("invalid PDF hex string")
			}
		}
		next, err := lexer.readByte()
		if err != nil {
			return errors.New("unterminated PDF hex string")
		}
		value = next
	}
	return errors.New("PDF hex string exceeds size limit")
}

func (lexer *pdfLexer) readRegularToken(first byte) (string, error) {
	value := []byte{first}
	for len(value) <= maxPDFTokenBytes {
		next, err := lexer.readByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return string(value), nil
			}
			return "", err
		}
		if isPDFWhitespace(next) || isPDFDelimiter(next) {
			if err := lexer.unreadByte(); err != nil {
				return "", err
			}
			return string(value), nil
		}
		value = append(value, next)
	}
	return "", errors.New("PDF token exceeds size limit")
}

func isPDFUnsignedInteger(value string) bool {
	if value == "" {
		return false
	}
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func trimPDFLineWhitespace(line []byte) []byte {
	return bytes.Trim(line, "\x00\t\f ")
}

func splitPDFWhitespaceFields(line []byte) [][]byte {
	var fields [][]byte
	for position := 0; position < len(line); {
		for position < len(line) && isPDFWhitespace(line[position]) {
			position++
		}
		start := position
		for position < len(line) && !isPDFWhitespace(line[position]) {
			position++
		}
		if start < position {
			fields = append(fields, line[start:position])
		}
	}
	return fields
}

func isPDFWhitespace(value byte) bool {
	switch value {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

func isPDFDelimiter(value byte) bool {
	switch value {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}

func pdfHexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	default:
		return 0, false
	}
}
