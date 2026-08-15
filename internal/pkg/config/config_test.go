package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

func TestSetDbDefaultsSetsDefaultLogLevel(t *testing.T) {
	var cfg DBConfig

	SetDbDefaults(&cfg)

	if cfg.LogLevel != DefaultDBLogLevel {
		t.Fatalf("expected default db log level %q, got %q", DefaultDBLogLevel, cfg.LogLevel)
	}
}

func TestReadConfigLoginRequiredDefaultsEnabled(t *testing.T) {
	t.Setenv(BBSGO_LOGIN_REQUIRED, "")
	reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))

	cfg, exists, err := readConfig(reader)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("missing config file must report exists=false")
	}
	if !cfg.LoginRequired {
		t.Fatal("loginRequired must default to enabled")
	}
}

func TestReadConfigLoginRequiredEnvironmentOverridesFile(t *testing.T) {
	t.Setenv(BBSGO_LOGIN_REQUIRED, "false")
	configPath := filepath.Join(t.TempDir(), "bbs-go.yaml")
	if err := os.WriteFile(configPath, []byte("loginRequired: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := newTestConfigReader(t, configPath)

	cfg, exists, err := readConfig(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("config file must report exists=true")
	}
	if cfg.LoginRequired {
		t.Fatal("BBSGO_LOGIN_REQUIRED=false must override the config file")
	}
}

func TestReadConfigLoginRequiredEnvironmentWorksWithoutFile(t *testing.T) {
	t.Setenv(BBSGO_LOGIN_REQUIRED, "false")
	reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))

	cfg, _, err := readConfig(reader)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LoginRequired {
		t.Fatal("BBSGO_LOGIN_REQUIRED=false must work without a config file")
	}
}

func TestReadConfigRejectsInvalidLoginRequiredEnvironment(t *testing.T) {
	t.Setenv(BBSGO_LOGIN_REQUIRED, "enabled")
	reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))

	_, _, err := readConfig(reader)
	if err == nil || !strings.Contains(err.Error(), BBSGO_LOGIN_REQUIRED) {
		t.Fatalf("expected an invalid %s error, got %v", BBSGO_LOGIN_REQUIRED, err)
	}
}

func TestReadConfigCalendarDefaults(t *testing.T) {
	t.Setenv(BBSGO_CALENDAR_FEED_TOKEN, "")
	reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))

	cfg, _, err := readConfig(reader)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Calendar.BaseURL != DefaultCalendarBaseURL {
		t.Fatalf("baseURL=%q want %q", cfg.Calendar.BaseURL, DefaultCalendarBaseURL)
	}
	if cfg.Calendar.TimeoutSeconds != DefaultCalendarTimeoutSeconds {
		t.Fatalf("timeoutSeconds=%d want %d", cfg.Calendar.TimeoutSeconds, DefaultCalendarTimeoutSeconds)
	}
	if cfg.Calendar.CacheSeconds != DefaultCalendarCacheSeconds {
		t.Fatalf("cacheSeconds=%d want %d", cfg.Calendar.CacheSeconds, DefaultCalendarCacheSeconds)
	}
	if cfg.Calendar.FeedToken != "" {
		t.Fatal("feed token must default to empty")
	}
}

func TestReadConfigCalendarEnvironmentOverridesFile(t *testing.T) {
	t.Setenv(BBSGO_CALENDAR_BASE_URL, "https://calendar-env.example.com/root/")
	t.Setenv(BBSGO_CALENDAR_TIMEOUT_SECONDS, "4")
	t.Setenv(BBSGO_CALENDAR_CACHE_SECONDS, "12")
	t.Setenv(BBSGO_CALENDAR_FEED_TOKEN, " top-secret ")
	configPath := filepath.Join(t.TempDir(), "bbs-go.yaml")
	content := "calendar:\n  baseUrl: https://calendar-file.example.com\n  timeoutSeconds: 9\n  cacheSeconds: 20\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	reader := newTestConfigReader(t, configPath)

	cfg, exists, err := readConfig(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected config file to exist")
	}
	if cfg.Calendar.BaseURL != "https://calendar-env.example.com/root" {
		t.Fatalf("baseURL=%q", cfg.Calendar.BaseURL)
	}
	if cfg.Calendar.TimeoutSeconds != 4 || cfg.Calendar.CacheSeconds != 12 {
		t.Fatalf("calendar durations=%+v", cfg.Calendar)
	}
	if cfg.Calendar.FeedToken != "top-secret" {
		t.Fatal("feed token environment value was not loaded")
	}
}

func TestReadConfigCalendarHTTPRequiresNoFeedToken(t *testing.T) {
	tests := []struct {
		name      string
		feedToken string
		wantErr   bool
	}{
		{name: "without token", feedToken: ""},
		{name: "with token", feedToken: "feed-secret", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(BBSGO_CALENDAR_BASE_URL, "http://calendar.example.com")
			t.Setenv(BBSGO_CALENDAR_FEED_TOKEN, test.feedToken)
			reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))

			cfg, _, err := readConfig(reader)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "HTTPS") {
					t.Fatalf("expected HTTPS validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Calendar.BaseURL != "http://calendar.example.com" {
				t.Fatalf("baseURL=%q", cfg.Calendar.BaseURL)
			}
		})
	}
}

