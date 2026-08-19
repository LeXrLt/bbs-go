package docpreview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	cfbHeaderSize       = 512
	cfbDirectorySize    = 128
	cfbMiniStreamCutoff = uint64(4096)

	cfbFreeSector  = uint32(0xffffffff)
	cfbEndOfChain  = uint32(0xfffffffe)
	cfbFATSector   = uint32(0xfffffffd)
	cfbDIFATSector = uint32(0xfffffffc)
	cfbNoStream    = uint32(0xffffffff)
)

type cfbHeader struct {
	majorVersion       uint16
	sectorSize         int64
	miniSectorSize     int64
	numDirectorySector uint32
	numFATSectors      uint32
	firstDirectory     uint32
	firstMiniFAT       uint32
	numMiniFATSectors  uint32
	firstDIFAT         uint32
	numDIFATSectors    uint32
	headerDIFAT        []uint32
}

type cfbDirectoryEntry struct {
	name   string
	typ    byte
	left   uint32
	right  uint32
	child  uint32
	start  uint32
	size   uint64
	parent uint32
}

type cfbDocument struct {
	reader       io.ReaderAt
	size         int64
	header       cfbHeader
	sectorCount  uint32
	fat          []uint32
	miniFAT      []uint32
	directories  []cfbDirectoryEntry
	regularChain map[uint32][]uint32
	miniChain    map[uint32][]uint32
	rootMiniData []byte
}

func parseCFB(reader io.ReaderAt, size int64) (*cfbDocument, error) {
	header, err := parseCFBHeader(reader, size)
	if err != nil {
		return nil, err
	}
	sectorCount64 := size/header.sectorSize - 1
	if sectorCount64 <= 0 || sectorCount64 > int64(math.MaxUint32) {
		return nil, errors.New("invalid CFB sector count")
	}
	document := &cfbDocument{
		reader:       reader,
		size:         size,
		header:       header,
		sectorCount:  uint32(sectorCount64),
		regularChain: make(map[uint32][]uint32),
		miniChain:    make(map[uint32][]uint32),
	}
	fatSectors, difatSectors, err := document.readDIFAT()
	if err != nil {
		return nil, err
	}
	if err := document.readFAT(fatSectors, difatSectors); err != nil {
		return nil, err
	}
	directoryChain, err := document.followRegularChain(header.firstDirectory, expectedDirectorySectors(header))
	if err != nil {
		return nil, fmt.Errorf("read directory chain: %w", err)
	}
	directoryData, err := document.readRegularSectors(directoryChain, int64(len(directoryChain))*header.sectorSize)
	if err != nil {
		return nil, err
	}
	if err := document.parseDirectories(directoryData); err != nil {
		return nil, err
	}
	if err := document.validateStreamChains(fatSectors, difatSectors, directoryChain); err != nil {
		return nil, err
	}
	return document, nil
}

