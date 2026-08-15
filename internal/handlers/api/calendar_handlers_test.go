package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"bbs-go/internal/models/dto"
	"bbs-go/internal/services"

	"github.com/gin-gonic/gin"
)

type fakeCalendarService struct {
	list func(context.Context, services.CalendarEventsQuery) (dto.CalendarEventsResponse, error)
	get  func(context.Context, dto.CalendarKind, int64) (dto.CalendarEvent, error)
}

func (fake *fakeCalendarService) ListEvents(ctx context.Context, query services.CalendarEventsQuery) (dto.CalendarEventsResponse, error) {
	if fake.list == nil {
		return dto.CalendarEventsResponse{}, errors.New("unexpected ListEvents call")
	}
	return fake.list(ctx, query)
}

func (fake *fakeCalendarService) GetEvent(ctx context.Context, kind dto.CalendarKind, id int64) (dto.CalendarEvent, error) {
	if fake.get == nil {
		return dto.CalendarEvent{}, errors.New("unexpected GetEvent call")
	}
	return fake.get(ctx, kind, id)
}

func TestCalendarEventsParsesQueryAndWritesEnvelope(t *testing.T) {
	const cursor = "v1:3:0000000000000000000000000000000000000000000000000000000000000000"
	var captured services.CalendarEventsQuery
	restoreCalendarService(t, &fakeCalendarService{list: func(_ context.Context, query services.CalendarEventsQuery) (dto.CalendarEventsResponse, error) {
		captured = query
		return dto.CalendarEventsResponse{
			GeneratedAt: "2026-08-15T00:00:00Z",
			DateFrom:    "2026-08-10",
			DateTo:      "2026-08-12",
			Timezone:    services.CalendarTimezone,
			Results:     []dto.CalendarEvent{},
		}, nil
	}})

	recorder := performCalendarHandlerRequest(http.MethodGet, "/api/calendar/events?dateFrom=2026-08-10&dateTo=2026-08-12&kind=economic&keyword=CPI&minImportance=2&cursor="+cursor+"&limit=25", CalendarEvents, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if captured.DateFrom.Format("2006-01-02") != "2026-08-10" || captured.DateTo.Format("2006-01-02") != "2026-08-12" {
		t.Fatalf("dates=%s..%s", captured.DateFrom, captured.DateTo)
	}
	if captured.Kind != dto.CalendarKindEconomic || captured.Keyword != "CPI" || captured.MinImportance != 2 || captured.Cursor != cursor || captured.Limit != 25 {
		t.Fatalf("query=%+v", captured)
	}
	var envelope struct {
		Success bool                       `json:"success"`
		Data    dto.CalendarEventsResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || envelope.Data.Timezone != services.CalendarTimezone || envelope.Data.Results == nil {
		t.Fatalf("envelope=%+v body=%s", envelope, recorder.Body.String())
	}
}

func TestCalendarEventsRejectsInvalidQuery(t *testing.T) {
	var calls int
	restoreCalendarService(t, &fakeCalendarService{list: func(_ context.Context, _ services.CalendarEventsQuery) (dto.CalendarEventsResponse, error) {
		calls++
		return dto.CalendarEventsResponse{}, nil
	}})
	tests := []string{
		"/api/calendar/events?dateTo=2026-08-10",
		"/api/calendar/events?dateFrom=2026-08-10",
		"/api/calendar/events?dateFrom=2026-8-1&dateTo=2026-08-10",
		"/api/calendar/events?dateFrom=2026-08-11&dateTo=2026-08-10",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-09-01",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-08-10&kind=macro",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-08-10&minImportance=0",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-08-10&minImportance=4",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-08-10&cursor=-1",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-08-10&limit=0",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-08-10&limit=101",
		"/api/calendar/events?dateFrom=2026-08-01&dateTo=2026-08-10&keyword=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			recorder := performCalendarHandlerRequest(http.MethodGet, target, CalendarEvents, nil)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			assertCalendarErrorEnvelope(t, recorder)
		})
	}
	if calls != 0 {
		t.Fatalf("service calls=%d want 0", calls)
	}
}

