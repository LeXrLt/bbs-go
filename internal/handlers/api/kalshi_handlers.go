package api

import (
	"bbs-go/internal/permissions"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/errs"
	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/services"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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
	date, ok := kalshiDate(ctx)
	if !ok {
		return
	}
	slot := ctx.Query("slot")
	if !kalshiSlot(ctx, slot) {
		return
	}
	status, err := services.KalshiStatus(date, slot)
	if err != nil {
		slog.Error("load Kalshi publish status", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load Kalshi publish status")
		return
	}
	ginx.WriteJSON(ctx, status)
}

func kalshiDate(ctx *gin.Context) (time.Time, bool) {
	date, err := time.Parse(time.DateOnly, ctx.Query("date"))
	if err != nil || date.Format(time.DateOnly) != ctx.Query("date") {
		writeDailyReviewError(ctx, http.StatusBadRequest, "date must use YYYY-MM-DD")
		return time.Time{}, false
	}
	return date, true
}

func kalshiSlot(ctx *gin.Context, slot string) bool {
	if !services.ValidKalshiSlot(slot) {
		writeDailyReviewError(ctx, http.StatusBadRequest, "slot must be morning or evening")
		return false
	}
	return true
}

func KalshiSnapshots(ctx *gin.Context) {
	date, ok := kalshiDate(ctx)
	if !ok {
		return
	}
	snapshots, err := services.KalshiSnapshots(date)
	if err != nil {
		slog.Error("load Kalshi snapshots", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load Kalshi snapshots")
		return
	}
	ginx.WriteJSON(ctx, gin.H{"date": date.Format(time.DateOnly), "snapshots": snapshots})
}

func KalshiReport(ctx *gin.Context) {
	date, ok := kalshiDate(ctx)
	if !ok {
		return
	}
	slot := ctx.Query("slot")
	if !kalshiSlot(ctx, slot) {
		return
	}
	report, err := services.KalshiGet(date, slot)
	if err != nil {
		slog.Error("load Kalshi report", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load Kalshi report")
		return
	}
	if report == nil {
		ginx.WriteJSON(ctx, gin.H{"date": date.Format(time.DateOnly), "slot": slot, "published": false})
		return
	}
	doc, err := services.ParseKalshiDocument(report.Markdown)
	if err != nil {
		slog.Error("parse stored Kalshi report", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "invalid stored Kalshi report")
		return
	}
	ginx.WriteJSON(ctx, gin.H{
		"date": date.Format(time.DateOnly), "slot": slot, "published": true, "content_hash": report.ContentHash, "as_of": doc.AsOf.Format(time.RFC3339),
		"updated_at": time.UnixMilli(report.UpdateTime).UTC().Format(time.RFC3339),
		"manifest":   doc.Manifest, "signals": doc.Signals, "trends": doc.Trends,
	})
}

func KalshiDownload(ctx *gin.Context) {
	date, ok := kalshiDate(ctx)
	if !ok {
		return
	}
	slot := ctx.Query("slot")
	if !kalshiSlot(ctx, slot) {
		return
	}
	report, err := services.KalshiGet(date, slot)
	if err != nil {
		slog.Error("download Kalshi report", slog.Any("err", err))
		writeDailyReviewError(ctx, http.StatusInternalServerError, "failed to load Kalshi report")
		return
	}
	if report == nil {
		writeDailyReviewError(ctx, http.StatusNotFound, "Kalshi report not found")
		return
	}
	ctx.Header("Content-Disposition", `attachment; filename="kalshi-`+date.Format(time.DateOnly)+`-`+slot+`.md"`)
	ctx.Header("Cache-Control", "private, no-store")
	ctx.Header("Content-Length", strconv.Itoa(len(report.Markdown)))
	ctx.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(report.Markdown))
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
	result, date, slot, hash, err := services.KalshiSave(upload.Markdown)
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
	ginx.WriteJSON(ctx, gin.H{"result": result, "date": date, "slot": slot, "content_hash": hash})
}
