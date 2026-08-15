package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bbs-go/internal/pkg/config"
)

func TestCalendarClientBoundsGlobalConcurrentRequests(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	var upstreamCalls atomic.Int32
	started := make(chan struct{}, calendarMaxConcurrentRequests)
	release := make(chan struct{})
	var releaseOnce sync.Once

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		current := active.Add(1)
		defer active.Add(-1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		_, _ = response.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
	})

	newClient := func() *calendarClient {
		return newCalendarClient(config.CalendarConfig{
			BaseURL:        server.URL,
			TimeoutSeconds: 10,
		}, server.Client())
	}
	firstClient := newClient()
	secondClient := newClient()
	results := make(chan error, calendarMaxConcurrentRequests)
	for range calendarMaxConcurrentRequests {
		go func() {
			_, err := firstClient.fetchFeed(context.Background(), "2026-08-14", "2026-08-16")
			results <- err
		}()
	}

	waitForAllCalendarRequests(t, started, calendarMaxConcurrentRequests)
	if got := active.Load(); got != calendarMaxConcurrentRequests {
		t.Fatalf("active upstream requests=%d want=%d", got, calendarMaxConcurrentRequests)
	}

	before := time.Now()
	_, err := secondClient.fetchFeed(context.Background(), "2026-08-17", "2026-08-19")
	if !errors.Is(err, ErrCalendarBusy) {
		t.Fatalf("over-limit error=%v want errors.Is(ErrCalendarBusy)", err)
	}
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("over-limit request blocked for %s", elapsed)
	}
	if got := upstreamCalls.Load(); got != calendarMaxConcurrentRequests {
		t.Fatalf("upstream calls=%d want=%d; over-limit request reached upstream", got, calendarMaxConcurrentRequests)
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = secondClient.fetchFeed(canceledContext, "2026-08-17", "2026-08-19")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled over-limit error=%v want context.Canceled", err)
	}

	releaseOnce.Do(func() { close(release) })
	for range calendarMaxConcurrentRequests {
		if err := <-results; err != nil {
			t.Fatalf("accepted calendar request failed: %v", err)
		}
	}
	if got := maxActive.Load(); got != calendarMaxConcurrentRequests {
		t.Fatalf("maximum concurrent upstream requests=%d want=%d", got, calendarMaxConcurrentRequests)
	}

	if _, err := secondClient.fetchFeed(context.Background(), "2026-08-17", "2026-08-19"); err != nil {
		t.Fatalf("request after slot release failed: %v", err)
	}
	if got := upstreamCalls.Load(); got != calendarMaxConcurrentRequests+1 {
		t.Fatalf("upstream calls after release=%d want=%d", got, calendarMaxConcurrentRequests+1)
	}
}

func waitForAllCalendarRequests(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for range count {
		select {
		case <-started:
		case <-timer.C:
			t.Fatalf("timed out waiting for %d concurrent calendar requests", count)
		}
	}
}
