package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/config"
)

const (
	calendarMaxResponseBytes      int64 = 8 << 20
	calendarMaxConcurrentRequests       = 8
)

var (
	ErrCalendarNotFound = errors.New("calendar event not found")
	ErrCalendarUpstream = errors.New("calendar upstream unavailable")
	ErrCalendarBusy     = errors.New("calendar request capacity exceeded")

	calendarOutboundSlots = make(chan struct{}, calendarMaxConcurrentRequests)
)

type upstreamCalendarEvent struct {
	Kind      string  `json:"kind"`
	ID        int64   `json:"id"`
	SourceID  *string `json:"source_id"`
	FetchedAt string  `json:"fetched_at"`
	Source    string  `json:"source"`
}

type upstreamEconomicEvent struct {
	upstreamCalendarEvent
	EventTime  string  `json:"event_time"`
	Country    string  `json:"country"`
	Indicator  string  `json:"indicator"`
	Importance *int    `json:"importance"`
	Actual     *string `json:"actual"`
	Forecast   *string `json:"forecast"`
	Previous   *string `json:"previous"`
}

type upstreamEarningsEvent struct {
	upstreamCalendarEvent
	ReportDate string  `json:"report_date"`
	Ticker     *string `json:"ticker"`
	Exchange   *string `json:"exchange"`
	Company    *string `json:"company"`
	Period     *string `json:"period"`
}

type upstreamCorporateEvent struct {
	upstreamCalendarEvent
	EventDate   string  `json:"event_date"`
	Ticker      *string `json:"ticker"`
	EventType   *string `json:"event_type"`
	Title       *string `json:"title"`
	Company     *string `json:"company"`
	Description *string `json:"description"`
	EventTime   *string `json:"event_time"`
	Timezone    *string `json:"timezone"`
	SourceURL   *string `json:"source_url"`
}

type upstreamIPOEvent struct {
	upstreamCalendarEvent
	EventDate string   `json:"event_date"`
	Ticker    *string  `json:"ticker"`
	Company   *string  `json:"company"`
	Exchange  *string  `json:"exchange"`
	PriceLow  *float64 `json:"price_low"`
	PriceHigh *float64 `json:"price_high"`
	Status    *string  `json:"status"`
}

type upstreamCalendarFeed struct {
	GeneratedAt string                   `json:"generated_at"`
	DateFrom    string                   `json:"date_from"`
	DateTo      string                   `json:"date_to"`
	Economic    []upstreamEconomicEvent  `json:"economic"`
	Earnings    []upstreamEarningsEvent  `json:"earnings"`
	Corporate   []upstreamCorporateEvent `json:"corporate"`
	IPO         []upstreamIPOEvent       `json:"ipo"`
}

type calendarClient struct {
	baseURL    string
	feedToken  string
	httpClient *http.Client
}

func newCalendarClient(cfg config.CalendarConfig, httpClient *http.Client) *calendarClient {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else {
		copyOfClient := *httpClient
		if copyOfClient.Timeout <= 0 {
			copyOfClient.Timeout = timeout
		}
		httpClient = &copyOfClient
	}
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &calendarClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		feedToken:  strings.TrimSpace(cfg.FeedToken),
		httpClient: httpClient,
	}
}

func (client *calendarClient) fetchFeed(ctx context.Context, dateFrom, dateTo string) (upstreamCalendarFeed, error) {
	query := url.Values{}
	query.Set("date_from", dateFrom)
	query.Set("date_to", dateTo)
	body, err := client.get(ctx, "/api/feed", query, false)
	if err != nil {
		return upstreamCalendarFeed{}, err
	}
	var feed upstreamCalendarFeed
	if err := json.Unmarshal(body, &feed); err != nil {
		return upstreamCalendarFeed{}, fmt.Errorf("%w: decode feed: %v", ErrCalendarUpstream, err)
	}
	return feed, nil
}

func (client *calendarClient) fetchEconomicEvent(ctx context.Context, id int64) (upstreamEconomicEvent, error) {
	body, err := client.fetchEventBody(ctx, dto.CalendarKindEconomic, id)
	if err != nil {
		return upstreamEconomicEvent{}, err
	}
	var event upstreamEconomicEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return upstreamEconomicEvent{}, fmt.Errorf("%w: decode economic event: %v", ErrCalendarUpstream, err)
	}
	return event, nil
}

func (client *calendarClient) fetchEarningsEvent(ctx context.Context, id int64) (upstreamEarningsEvent, error) {
	body, err := client.fetchEventBody(ctx, dto.CalendarKindEarnings, id)
	if err != nil {
		return upstreamEarningsEvent{}, err
	}
	var event upstreamEarningsEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return upstreamEarningsEvent{}, fmt.Errorf("%w: decode earnings event: %v", ErrCalendarUpstream, err)
	}
	return event, nil
}

func (client *calendarClient) fetchCorporateEvent(ctx context.Context, id int64) (upstreamCorporateEvent, error) {
	body, err := client.fetchEventBody(ctx, dto.CalendarKindCorporate, id)
	if err != nil {
		return upstreamCorporateEvent{}, err
	}
	var event upstreamCorporateEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return upstreamCorporateEvent{}, fmt.Errorf("%w: decode corporate event: %v", ErrCalendarUpstream, err)
	}
	return event, nil
}

func (client *calendarClient) fetchIPOEvent(ctx context.Context, id int64) (upstreamIPOEvent, error) {
	body, err := client.fetchEventBody(ctx, dto.CalendarKindIPO, id)
	if err != nil {
		return upstreamIPOEvent{}, err
	}
	var event upstreamIPOEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return upstreamIPOEvent{}, fmt.Errorf("%w: decode IPO event: %v", ErrCalendarUpstream, err)
	}
	return event, nil
}

func (client *calendarClient) fetchEventBody(ctx context.Context, kind dto.CalendarKind, id int64) ([]byte, error) {
	return client.get(ctx, "/api/event/"+string(kind)+"/"+strconv.FormatInt(id, 10), nil, true)
}

func (client *calendarClient) get(ctx context.Context, path string, query url.Values, notFoundIsEvent bool) ([]byte, error) {
	endpoint, err := url.Parse(client.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("%w: build request URL: %v", ErrCalendarUpstream, err)
	}
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrCalendarUpstream, err)
	}
	request.Header.Set("Accept", "application/json")
	if client.feedToken != "" {
		request.Header.Set("X-Feed-Token", client.feedToken)
	}
	if err := acquireCalendarOutboundSlot(ctx); err != nil {
		return nil, err
	}
	defer releaseCalendarOutboundSlot()

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed: %w", ErrCalendarUpstream, err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound && notFoundIsEvent {
		return nil, ErrCalendarNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP %d", ErrCalendarUpstream, response.StatusCode)
	}
	if response.ContentLength > calendarMaxResponseBytes {
		return nil, fmt.Errorf("%w: response exceeds %d bytes", ErrCalendarUpstream, calendarMaxResponseBytes)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, calendarMaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrCalendarUpstream, err)
	}
	if int64(len(body)) > calendarMaxResponseBytes {
		return nil, fmt.Errorf("%w: response exceeds %d bytes", ErrCalendarUpstream, calendarMaxResponseBytes)
	}
	return body, nil
}

func acquireCalendarOutboundSlot(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	select {
	case calendarOutboundSlots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			releaseCalendarOutboundSlot()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrCalendarBusy
	}
}

func releaseCalendarOutboundSlot() {
	<-calendarOutboundSlots
}
