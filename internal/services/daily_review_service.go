package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/pkg/markdown"
	"bbs-go/internal/repositories"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
)

const (
	MaxDailyReviewMarkdownLength = 1_000_000
	maxDailyReviewTitleRunes     = 250
	maxDailyReviewSaveAttempts   = 5
)

var errDailyReviewWriteConflict = errors.New("daily review write conflict")

type DailyReviewValidationError struct {
	message string
}

func (e *DailyReviewValidationError) Error() string {
	return e.message
}

func IsDailyReviewValidationError(err error) bool {
	var validationError *DailyReviewValidationError
	return errors.As(err, &validationError)
}

type DailyReviewType struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

type DailyReviewMetric struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type DailyReviewSummary struct {
	Metrics []DailyReviewMetric `json:"metrics"`
	NoItems bool                `json:"no_items"`
}

type DailyReviewSection struct {
	Kind      string              `json:"kind"`
	Label     string              `json:"label"`
	Icon      string              `json:"icon"`
	Status    string              `json:"status"`
	Metrics   []DailyReviewMetric `json:"metrics"`
	UpdatedAt string              `json:"updated_at,omitempty"`
}

type DailyReviewDashboard struct {
	Date     string               `json:"date"`
	Sections []DailyReviewSection `json:"sections"`
}

type DailyReviewDates struct {
	Dates  []string `json:"dates"`
	Latest string   `json:"latest,omitempty"`
}

type DailyReviewSaveResult struct {
	Result string `json:"result"`
	Kind   string `json:"kind"`
	Date   string `json:"date"`
}

var dailyReviewTypes = []DailyReviewType{
	{Kind: "shareholding", Label: "港股增持", Icon: "trending-up"},
	{Kind: "announcements", Label: "公告筛选", Icon: "file-text"},
	{Kind: "movers", Label: "港股涨跌幅", Icon: "chart-no-axes-combined"},
	{Kind: "focus", Label: "重点公司", Icon: "building-2"},
	{Kind: "buybacks", Label: "港股回购", Icon: "rotate-ccw"},
	{Kind: "shorts", Label: "港股做空", Icon: "activity"},
}

var focusCompanyPattern = regexp.MustCompile(`(?m)^【[^】]+】\s*$`)

var DailyReviewService = newDailyReviewService()

func newDailyReviewService() *dailyReviewService {
	return &dailyReviewService{}
}

type dailyReviewService struct{}

var dailyReviewSaveMu sync.Mutex

func (s *dailyReviewService) Types() []DailyReviewType {
	return append([]DailyReviewType(nil), dailyReviewTypes...)
}

func (s *dailyReviewService) Type(kind string) (DailyReviewType, bool) {
	for _, reportType := range dailyReviewTypes {
		if reportType.Kind == kind {
			return reportType, true
		}
	}
	return DailyReviewType{}, false
}

func (s *dailyReviewService) Dashboard(reportDate time.Time) (DailyReviewDashboard, error) {
	reports, err := repositories.DailyReviewRepository.FindByDate(sqls.DB(), reportDate)
	if err != nil {
		return DailyReviewDashboard{}, err
	}
	found := make(map[string]models.DailyReviewReport, len(reports))
	for _, report := range reports {
		found[report.Kind] = report
	}

	sections := make([]DailyReviewSection, 0, len(dailyReviewTypes))
	for _, reportType := range dailyReviewTypes {
		section := DailyReviewSection{
			Kind: reportType.Kind, Label: reportType.Label, Icon: reportType.Icon,
			Status: "pending", Metrics: []DailyReviewMetric{},
		}
		if report, ok := found[reportType.Kind]; ok {
			summary := decodeDailyReviewSummary(report.SummaryJson)
			section.Metrics = summary.Metrics
			section.Status = "ready"
			if summary.NoItems {
				section.Status = "no_items"
			}
			section.UpdatedAt = dailyReviewTimestamp(report.UpdateTime)
		}
		sections = append(sections, section)
	}
	return DailyReviewDashboard{Date: reportDate.Format(time.DateOnly), Sections: sections}, nil
}

func (s *dailyReviewService) Get(reportDate time.Time, kind string) (*models.DailyReviewReport, error) {
	return repositories.DailyReviewRepository.GetByDateAndKind(sqls.DB(), reportDate, kind)
}