func parseCFBHeader(reader io.ReaderAt, size int64) (cfbHeader, error) {
	if size < cfbHeaderSize {
		return cfbHeader{}, errors.New("truncated CFB header")
	}
	data := make([]byte, cfbHeaderSize)
	if _, err := reader.ReadAt(data, 0); err != nil {
		return cfbHeader{}, fmt.Errorf("read CFB header: %w", err)
	}
	if !bytes.Equal(data[:8], oleCFBMagic) {
		return cfbHeader{}, errors.New("invalid CFB signature")
	}
	if !allZero(data[8:24]) {
		return cfbHeader{}, errors.New("non-zero CFB header CLSID")
	}
	if binary.LittleEndian.Uint16(data[28:30]) != 0xfffe {
		return cfbHeader{}, errors.New("invalid CFB byte order")
	}
	major := binary.LittleEndian.Uint16(data[26:28])
	sectorShift := binary.LittleEndian.Uint16(data[30:32])
	if (major != 3 || sectorShift != 9) && (major != 4 || sectorShift != 12) {
		return cfbHeader{}, fmt.Errorf("unsupported CFB version %d with sector shift %d", major, sectorShift)
	}
	if binary.LittleEndian.Uint16(data[32:34]) != 6 {
		return cfbHeader{}, errors.New("invalid CFB mini-sector shift")
	}
	if !allZero(data[34:40]) {
		return cfbHeader{}, errors.New("non-zero CFB reserved header bytes")
	}
	sectorSize := int64(1) << sectorShift
	if size < sectorSize*2 || size%sectorSize != 0 {
		return cfbHeader{}, errors.New("CFB size is not aligned to its sector size")
	}
	numDirectorySectors := binary.LittleEndian.Uint32(data[40:44])
	if major == 3 && numDirectorySectors != 0 {
		return cfbHeader{}, errors.New("version 3 CFB declares directory sector count")
	}
	if binary.LittleEndian.Uint32(data[56:60]) != uint32(cfbMiniStreamCutoff) {
		return cfbHeader{}, errors.New("invalid CFB mini-stream cutoff")
	}
	header := cfbHeader{
		majorVersion:       major,
		sectorSize:         sectorSize,
		miniSectorSize:     1 << binary.LittleEndian.Uint16(data[32:34]),
		numDirectorySector: numDirectorySectors,
		numFATSectors:      binary.LittleEndian.Uint32(data[44:48]),
		firstDirectory:     binary.LittleEndian.Uint32(data[48:52]),
		firstMiniFAT:       binary.LittleEndian.Uint32(data[60:64]),
		numMiniFATSectors:  binary.LittleEndian.Uint32(data[64:68]),
		firstDIFAT:         binary.LittleEndian.Uint32(data[68:72]),
		numDIFATSectors:    binary.LittleEndian.Uint32(data[72:76]),
		headerDIFAT:        make([]uint32, 109),
	}
	for index := range header.headerDIFAT {
		header.headerDIFAT[index] = binary.LittleEndian.Uint32(data[76+index*4:])
	}
	if header.numFATSectors == 0 {
		return cfbHeader{}, errors.New("CFB has no FAT sectors")
	}
	return header, nil
}

func expectedDirectorySectors(header cfbHeader) int64 {
	if header.majorVersion == 4 {
		if header.numDirectorySector == 0 {
			return -2
		}
		return int64(header.numDirectorySector)
	}
	return -1
}

func (document *cfbDocument) readDIFAT() ([]uint32, []uint32, error) {
	if document.header.numFATSectors > document.sectorCount || document.header.numDIFATSectors > document.sectorCount {
		return nil, nil, errors.New("CFB allocation table count exceeds file sectors")
	}
	fatSectors := make([]uint32, 0, document.header.numFATSectors)
	for _, sector := range document.header.headerDIFAT {
		if sector == cfbFreeSector {
			continue
		}
		if !document.validSector(sector) {
			return nil, nil, fmt.Errorf("invalid FAT sector %08x in header DIFAT", sector)
		}
		fatSectors = append(fatSectors, sector)
	}

	difatSectors := make([]uint32, 0, document.header.numDIFATSectors)
	next := document.header.firstDIFAT
	entriesPerSector := int(document.header.sectorSize/4) - 1
	seen := make(map[uint32]struct{}, document.header.numDIFATSectors)
	for index := uint32(0); index < document.header.numDIFATSectors; index++ {
		if !document.validSector(next) {
			return nil, nil, fmt.Errorf("invalid DIFAT sector %08x", next)
		}
		if _, duplicate := seen[next]; duplicate {
			return nil, nil, errors.New("cycle in DIFAT chain")
		}
		seen[next] = struct{}{}
		difatSectors = append(difatSectors, next)
		data, err := document.readSector(next)
		if err != nil {
			return nil, nil, err
		}
		for entryIndex := 0; entryIndex < entriesPerSector; entryIndex++ {
			sector := binary.LittleEndian.Uint32(data[entryIndex*4:])
			if sector == cfbFreeSector {
				continue
			}
			if !document.validSector(sector) {
				return nil, nil, fmt.Errorf("invalid FAT sector %08x in DIFAT", sector)
			}
			fatSectors = append(fatSectors, sector)
		}
		next = binary.LittleEndian.Uint32(data[entriesPerSector*4:])
	}
	if document.header.numDIFATSectors == 0 {
		if document.header.firstDIFAT != cfbEndOfChain {
			return nil, nil, errors.New("CFB has unexpected first DIFAT sector")
		}
	} else if next != cfbEndOfChain {
		return nil, nil, errors.New("DIFAT chain is longer than declared")
	}
	if uint32(len(fatSectors)) != document.header.numFATSectors {
		return nil, nil, fmt.Errorf("found %d FAT sectors, header declares %d", len(fatSectors), document.header.numFATSectors)
	}
	if hasDuplicateSectors(fatSectors) {
		return nil, nil, errors.New("duplicate FAT sector")
	}
	return fatSectors, difatSectors, nil
}

