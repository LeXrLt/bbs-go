package services

import (
	"bbs-go/internal/models"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestDailyReviewTypesUseStableDisplayOrder(t *testing.T) {
	want := []string{"focus", "shareholding", "announcements", "movers", "buybacks", "shorts"}
	types := DailyReviewService.Types()
	if len(types) != len(want) {
		t.Fatalf("types length = %d, want %d", len(types), len(want))
	}
	for i, kind := range want {
		if types[i].Kind != kind {
			t.Fatalf("types[%d].Kind = %q, want %q", i, types[i].Kind, kind)
		}
	}
	if types[0].Label != "重点公司变化" || types[1].Label != "增持" || types[3].Label != "涨跌幅" {
		t.Fatalf("unexpected display labels: %#v", types)
	}
}

func TestDailyReviewSaveIsIdempotentAndCreatesRevisionOnChange(t *testing.T) {
	db := setupDailyReviewServiceTestDB(t)
	reportDate := mustDailyReviewDate(t, "2026-10-28")
	first := "# 港股增持\n\n| 公司 | 金额 |\n| --- | --- |\n| A | 1 |"

	created, err := DailyReviewService.Save(reportDate, "shareholding", first)
	if err != nil {
		t.Fatalf("create daily review: %v", err)
	}
	if created.Result != "created" {
		t.Fatalf("create result = %q", created.Result)
	}
	unchanged, err := DailyReviewService.Save(reportDate, "shareholding", first)
	if err != nil {
		t.Fatalf("repeat daily review: %v", err)
	}
	if unchanged.Result != "unchanged" {
		t.Fatalf("repeat result = %q", unchanged.Result)
	}

	updated, err := DailyReviewService.Save(reportDate, "shareholding", first+"\n| B | 2 |")
	if err != nil {
		t.Fatalf("update daily review: %v", err)
	}
	if updated.Result != "updated" {
		t.Fatalf("update result = %q", updated.Result)
	}

	var revisionCount int64
	if err := db.Model(&models.DailyReviewReportRevision{}).Count(&revisionCount).Error; err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if revisionCount != 1 {
		t.Fatalf("revision count = %d, want 1", revisionCount)
	}
	var revision models.DailyReviewReportRevision
	if err := db.First(&revision).Error; err != nil {
		t.Fatalf("load revision: %v", err)
	}
	if revision.Markdown != first+"\n" || revision.ContentHash == "" || revision.Title != "港股增持" {
		t.Fatalf("unexpected revision: %#v", revision)
	}
}

func TestDailyReviewSaveConcurrentSameContentIsIdempotent(t *testing.T) {
	db := setupDailyReviewServiceTestDB(t)
	reportDate := mustDailyReviewDate(t, "2026-10-28")
	results, errs := saveDailyReviewsConcurrently(reportDate, "focus", []string{
		"# 重点公司\n\n【公司A】",
		"# 重点公司\n\n【公司A】",
	})
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent save: %v", err)
		}
	}
	resultCounts := map[string]int{}
	for _, result := range results {
		resultCounts[result.Result]++
	}
	if resultCounts["created"] != 1 || resultCounts["unchanged"] != 1 {
		t.Fatalf("unexpected concurrent results: %#v", results)
	}
	var reportCount, revisionCount int64
	if err := db.Model(&models.DailyReviewReport{}).Count(&reportCount).Error; err != nil {
		t.Fatalf("count reports: %v", err)
	}
	if err := db.Model(&models.DailyReviewReportRevision{}).Count(&revisionCount).Error; err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if reportCount != 1 || revisionCount != 0 {
		t.Fatalf("report count=%d revision count=%d", reportCount, revisionCount)
	}
}

func TestDailyReviewSaveConcurrentDifferentContentKeepsRevision(t *testing.T) {
	db := setupDailyReviewServiceTestDB(t)
	reportDate := mustDailyReviewDate(t, "2026-10-28")
	contents := []string{
		"# 重点公司\n\n【公司A】",
		"# 重点公司\n\n【公司B】",
	}
	results, errs := saveDailyReviewsConcurrently(reportDate, "focus", contents)
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent save: %v", err)
		}
	}
	resultCounts := map[string]int{}
	for _, result := range results {
		resultCounts[result.Result]++
	}
	if resultCounts["created"] != 1 || resultCounts["updated"] != 1 {
		t.Fatalf("unexpected concurrent results: %#v", results)
	}
	var revisionCount int64
	if err := db.Model(&models.DailyReviewReportRevision{}).Count(&revisionCount).Error; err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if revisionCount != 1 {
		t.Fatalf("revision count=%d, want 1", revisionCount)
	}
	var current models.DailyReviewReport
	if err := db.First(&current).Error; err != nil {
		t.Fatalf("load current report: %v", err)
	}
	var revision models.DailyReviewReportRevision
	if err := db.First(&revision).Error; err != nil {
		t.Fatalf("load revision: %v", err)
	}
	seen := map[string]bool{
		strings.TrimSpace(current.Markdown):  true,
		strings.TrimSpace(revision.Markdown): true,
	}
	for _, content := range contents {
		if !seen[content] {
			t.Fatalf("content %q missing from current/revision: current=%q revision=%q", content, current.Markdown, revision.Markdown)
		}
	}
}

