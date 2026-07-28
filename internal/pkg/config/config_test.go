package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
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

func newTestConfigReader(t *testing.T, configPath string) *viper.Viper {
	t.Helper()
	reader := viper.New()
	reader.SetConfigFile(configPath)
	reader.SetDefault("loginRequired", DefaultLoginRequired)
	if err := reader.BindEnv("loginRequired", BBSGO_LOGIN_REQUIRED); err != nil {
		t.Fatal(err)
	}
	return reader
}
