package services

import (
	"bbs-go/internal/models"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxKalshiMarkdownLength = 16 * 1024 * 1024

var ErrKalshiInvalid = errors.New("invalid Kalshi document")
var ErrKalshiStale = errors.New("an older Kalshi snapshot cannot replace the published report")
var errKalshiConflict = errors.New("concurrent Kalshi update")

type kalshiFile struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type KalshiManifest struct {
	SchemaVersion    string                `json:"schema_version"`
	Date             string                `json:"date"`
	AsOf             string                `json:"as_of"`
	PublicationReady bool                  `json:"publication_ready"`
	CollectionStatus string                `json:"collection_status"`
	Files            map[string]kalshiFile `json:"files"`
	Counts           struct {
		SignalsTotal int `json:"signals_total"`
		TrendRows    int `json:"trend_rows"`
	} `json:"counts"`
	Metadata struct {
		RunSlot string `json:"run_slot"`
	} `json:"metadata"`
}

type KalshiDocument struct {
	Date             time.Time
	Slot             string
	AsOf             time.Time
	CollectionStatus string
	Manifest         json.RawMessage
	Signals          json.RawMessage
	Trends           json.RawMessage
	Narrative        string
}

type KalshiPublishStatus struct {
	Date        string `json:"date"`
	Slot        string `json:"slot"`
	Published   bool   `json:"published"`
	ContentHash string `json:"content_hash,omitempty"`
	AsOf        string `json:"as_of,omitempty"`
}

// Extract only named fenced payloads. Never execute commands or HTML from the document.
func kalshiBlock(source, language, name string) (string, error) {
	pattern := regexp.MustCompile("(?ms)^```" + language + " " + regexp.QuoteMeta(name) + "\\r?\\n(.*?)^```\\r?$")
	matches := pattern.FindAllStringSubmatch(source, -1)
	if len(matches) != 1 {
		return "", fmt.Errorf("%w: expected exactly one %s block", ErrKalshiInvalid, name)
	}
	return matches[0][1], nil
}

func ParseKalshiDocument(source string) (KalshiDocument, error) {
	if len(source) > MaxKalshiMarkdownLength {
		return KalshiDocument{}, fmt.Errorf("%w: Markdown exceeds 16 MiB", ErrKalshiInvalid)
	}
	if len(source) == 0 || !utf8.ValidString(source) {
		return KalshiDocument{}, fmt.Errorf("%w: empty or non-UTF-8 Markdown", ErrKalshiInvalid)
	}
	manifestBlock, err := kalshiBlock(source, "json", "manifest.json")
	if err != nil {
		return KalshiDocument{}, err
	}
	var manifest KalshiManifest
	if err := json.Unmarshal([]byte(manifestBlock), &manifest); err != nil {
		return KalshiDocument{}, fmt.Errorf("%w: manifest JSON: %v", ErrKalshiInvalid, err)
	}
	if manifest.SchemaVersion != "kalshi-dashboard-handoff/2" || !manifest.PublicationReady {
		return KalshiDocument{}, fmt.Errorf("%w: unsupported schema or publication_ready=false", ErrKalshiInvalid)
	}
	date, err := time.Parse(time.DateOnly, manifest.Date)
	if err != nil || date.Format(time.DateOnly) != manifest.Date {
		return KalshiDocument{}, fmt.Errorf("%w: invalid manifest date", ErrKalshiInvalid)
	}
	asOf, err := time.Parse(time.RFC3339Nano, manifest.AsOf)
	if err != nil || asOf.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.DateOnly) != manifest.Date {
		return KalshiDocument{}, fmt.Errorf("%w: invalid as_of or date mismatch", ErrKalshiInvalid)
	}
	if manifest.CollectionStatus != "complete" && manifest.CollectionStatus != "partial" {
		return KalshiDocument{}, fmt.Errorf("%w: unknown collection_status", ErrKalshiInvalid)
	}
	if !ValidKalshiSlot(manifest.Metadata.RunSlot) {
		return KalshiDocument{}, fmt.Errorf("%w: invalid metadata.run_slot", ErrKalshiInvalid)
	}
	blocks := make(map[string]json.RawMessage, 2)
	for _, file := range []struct{ name, language string }{{"data/signals.json", "json"}, {"data/trends.json", "json"}, {"index.html", "html"}} {
		content, err := kalshiBlock(source, file.language, file.name)
		if err != nil {
			return KalshiDocument{}, err
		}
		expected, ok := manifest.Files[file.name]
		digest := sha256.Sum256([]byte(content))
		if !ok || len(content) != expected.Bytes || hex.EncodeToString(digest[:]) != expected.SHA256 {
			return KalshiDocument{}, fmt.Errorf("%w: %s integrity mismatch", ErrKalshiInvalid, file.name)
		}
		if file.language == "json" {
			var payload struct {
				Rows []json.RawMessage `json:"rows"`
				Meta json.RawMessage   `json:"meta"`
			}
			if err := json.Unmarshal([]byte(content), &payload); err != nil || payload.Rows == nil || !json.Valid(payload.Meta) {
				return KalshiDocument{}, fmt.Errorf("%w: invalid %s rows or meta", ErrKalshiInvalid, file.name)
			}
			if file.name == "data/signals.json" && len(payload.Rows) != manifest.Counts.SignalsTotal || file.name == "data/trends.json" && len(payload.Rows) != manifest.Counts.TrendRows {
				return KalshiDocument{}, fmt.Errorf("%w: %s row count mismatch", ErrKalshiInvalid, file.name)
			}
			blocks[file.name] = json.RawMessage(content)
		}
	}
	narrative := source
	if index := strings.Index(source, "\n## 数据缺口与来源\n"); index >= 0 {
		narrative = source[:index]
	}
	return KalshiDocument{Date: date, Slot: manifest.Metadata.RunSlot, AsOf: asOf, CollectionStatus: manifest.CollectionStatus, Manifest: json.RawMessage(manifestBlock), Signals: blocks["data/signals.json"], Trends: blocks["data/trends.json"], Narrative: narrative}, nil
}

