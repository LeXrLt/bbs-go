package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/config"
)

const (
	CalendarTimezone        = "Asia/Shanghai"
	calendarCacheMaxEntries = 64
	calendarCursorVersion   = "v1"
)

var (
	ErrCalendarInvalidQuery = errors.New("invalid calendar query")
	ErrCalendarCursorStale  = errors.New("calendar cursor is stale")
)

var (
	calendarLocationOnce sync.Once
	calendarLocation     *time.Location
	calendarLocationErr  error
)

type CalendarEventsQuery struct {
	DateFrom      time.Time
	DateTo        time.Time
	Kind          dto.CalendarKind
	Keyword       string
	MinImportance int
	Cursor        string
	Limit         int
}

type calendarPageCursor struct {
	offset      int
	fingerprint string
}

type CalendarServiceProvider interface {
	ListEvents(ctx context.Context, query CalendarEventsQuery) (dto.CalendarEventsResponse, error)
	GetEvent(ctx context.Context, kind dto.CalendarKind, id int64) (dto.CalendarEvent, error)
}

var CalendarService CalendarServiceProvider = &calendarService{}

type cachedCalendarFeed struct {
	generatedAt string
	events      []dto.CalendarEvent
	cachedAt    time.Time
}

type calendarFeedCall struct {
	done  chan struct{}
	entry cachedCalendarFeed
	err   error
}

type calendarService struct {
	mu           sync.Mutex
	fixedConfig  *config.CalendarConfig
	httpClient   *http.Client
	activeConfig config.CalendarConfig
	client       *calendarClient
	cache        map[string]cachedCalendarFeed
	inflight     map[string]*calendarFeedCall
	now          func() time.Time
}

func newCalendarService(cfg config.CalendarConfig, httpClient *http.Client) *calendarService {
	normalizeCalendarConfig(&cfg)
	return &calendarService{
		fixedConfig: &cfg,
		httpClient:  httpClient,
		cache:       make(map[string]cachedCalendarFeed),
		inflight:    make(map[string]*calendarFeedCall),
		now:         time.Now,
	}
}

func (service *calendarService) ListEvents(ctx context.Context, query CalendarEventsQuery) (dto.CalendarEventsResponse, error) {
	if err := validateCalendarEventsQuery(query); err != nil {
		return dto.CalendarEventsResponse{}, err
	}
	pageCursor, err := decodeCalendarCursor(query.Cursor)
	if err != nil {
		return dto.CalendarEventsResponse{}, err
	}

	expandedFrom := query.DateFrom.AddDate(0, 0, -1).Format(time.DateOnly)
	expandedTo := query.DateTo.AddDate(0, 0, 1).Format(time.DateOnly)
	feed, err := service.loadFeed(ctx, expandedFrom, expandedTo)
	if err != nil {
		return dto.CalendarEventsResponse{}, err
	}

	dateFrom := query.DateFrom.Format(time.DateOnly)
	dateTo := query.DateTo.Format(time.DateOnly)
	events := make([]dto.CalendarEvent, 0, len(feed.events))
	for _, event := range feed.events {
		if event.Date >= dateFrom && event.Date <= dateTo {
			events = append(events, event)
		}
	}
	sortCalendarEvents(events)

	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	filtered := make([]dto.CalendarEvent, 0, len(events))
	for _, event := range events {
		if query.MinImportance > 0 && (event.Importance == nil || *event.Importance < query.MinImportance) {
			continue
		}
		if keyword != "" && !calendarEventMatchesKeyword(event, keyword) {
			continue
		}
		filtered = append(filtered, event)
	}

	var counts dto.CalendarEventCounts
	for _, event := range filtered {
		counts.Increment(event.Kind)
	}
	if query.Kind != "" {
		kindFiltered := make([]dto.CalendarEvent, 0, len(filtered))
		for _, event := range filtered {
			if event.Kind == query.Kind {
				kindFiltered = append(kindFiltered, event)
			}
		}
		filtered = kindFiltered
	}

	total := len(filtered)
	fingerprint := calendarPageFingerprint(filtered)
	start := pageCursor.offset
	if pageCursor.fingerprint != "" && pageCursor.fingerprint != fingerprint {
		return dto.CalendarEventsResponse{}, ErrCalendarCursorStale
	}
	if start > total {
		return dto.CalendarEventsResponse{}, ErrCalendarCursorStale
	}
	end := start + query.Limit
	if end > total {
		end = total
	}
	results := append([]dto.CalendarEvent(nil), filtered[start:end]...)
	if results == nil {
		results = make([]dto.CalendarEvent, 0)
	}
	hasMore := end < total
	nextCursor := ""
	if hasMore {
		nextCursor = encodeCalendarCursor(end, fingerprint)
	}

	return dto.CalendarEventsResponse{
		GeneratedAt: feed.generatedAt,
		DateFrom:    dateFrom,
		DateTo:      dateTo,
		Timezone:    CalendarTimezone,
		Counts:      counts,
		Total:       total,
		Results:     results,
		Cursor:      nextCursor,
		HasMore:     hasMore,
	}, nil
}