func (document *cfbDocument) readFAT(fatSectors, difatSectors []uint32) error {
	entriesPerSector := int(document.header.sectorSize / 4)
	document.fat = make([]uint32, 0, len(fatSectors)*entriesPerSector)
	for _, sector := range fatSectors {
		data, err := document.readSector(sector)
		if err != nil {
			return err
		}
		for offset := 0; offset < len(data); offset += 4 {
			document.fat = append(document.fat, binary.LittleEndian.Uint32(data[offset:]))
		}
	}
	if len(document.fat) < int(document.sectorCount) {
		return errors.New("FAT does not cover all file sectors")
	}
	for _, sector := range fatSectors {
		if document.fat[sector] != cfbFATSector {
			return fmt.Errorf("FAT sector %d is not marked as FAT", sector)
		}
	}
	for _, sector := range difatSectors {
		if document.fat[sector] != cfbDIFATSector {
			return fmt.Errorf("DIFAT sector %d is not marked as DIFAT", sector)
		}
	}
	return nil
}

func (document *cfbDocument) parseDirectories(data []byte) error {
	if len(data) < cfbDirectorySize || len(data)%cfbDirectorySize != 0 {
		return errors.New("invalid CFB directory stream size")
	}
	document.directories = make([]cfbDirectoryEntry, len(data)/cfbDirectorySize)
	for index := range document.directories {
		raw := data[index*cfbDirectorySize : (index+1)*cfbDirectorySize]
		entry, err := parseCFBDirectoryEntry(raw, document.header.majorVersion)
		if err != nil {
			return fmt.Errorf("directory entry %d: %w", index, err)
		}
		document.directories[index] = entry
	}
	if document.directories[0].typ != 5 {
		return errors.New("first CFB directory entry is not the root storage")
	}
	if document.directories[0].left != cfbNoStream || document.directories[0].right != cfbNoStream {
		return errors.New("root CFB directory entry has siblings")
	}

	type pendingEntry struct {
		id     uint32
		parent uint32
	}
	pending := []pendingEntry{{id: document.directories[0].child, parent: 0}}
	visited := map[uint32]struct{}{0: {}}
	namesByParent := make(map[uint32]map[string]struct{})
	for len(pending) > 0 {
		last := len(pending) - 1
		current := pending[last]
		pending = pending[:last]
		if current.id == cfbNoStream {
			continue
		}
		if int(current.id) >= len(document.directories) || current.id == 0 {
			return errors.New("invalid CFB directory tree reference")
		}
		if _, duplicate := visited[current.id]; duplicate {
			return errors.New("cycle or duplicate reference in CFB directory tree")
		}
		visited[current.id] = struct{}{}
		entry := &document.directories[current.id]
		if entry.typ != 1 && entry.typ != 2 {
			return fmt.Errorf("reachable directory entry %d has invalid type %d", current.id, entry.typ)
		}
		entry.parent = current.parent
		nameKey := strings.ToLower(entry.name)
		if namesByParent[current.parent] == nil {
			namesByParent[current.parent] = make(map[string]struct{})
		}
		if _, duplicate := namesByParent[current.parent][nameKey]; duplicate {
			return fmt.Errorf("duplicate directory name %q", entry.name)
		}
		namesByParent[current.parent][nameKey] = struct{}{}
		pending = append(pending,
			pendingEntry{id: entry.left, parent: current.parent},
			pendingEntry{id: entry.right, parent: current.parent},
		)
		if entry.typ == 1 {
			pending = append(pending, pendingEntry{id: entry.child, parent: current.id})
		} else if entry.child != cfbNoStream {
			return fmt.Errorf("stream %q has a child directory", entry.name)
		}
	}
	for index, entry := range document.directories {
		if entry.typ != 0 {
			if _, ok := visited[uint32(index)]; !ok {
				return fmt.Errorf("orphaned CFB directory entry %d", index)
			}
		}
	}
	return nil
}