func (s *dailyReviewService) Dates() (DailyReviewDates, error) {
	datesList, err := repositories.DailyReviewRepository.AvailableDates(sqls.DB())
	if err != nil {
		return DailyReviewDates{}, err
	}
	result := DailyReviewDates{Dates: make([]string, 0, len(datesList))}
	for _, reportDate := range datesList {
		result.Dates = append(result.Dates, reportDate.Format(time.DateOnly))
	}
	if len(result.Dates) > 0 {
		result.Latest = result.Dates[0]
	}
	return result, nil
}

func (s *dailyReviewService) Save(reportDate time.Time, kind, source string) (DailyReviewSaveResult, error) {
	reportType, ok := s.Type(kind)
	if !ok {
		return DailyReviewSaveResult{}, &DailyReviewValidationError{message: "unknown daily review kind"}
	}
	content := strings.TrimSpace(source)
	if content == "" {
		return DailyReviewSaveResult{}, &DailyReviewValidationError{message: "markdown must not be empty"}
	}
	if len(content) > MaxDailyReviewMarkdownLength {
		return DailyReviewSaveResult{}, &DailyReviewValidationError{message: fmt.Sprintf("markdown exceeds %d bytes", MaxDailyReviewMarkdownLength)}
	}
	content += "\n"
	digestBytes := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(digestBytes[:])
	summaryJson, err := json.Marshal(buildDailyReviewSummary(kind, content))
	if err != nil {
		return DailyReviewSaveResult{}, err
	}

	// Serialize writes in this process to reduce database contention. The
	// conditional update below still protects deployments with multiple instances.
	dailyReviewSaveMu.Lock()
	defer dailyReviewSaveMu.Unlock()

	for attempt := 0; attempt < maxDailyReviewSaveAttempts; attempt++ {
		result, saveErr := s.saveSnapshot(reportDate, kind, reportType, content, string(summaryJson), digest)
		if errors.Is(saveErr, errDailyReviewWriteConflict) {
			continue
		}
		if saveErr != nil {
			return DailyReviewSaveResult{}, saveErr
		}
		return DailyReviewSaveResult{Result: result, Kind: kind, Date: reportDate.Format(time.DateOnly)}, nil
	}
	return DailyReviewSaveResult{}, fmt.Errorf("save daily review after %d attempts: %w", maxDailyReviewSaveAttempts, errDailyReviewWriteConflict)
}

func (s *dailyReviewService) saveSnapshot(
	reportDate time.Time,
	kind string,
	reportType DailyReviewType,
	content string,
	summaryJson string,
	digest string,
) (string, error) {
	now := dates.NowTimestamp()
	result := "created"
	err := sqls.DB().Transaction(func(tx *gorm.DB) error {
		report, err := repositories.DailyReviewRepository.GetByDateAndKind(tx, reportDate, kind)
		if err != nil {
			return err
		}
		if report != nil && report.ContentHash == digest {
			result = "unchanged"
			return nil
		}

		if report == nil {
			created, err := repositories.DailyReviewRepository.CreateIfAbsent(tx, &models.DailyReviewReport{
				ReportDate:  reportDate,
				Kind:        kind,
				Title:       dailyReviewTitle(content, reportType.Label),
				Markdown:    content,
				SummaryJson: summaryJson,
				ContentHash: digest,
				CreateTime:  now,
				UpdateTime:  now,
			})
			if err != nil {
				return err
			}
			if !created {
				return errDailyReviewWriteConflict
			}
			return nil
		}

		previousHash := report.ContentHash
		if err := repositories.DailyReviewRepository.CreateRevision(tx, &models.DailyReviewReportRevision{
			ReportId:         report.Id,
			Title:            report.Title,
			Markdown:         report.Markdown,
			SummaryJson:      report.SummaryJson,
			ContentHash:      report.ContentHash,
			ReportUpdateTime: report.UpdateTime,
			ReplacedAt:       now,
		}); err != nil {
			return err
		}
		report.Title = dailyReviewTitle(content, reportType.Label)
		report.Markdown = content
		report.SummaryJson = summaryJson
		report.ContentHash = digest
		report.UpdateTime = now
		updated, err := repositories.DailyReviewRepository.UpdateIfHash(tx, report, previousHash)
		if err != nil {
			return err
		}
		if !updated {
			return errDailyReviewWriteConflict
		}
		result = "updated"
		return nil
	})
	return result, err
}

func DailyReviewSummaryForReport(report *models.DailyReviewReport) DailyReviewSummary {
	if report == nil {
		return DailyReviewSummary{Metrics: []DailyReviewMetric{}}
	}
	return decodeDailyReviewSummary(report.SummaryJson)
}

func dailyReviewTimestamp(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}
	return time.UnixMilli(timestamp).UTC().Format(time.RFC3339)
}