func (service *calendarService) GetEvent(ctx context.Context, kind dto.CalendarKind, id int64) (dto.CalendarEvent, error) {
	if !kind.IsValid() || id <= 0 {
		return dto.CalendarEvent{}, ErrCalendarInvalidQuery
	}
	client, _ := service.runtime()
	switch kind {
	case dto.CalendarKindEconomic:
		raw, err := client.fetchEconomicEvent(ctx, id)
		if err != nil {
			return dto.CalendarEvent{}, err
		}
		if err := validateDetailEvent(raw.upstreamCalendarEvent, kind, id); err != nil {
			return dto.CalendarEvent{}, err
		}
		return normalizeEconomicEvent(raw)
	case dto.CalendarKindEarnings:
		raw, err := client.fetchEarningsEvent(ctx, id)
		if err != nil {
			return dto.CalendarEvent{}, err
		}
		if err := validateDetailEvent(raw.upstreamCalendarEvent, kind, id); err != nil {
			return dto.CalendarEvent{}, err
		}
		return normalizeEarningsEvent(raw)
	case dto.CalendarKindCorporate:
		raw, err := client.fetchCorporateEvent(ctx, id)
		if err != nil {
			return dto.CalendarEvent{}, err
		}
		if err := validateDetailEvent(raw.upstreamCalendarEvent, kind, id); err != nil {
			return dto.CalendarEvent{}, err
		}
		return normalizeCorporateEvent(raw)
	case dto.CalendarKindIPO:
		raw, err := client.fetchIPOEvent(ctx, id)
		if err != nil {
			return dto.CalendarEvent{}, err
		}
		if err := validateDetailEvent(raw.upstreamCalendarEvent, kind, id); err != nil {
			return dto.CalendarEvent{}, err
		}
		return normalizeIPOEvent(raw)
	default:
		return dto.CalendarEvent{}, ErrCalendarInvalidQuery
	}
}

func (service *calendarService) loadFeed(ctx context.Context, dateFrom, dateTo string) (cachedCalendarFeed, error) {
	if err := ctx.Err(); err != nil {
		return cachedCalendarFeed{}, err
	}
	client, cfg := service.runtime()
	cacheKey := dateFrom + ":" + dateTo
	flightKey := fmt.Sprintf("%p:%s", client, cacheKey)
	now := service.currentTime()

	service.mu.Lock()
	if service.cache == nil {
		service.cache = make(map[string]cachedCalendarFeed)
	}
	if service.inflight == nil {
		service.inflight = make(map[string]*calendarFeedCall)
	}
	if entry, ok := service.cache[cacheKey]; ok {
		if now.Sub(entry.cachedAt) < time.Duration(cfg.CacheSeconds)*time.Second {
			service.mu.Unlock()
			return entry, nil
		}
		delete(service.cache, cacheKey)
	}
	if call, ok := service.inflight[flightKey]; ok {
		service.mu.Unlock()
		return waitForCalendarFeed(ctx, call)
	}
	call := &calendarFeedCall{done: make(chan struct{})}
	service.inflight[flightKey] = call
	service.mu.Unlock()

	go service.fetchCalendarFeed(client, cfg, cacheKey, flightKey, dateFrom, dateTo, now, call)
	return waitForCalendarFeed(ctx, call)
}

func waitForCalendarFeed(ctx context.Context, call *calendarFeedCall) (cachedCalendarFeed, error) {
	select {
	case <-call.done:
		return call.entry, call.err
	case <-ctx.Done():
		return cachedCalendarFeed{}, ctx.Err()
	}
}

