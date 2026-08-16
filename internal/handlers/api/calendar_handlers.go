package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/services"

	"github.com/gin-gonic/gin"
)

const (
	defaultCalendarLimit = 50
	maxCalendarLimit     = 100
)

func CalendarEvents(ctx *gin.Context) {
	query, err := parseCalendarEventsQuery(ctx)
	if err != nil {
		writeCalendarError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response, err := services.CalendarService.ListEvents(ctx.Request.Context(), query)
	if err != nil {
		writeCalendarServiceError(ctx, err)
		return
	}
	ginx.WriteJSON(ctx, response)
}

func CalendarEventDetail(ctx *gin.Context) {
	kind := dto.CalendarKind(strings.TrimSpace(ctx.Param("kind")))
	if !kind.IsValid() {
		writeCalendarError(ctx, http.StatusBadRequest, "kind must be one of economic, earnings, corporate, ipo")
		return
	}
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeCalendarError(ctx, http.StatusBadRequest, "id must be a positive integer")
		return
	}
	event, err := services.CalendarService.GetEvent(ctx.Request.Context(), kind, id)
	if err != nil {
		writeCalendarServiceError(ctx, err)
		return
	}
	ginx.WriteJSON(ctx, event)
}

func parseCalendarEventsQuery(ctx *gin.Context) (services.CalendarEventsQuery, error) {
	dateFrom, err := parseRequiredCalendarDate(ctx.Query("dateFrom"), "dateFrom")
	if err != nil {
		return services.CalendarEventsQuery{}, err
	}
	dateTo, err := parseRequiredCalendarDate(ctx.Query("dateTo"), "dateTo")
	if err != nil {
		return services.CalendarEventsQuery{}, err
	}
	if dateFrom.After(dateTo) {
		return services.CalendarEventsQuery{}, ginx.ErrorMessage("dateFrom must be on or before dateTo")
	}
	if dateTo.Sub(dateFrom) > 30*24*time.Hour {
		return services.CalendarEventsQuery{}, ginx.ErrorMessage("date range must not exceed 31 days")
	}

	kind := dto.CalendarKind(strings.TrimSpace(ctx.Query("kind")))
	if kind != "" && !kind.IsValid() {
		return services.CalendarEventsQuery{}, ginx.ErrorMessage("kind must be one of economic, earnings, corporate, ipo")
	}
	minImportance, err := parseOptionalCalendarInt(ctx.Query("minImportance"), 0, 0, 3, "minImportance must be 1, 2, or 3")
	if err != nil {
		return services.CalendarEventsQuery{}, err
	}
	if strings.TrimSpace(ctx.Query("minImportance")) != "" && minImportance == 0 {
		return services.CalendarEventsQuery{}, ginx.ErrorMessage("minImportance must be 1, 2, or 3")
	}
	cursor := strings.TrimSpace(ctx.Query("cursor"))
	if err := services.ValidateCalendarCursor(cursor); err != nil {
		return services.CalendarEventsQuery{}, ginx.ErrorMessage("cursor is invalid")
	}
	limit, err := parseOptionalCalendarInt(ctx.Query("limit"), defaultCalendarLimit, 1, maxCalendarLimit, "limit must be between 1 and 100")
	if err != nil {
		return services.CalendarEventsQuery{}, err
	}

	keyword := strings.TrimSpace(ctx.Query("keyword"))
	if utf8.RuneCountInString(keyword) > 100 {
		return services.CalendarEventsQuery{}, ginx.ErrorMessage("keyword must not exceed 100 characters")
	}

	return services.CalendarEventsQuery{
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		Kind:          kind,
		Keyword:       keyword,
		MinImportance: minImportance,
		Cursor:        cursor,
		Limit:         limit,
	}, nil
}

func parseRequiredCalendarDate(raw, name string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, ginx.ErrorMessage(name + " is required")
	}
	location, err := time.LoadLocation(services.CalendarTimezone)
	if err != nil {
		return time.Time{}, ginx.ErrorMessage("calendar timezone is unavailable")
	}
	parsed, err := time.ParseInLocation(time.DateOnly, value, location)
	if err != nil || parsed.Format(time.DateOnly) != value {
		return time.Time{}, ginx.ErrorMessage(name + " must use YYYY-MM-DD")
	}
	return parsed, nil
}

func parseOptionalCalendarInt(raw string, defaultValue, minimum, maximum int, message string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, ginx.ErrorMessage(message)
	}
	return parsed, nil
}

func writeCalendarServiceError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrCalendarCursorStale):
		writeCalendarError(ctx, http.StatusConflict, "calendar data changed; reload the first page")
	case errors.Is(err, services.ErrCalendarInvalidQuery):
		writeCalendarError(ctx, http.StatusBadRequest, "invalid calendar request")
	case errors.Is(err, services.ErrCalendarNotFound):
		writeCalendarError(ctx, http.StatusNotFound, "calendar event not found")
	case errors.Is(err, services.ErrCalendarBusy):
		writeCalendarError(ctx, http.StatusTooManyRequests, "calendar data source is busy; try again shortly")
	case errors.Is(err, context.DeadlineExceeded), isTimeoutError(err):
		writeCalendarError(ctx, http.StatusGatewayTimeout, "calendar data source timed out")
	case errors.Is(err, services.ErrCalendarUpstream):
		writeCalendarError(ctx, http.StatusBadGateway, "calendar data source is unavailable")
	default:
		writeCalendarError(ctx, http.StatusInternalServerError, "failed to load calendar data")
	}
}

func isTimeoutError(err error) bool {
	var netError net.Error
	return errors.As(err, &netError) && netError.Timeout()
}

func writeCalendarError(ctx *gin.Context, status int, message string) {
	ginx.WriteHttpStatusJSON(ctx, status, ginx.ErrorMessage(message))
}
