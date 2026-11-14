package services

import (
	"bbs-go/internal/models"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func kalshiFixture(t *testing.T, asOf, slot, suffix string) string {
	t.Helper()
	files := map[string]string{
		"data/signals.json": `{"rows":[{"title_zh":"线索","note":"` + suffix + `"}],"meta":{}}` + "\n",
		"data/trends.json":  `{"rows":[{"ts":"2026-11-09T10:00:00Z","close":null}],"meta":{}}` + "\n",
		"index.html":        "<script>doNotExecute()</script>\n",
	}
	manifest := map[string]any{
		"schema_version": "kalshi-dashboard-handoff/2", "date": "2026-11-09", "as_of": asOf,
		"publication_ready": true, "collection_status": "complete",
		"counts":   map[string]int{"signals_total": 1, "trend_rows": 1},
		"metadata": map[string]any{"scope": "test", "run_slot": slot},
		"files":    map[string]kalshiFile{},
	}
	for name, content := range files {
		digest := sha256.Sum256([]byte(content))
		manifest["files"].(map[string]kalshiFile)[name] = kalshiFile{SHA256: hex.EncodeToString(digest[:]), Bytes: len(content)}
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return "# Kalshi\n\n线索摘要 " + suffix + "\n\n```json manifest.json\n" + string(raw) + "\n```\n\n" +
		"```json data/signals.json\n" + files["data/signals.json"] + "```\n\n" +
		"```json data/trends.json\n" + files["data/trends.json"] + "```\n\n" +
		"```html index.html\n" + files["index.html"] + "```\n"
}

func setupKalshiDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:kalshi_service_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true}, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)
	if err := db.AutoMigrate(&models.KalshiReport{}, &models.KalshiReportRevision{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestKalshiSaveCreateUpdateAndDeduplicate(t *testing.T) {
	db := setupKalshiDB(t)
	date := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	status, err := KalshiStatus(date, "evening")
	if err != nil || status.Published || status.ContentHash != "" || status.AsOf != "" || status.Date != "2026-11-09" || status.Slot != "evening" {
		t.Fatalf("unpublished status: %#v %v", status, err)
	}
	first := kalshiFixture(t, "2026-11-09T18:00:00+08:00", "evening", "first")
	second := kalshiFixture(t, "2026-11-09T19:00:00+08:00", "evening", "second")
	for _, step := range []struct{ source, want string }{{first, "created"}, {first, "unchanged"}, {second, "updated"}, {second, "unchanged"}} {
		result, date, slot, hash, err := KalshiSave(step.source)
		if err != nil || result != step.want || date != "2026-11-09" || slot != "evening" || len(hash) != 64 {
			t.Fatalf("save: result=%q date=%q slot=%q err=%v", result, date, slot, err)
		}
	}
	if _, _, _, _, err := KalshiSave(kalshiFixture(t, "2026-11-09T19:00:00+08:00", "evening", "same-time-changed")); !errors.Is(err, ErrKalshiStale) {
		t.Fatalf("same timestamp changed: %v", err)
	}
	if _, _, _, _, err := KalshiSave(kalshiFixture(t, "2026-11-09T17:00:00+08:00", "evening", "stale")); !errors.Is(err, ErrKalshiStale) {
		t.Fatalf("older snapshot: %v", err)
	}
	report, err := KalshiGet(date, "evening")
	if err != nil || report == nil || report.Markdown != second {
		t.Fatalf("current report: %#v %v", report, err)
	}
	status, err = KalshiStatus(date, "evening")
	digest := sha256.Sum256([]byte(second))
	if err != nil || !status.Published || status.ContentHash != hex.EncodeToString(digest[:]) || status.AsOf != "2026-11-09T19:00:00+08:00" {
		t.Fatalf("published status: %#v %v", status, err)
	}
	var revisions []models.KalshiReportRevision
	if err := db.Find(&revisions).Error; err != nil || len(revisions) != 1 || revisions[0].Markdown != first {
		t.Fatalf("revisions: %#v %v", revisions, err)
	}
	dates, err := KalshiDates()
	if err != nil || len(dates) != 1 || dates[0] != "2026-11-09" {
		t.Fatalf("dates: %v %v", dates, err)
	}
	morning := kalshiFixture(t, "2026-11-09T08:00:00+08:00", "morning", "morning")
	if result, _, _, _, err := KalshiSave(morning); err != nil || result != "created" {
		t.Fatalf("morning save: %s %v", result, err)
	}
	if dates, err := KalshiDates(); err != nil || len(dates) != 1 {
		t.Fatalf("duplicate date: %v %v", dates, err)
	}
	if morningReport, err := KalshiGet(date, "morning"); err != nil || morningReport == nil || morningReport.Markdown != morning {
		t.Fatalf("morning report: %v %v", morningReport, err)
	}
	if eveningReport, err := KalshiGet(date, "evening"); err != nil || eveningReport == nil || eveningReport.Markdown != second {
		t.Fatalf("evening changed: %v %v", eveningReport, err)
	}
	snapshots, err := KalshiSnapshots(date)
	if err != nil || len(snapshots) != 2 || snapshots[0].Slot != "evening" || snapshots[1].Slot != "morning" {
		t.Fatalf("snapshots: %+v %v", snapshots, err)
	}
	if status, err := KalshiStatus(date, "morning"); err != nil || !status.Published || status.Slot != "morning" {
		t.Fatalf("morning status: %+v %v", status, err)
	}
	if status, err := KalshiStatus(date, "evening"); err != nil || status.ContentHash != hex.EncodeToString(digest[:]) {
		t.Fatalf("evening status changed: %+v %v", status, err)
	}
	if err := db.Find(&revisions).Error; err != nil || len(revisions) != 1 {
		t.Fatalf("cross-slot revisions: %+v %v", revisions, err)
	}
	partial := strings.Replace(kalshiFixture(t, "2026-11-09T09:00:00+08:00", "morning", "partial gap"), `"collection_status":"complete"`, `"collection_status":"partial"`, 1)
	if result, _, _, _, err := KalshiSave(partial); err != nil || result != "updated" {
		t.Fatalf("morning partial update: %s %v", result, err)
	}
	snapshots, err = KalshiSnapshots(date)
	if err != nil || len(snapshots) != 2 || snapshots[1].CollectionStatus != "partial" || snapshots[0].CollectionStatus != "complete" {
		t.Fatalf("independent coverage: %+v %v", snapshots, err)
	}
	if eveningReport, err := KalshiGet(date, "evening"); err != nil || eveningReport.Markdown != second {
		t.Fatalf("evening overwritten by morning: %v %v", eveningReport, err)
	}
	if err := db.Find(&revisions).Error; err != nil || len(revisions) != 2 {
		t.Fatalf("independent revisions: %+v %v", revisions, err)
	}
}

func TestKalshiParserRejectsBrokenPayloads(t *testing.T) {
	valid := kalshiFixture(t, "2026-11-09T19:00:00+08:00", "evening", "safe")
	for _, source := range []string{
		strings.Replace(valid, "线索摘要", "duplicate\n```json manifest.json\n{}\n```\n线索摘要", 1),
		strings.Replace(valid, "doNotExecute", "tampered", 1),
		strings.Replace(valid, `"publication_ready":true`, `"publication_ready":false`, 1),
		strings.Replace(valid, `"signals_total":1`, `"signals_total":2`, 1),
		strings.Replace(valid, `"run_slot":"evening"`, `"run_slot":"unknown"`, 1),
		strings.Replace(valid, `"run_slot":"evening"`, `"scope":"test"`, 1),
		kalshiFixture(t, "2026-11-08T19:00:00+08:00", "evening", "bad date"),
	} {
		if _, err := ParseKalshiDocument(source); !errors.Is(err, ErrKalshiInvalid) {
			t.Fatalf("expected invalid payload, got %v", err)
		}
	}
}

func TestKalshiLargeReportRoundTripAndLimits(t *testing.T) {
	setupKalshiDB(t)
	date := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	largeReport := func(asOf, suffix string) string {
		base := kalshiFixture(t, asOf, "evening", suffix)
		return base + strings.Repeat("x", 2_225_309-len(base))
	}
	first := largeReport("2026-11-09T18:00:00+08:00", "first")
	second := largeReport("2026-11-09T19:00:00+08:00", "second")
	for _, step := range []struct{ source, want string }{{first, "created"}, {first, "unchanged"}, {second, "updated"}, {second, "unchanged"}} {
		result, gotDate, slot, hash, err := KalshiSave(step.source)
		digest := sha256.Sum256([]byte(step.source))
		if err != nil || result != step.want || gotDate != "2026-11-09" || slot != "evening" || hash != hex.EncodeToString(digest[:]) {
			t.Fatalf("save large report: result=%s date=%s slot=%s hash=%s err=%v", result, gotDate, slot, hash, err)
		}
	}
	report, err := KalshiGet(date, "evening")
	if err != nil || report == nil || report.Markdown != second {
		t.Fatalf("large report was not stored intact: %v", err)
	}
	status, err := KalshiStatus(date, "evening")
	digest := sha256.Sum256([]byte(second))
	if err != nil || !status.Published || status.ContentHash != hex.EncodeToString(digest[:]) {
		t.Fatalf("large report status hash: %+v %v", status, err)
	}
	var revision models.KalshiReportRevision
	if err := sqls.DB().Where("report_id = ?", report.Id).Take(&revision).Error; err != nil || revision.Markdown != first {
		t.Fatalf("large report revision was not stored intact: %v", err)
	}

	base := kalshiFixture(t, "2026-11-09T19:00:00+08:00", "evening", "boundary")
	// Keep the embedded manifest and block hashes valid; only pad the narrative.
	atLimit := base + strings.Repeat("x", MaxKalshiMarkdownLength-len(base))
	if _, err := ParseKalshiDocument(atLimit); err != nil {
		t.Fatalf("16 MiB report rejected: %v", err)
	}
	if _, err := ParseKalshiDocument(atLimit + "x"); !errors.Is(err, ErrKalshiInvalid) || !strings.Contains(err.Error(), "16 MiB") {
		t.Fatalf("over-limit report accepted or error unclear: %v", err)
	}
}

func TestKalshiRealHandoffWhenProvided(t *testing.T) {
	path := os.Getenv("KALSHI_SAMPLE_FILE")
	if path == "" {
		t.Skip("set KALSHI_SAMPLE_FILE to check a real handoff")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ParseKalshiDocument(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Signals) == 0 || len(doc.Trends) == 0 || !strings.Contains(doc.Narrative, "本日线索") {
		t.Fatal("real handoff content is incomplete")
	}
}