func parseCFBDirectoryEntry(data []byte, majorVersion uint16) (cfbDirectoryEntry, error) {
	entry := cfbDirectoryEntry{
		typ:    data[66],
		left:   binary.LittleEndian.Uint32(data[68:72]),
		right:  binary.LittleEndian.Uint32(data[72:76]),
		child:  binary.LittleEndian.Uint32(data[76:80]),
		start:  binary.LittleEndian.Uint32(data[116:120]),
		size:   binary.LittleEndian.Uint64(data[120:128]),
		parent: cfbNoStream,
	}
	if entry.typ == 0 {
		return entry, nil
	}
	if entry.typ != 1 && entry.typ != 2 && entry.typ != 5 {
		return cfbDirectoryEntry{}, fmt.Errorf("invalid object type %d", entry.typ)
	}
	nameLength := int(binary.LittleEndian.Uint16(data[64:66]))
	if nameLength < 2 || nameLength > 64 || nameLength%2 != 0 || data[nameLength-2] != 0 || data[nameLength-1] != 0 {
		return cfbDirectoryEntry{}, errors.New("invalid directory name length or terminator")
	}
	name, err := decodeCFBName(data[:nameLength-2])
	if err != nil || name == "" {
		return cfbDirectoryEntry{}, errors.New("invalid directory name")
	}
	entry.name = name
	if majorVersion == 3 && entry.size>>32 != 0 {
		return cfbDirectoryEntry{}, errors.New("version 3 stream size exceeds 32 bits")
	}
	return entry, nil
}

func decodeCFBName(data []byte) (string, error) {
	codeUnits := make([]uint16, len(data)/2)
	for index := range codeUnits {
		codeUnits[index] = binary.LittleEndian.Uint16(data[index*2:])
	}
	for index := 0; index < len(codeUnits); index++ {
		unit := codeUnits[index]
		if unit >= 0xd800 && unit <= 0xdbff {
			if index+1 >= len(codeUnits) || codeUnits[index+1] < 0xdc00 || codeUnits[index+1] > 0xdfff {
				return "", errors.New("invalid UTF-16 surrogate")
			}
			index++
		} else if unit >= 0xdc00 && unit <= 0xdfff {
			return "", errors.New("unpaired UTF-16 surrogate")
		}
	}
	name := string(utf16.Decode(codeUnits))
	if !utf8.ValidString(name) || strings.ContainsRune(name, '\x00') || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return "", errors.New("invalid directory name characters")
	}
	return name, nil
}

func (document *cfbDocument) validateStreamChains(fatSectors, difatSectors, directoryChain []uint32) error {
	usedRegular := make(map[uint32]string)
	mark := func(label string, sectors []uint32) error {
		for _, sector := range sectors {
			if previous, exists := usedRegular[sector]; exists {
				return fmt.Errorf("sector %d is shared by %s and %s", sector, previous, label)
			}
			usedRegular[sector] = label
		}
		return nil
	}
	if err := mark("FAT", fatSectors); err != nil {
		return err
	}
	if err := mark("DIFAT", difatSectors); err != nil {
		return err
	}
	if err := mark("directory", directoryChain); err != nil {
		return err
	}

	if document.header.numMiniFATSectors == 0 {
		if document.header.firstMiniFAT != cfbEndOfChain {
			return errors.New("CFB has unexpected first mini FAT sector")
		}
	} else {
		chain, err := document.followRegularChain(document.header.firstMiniFAT, int64(document.header.numMiniFATSectors))
		if err != nil {
			return fmt.Errorf("read mini FAT chain: %w", err)
		}
		if err := mark("mini FAT", chain); err != nil {
			return err
		}
		data, err := document.readRegularSectors(chain, int64(len(chain))*document.header.sectorSize)
		if err != nil {
			return err
		}
		document.miniFAT = make([]uint32, len(data)/4)
		for index := range document.miniFAT {
			document.miniFAT[index] = binary.LittleEndian.Uint32(data[index*4:])
		}
	}

	root := document.directories[0]
	rootChain, err := document.streamRegularChain(root)
	if err != nil {
		return fmt.Errorf("root mini stream: %w", err)
	}
	if err := mark("root mini stream", rootChain); err != nil {
		return err
	}
	document.regularChain[0] = rootChain
	if root.size > 0 {
		document.rootMiniData, err = document.readRegularSectors(rootChain, int64(root.size))
		if err != nil {
			return err
		}
	}

	usedMini := make(map[uint32]uint32)
	for index, entry := range document.directories {
		if entry.typ != 2 {
			continue
		}
		entryID := uint32(index)
		if entry.size == 0 {
			if entry.start != cfbEndOfChain && entry.start != cfbFreeSector {
				return fmt.Errorf("empty stream %q has an allocated start sector", entry.name)
			}
			continue
		}
		if entry.size < cfbMiniStreamCutoff {
			chain, err := document.followMiniChain(entry.start, sectorsForSize(entry.size, uint64(document.header.miniSectorSize)))
			if err != nil {
				return fmt.Errorf("mini stream %q: %w", entry.name, err)
			}
			for _, sector := range chain {
				if previous, exists := usedMini[sector]; exists {
					return fmt.Errorf("mini sector %d is shared by streams %d and %d", sector, previous, entryID)
				}
				end := (uint64(sector) + 1) * uint64(document.header.miniSectorSize)
				if end > uint64(len(document.rootMiniData)) {
					return fmt.Errorf("mini stream %q is outside the root mini stream", entry.name)
				}
				usedMini[sector] = entryID
			}
			document.miniChain[entryID] = chain
			continue
		}
		chain, err := document.streamRegularChain(entry)
		if err != nil {
			return fmt.Errorf("stream %q: %w", entry.name, err)
		}
		if err := mark("stream "+entry.name, chain); err != nil {
			return err
		}
		document.regularChain[entryID] = chain
	}
	return nil
}

