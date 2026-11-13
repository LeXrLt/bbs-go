package api

import (
	"bbs-go/internal/models"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupKalshiHandlerReport(t *testing.T) string {
	t.Helper()
	db := setupDailyReviewHandlerTestDB(t)
	if err := db.AutoMigrate(&models.KalshiReport{}); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"data/signals.json": `{"rows":[{"title_zh":"线索"}],"meta":{}}` + "\n",
		"data/trends.json":  `{"rows":[],"meta":{}}` + "\n",
		"index.html":        "<p>offline report</p>\n",
	}
	manifestFiles := make(map[string]any, len(files))
	for name, content := range files {
		digest := sha256.Sum256([]byte(content))
		manifestFiles[name] = map[string]any{"sha256": hex.EncodeToString(digest[:]), "bytes": len(content)}
	}
	manifest, err := json.Marshal(map[string]any{
		"schema_version": "kalshi-dashboard-handoff/2", "date": "2026-11-09",
		"as_of": "2026-11-09T19:00:00+08:00", "publication_ready": true,
		"collection_status": "complete", "counts": map[string]int{"signals_total": 1, "trend_rows": 0},
		"metadata": map[string]string{"run_slot": "evening"}, "files": manifestFiles,
	})
	if err != nil {
		t.Fatal(err)
	}
	markdown := fmt.Sprintf("# Kalshi\n\n```json manifest.json\n%s\n```\n\n```json data/signals.json\n%s```\n\n```json data/trends.json\n%s```\n\n```html index.html\n%s```\n", manifest, files["data/signals.json"], files["data/trends.json"], files["index.html"])
	date, _ := time.Parse(time.DateOnly, "2026-11-09")
	if err := db.Create(&models.KalshiReport{ReportDate: date, Slot: "evening", AsOf: date, Markdown: markdown, ContentHash: "hash"}).Error; err != nil {
		t.Fatal(err)
	}
	return markdown
}

func runKalshiRead(t *testing.T, path string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	handler(ctx)
	return recorder
}

func TestKalshiReportOmitsRawDocument(t *testing.T) {
	markdown := setupKalshiHandlerReport(t)
	response := runKalshiRead(t, "/api/kalshi/report?date=2026-11-09&slot=evening", KalshiReport)
	var result struct {
		Success bool                       `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.Success {
		t.Fatalf("report response: %d %s", response.Code, response.Body.String())
	}
	for _, key := range []string{"markdown", "content_html", "narrative_html"} {
		if _, ok := result.Data[key]; ok {
			t.Fatalf("report contains unused %s", key)
		}
	}
	for _, key := range []string{"manifest", "signals", "trends", "published"} {
		if _, ok := result.Data[key]; !ok {
			t.Fatalf("report missing %s", key)
		}
	}
	if strings.Contains(response.Body.String(), markdown) {
		t.Fatal("report embeds full Markdown")
	}
}

func TestKalshiDownloadExactDocumentAndValidation(t *testing.T) {
	markdown := setupKalshiHandlerReport(t)
	response := runKalshiRead(t, "/api/kalshi/download?date=2026-11-09&slot=evening", KalshiDownload)
	if response.Code != http.StatusOK || response.Body.String() != markdown {
		t.Fatalf("download status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Disposition") != `attachment; filename="kalshi-2026-11-09-evening.md"` || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("download headers: %v", response.Header())
	}
	for _, test := range []struct {
		query string
		want  int
	}{
		{"date=2026-02-30&slot=evening", http.StatusBadRequest},
		{"date=2026-11-09&slot=other", http.StatusBadRequest},
		{"date=2026-11-09&slot=morning", http.StatusNotFound},
	} {
		response := runKalshiRead(t, "/api/kalshi/download?"+test.query, KalshiDownload)
		if response.Code != test.want {
			t.Fatalf("%s status=%d body=%s", test.query, response.Code, response.Body.String())
		}
	}
}
