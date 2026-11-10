package api

import (
	"bbs-go/internal/handlers/render"
	"bbs-go/internal/permissions"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/errs"
	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/services"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type kalshiUpload struct {
	Markdown string `json:"markdown"`
}

func KalshiDates(ctx *gin.Context) {
	dates, err := services.KalshiDates()
	if err != nil {
		slog.Error("load Kalshi dates", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load Kalshi dates")
		return
	}
	ginx.WriteJSON(ctx, gin.H{"dates": dates})
}

func KalshiStatus(ctx *gin.Context) {
	user, err := common.CheckLogin(ctx)
	if err != nil {
		ginx.WriteJSON(ctx, errs.NotLogin())
		return
	}
	if !services.PermissionService.HasPermission(user, permissions.PermissionKalshiIngest.Code) {
		ginx.WriteJSON(ctx, errs.NoPermission())
		return
	}
	date, err := time.Parse(time.DateOnly, ctx.Query("date"))
	if err != nil || date.Format(time.DateOnly) != ctx.Query("date") {
		writeDailyReviewError(ctx, http.StatusBadRequest, "date must use YYYY-MM-DD")
		return
	}
	status, err := services.KalshiStatus(date)
	if err != nil {
		slog.Error("load Kalshi publish status", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load Kalshi publish status")
		return
	}
	ginx.WriteJSON(ctx, status)
}

func KalshiReport(ctx *gin.Context) {
	date, err := time.Parse(time.DateOnly, strings.TrimSpace(ctx.Query("date")))
	if err != nil || date.Format(time.DateOnly) != ctx.Query("date") {
		writeDailyReviewError(ctx, http.StatusBadRequest, "date must use YYYY-MM-DD")
		return
	}
	report, err := services.KalshiGet(date)
	if err != nil {
		slog.Error("load Kalshi report", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load Kalshi report")
		return
	}
	if report == nil {
		writeDailyReviewError(ctx, http.StatusNotFound, "Kalshi report not found")
		return
	}
	doc, err := services.ParseKalshiDocument(report.Markdown)
	if err != nil {
		slog.Error("parse stored Kalshi report", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "invalid stored Kalshi report")
		return
	}
	ginx.WriteJSON(ctx, gin.H{
		"date": date.Format(time.DateOnly), "as_of": doc.AsOf.Format(time.RFC3339),
		"updated_at": time.UnixMilli(report.UpdateTime).UTC().Format(time.RFC3339),
		"manifest":   doc.Manifest, "signals": doc.Signals, "trends": doc.Trends,
		"narrative_html": render.BuildSafeMarkdownContent(doc.Narrative),
		"content_html":   render.BuildSafeMarkdownContent(report.Markdown),
		"markdown":       report.Markdown,
	})
}

func KalshiIngest(ctx *gin.Context) {
	user, err := common.CheckLogin(ctx)
	if err != nil {
		ginx.WriteJSON(ctx, errs.NotLogin())
		return
	}
	if !services.PermissionService.HasPermission(user, permissions.PermissionKalshiIngest.Code) {
		ginx.WriteJSON(ctx, errs.NoPermission())
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 6*services.MaxKalshiMarkdownLength+64*1024)
	var upload kalshiUpload
	if err := ginx.BindJSON(ctx, &upload); err != nil {
		writeDailyReviewError(ctx, http.StatusBadRequest, "invalid Kalshi payload")
		return
	}
	result, date, err := services.KalshiSave(upload.Markdown)
	if errors.Is(err, services.ErrKalshiStale) {
		writeDailyReviewError(ctx, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, services.ErrKalshiInvalid) {
		writeDailyReviewError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		slog.Error("ingest Kalshi report", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to save Kalshi report")
		return
	}
	ginx.WriteJSON(ctx, gin.H{"result": result, "date": date})
}