func TestDailyReviewSaveTruncatesLongTitleByRunes(t *testing.T) {
	setupDailyReviewServiceTestDB(t)
	reportDate := mustDailyReviewDate(t, "2026-10-28")
	longTitle := strings.Repeat("港", maxDailyReviewTitleRunes+5)
	if _, err := DailyReviewService.Save(reportDate, "focus", "# "+longTitle+"\n\n内容"); err != nil {
		t.Fatalf("save long title: %v", err)
	}
	report, err := DailyReviewService.Get(reportDate, "focus")
	if err != nil {
		t.Fatalf("get report: %v", err)
	}
	if got := len([]rune(report.Title)); got != maxDailyReviewTitleRunes {
		t.Fatalf("title rune count=%d, want %d", got, maxDailyReviewTitleRunes)
	}
}

func TestDailyReviewGetPropagatesDatabaseErrors(t *testing.T) {
	db := setupDailyReviewServiceTestDB(t)
	if err := db.Migrator().DropTable(&models.DailyReviewReport{}); err != nil {
		t.Fatalf("drop reports table: %v", err)
	}
	if _, err := DailyReviewService.Get(mustDailyReviewDate(t, "2026-10-28"), "focus"); err == nil {
		t.Fatal("expected database error")
	}
}

func TestDailyReviewDashboardKeepsMissingSectionsPending(t *testing.T) {
	setupDailyReviewServiceTestDB(t)
	reportDate := mustDailyReviewDate(t, "2026-10-28")
	if _, err := DailyReviewService.Save(reportDate, "focus", "# 重点公司\n\n【公司A】\n\n内容"); err != nil {
		t.Fatalf("save focus report: %v", err)
	}

	dashboard, err := DailyReviewService.Dashboard(reportDate)
	if err != nil {
		t.Fatalf("load dashboard: %v", err)
	}
	if len(dashboard.Sections) != 6 {
		t.Fatalf("sections length = %d", len(dashboard.Sections))
	}
	for i, section := range dashboard.Sections {
		if section.Kind != DailyReviewService.Types()[i].Kind {
			t.Fatalf("section order mismatch at %d: %#v", i, section)
		}
		if section.Kind == "focus" {
			if section.Status != "ready" || len(section.Metrics) != 1 || section.Metrics[0].Value != "1" {
				t.Fatalf("unexpected focus section: %#v", section)
			}
		} else if section.Status != "pending" {
			t.Fatalf("missing section %q status = %q", section.Kind, section.Status)
		}
	}
}

func TestDailyReviewSummaryCountsParsedMarkdownTables(t *testing.T) {
	summary := buildDailyReviewSummary("announcements", `# 公告筛选

## 值得关注

| 公司 | 标题 |
| --- | --- |
| A | 一 |
| B | 二 |

## 其他符合口径

| 公司 | 标题 |
| --- | --- |
| C | 三 |
`)
	if len(summary.Metrics) != 2 || summary.Metrics[0].Value != "2" || summary.Metrics[1].Value != "1" {
		t.Fatalf("unexpected metrics: %#v", summary.Metrics)
	}
}

func setupDailyReviewServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:daily_review_service_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)
	if err := db.AutoMigrate(&models.DailyReviewReport{}, &models.DailyReviewReportRevision{}); err != nil {
		t.Fatalf("migrate daily review models: %v", err)
	}
	return db
}

func mustDailyReviewDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	return parsed
}

func saveDailyReviewsConcurrently(reportDate time.Time, kind string, contents []string) ([]DailyReviewSaveResult, []error) {
	results := make([]DailyReviewSaveResult, len(contents))
	errs := make([]error, len(contents))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, content := range contents {
		wait.Add(1)
		go func(index int, content string) {
			defer wait.Done()
			<-start
			results[index], errs[index] = DailyReviewService.Save(reportDate, kind, content)
		}(index, content)
	}
	close(start)
	wait.Wait()
	return results, errs
}