func ValidKalshiSlot(slot string) bool { return slot == "morning" || slot == "evening" }

func KalshiDates() ([]string, error) {
	var datesList []time.Time
	err := sqls.DB().Model(&models.KalshiReport{}).Where("slot IN ?", []string{"morning", "evening"}).Distinct().Order("report_date DESC").Pluck("report_date", &datesList).Error
	result := make([]string, 0, len(datesList))
	for _, date := range datesList {
		result = append(result, date.Format(time.DateOnly))
	}
	return result, err
}

func KalshiGet(date time.Time, slot string) (*models.KalshiReport, error) {
	var report models.KalshiReport
	err := sqls.DB().Where("report_date = ? AND slot = ?", date, slot).Take(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &report, err
}

func KalshiStatus(date time.Time, slot string) (KalshiPublishStatus, error) {
	status := KalshiPublishStatus{Date: date.Format(time.DateOnly), Slot: slot}
	var report models.KalshiReport
	err := sqls.DB().Model(&models.KalshiReport{}).Select("content_hash", "as_of").Where("report_date = ? AND slot = ?", date, slot).Take(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Published = true
	status.ContentHash = report.ContentHash
	status.AsOf = report.AsOf.Format(time.RFC3339)
	return status, nil
}

type KalshiSnapshot struct {
	Slot             string `json:"slot"`
	AsOf             string `json:"as_of"`
	CollectionStatus string `json:"collection_status"`
}

func KalshiSnapshots(date time.Time) ([]KalshiSnapshot, error) {
	var reports []models.KalshiReport
	if err := sqls.DB().Model(&models.KalshiReport{}).Select("id", "slot", "as_of", "collection_status").Where("report_date = ? AND slot IN ?", date, []string{"morning", "evening"}).Order("slot ASC").Find(&reports).Error; err != nil {
		return nil, err
	}
	snapshots := make([]KalshiSnapshot, 0, len(reports))
	for _, report := range reports {
		status := report.CollectionStatus
		if status == "" {
			// Existing rows predate the summary column. Backfill each once.
			var legacy models.KalshiReport
			if err := sqls.DB().Model(&models.KalshiReport{}).Select("markdown", "content_hash").Where("id = ?", report.Id).Take(&legacy).Error; err != nil {
				return nil, err
			}
			block, err := kalshiBlock(legacy.Markdown, "json", "manifest.json")
			if err != nil {
				return nil, err
			}
			var manifest struct {
				CollectionStatus string `json:"collection_status"`
			}
			if err := json.Unmarshal([]byte(block), &manifest); err != nil || (manifest.CollectionStatus != "complete" && manifest.CollectionStatus != "partial") {
				return nil, fmt.Errorf("%w: invalid stored collection_status", ErrKalshiInvalid)
			}
			status = manifest.CollectionStatus
			if err := sqls.DB().Model(&models.KalshiReport{}).Where("id = ? AND content_hash = ? AND collection_status = ?", report.Id, legacy.ContentHash, "").Update("collection_status", status).Error; err != nil {
				return nil, err
			}
		}
		snapshots = append(snapshots, KalshiSnapshot{Slot: report.Slot, AsOf: report.AsOf.Format(time.RFC3339), CollectionStatus: status})
	}
	return snapshots, nil
}

func KalshiSave(source string) (string, string, string, string, error) {
	doc, err := ParseKalshiDocument(source)
	if err != nil {
		return "", "", "", "", err
	}
	digest := sha256.Sum256([]byte(source))
	hash := hex.EncodeToString(digest[:])
	for attempt := 0; attempt < 5; attempt++ {
		result := "created"
		err = sqls.DB().Transaction(func(tx *gorm.DB) error {
			var current models.KalshiReport
			found := tx.Where("report_date = ? AND slot = ?", doc.Date, doc.Slot).Take(&current).Error
			if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
				return found
			}
			now := dates.NowTimestamp()
			if errors.Is(found, gorm.ErrRecordNotFound) {
				insert := &models.KalshiReport{ReportDate: doc.Date, Slot: doc.Slot, AsOf: doc.AsOf, Markdown: source, ContentHash: hash, CollectionStatus: doc.CollectionStatus, CreateTime: now, UpdateTime: now}
				created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(insert)
				if created.Error != nil {
					return created.Error
				}
				if created.RowsAffected == 0 {
					return errKalshiConflict
				}
				return nil
			}
			if current.ContentHash == hash {
				result = "unchanged"
				return nil
			}
			if !doc.AsOf.After(current.AsOf) {
				return ErrKalshiStale
			}
			if err := tx.Create(&models.KalshiReportRevision{ReportId: current.Id, AsOf: current.AsOf, Markdown: current.Markdown, ContentHash: current.ContentHash, ReplacedAt: now}).Error; err != nil {
				return err
			}
			updated := tx.Model(&models.KalshiReport{}).Where("id = ? AND content_hash = ? AND as_of < ?", current.Id, current.ContentHash, doc.AsOf).
				Updates(map[string]any{"as_of": doc.AsOf, "markdown": source, "content_hash": hash, "collection_status": doc.CollectionStatus, "update_time": now})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return errKalshiConflict
			}
			result = "updated"
			return nil
		})
		if errors.Is(err, errKalshiConflict) {
			continue
		}
		return result, doc.Date.Format(time.DateOnly), doc.Slot, hash, err
	}
	return "", "", "", "", errKalshiConflict
}