func (service *calendarService) fetchCalendarFeed(
	client *calendarClient,
	cfg config.CalendarConfig,
	cacheKey, flightKey, dateFrom, dateTo string,
	now time.Time,
	call *calendarFeedCall,
) {
	// The flight is shared by every waiter, so no individual request owns its
	// cancellation. A service-owned deadline still bounds the upstream call.
	fetchContext, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	rawFeed, err := client.fetchFeed(fetchContext, dateFrom, dateTo)
	if err == nil {
		err = validateCalendarFeedMetadata(rawFeed, dateFrom, dateTo)
	}
	var events []dto.CalendarEvent
	if err == nil {
		events, err = normalizeCalendarFeed(rawFeed)
	}
	generatedAt := strings.TrimSpace(rawFeed.GeneratedAt)
	entry := cachedCalendarFeed{generatedAt: generatedAt, events: events, cachedAt: now}

	service.mu.Lock()
	if err == nil && service.client == client && service.activeConfig == cfg {
		service.evictCalendarCache(now, time.Duration(cfg.CacheSeconds)*time.Second)
		service.cache[cacheKey] = entry
	}
	call.entry = entry
	call.err = err
	delete(service.inflight, flightKey)
	close(call.done)
	service.mu.Unlock()
}

func (service *calendarService) runtime() (*calendarClient, config.CalendarConfig) {
	cfg := service.calendarConfig()
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.client == nil || service.activeConfig != cfg {
		service.activeConfig = cfg
		service.client = newCalendarClient(cfg, service.httpClient)
		service.cache = make(map[string]cachedCalendarFeed)
	}
	return service.client, cfg
}

func (service *calendarService) calendarConfig() config.CalendarConfig {
	if service.fixedConfig != nil {
		return *service.fixedConfig
	}
	cfg := config.CalendarConfig{}
	if config.Instance != nil {
		cfg = config.Instance.Calendar
	}
	cfg.FeedToken = strings.TrimSpace(os.Getenv(config.BBSGO_CALENDAR_FEED_TOKEN))
	normalizeCalendarConfig(&cfg)
	return cfg
}

func normalizeCalendarConfig(cfg *config.CalendarConfig) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = config.DefaultCalendarBaseURL
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = config.DefaultCalendarTimeoutSeconds
	}
	if cfg.CacheSeconds <= 0 {
		cfg.CacheSeconds = config.DefaultCalendarCacheSeconds
	}
	cfg.FeedToken = strings.TrimSpace(cfg.FeedToken)
}

func (service *calendarService) currentTime() time.Time {
	if service.now != nil {
		return service.now()
	}
	return time.Now()
}

func (service *calendarService) evictCalendarCache(now time.Time, ttl time.Duration) {
	for key, entry := range service.cache {
		if now.Sub(entry.cachedAt) >= ttl {
			delete(service.cache, key)
		}
	}
	for len(service.cache) >= calendarCacheMaxEntries {
		var oldestKey string
		var oldestTime time.Time
		for key, entry := range service.cache {
			if oldestKey == "" || entry.cachedAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.cachedAt
			}
		}
		delete(service.cache, oldestKey)
	}
}

func validateCalendarEventsQuery(query CalendarEventsQuery) error {
	if query.DateFrom.IsZero() || query.DateTo.IsZero() || query.DateFrom.After(query.DateTo) {
		return ErrCalendarInvalidQuery
	}
	if query.DateTo.Sub(query.DateFrom) > 30*24*time.Hour {
		return ErrCalendarInvalidQuery
	}
	if query.Kind != "" && !query.Kind.IsValid() {
		return ErrCalendarInvalidQuery
	}
	if query.MinImportance < 0 || query.MinImportance > 3 {
		return ErrCalendarInvalidQuery
	}
	if utf8.RuneCountInString(strings.TrimSpace(query.Keyword)) > 100 {
		return ErrCalendarInvalidQuery
	}
	if _, err := decodeCalendarCursor(query.Cursor); err != nil {
		return err
	}
	if query.Limit <= 0 || query.Limit > 100 {
		return ErrCalendarInvalidQuery
	}
	return nil
}

func ValidateCalendarCursor(cursor string) error {
	_, err := decodeCalendarCursor(cursor)
	return err
}

func decodeCalendarCursor(raw string) (calendarPageCursor, error) {
	cursor := strings.TrimSpace(raw)
	if cursor == "" || cursor == "0" {
		return calendarPageCursor{}, nil
	}
	parts := strings.Split(cursor, ":")
	if len(parts) != 3 || parts[0] != calendarCursorVersion {
		return calendarPageCursor{}, ErrCalendarInvalidQuery
	}
	offset, err := strconv.Atoi(parts[1])
	if err != nil || offset <= 0 {
		return calendarPageCursor{}, ErrCalendarInvalidQuery
	}
	if len(parts[2]) != sha256.Size*2 {
		return calendarPageCursor{}, ErrCalendarInvalidQuery
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return calendarPageCursor{}, ErrCalendarInvalidQuery
	}
	return calendarPageCursor{offset: offset, fingerprint: strings.ToLower(parts[2])}, nil
}