func (document *cfbDocument) streamRegularChain(entry cfbDirectoryEntry) ([]uint32, error) {
	if entry.size == 0 {
		if entry.start != cfbEndOfChain && entry.start != cfbFreeSector {
			return nil, errors.New("empty stream has an allocated start sector")
		}
		return nil, nil
	}
	if entry.size > uint64(document.size) {
		return nil, errors.New("declared stream size exceeds CFB file size")
	}
	return document.followRegularChain(entry.start, sectorsForSize(entry.size, uint64(document.header.sectorSize)))
}

func (document *cfbDocument) followRegularChain(start uint32, expected int64) ([]uint32, error) {
	if expected == -2 {
		return nil, errors.New("version 4 CFB declares zero directory sectors")
	}
	if expected == 0 {
		if start != cfbEndOfChain && start != cfbFreeSector {
			return nil, errors.New("zero-length chain has a start sector")
		}
		return nil, nil
	}
	chain := make([]uint32, 0)
	seen := make(map[uint32]struct{})
	next := start
	for next != cfbEndOfChain {
		if !document.validSector(next) {
			return nil, fmt.Errorf("invalid regular sector %08x", next)
		}
		if _, duplicate := seen[next]; duplicate {
			return nil, errors.New("cycle in regular sector chain")
		}
		seen[next] = struct{}{}
		chain = append(chain, next)
		if int64(len(chain)) > int64(document.sectorCount) || (expected >= 0 && int64(len(chain)) > expected) {
			return nil, errors.New("regular sector chain is longer than expected")
		}
		next = document.fat[next]
	}
	if expected >= 0 && int64(len(chain)) != expected {
		return nil, fmt.Errorf("regular sector chain has %d sectors, want %d", len(chain), expected)
	}
	return chain, nil
}

func (document *cfbDocument) followMiniChain(start uint32, expected int64) ([]uint32, error) {
	if expected <= 0 {
		return nil, errors.New("invalid expected mini-sector count")
	}
	chain := make([]uint32, 0, expected)
	seen := make(map[uint32]struct{})
	next := start
	for next != cfbEndOfChain {
		if int(next) >= len(document.miniFAT) {
			return nil, fmt.Errorf("invalid mini sector %08x", next)
		}
		if _, duplicate := seen[next]; duplicate {
			return nil, errors.New("cycle in mini-sector chain")
		}
		seen[next] = struct{}{}
		chain = append(chain, next)
		if int64(len(chain)) > expected {
			return nil, errors.New("mini-sector chain is longer than expected")
		}
		next = document.miniFAT[next]
	}
	if int64(len(chain)) != expected {
		return nil, fmt.Errorf("mini-sector chain has %d sectors, want %d", len(chain), expected)
	}
	return chain, nil
}