func DailyReviewUpdatedAt(report *models.DailyReviewReport) string {
	if report == nil {
		return ""
	}
	return dailyReviewTimestamp(report.UpdateTime)
}

func dailyReviewTitle(content, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			return truncateDailyReviewTitle(strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")))
		}
		break
	}
	return truncateDailyReviewTitle(fallback)
}

func truncateDailyReviewTitle(value string) string {
	runes := []rune(value)
	if len(runes) <= maxDailyReviewTitleRunes {
		return value
	}
	return string(runes[:maxDailyReviewTitleRunes])
}

func decodeDailyReviewSummary(raw string) DailyReviewSummary {
	summary := DailyReviewSummary{Metrics: []DailyReviewMetric{}}
	if err := json.Unmarshal([]byte(raw), &summary); err != nil || summary.Metrics == nil {
		summary.Metrics = []DailyReviewMetric{}
	}
	return summary
}

type dailyReviewTable struct {
	Rows [][]string
}

func buildDailyReviewSummary(kind, content string) DailyReviewSummary {
	summary := DailyReviewSummary{
		Metrics: []DailyReviewMetric{},
		NoItems: strings.Contains(content, "无符合口径的港股增持披露"),
	}
	tables := dailyReviewTablesBySection(content)
	rowCount := func(prefix string) int {
		count := 0
		for heading, sectionTables := range tables {
			if !strings.HasPrefix(heading, prefix) {
				continue
			}
			for _, table := range sectionTables {
				count += len(table.Rows)
			}
		}
		return count
	}

	switch kind {
	case "shareholding":
		count := 0
		for _, sectionTables := range tables {
			for _, table := range sectionTables {
				count += len(table.Rows)
			}
		}
		summary.Metrics = []DailyReviewMetric{{Label: "增持记录", Value: fmt.Sprint(count)}}
	case "announcements":
		summary.Metrics = []DailyReviewMetric{
			{Label: "值得关注", Value: fmt.Sprint(rowCount("值得关注"))},
			{Label: "其他符合口径", Value: fmt.Sprint(rowCount("其他符合口径"))},
		}
	case "movers":
		summary.Metrics = []DailyReviewMetric{
			{Label: "涨幅榜", Value: fmt.Sprint(rowCount("涨幅前十"))},
			{Label: "跌幅榜", Value: fmt.Sprint(rowCount("跌幅前十"))},
		}
	case "focus":
		summary.Metrics = []DailyReviewMetric{{Label: "跟踪公司", Value: fmt.Sprint(len(focusCompanyPattern.FindAllString(content, -1)))}}
	case "buybacks":
		large := rowCount("市值 ≥")
		small := rowCount("市值 ＜")
		summary.Metrics = []DailyReviewMetric{
			{Label: "回购记录", Value: fmt.Sprint(large + small)},
			{Label: "市值 ≥ 30亿", Value: fmt.Sprint(large)},
			{Label: "市值 < 30亿", Value: fmt.Sprint(small)},
		}
	case "shorts":
		if marketTables := tables["大市"]; len(marketTables) > 0 {
			for _, row := range marketTables[0].Rows {
				if len(row) >= 2 && len(summary.Metrics) < 4 {
					summary.Metrics = append(summary.Metrics, DailyReviewMetric{Label: row[0], Value: row[1]})
				}
			}
		}
		summary.Metrics = append(summary.Metrics, DailyReviewMetric{Label: "异动证券", Value: fmt.Sprint(rowCount("异动名单"))})
	}
	return summary
}

func dailyReviewTablesBySection(content string) map[string][]dailyReviewTable {
	result := make(map[string][]dailyReviewTable)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(markdown.ToHTML(content)))
	if err != nil {
		return result
	}
	heading := ""
	doc.Find("h1,h2,h3,h4,table").Each(func(_ int, selection *goquery.Selection) {
		switch goquery.NodeName(selection) {
		case "h1", "h2", "h3", "h4":
			heading = strings.TrimSpace(selection.Text())
		case "table":
			table := dailyReviewTable{}
			selection.Find("tbody tr").Each(func(_ int, rowSelection *goquery.Selection) {
				row := make([]string, 0)
				rowSelection.Find("td").Each(func(_ int, cell *goquery.Selection) {
					row = append(row, strings.TrimSpace(cell.Text()))
				})
				if len(row) > 0 {
					table.Rows = append(table.Rows, row)
				}
			})
			result[heading] = append(result[heading], table)
		}
	})
	return result
}