func encodeCalendarCursor(offset int, fingerprint string) string {
	return calendarCursorVersion + ":" + strconv.Itoa(offset) + ":" + fingerprint
}

func calendarPageFingerprint(events []dto.CalendarEvent) string {
	hash := sha256.New()
	for _, event := range events {
		_, _ = hash.Write([]byte(event.Kind))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(event.ID, 10)))
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validateDetailEvent(raw upstreamCalendarEvent, expectedKind dto.CalendarKind, expectedID int64) error {
	if raw.Kind != string(expectedKind) {
		return fmt.Errorf("%w: detail kind mismatch", ErrCalendarUpstream)
	}
	if raw.ID != expectedID {
		return fmt.Errorf("%w: detail ID mismatch", ErrCalendarUpstream)
	}
	return nil
}

func validateCalendarFeedMetadata(feed upstreamCalendarFeed, expectedFrom, expectedTo string) error {
	if feed.DateFrom != expectedFrom || feed.DateTo != expectedTo {
		return fmt.Errorf("%w: feed date range mismatch", ErrCalendarUpstream)
	}
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(feed.GeneratedAt)); err != nil {
		return fmt.Errorf("%w: invalid generated_at", ErrCalendarUpstream)
	}
	return nil
}

func normalizeCalendarFeed(feed upstreamCalendarFeed) ([]dto.CalendarEvent, error) {
	capacity := len(feed.Economic) + len(feed.Earnings) + len(feed.Corporate) + len(feed.IPO)
	events := make([]dto.CalendarEvent, 0, capacity)
	for _, raw := range feed.Economic {
		event, err := normalizeEconomicEvent(raw)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	for _, raw := range feed.Earnings {
		event, err := normalizeEarningsEvent(raw)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	for _, raw := range feed.Corporate {
		event, err := normalizeCorporateEvent(raw)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	for _, raw := range feed.IPO {
		event, err := normalizeIPOEvent(raw)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func normalizeEconomicEvent(raw upstreamEconomicEvent) (dto.CalendarEvent, error) {
	if err := validateUpstreamCalendarEvent(raw.upstreamCalendarEvent); err != nil {
		return dto.CalendarEvent{}, err
	}
	date, dateTime, err := normalizeCalendarDateTime(raw.EventTime)
	if err != nil {
		return dto.CalendarEvent{}, err
	}
	eventTime := raw.EventTime
	event := newCalendarEvent(dto.CalendarKindEconomic, raw.upstreamCalendarEvent, date, dateTime, firstCalendarText(raw.Indicator, raw.Country, "Economic event"))
	event.EventTime = &eventTime
	event.Country = raw.Country
	event.Indicator = raw.Indicator
	event.Importance = raw.Importance
	event.Actual = raw.Actual
	event.Forecast = raw.Forecast
	event.Previous = raw.Previous
	return event, nil
}

func normalizeEarningsEvent(raw upstreamEarningsEvent) (dto.CalendarEvent, error) {
	if err := validateUpstreamCalendarEvent(raw.upstreamCalendarEvent); err != nil {
		return dto.CalendarEvent{}, err
	}
	date, err := normalizeCalendarDate(raw.ReportDate)
	if err != nil {
		return dto.CalendarEvent{}, err
	}
	title := firstCalendarPointerText(raw.Company, raw.Ticker)
	if period := calendarPointerText(raw.Period); period != "" {
		if title == "" {
			title = period
		} else if !strings.Contains(strings.ToLower(title), strings.ToLower(period)) {
			title += " - " + period
		}
	}
	event := newCalendarEvent(dto.CalendarKindEarnings, raw.upstreamCalendarEvent, date, "", firstCalendarText(title, "Earnings event"))
	event.ReportDate = raw.ReportDate
	event.Ticker = raw.Ticker
	event.Exchange = raw.Exchange
	event.Company = raw.Company
	event.Period = raw.Period
	return event, nil
}

func normalizeCorporateEvent(raw upstreamCorporateEvent) (dto.CalendarEvent, error) {
	if err := validateUpstreamCalendarEvent(raw.upstreamCalendarEvent); err != nil {
		return dto.CalendarEvent{}, err
	}
	date, err := normalizeCalendarDate(raw.EventDate)
	if err != nil {
		return dto.CalendarEvent{}, err
	}
	dateTime := ""
	if raw.EventTime != nil && strings.TrimSpace(*raw.EventTime) != "" {
		date, dateTime, err = normalizeCalendarDateTime(*raw.EventTime)
		if err != nil {
			return dto.CalendarEvent{}, err
		}
	}
	title := firstCalendarPointerText(raw.Title, raw.Company, raw.Description, raw.EventType, raw.Ticker)
	event := newCalendarEvent(dto.CalendarKindCorporate, raw.upstreamCalendarEvent, date, dateTime, firstCalendarText(title, "Corporate event"))
	event.EventDate = raw.EventDate
	event.Ticker = raw.Ticker
	event.EventType = raw.EventType
	event.Company = raw.Company
	event.Description = raw.Description
	event.EventTime = raw.EventTime
	event.Timezone = raw.Timezone
	event.SourceURL = raw.SourceURL
	return event, nil
}

func normalizeIPOEvent(raw upstreamIPOEvent) (dto.CalendarEvent, error) {
	if err := validateUpstreamCalendarEvent(raw.upstreamCalendarEvent); err != nil {
		return dto.CalendarEvent{}, err
	}
	date, err := normalizeCalendarDate(raw.EventDate)
	if err != nil {
		return dto.CalendarEvent{}, err
	}
	title := firstCalendarPointerText(raw.Company, raw.Ticker)
	event := newCalendarEvent(dto.CalendarKindIPO, raw.upstreamCalendarEvent, date, "", firstCalendarText(title, "IPO event"))
	event.EventDate = raw.EventDate
	event.Ticker = raw.Ticker
	event.Company = raw.Company
	event.Exchange = raw.Exchange
	event.PriceLow = raw.PriceLow
	event.PriceHigh = raw.PriceHigh
	event.Status = raw.Status
	return event, nil
}

func validateUpstreamCalendarEvent(raw upstreamCalendarEvent) error {
	if raw.ID <= 0 {
		return fmt.Errorf("%w: invalid event ID", ErrCalendarUpstream)
	}
	return nil
}

func newCalendarEvent(kind dto.CalendarKind, raw upstreamCalendarEvent, date, dateTime, title string) dto.CalendarEvent {
	sourceID := calendarPointerText(raw.SourceID)
	keyID := sourceID
	if keyID == "" {
		keyID = strconv.FormatInt(raw.ID, 10)
	}
	return dto.CalendarEvent{
		Kind:      kind,
		ID:        raw.ID,
		Key:       string(kind) + ":" + keyID,
		SourceID:  sourceID,
		FetchedAt: raw.FetchedAt,
		Source:    raw.Source,
		Date:      date,
		DateTime:  dateTime,
		Title:     title,
	}
}

func normalizeCalendarDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	parsed, err := time.Parse(time.DateOnly, trimmed)
	if err != nil || parsed.Format(time.DateOnly) != trimmed {
		return "", fmt.Errorf("%w: invalid event date %q", ErrCalendarUpstream, value)
	}
	return trimmed, nil
}

func normalizeCalendarDateTime(value string) (string, string, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid event time %q", ErrCalendarUpstream, value)
	}
	shanghai, err := loadCalendarLocation()
	if err != nil {
		return "", "", fmt.Errorf("load calendar timezone: %w", err)
	}
	localized := parsed.In(shanghai)
	return localized.Format(time.DateOnly), localized.Format(time.RFC3339), nil
}

func loadCalendarLocation() (*time.Location, error) {
	calendarLocationOnce.Do(func() {
		calendarLocation, calendarLocationErr = time.LoadLocation(CalendarTimezone)
	})
	return calendarLocation, calendarLocationErr
}

func sortCalendarEvents(events []dto.CalendarEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		left, right := events[i], events[j]
		if left.Date != right.Date {
			return left.Date < right.Date
		}
		if left.DateTime != right.DateTime {
			return left.DateTime < right.DateTime
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		leftTitle, rightTitle := strings.ToLower(left.Title), strings.ToLower(right.Title)
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
		return left.ID < right.ID
	})
}

func calendarEventMatchesKeyword(event dto.CalendarEvent, keyword string) bool {
	values := []string{
		event.Title,
		event.Source,
		event.Country,
		event.Indicator,
		event.ReportDate,
		event.EventDate,
		calendarPointerText(event.Ticker),
		calendarPointerText(event.Exchange),
		calendarPointerText(event.Company),
		calendarPointerText(event.Period),
		calendarPointerText(event.EventType),
		calendarPointerText(event.Description),
		calendarPointerText(event.Status),
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), keyword) {
			return true
		}
	}
	return false
}

func calendarPointerText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstCalendarPointerText(values ...*string) string {
	for _, value := range values {
		if text := calendarPointerText(value); text != "" {
			return text
		}
	}
	return ""
}

func firstCalendarText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