func TestCalendarEventDetailValidatesAndMapsResult(t *testing.T) {
	restoreCalendarService(t, &fakeCalendarService{get: func(_ context.Context, kind dto.CalendarKind, id int64) (dto.CalendarEvent, error) {
		if kind != dto.CalendarKindIPO || id != 42 {
			t.Fatalf("kind=%s id=%d", kind, id)
		}
		return dto.CalendarEvent{Kind: kind, ID: id, Key: "ipo:42", Date: "2026-08-15", Title: "Example IPO"}, nil
	}})

	recorder := performCalendarHandlerRequest(http.MethodGet, "/api/calendar/events/ipo/42", CalendarEventDetail, map[string]string{"kind": "ipo", "id": "42"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Success bool              `json:"success"`
		Data    dto.CalendarEvent `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || envelope.Data.ID != 42 || envelope.Data.Kind != dto.CalendarKindIPO {
		t.Fatalf("envelope=%+v", envelope)
	}
}

func TestCalendarEventDetailRejectsInvalidPath(t *testing.T) {
	restoreCalendarService(t, &fakeCalendarService{})
	tests := []map[string]string{
		{"kind": "macro", "id": "1"},
		{"kind": "economic", "id": "abc"},
		{"kind": "ipo", "id": "0"},
	}
	for _, params := range tests {
		t.Run(fmt.Sprintf("%s/%s", params["kind"], params["id"]), func(t *testing.T) {
			recorder := performCalendarHandlerRequest(http.MethodGet, "/api/calendar/events/invalid", CalendarEventDetail, params)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			assertCalendarErrorEnvelope(t, recorder)
		})
	}
}

func TestCalendarHandlerMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{name: "stale cursor", err: services.ErrCalendarCursorStale, status: http.StatusConflict, message: "calendar data changed; reload the first page"},
		{name: "invalid", err: services.ErrCalendarInvalidQuery, status: http.StatusBadRequest},
		{name: "not found", err: services.ErrCalendarNotFound, status: http.StatusNotFound},
		{name: "busy", err: fmt.Errorf("wrapped: %w", services.ErrCalendarBusy), status: http.StatusTooManyRequests, message: "calendar data source is busy; try again shortly"},
		{name: "timeout", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), status: http.StatusGatewayTimeout},
		{name: "upstream", err: services.ErrCalendarUpstream, status: http.StatusBadGateway},
		{name: "internal", err: errors.New("internal"), status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restoreCalendarService(t, &fakeCalendarService{list: func(_ context.Context, _ services.CalendarEventsQuery) (dto.CalendarEventsResponse, error) {
				return dto.CalendarEventsResponse{}, test.err
			}})
			recorder := performCalendarHandlerRequest(http.MethodGet, "/api/calendar/events?dateFrom=2026-08-10&dateTo=2026-08-10", CalendarEvents, nil)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			assertCalendarErrorEnvelope(t, recorder)
			if test.message != "" {
				var envelope struct {
					Message string `json:"message"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Message != test.message {
					t.Fatalf("message=%q want=%q", envelope.Message, test.message)
				}
			}
		})
	}
}

func restoreCalendarService(t *testing.T, replacement services.CalendarServiceProvider) {
	t.Helper()
	previous := services.CalendarService
	services.CalendarService = replacement
	t.Cleanup(func() {
		services.CalendarService = previous
	})
}

func performCalendarHandlerRequest(method, target string, handler gin.HandlerFunc, params map[string]string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	for key, value := range params {
		ctx.Params = append(ctx.Params, gin.Param{Key: key, Value: value})
	}
	handler(ctx)
	return recorder
}

func assertCalendarErrorEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var envelope struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	if envelope.Success || envelope.Message == "" {
		t.Fatalf("invalid error envelope: %s", recorder.Body.String())
	}
}
