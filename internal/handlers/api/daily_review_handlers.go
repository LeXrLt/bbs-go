package api

import (
	"bbs-go/internal/handlers/render"
	"bbs-go/internal/permissions"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/errs"
	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/services"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const dailyReviewUploadMaxBodyBytes = 6*services.MaxDailyReviewMarkdownLength + 64*1024

var dailyReviewLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type dailyReviewUpload struct {
	Markdown string `json:"markdown"`
}

func DailyReviewDashboard(ctx *gin.Context) {
	reportDate, ok := parseDailyReviewDate(ctx.Query("date"), false)
	if !ok {
		writeDailyReviewError(ctx, http.StatusBadRequest, "date must use YYYY-MM-DD")
		return
	}
	result, err := services.DailyReviewService.Dashboard(reportDate)
	if err != nil {
		slog.Error("load daily review dashboard", slog.String("date", reportDate.Format(time.DateOnly)), slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load daily review")
		return
	}
	ginx.WriteJSON(ctx, result)
}

func DailyReviewDates(ctx *gin.Context) {
	result, err := services.DailyReviewService.Dates()
	if err != nil {
		slog.Error("load daily review dates", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load daily review dates")
		return
	}
	ginx.WriteJSON(ctx, result)
}

func DailyReviewReport(ctx *gin.Context) {
	kind := strings.TrimSpace(ctx.Param("kind"))
	reportType, ok := services.DailyReviewService.Type(kind)
	if !ok {
		writeDailyReviewError(ctx, http.StatusNotFound, "unknown daily review kind")
		return
	}
	reportDate, ok := parseDailyReviewDate(ctx.Query("date"), false)
	if !ok {
		writeDailyReviewError(ctx, http.StatusBadRequest, "date must use YYYY-MM-DD")
		return
	}
	report, err := services.DailyReviewService.Get(reportDate, kind)
	if err != nil {
		slog.Error("load daily review report", slog.String("date", reportDate.Format(time.DateOnly)), slog.String("kind", kind), slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load daily review")
		return
	}
	if report == nil {
		writeDailyReviewError(ctx, http.StatusNotFound, "daily review is not available for this date")
		return
	}
	summary := services.DailyReviewSummaryForReport(report)
	status := "ready"
	if summary.NoItems {
		status = "no_items"
	}
	ginx.WriteJSON(ctx, map[string]any{
		"kind":         kind,
		"label":        reportType.Label,
		"date":         reportDate.Format(time.DateOnly),
		"title":        report.Title,
		"status":       status,
		"metrics":      summary.Metrics,
		"content_html": render.BuildSafeMarkdownContent(report.Markdown),
		"updated_at":   services.DailyReviewUpdatedAt(report),
	})
}

func DailyReviewIngest(ctx *gin.Context) {
	user, err := common.CheckLogin(ctx)
	if err != nil {
		ginx.WriteJSON(ctx, errs.NotLogin())
		return
	}
	if !services.PermissionService.HasPermission(user, permissions.PermissionDailyReviewIngest.Code) {
		ginx.WriteJSON(ctx, errs.NoPermission())
		return
	}
	kind := strings.TrimSpace(ctx.Param("kind"))
	if _, ok := services.DailyReviewService.Type(kind); !ok {
		writeDailyReviewError(ctx, http.StatusNotFound, "unknown daily review kind")
		return
	}
	reportDate, ok := parseDailyReviewDate(ctx.Query("date"), true)
	if !ok {
		writeDailyReviewError(ctx, http.StatusBadRequest, "date must use YYYY-MM-DD")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(
		ctx.Writer,
		ctx.Request.Body,
		dailyReviewUploadMaxBodyBytes,
	)
	var upload dailyReviewUpload
	if err := ginx.BindJSON(ctx, &upload); err != nil {
		writeDailyReviewError(ctx, http.StatusBadRequest, "invalid daily review payload")
		return
	}
	result, err := services.DailyReviewService.Save(reportDate, kind, upload.Markdown)
	if err != nil {
		if services.IsDailyReviewValidationError(err) {
			writeDailyReviewError(ctx, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("ingest daily review", slog.String("date", reportDate.Format(time.DateOnly)), slog.String("kind", kind), slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to save daily review")
		return
	}
	ginx.WriteJSON(ctx, result)
}

func parseDailyReviewDate(value string, required bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return time.Time{}, false
		}
		now := time.Now().In(dailyReviewLocation)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), true
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil || parsed.Format(time.DateOnly) != value {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func writeDailyReviewError(ctx *gin.Context, status int, message string) {
	ginx.WriteHttpStatusJSON(ctx, status, ginx.ErrorMessage(message))
}
