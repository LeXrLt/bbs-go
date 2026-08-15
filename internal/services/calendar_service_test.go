package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/config"
)

func TestCalendarServiceListEventsMapsFiltersSortsAndPaginates(t *testing.T) {
	var feedCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		feedCalls.Add(1)
		if request.URL.Path != "/api/feed" {
			t.Errorf("path=%q", request.URL.Path)
		}
		if request.URL.Query().Get("date_from") != "2026-08-09" || request.URL.Query().Get("date_to") != "2026-08-13" {
			t.Errorf("unexpected expanded query: %s", request.URL.RawQuery)
		}
		if request.Header.Get("X-Feed-Token") != "feed-secret" {
			t.Error("missing X-Feed-Token header")
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
  "generated_at":"2026-08-15T04:17:33.836923+00:00",
  "date_from":"2026-08-09",
  "date_to":"2026-08-13",
  "economic":[
    {"id":1,"source_id":"econ-alpha","fetched_at":"2026-08-15T01:00:00Z","event_time":"2026-08-09T16:30:00Z","country":"US","indicator":"Alpha CPI","importance":3,"actual":"2.5%","forecast":"2.4%","previous":"2.3%","source":"finnhub","post":{"content_md":"discard me"}},
    {"id":2,"source_id":"before","fetched_at":"2026-08-15T01:00:00Z","event_time":"2026-08-09T15:59:00Z","country":"US","indicator":"Alpha before","importance":3,"source":"finnhub"},
    {"id":3,"source_id":"after","fetched_at":"2026-08-15T01:00:00Z","event_time":"2026-08-12T16:30:00Z","country":"US","indicator":"Alpha after","importance":3,"source":"finnhub"}
  ],
  "earnings":[
    {"id":4,"source_id":null,"fetched_at":"2026-08-15T01:00:00Z","report_date":"2026-08-10","ticker":"ALP","exchange":"NASDAQ","company":"Alpha Corp","period":"2026Q3","source":"finnhub"}
  ],
  "corporate":[
    {"id":5,"source_id":"corp-alpha","fetched_at":"2026-08-15T01:00:00Z","event_date":"2026-08-10","ticker":"ALP","event_type":"earnings_call","title":"Alpha call","company":"Alpha Corp","description":"Results call","event_time":"2026-08-11T01:00:00Z","timezone":"UTC","source_url":"https://example.com/call","source":"ir"}
  ],
  "ipo":[
    {"id":6,"source_id":"ipo-alpha","fetched_at":"2026-08-15T01:00:00Z","event_date":"2026-08-12","ticker":"AIPO","company":"Alpha IPO","exchange":"NYSE","price_low":10.5,"price_high":12.5,"status":"expected","source":"finnhub"}
  ]
}`))
	}))
	defer server.Close()

	service := newCalendarService(config.CalendarConfig{
		BaseURL:        server.URL,
		TimeoutSeconds: 2,
		CacheSeconds:   30,
		FeedToken:      "feed-secret",
	}, server.Client())
	dateFrom := mustCalendarServiceDate(t, "2026-08-10")
	dateTo := mustCalendarServiceDate(t, "2026-08-12")

	economic, err := service.ListEvents(context.Background(), CalendarEventsQuery{
		DateFrom: dateFrom,
		DateTo:   dateTo,
		Kind:     dto.CalendarKindEconomic,
		Keyword:  " alpha ",
		Limit:    50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if economic.DateFrom != "2026-08-10" || economic.DateTo != "2026-08-12" || economic.Timezone != CalendarTimezone {
		t.Fatalf("response metadata=%+v", economic)
	}
	if economic.Counts != (dto.CalendarEventCounts{Economic: 1, Earnings: 1, Corporate: 1, IPO: 1}) {
		t.Fatalf("counts=%+v", economic.Counts)
	}
	if economic.Total != 1 || len(economic.Results) != 1 {
		t.Fatalf("economic result=%+v", economic)
	}
	economicEvent := economic.Results[0]
	if economicEvent.Date != "2026-08-10" || economicEvent.DateTime != "2026-08-10T00:30:00+08:00" {
		t.Fatalf("normalized economic time=%+v", economicEvent)
	}
	if economicEvent.Title != "Alpha CPI" || economicEvent.Key != "economic:econ-alpha" || economicEvent.Actual == nil || *economicEvent.Actual != "2.5%" {
		t.Fatalf("mapped economic event=%+v", economicEvent)
	}
	encoded, err := json.Marshal(economic)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "content_md") || strings.Contains(string(encoded), `"post"`) {
		t.Fatalf("BBS response leaked upstream post payload: %s", encoded)
	}

	firstPage, err := service.ListEvents(context.Background(), CalendarEventsQuery{
		DateFrom: dateFrom,
		DateTo:   dateTo,
		Keyword:  "alpha",
		Limit:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstPage.Total != 4 || !firstPage.HasMore || !strings.HasPrefix(firstPage.Cursor, "v1:2:") || len(firstPage.Results) != 2 {
		t.Fatalf("first page=%+v", firstPage)
	}
	if firstPage.Results[0].Kind != dto.CalendarKindEarnings || firstPage.Results[0].Key != "earnings:4" {
		t.Fatalf("first sorted event=%+v", firstPage.Results[0])
	}
	if firstPage.Results[1].Kind != dto.CalendarKindEconomic {
		t.Fatalf("second sorted event=%+v", firstPage.Results[1])
	}

	secondPage, err := service.ListEvents(context.Background(), CalendarEventsQuery{
		DateFrom: dateFrom,
		DateTo:   dateTo,
		Keyword:  "alpha",
		Cursor:   firstPage.Cursor,
		Limit:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondPage.HasMore || secondPage.Cursor != "" || len(secondPage.Results) != 2 {
		t.Fatalf("second page=%+v", secondPage)
	}
	if secondPage.Results[0].Kind != dto.CalendarKindCorporate || secondPage.Results[0].Date != "2026-08-11" || secondPage.Results[1].Kind != dto.CalendarKindIPO {
		t.Fatalf("second page ordering=%+v", secondPage.Results)
	}

	important, err := service.ListEvents(context.Background(), CalendarEventsQuery{
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		MinImportance: 3,
		Limit:         50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if important.Total != 1 || important.Counts.Economic != 1 || important.Counts.Earnings != 0 {
		t.Fatalf("importance filtering=%+v", important)
	}
	if feedCalls.Load() != 1 {
		t.Fatalf("feed calls=%d want 1 cached call", feedCalls.Load())
	}
}

func TestCalendarServiceGetEventMapsDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/event/corporate/42" {
			t.Errorf("path=%q", request.URL.Path)
		}
		if request.Header.Get("X-Feed-Token") != "detail-secret" {
			t.Error("missing detail feed token")
		}
		_, _ = response.Write([]byte(`{
  "kind":"corporate","id":42,"source_id":"corp-42","fetched_at":"2026-08-15T01:00:00Z",
  "event_date":"2026-08-15","ticker":"BVC","event_type":"meeting","title":null,
  "company":"BVC Corp","description":"Investor meeting","event_time":"2026-08-15T02:30:00Z",
  "timezone":"UTC","source_url":"https://example.com/42","source":"ir","post":{"content_md":"discard"}
}`))
	}))
	defer server.Close()

	service := newCalendarService(config.CalendarConfig{
		BaseURL:        server.URL,
		TimeoutSeconds: 2,
		CacheSeconds:   30,
		FeedToken:      "detail-secret",
	}, server.Client())
	event, err := service.GetEvent(context.Background(), dto.CalendarKindCorporate, 42)
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != dto.CalendarKindCorporate || event.ID != 42 || event.Title != "BVC Corp" || event.DateTime != "2026-08-15T10:30:00+08:00" {
		t.Fatalf("detail=%+v", event)
	}
	if event.SourceURL == nil || *event.SourceURL != "https://example.com/42" {
		t.Fatalf("sourceURL=%v", event.SourceURL)
	}
}

func TestCalendarClientErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		httpClient *http.Client
		want       error
	}{
		{
			name: "upstream 500",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(http.StatusInternalServerError)
			},
			want: ErrCalendarUpstream,
		},
		{
			name: "bad JSON",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				_, _ = response.Write([]byte(`{"economic":`))
			},
			want: ErrCalendarUpstream,
		},
		{
			name: "oversized response",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Length", strconv.FormatInt(calendarMaxResponseBytes+1, 10))
				response.WriteHeader(http.StatusOK)
			},
			want: ErrCalendarUpstream,
		},
		{
			name: "oversized streamed response",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(http.StatusOK)
				if flusher, ok := response.(http.Flusher); ok {
					flusher.Flush()
				}
				_, _ = response.Write([]byte(strings.Repeat("x", int(calendarMaxResponseBytes+1))))
			},
			want: ErrCalendarUpstream,
		},
		{
			name: "timeout",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				time.Sleep(100 * time.Millisecond)
				_, _ = response.Write([]byte(`{}`))
			},
			httpClient: &http.Client{Timeout: 20 * time.Millisecond},
			want:       context.DeadlineExceeded,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			client := test.httpClient
			if client == nil {
				client = server.Client()
			}
			service := newCalendarService(config.CalendarConfig{
				BaseURL:        server.URL,
				TimeoutSeconds: 1,
				CacheSeconds:   1,
			}, client)
			_, err := service.ListEvents(context.Background(), CalendarEventsQuery{
				DateFrom: mustCalendarServiceDate(t, "2026-08-10"),
				DateTo:   mustCalendarServiceDate(t, "2026-08-10"),
				Limit:    50,
			})
			if err == nil || !errors.Is(err, test.want) {
				t.Fatalf("error=%v want errors.Is(%v)", err, test.want)
			}
		})
	}
}

func TestCalendarClientDetailStatusHandling(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{status: http.StatusNotFound, want: ErrCalendarNotFound},
		{status: http.StatusUnauthorized, want: ErrCalendarUpstream},
		{status: http.StatusForbidden, want: ErrCalendarUpstream},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
			}))
			defer server.Close()
			service := newCalendarService(config.CalendarConfig{
				BaseURL:        server.URL,
				TimeoutSeconds: 1,
				CacheSeconds:   1,
			}, server.Client())

			_, err := service.GetEvent(context.Background(), dto.CalendarKindIPO, 99)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want errors.Is(%v)", err, test.want)
			}
		})
	}
}

func TestCalendarClientDoesNotForwardTokenAcrossRedirect(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Location", target.URL)
		response.WriteHeader(http.StatusFound)
	}))
	defer source.Close()
	service := newCalendarService(config.CalendarConfig{
		BaseURL:        source.URL,
		TimeoutSeconds: 1,
		CacheSeconds:   1,
		FeedToken:      "must-not-leave-source",
	}, source.Client())

	_, err := service.ListEvents(context.Background(), CalendarEventsQuery{
		DateFrom: mustCalendarServiceDate(t, "2026-08-10"),
		DateTo:   mustCalendarServiceDate(t, "2026-08-10"),
		Limit:    50,
	})
	if !errors.Is(err, ErrCalendarUpstream) {
		t.Fatalf("error=%v want redirect rejection", err)
	}
	if redirected.Load() {
		t.Fatal("calendar client followed an upstream redirect")
	}
}

func TestCalendarServiceCoalescesConcurrentFeedMisses(t *testing.T) {
	var calls atomic.Int32
	var startedOnce sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		_, _ = response.Write([]byte(`{"generated_at":"2026-08-15T00:00:00Z","date_from":"2026-08-09","date_to":"2026-08-11","economic":[],"earnings":[],"corporate":[],"ipo":[]}`))
	}))
	defer server.Close()

	service := newCalendarService(config.CalendarConfig{
		BaseURL:        server.URL,
		TimeoutSeconds: 2,
		CacheSeconds:   30,
	}, server.Client())
	query := CalendarEventsQuery{
		DateFrom: mustCalendarServiceDate(t, "2026-08-10"),
		DateTo:   mustCalendarServiceDate(t, "2026-08-10"),
		Limit:    50,
	}

	const requestCount = 12
	start := make(chan struct{})
	results := make(chan error, requestCount)
	var ready sync.WaitGroup
	ready.Add(requestCount)
	for range requestCount {
		go func() {
			ready.Done()
			<-start
			_, err := service.ListEvents(context.Background(), query)
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	<-started

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ListEvents(canceledContext, query); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error=%v want context.Canceled", err)
	}
	close(release)

	for range requestCount {
		if err := <-results; err != nil {
			t.Fatalf("concurrent calendar request failed: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream feed calls=%d want 1", got)
	}
}

func TestCalendarServiceLeaderCancellationDoesNotCancelSharedFeed(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		close(started)
		<-release
		_, _ = response.Write([]byte(`{"generated_at":"2026-08-15T00:00:00Z","date_from":"2026-08-09","date_to":"2026-08-11","economic":[],"earnings":[],"corporate":[],"ipo":[]}`))
	}))
	defer server.Close()

	service := newCalendarService(config.CalendarConfig{
		BaseURL:        server.URL,
		TimeoutSeconds: 2,
		CacheSeconds:   30,
	}, server.Client())
	query := CalendarEventsQuery{
		DateFrom: mustCalendarServiceDate(t, "2026-08-10"),
		DateTo:   mustCalendarServiceDate(t, "2026-08-10"),
		Limit:    50,
	}

	leaderContext, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := service.ListEvents(leaderContext, query)
		leaderResult <- err
	}()
	<-started
	cancelLeader()
	if err := <-leaderResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error=%v want context.Canceled", err)
	}

	waiterResult := make(chan error, 1)
	go func() {
		_, err := service.ListEvents(context.Background(), query)
		waiterResult <- err
	}()
	close(release)
	if err := <-waiterResult; err != nil {
		t.Fatalf("waiter failed after leader cancellation: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream feed calls=%d want 1", got)
	}
}

func TestCalendarServiceRejectsCursorAfterFeedSequenceChanges(t *testing.T) {
	const initialFeed = `{
  "generated_at":"2026-08-15T00:00:00Z","date_from":"2026-08-09","date_to":"2026-08-11",
  "economic":[],
  "earnings":[
    {"id":1,"fetched_at":"2026-08-15T00:00:00Z","report_date":"2026-08-10","company":"Alpha"},
    {"id":2,"fetched_at":"2026-08-15T00:00:00Z","report_date":"2026-08-10","company":"Bravo"},
    {"id":3,"fetched_at":"2026-08-15T00:00:00Z","report_date":"2026-08-10","company":"Charlie"}
  ],
  "corporate":[],"ipo":[]
}`
	tests := []struct {
		name           string
		refreshedFeed  string
		refreshedFirst int64
	}{
		{
			name: "insertion",
			refreshedFeed: `{
  "generated_at":"2026-08-15T00:01:00Z","date_from":"2026-08-09","date_to":"2026-08-11",
  "economic":[],
  "earnings":[
    {"id":1,"fetched_at":"2026-08-15T00:01:00Z","report_date":"2026-08-10","company":"Alpha"},
    {"id":2,"fetched_at":"2026-08-15T00:01:00Z","report_date":"2026-08-10","company":"Bravo"},
    {"id":3,"fetched_at":"2026-08-15T00:01:00Z","report_date":"2026-08-10","company":"Charlie"},
    {"id":4,"fetched_at":"2026-08-15T00:01:00Z","report_date":"2026-08-10","company":"Aaron"}
  ],
  "corporate":[],"ipo":[]
}`,
			refreshedFirst: 4,
		},
		{
			name: "reorder",
			refreshedFeed: `{
  "generated_at":"2026-08-15T00:01:00Z","date_from":"2026-08-09","date_to":"2026-08-11",
  "economic":[],
  "earnings":[
    {"id":1,"fetched_at":"2026-08-15T00:01:00Z","report_date":"2026-08-10","company":"Alpha"},
    {"id":2,"fetched_at":"2026-08-15T00:01:00Z","report_date":"2026-08-10","company":"Bravo"},
    {"id":3,"fetched_at":"2026-08-15T00:01:00Z","report_date":"2026-08-10","company":"Aardvark"}
  ],
  "corporate":[],"ipo":[]
}`,
			refreshedFirst: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var refreshed atomic.Bool
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if refreshed.Load() {
					_, _ = response.Write([]byte(test.refreshedFeed))
					return
				}
				_, _ = response.Write([]byte(initialFeed))
			}))
			defer server.Close()

			service := newCalendarService(config.CalendarConfig{
				BaseURL:        server.URL,
				TimeoutSeconds: 2,
				CacheSeconds:   1,
			}, server.Client())
			now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
			service.now = func() time.Time { return now }
			query := CalendarEventsQuery{
				DateFrom: mustCalendarServiceDate(t, "2026-08-10"),
				DateTo:   mustCalendarServiceDate(t, "2026-08-10"),
				Limit:    2,
			}

			firstPage, err := service.ListEvents(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			if len(firstPage.Results) != 2 || firstPage.Results[0].ID != 1 || firstPage.Results[1].ID != 2 || firstPage.Cursor == "" {
				t.Fatalf("initial page=%+v", firstPage)
			}

			refreshed.Store(true)
			now = now.Add(2 * time.Second)
			query.Cursor = firstPage.Cursor
			if _, err := service.ListEvents(context.Background(), query); !errors.Is(err, ErrCalendarCursorStale) {
				t.Fatalf("stale cursor error=%v want ErrCalendarCursorStale", err)
			}

			query.Cursor = ""
			refreshedPage, err := service.ListEvents(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			if len(refreshedPage.Results) == 0 || refreshedPage.Results[0].ID != test.refreshedFirst {
				t.Fatalf("refreshed page=%+v", refreshedPage)
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("upstream feed calls=%d want 2", got)
			}
		})
	}
}

func TestCalendarServiceDoesNotServeExpiredDataAfterRefreshFailure(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			response.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = response.Write([]byte(`{"generated_at":"2026-08-15T00:00:00Z","date_from":"2026-08-09","date_to":"2026-08-11","economic":[],"earnings":[],"corporate":[],"ipo":[]}`))
	}))
	defer server.Close()
	service := newCalendarService(config.CalendarConfig{
		BaseURL:        server.URL,
		TimeoutSeconds: 1,
		CacheSeconds:   1,
	}, server.Client())
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	query := CalendarEventsQuery{
		DateFrom: mustCalendarServiceDate(t, "2026-08-10"),
		DateTo:   mustCalendarServiceDate(t, "2026-08-10"),
		Limit:    50,
	}
	if _, err := service.ListEvents(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	now = now.Add(2 * time.Second)
	if _, err := service.ListEvents(context.Background(), query); !errors.Is(err, ErrCalendarUpstream) {
		t.Fatalf("error=%v want refresh failure", err)
	}
}

func TestValidateCalendarEventsQuery(t *testing.T) {
	valid := CalendarEventsQuery{
		DateFrom: mustCalendarServiceDate(t, "2026-08-01"),
		DateTo:   mustCalendarServiceDate(t, "2026-08-31"),
		Limit:    100,
	}
	if err := validateCalendarEventsQuery(valid); err != nil {
		t.Fatalf("31-day range must be valid: %v", err)
	}
	invalid := valid
	invalid.DateTo = mustCalendarServiceDate(t, "2026-09-01")
	if !errors.Is(validateCalendarEventsQuery(invalid), ErrCalendarInvalidQuery) {
		t.Fatal("32-day range must be rejected")
	}
	invalid = valid
	invalid.Kind = dto.CalendarKind("unknown")
	if !errors.Is(validateCalendarEventsQuery(invalid), ErrCalendarInvalidQuery) {
		t.Fatal("unknown kind must be rejected")
	}
	invalid = valid
	invalid.MinImportance = 4
	if !errors.Is(validateCalendarEventsQuery(invalid), ErrCalendarInvalidQuery) {
		t.Fatal("importance above 3 must be rejected")
	}
	invalid = valid
	invalid.Keyword = strings.Repeat("a", 101)
	if !errors.Is(validateCalendarEventsQuery(invalid), ErrCalendarInvalidQuery) {
		t.Fatal("keyword above 100 characters must be rejected")
	}
	invalid = valid
	invalid.Cursor = "2"
	if !errors.Is(validateCalendarEventsQuery(invalid), ErrCalendarInvalidQuery) {
		t.Fatal("unbound offset cursor must be rejected")
	}
}

func TestNormalizeCalendarFeedRejectsInvalidDate(t *testing.T) {
	_, err := normalizeCalendarFeed(upstreamCalendarFeed{
		Earnings: []upstreamEarningsEvent{{
			upstreamCalendarEvent: upstreamCalendarEvent{ID: 1},
			ReportDate:            "2026-8-1",
		}},
	})
	if !errors.Is(err, ErrCalendarUpstream) || !strings.Contains(err.Error(), "invalid event date") {
		t.Fatalf("error=%v", err)
	}
}

func mustCalendarServiceDate(t *testing.T, value string) time.Time {
	t.Helper()
	location, err := time.LoadLocation(CalendarTimezone)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := time.ParseInLocation(time.DateOnly, value, location)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
