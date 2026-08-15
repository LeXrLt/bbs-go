package dto

type CalendarKind string

const (
	CalendarKindEconomic  CalendarKind = "economic"
	CalendarKindEarnings  CalendarKind = "earnings"
	CalendarKindCorporate CalendarKind = "corporate"
	CalendarKindIPO       CalendarKind = "ipo"
)

func (kind CalendarKind) IsValid() bool {
	switch kind {
	case CalendarKindEconomic, CalendarKindEarnings, CalendarKindCorporate, CalendarKindIPO:
		return true
	default:
		return false
	}
}

// CalendarEvent is the BBS-facing union of all supported financial-calendar
// events. Common display fields are normalized while kind-specific source
// fields remain available with camelCase names.
type CalendarEvent struct {
	Kind        CalendarKind `json:"kind"`
	ID          int64        `json:"id"`
	Key         string       `json:"key"`
	SourceID    string       `json:"sourceId"`
	FetchedAt   string       `json:"fetchedAt"`
	Source      string       `json:"source"`
	Date        string       `json:"date"`
	DateTime    string       `json:"dateTime"`
	Title       string       `json:"title"`
	EventTime   *string      `json:"eventTime,omitempty"`
	Country     string       `json:"country,omitempty"`
	Indicator   string       `json:"indicator,omitempty"`
	Importance  *int         `json:"importance,omitempty"`
	Actual      *string      `json:"actual,omitempty"`
	Forecast    *string      `json:"forecast,omitempty"`
	Previous    *string      `json:"previous,omitempty"`
	ReportDate  string       `json:"reportDate,omitempty"`
	Ticker      *string      `json:"ticker,omitempty"`
	Exchange    *string      `json:"exchange,omitempty"`
	Company     *string      `json:"company,omitempty"`
	Period      *string      `json:"period,omitempty"`
	EventDate   string       `json:"eventDate,omitempty"`
	EventType   *string      `json:"eventType,omitempty"`
	Description *string      `json:"description,omitempty"`
	Timezone    *string      `json:"timezone,omitempty"`
	SourceURL   *string      `json:"sourceUrl,omitempty"`
	PriceLow    *float64     `json:"priceLow,omitempty"`
	PriceHigh   *float64     `json:"priceHigh,omitempty"`
	Status      *string      `json:"status,omitempty"`
}

type CalendarEventCounts struct {
	Economic  int `json:"economic"`
	Earnings  int `json:"earnings"`
	Corporate int `json:"corporate"`
	IPO       int `json:"ipo"`
}

func (counts *CalendarEventCounts) Increment(kind CalendarKind) {
	switch kind {
	case CalendarKindEconomic:
		counts.Economic++
	case CalendarKindEarnings:
		counts.Earnings++
	case CalendarKindCorporate:
		counts.Corporate++
	case CalendarKindIPO:
		counts.IPO++
	}
}

type CalendarEventsResponse struct {
	GeneratedAt string              `json:"generatedAt"`
	DateFrom    string              `json:"dateFrom"`
	DateTo      string              `json:"dateTo"`
	Timezone    string              `json:"timezone"`
	Counts      CalendarEventCounts `json:"counts"`
	Total       int                 `json:"total"`
	Results     []CalendarEvent     `json:"results"`
	Cursor      string              `json:"cursor"`
	HasMore     bool                `json:"hasMore"`
}
