package api

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/common"
	"bytes"
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
	"github.com/mlogclub/simple/sqls"
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

func runKalshiIngest(t *testing.T, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/kalshi/ingest", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetCurrentUser(ctx, &models.User{Model: models.Model{Id: 1}, Roles: constants.RoleOwner})
	KalshiIngest(ctx)
	return recorder
}

func TestKalshiIngestLargeDocumentAndBodyLimit(t *testing.T) {
	// The fixture is already a valid stored report; a newer, larger source updates it.
	previous := setupKalshiHandlerReport(t)
	if err := sqls.DB().AutoMigrate(&models.KalshiReportRevision{}); err != nil {
		t.Fatal(err)
	}
	large := strings.Replace(previous, `"as_of":"2026-11-09T19:00:00+08:00"`, `"as_of":"2026-11-09T20:00:00+08:00"`, 1)
	large += strings.Repeat("x", 2_225_309-len(large))
	if len(large) != 2_225_309 {
		t.Fatalf("large fixture size = %d", len(large))
	}
	body, err := json.Marshal(kalshiUpload{Markdown: large})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"updated", "unchanged"} {
		response := runKalshiIngest(t, body)
		if response.Code != http.StatusOK || !responseSucceeded(response.Body.Bytes()) || !strings.Contains(response.Body.String(), `"result":"`+want+`"`) {
			t.Fatalf("large upload %s: %d %s", want, response.Code, response.Body.String())
		}
	}
	download := runKalshiRead(t, "/api/kalshi/download?date=2026-11-09&slot=evening", KalshiDownload)
	if download.Code != http.StatusOK || download.Body.String() != large || download.Header().Get("Content-Length") != fmt.Sprint(len(large)) {
		t.Fatalf("large download mismatch: status=%d bytes=%d", download.Code, download.Body.Len())
	}
	tooLargeBody, err := json.Marshal(kalshiUpload{Markdown: strings.Repeat("x", kalshiUploadMaxBodyBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if response := runKalshiIngest(t, tooLargeBody); response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "20 MiB") {
		t.Fatalf("oversized JSON body: %d %s", response.Code, response.Body.String())
	}
}