func (document *cfbDocument) readStream(entryID uint32) ([]byte, error) {
	if int(entryID) >= len(document.directories) {
		return nil, errors.New("invalid stream directory ID")
	}
	entry := document.directories[entryID]
	if entry.typ != 2 {
		return nil, errors.New("directory entry is not a stream")
	}
	if entry.size == 0 {
		return nil, nil
	}
	if entry.size < cfbMiniStreamCutoff {
		chain := document.miniChain[entryID]
		data := make([]byte, 0, len(chain)*int(document.header.miniSectorSize))
		for _, sector := range chain {
			start := int64(sector) * document.header.miniSectorSize
			data = append(data, document.rootMiniData[start:start+document.header.miniSectorSize]...)
		}
		return data[:entry.size], nil
	}
	return document.readRegularSectors(document.regularChain[entryID], int64(entry.size))
}

func (document *cfbDocument) readRegularSectors(chain []uint32, wanted int64) ([]byte, error) {
	if wanted < 0 || wanted > int64(len(chain))*document.header.sectorSize {
		return nil, errors.New("invalid regular stream size")
	}
	data := make([]byte, 0, len(chain)*int(document.header.sectorSize))
	for _, sector := range chain {
		sectorData, err := document.readSector(sector)
		if err != nil {
			return nil, err
		}
		data = append(data, sectorData...)
	}
	return data[:wanted], nil
}

func (document *cfbDocument) readSector(sector uint32) ([]byte, error) {
	if !document.validSector(sector) {
		return nil, fmt.Errorf("invalid CFB sector %08x", sector)
	}
	offset := (int64(sector) + 1) * document.header.sectorSize
	data := make([]byte, document.header.sectorSize)
	if _, err := document.reader.ReadAt(data, offset); err != nil {
		return nil, fmt.Errorf("read CFB sector %d: %w", sector, err)
	}
	return data, nil
}

func (document *cfbDocument) validSector(sector uint32) bool {
	return sector < document.sectorCount
}

func (document *cfbDocument) rootStream(name string) (uint32, bool) {
	for index, entry := range document.directories {
		if entry.typ == 2 && entry.parent == 0 && strings.EqualFold(entry.name, name) {
			return uint32(index), true
		}
	}
	return 0, false
}

func (document *cfbDocument) validateOfficeDocument(format Format) error {
	if document.containsMacroStorage() {
		return ErrMacroEnabled
	}
	if document.containsEncryptionStream() {
		return ErrEncryptedDocument
	}

	wordID, hasWord := document.rootStream("WordDocument")
	workbookID, hasWorkbook := document.rootStream("Workbook")
	if !hasWorkbook {
		workbookID, hasWorkbook = document.rootStream("Book")
	}
	powerPointID, hasPowerPoint := document.rootStream("PowerPoint Document")
	familyCount := 0
	for _, found := range []bool{hasWord, hasWorkbook, hasPowerPoint} {
		if found {
			familyCount++
		}
	}
	if familyCount != 1 {
		return fmt.Errorf("%w: OLE container has %d recognizable Office document streams", ErrContentMismatch, familyCount)
	}

	switch format {
	case FormatDOC:
		if !hasWord {
			return fmt.Errorf("%w: OLE container is not a Word document", ErrContentMismatch)
		}
		return document.validateWordStream(wordID)
	case FormatXLS:
		if !hasWorkbook {
			return fmt.Errorf("%w: OLE container is not an Excel workbook", ErrContentMismatch)
		}
		return document.validateWorkbookStream(workbookID)
	case FormatPPT:
		if !hasPowerPoint {
			return fmt.Errorf("%w: OLE container is not a PowerPoint presentation", ErrContentMismatch)
		}
		return document.validatePowerPointStream(powerPointID)
	default:
		return ErrUnsupportedFormat
	}
}

func (document *cfbDocument) containsMacroStorage() bool {
	for _, entry := range document.directories {
		name := strings.ToLower(entry.name)
		if entry.typ == 1 && (name == "vba" || name == "_vba_project_cur") {
			return true
		}
		if entry.typ == 2 && (name == "_vba_project" || name == "vba project") {
			return true
		}
	}
	return false
}