func TestReadConfigCalendarBaseURLErrorDoesNotLeakInput(t *testing.T) {
	t.Setenv(BBSGO_CALENDAR_FEED_TOKEN, "")
	tests := []struct {
		name   string
		value  string
		secret string
	}{
		{name: "credentials", value: "https://calendar-user:credential-secret@example.com", secret: "credential-secret"},
		{name: "query", value: "https://example.com/feed?api_key=query-secret", secret: "query-secret"},
		{name: "fragment", value: "https://example.com/feed#fragment-secret", secret: "fragment-secret"},
		{name: "parse error", value: "https://example.com/%zz/malformed-secret", secret: "malformed-secret"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(BBSGO_CALENDAR_BASE_URL, test.value)
			reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))

			_, _, err := readConfig(reader)
			if err == nil || !strings.Contains(err.Error(), BBSGO_CALENDAR_BASE_URL) {
				t.Fatalf("expected base URL validation error, got %v", err)
			}
			if strings.Contains(err.Error(), test.value) || strings.Contains(err.Error(), test.secret) {
				t.Fatalf("validation error leaked calendar URL input: %v", err)
			}
		})
	}
}

func TestReadConfigCalendarDurationBounds(t *testing.T) {
	t.Setenv(BBSGO_CALENDAR_FEED_TOKEN, "")
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "maximum timeout", envName: BBSGO_CALENDAR_TIMEOUT_SECONDS, value: strconv.Itoa(MaxCalendarTimeoutSeconds)},
		{name: "maximum cache lifetime", envName: BBSGO_CALENDAR_CACHE_SECONDS, value: strconv.Itoa(MaxCalendarCacheSeconds)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.envName, test.value)
			reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))
			if _, _, err := readConfig(reader); err != nil {
				t.Fatalf("maximum value must be accepted: %v", err)
			}
		})
	}

	invalid := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "timeout above maximum", envName: BBSGO_CALENDAR_TIMEOUT_SECONDS, value: strconv.Itoa(MaxCalendarTimeoutSeconds + 1)},
		{name: "cache lifetime above maximum", envName: BBSGO_CALENDAR_CACHE_SECONDS, value: strconv.Itoa(MaxCalendarCacheSeconds + 1)},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.envName, test.value)
			reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))
			_, _, err := readConfig(reader)
			if err == nil || !strings.Contains(err.Error(), test.envName) {
				t.Fatalf("expected bounded integer validation error, got %v", err)
			}
		})
	}
}

func TestReadConfigRejectsInvalidCalendarEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "base URL", envName: BBSGO_CALENDAR_BASE_URL, value: "ftp://calendar.example.com"},
		{name: "timeout", envName: BBSGO_CALENDAR_TIMEOUT_SECONDS, value: "zero"},
		{name: "cache", envName: BBSGO_CALENDAR_CACHE_SECONDS, value: "0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.envName, test.value)
			reader := newTestConfigReader(t, filepath.Join(t.TempDir(), "missing.yaml"))
			_, _, err := readConfig(reader)
			if err == nil || !strings.Contains(err.Error(), test.envName) {
				t.Fatalf("expected error for %s, got %v", test.envName, err)
			}
		})
	}
}

func TestCalendarFeedTokenIsNotSerialized(t *testing.T) {
	cfg := &Config{Calendar: CalendarConfig{
		BaseURL:        DefaultCalendarBaseURL,
		TimeoutSeconds: DefaultCalendarTimeoutSeconds,
		CacheSeconds:   DefaultCalendarCacheSeconds,
		FeedToken:      "top-secret",
	}}
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "top-secret") || strings.Contains(string(encoded), "feedToken") {
		t.Fatalf("serialized config leaked feed token: %s", encoded)
	}
	encoded, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "top-secret") || strings.Contains(string(encoded), "FeedToken") {
		t.Fatalf("JSON config leaked feed token: %s", encoded)
	}
}

func newTestConfigReader(t *testing.T, configPath string) *viper.Viper {
	t.Helper()
	reader := viper.New()
	reader.SetConfigFile(configPath)
	reader.SetDefault("loginRequired", DefaultLoginRequired)
	reader.SetDefault("calendar.baseUrl", DefaultCalendarBaseURL)
	reader.SetDefault("calendar.timeoutSeconds", DefaultCalendarTimeoutSeconds)
	reader.SetDefault("calendar.cacheSeconds", DefaultCalendarCacheSeconds)
	if err := reader.BindEnv("loginRequired", BBSGO_LOGIN_REQUIRED); err != nil {
		t.Fatal(err)
	}
	for key, envName := range map[string]string{
		"calendar.baseUrl":        BBSGO_CALENDAR_BASE_URL,
		"calendar.timeoutSeconds": BBSGO_CALENDAR_TIMEOUT_SECONDS,
		"calendar.cacheSeconds":   BBSGO_CALENDAR_CACHE_SECONDS,
	} {
		if err := reader.BindEnv(key, envName); err != nil {
			t.Fatal(err)
		}
	}
	return reader
}