func (document *cfbDocument) containsEncryptionStream() bool {
	for _, entry := range document.directories {
		if entry.typ != 2 {
			continue
		}
		switch strings.ToLower(entry.name) {
		case "encryptioninfo", "encryptedpackage", "encryptedsummary", "strongencryptiondataspace":
			return true
		}
	}
	return false
}

func (document *cfbDocument) validateWordStream(entryID uint32) error {
	data, err := document.readStream(entryID)
	if err != nil {
		return fmt.Errorf("%w: read WordDocument stream: %v", ErrInvalidOLE, err)
	}
	if len(data) < 12 || binary.LittleEndian.Uint16(data[:2]) != 0xa5ec {
		return fmt.Errorf("%w: invalid Word FIB header", ErrContentMismatch)
	}
	flags := binary.LittleEndian.Uint16(data[10:12])
	if flags&(1<<8) != 0 || flags&(1<<15) != 0 {
		return ErrEncryptedDocument
	}
	return nil
}

func (document *cfbDocument) validateWorkbookStream(entryID uint32) error {
	data, err := document.readStream(entryID)
	if err != nil {
		return fmt.Errorf("%w: read workbook stream: %v", ErrInvalidOLE, err)
	}
	if len(data) < 8 || !isBIFFBOF(binary.LittleEndian.Uint16(data[:2])) {
		return fmt.Errorf("%w: invalid Excel BIFF BOF record", ErrContentMismatch)
	}

	position := 0
	foundEOF := false
	for position+4 <= len(data) {
		recordType := binary.LittleEndian.Uint16(data[position : position+2])
		recordSize := int(binary.LittleEndian.Uint16(data[position+2 : position+4]))
		if recordType == 0 && recordSize == 0 {
			if !allZero(data[position:]) {
				return fmt.Errorf("%w: invalid Excel BIFF padding", ErrInvalidOLE)
			}
			break
		}
		position += 4
		if recordSize > len(data)-position {
			return fmt.Errorf("%w: truncated Excel BIFF record", ErrInvalidOLE)
		}
		record := data[position : position+recordSize]
		if recordType == 0x002f {
			return ErrEncryptedDocument
		}
		if isBIFFBOF(recordType) {
			if len(record) < 4 {
				return fmt.Errorf("%w: truncated Excel BIFF BOF record", ErrInvalidOLE)
			}
			substreamType := binary.LittleEndian.Uint16(record[2:4])
			if substreamType == 0x0040 || substreamType == 0x0006 {
				return ErrMacroEnabled
			}
		}
		if recordType == 0x000a {
			foundEOF = true
		}
		position += recordSize
	}
	if !foundEOF {
		return fmt.Errorf("%w: Excel BIFF stream has no EOF record", ErrInvalidOLE)
	}
	return nil
}

func isBIFFBOF(recordType uint16) bool {
	switch recordType {
	case 0x0009, 0x0209, 0x0409, 0x0809:
		return true
	default:
		return false
	}
}

func (document *cfbDocument) validatePowerPointStream(entryID uint32) error {
	data, err := document.readStream(entryID)
	if err != nil {
		return fmt.Errorf("%w: read PowerPoint Document stream: %v", ErrInvalidOLE, err)
	}
	if len(data) < 8 {
		return fmt.Errorf("%w: truncated PowerPoint record header", ErrContentMismatch)
	}
	versionAndInstance := binary.LittleEndian.Uint16(data[:2])
	recordType := binary.LittleEndian.Uint16(data[2:4])
	recordSize := binary.LittleEndian.Uint32(data[4:8])
	if versionAndInstance&0x000f != 0x000f || recordType != 0x03e8 || uint64(recordSize) > uint64(len(data)-8) {
		return fmt.Errorf("%w: invalid PowerPoint Document record", ErrContentMismatch)
	}
	return nil
}

func sectorsForSize(size, sectorSize uint64) int64 {
	return int64((size + sectorSize - 1) / sectorSize)
}

func hasDuplicateSectors(sectors []uint32) bool {
	seen := make(map[uint32]struct{}, len(sectors))
	for _, sector := range sectors {
		if _, duplicate := seen[sector]; duplicate {
			return true
		}
		seen[sector] = struct{}{}
	}
	return false
}

func allZero(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
}
