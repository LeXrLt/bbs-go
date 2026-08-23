package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mlogclub/simple/common/strs"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

const (
	BBSGO_ENV                                = "BBSGO_ENV"
	BBSGO_LOGIN_REQUIRED                     = "BBSGO_LOGIN_REQUIRED"
	BBSGO_CALENDAR_BASE_URL                  = "BBSGO_CALENDAR_BASE_URL"
	BBSGO_CALENDAR_TIMEOUT_SECONDS           = "BBSGO_CALENDAR_TIMEOUT_SECONDS"
	BBSGO_CALENDAR_CACHE_SECONDS             = "BBSGO_CALENDAR_CACHE_SECONDS"
	BBSGO_CALENDAR_FEED_TOKEN                = "BBSGO_CALENDAR_FEED_TOKEN"
	BBSGO_DOCUMENT_CONVERTER_URL             = "BBSGO_DOCUMENT_CONVERTER_URL"
	BBSGO_DOCUMENT_CONVERTER_TIMEOUT_SECONDS = "BBSGO_DOCUMENT_CONVERTER_TIMEOUT_SECONDS"
	BBSGO_DOCUMENT_PREVIEW_MAX_OUTPUT_MB     = "BBSGO_DOCUMENT_PREVIEW_MAX_OUTPUT_MB"
	ENV_PREFIX                               = "BBSGO"

	EnvDev  = "dev"
	EnvTest = "test"
	EnvProd = "prod"

	DefaultLoginRequired                   = true
	DefaultCalendarBaseURL                 = "https://calendar.bvcportal.com"
	DefaultCalendarTimeoutSeconds          = 8
	DefaultCalendarCacheSeconds            = 30
	DefaultDocumentConverterTimeoutSeconds = 300
	DefaultDocumentPreviewMaxOutputMB      = 256
	// MaxCalendarTimeoutSeconds bounds the upstream request timeout.
	MaxCalendarTimeoutSeconds = 60
	// MaxCalendarCacheSeconds bounds the lifetime of an in-memory feed entry.
	MaxCalendarCacheSeconds            = 3600
	MaxDocumentConverterTimeoutSeconds = 300
	MaxDocumentPreviewOutputMB         = 256
)

type Language string

const (
	LanguageZhCN Language = "zh-CN"
	LanguageEnUS Language = "en-US"

	DefaultLanguage = LanguageEnUS
)

func (l Language) IsValid() bool {
	switch l {
	case LanguageZhCN, LanguageEnUS:
		return true
	}
	return false
}

var (
	Instance   *Config
	v          *viper.Viper
	configFile string
	writeMx    sync.Mutex
)

func init() {
	var (
		configFileName = "bbs-go.yaml"
	)
	v = viper.New()
	v.SetConfigFile(configFileName)
	v.AddConfigPath(".")
	if workDir, err := os.Executable(); err == nil {
		v.AddConfigPath(filepath.Dir(workDir))
	}
	v.AutomaticEnv()
	v.SetEnvPrefix(ENV_PREFIX)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetDefault("loginRequired", DefaultLoginRequired)
	v.SetDefault("calendar.baseUrl", DefaultCalendarBaseURL)
	v.SetDefault("calendar.timeoutSeconds", DefaultCalendarTimeoutSeconds)
	v.SetDefault("calendar.cacheSeconds", DefaultCalendarCacheSeconds)
	v.SetDefault("documentPreview.timeoutSeconds", DefaultDocumentConverterTimeoutSeconds)
	v.SetDefault("documentPreview.maxOutputMB", DefaultDocumentPreviewMaxOutputMB)
	if err := v.BindEnv("loginRequired", BBSGO_LOGIN_REQUIRED); err != nil {
		panic(fmt.Errorf("bind %s: %w", BBSGO_LOGIN_REQUIRED, err))
	}
	calendarEnvBindings := map[string]string{
		"calendar.baseUrl":        BBSGO_CALENDAR_BASE_URL,
		"calendar.timeoutSeconds": BBSGO_CALENDAR_TIMEOUT_SECONDS,
		"calendar.cacheSeconds":   BBSGO_CALENDAR_CACHE_SECONDS,
	}
	for key, envName := range calendarEnvBindings {
		if err := v.BindEnv(key, envName); err != nil {
			panic(fmt.Errorf("bind %s: %w", envName, err))
		}
	}
	documentPreviewEnvBindings := map[string]string{
		"documentPreview.converterUrl":   BBSGO_DOCUMENT_CONVERTER_URL,
		"documentPreview.timeoutSeconds": BBSGO_DOCUMENT_CONVERTER_TIMEOUT_SECONDS,
		"documentPreview.maxOutputMB":    BBSGO_DOCUMENT_PREVIEW_MAX_OUTPUT_MB,
	}
	for key, envName := range documentPreviewEnvBindings {
		if err := v.BindEnv(key, envName); err != nil {
			panic(fmt.Errorf("bind %s: %w", envName, err))
		}
	}

	configFile = getConfigFilePath(configFileName)
}

type Config struct {
	Language        Language              `yaml:"language"`        // 语言
	Port            int                   `yaml:"port"`            // 端口
	IPLocator       IPLocator             `yaml:"ipLocator"`       // IP定位配置
	AllowedOrigins  []string              `yaml:"allowedOrigins"`  // 跨域白名单
	Installed       bool                  `yaml:"installed"`       // 是否已安装
	LoginRequired   bool                  `yaml:"loginRequired"`   // 是否强制登录后访问站点内容
	IDCodec         IDCodecConfig         `yaml:"idCodec"`         // ID 编解码配置
	Logger          LoggerConfig          `yaml:"logger"`          // 日志配置
	DB              DBConfig              `yaml:"db"`              // 数据库配置
	Smtp            SmtpConfig            `yaml:"smtp"`            // smtp
	Search          SearchConfig          `yaml:"search"`          // 搜索配置
	Calendar        CalendarConfig        `yaml:"calendar"`        // 金融日历数据源
	DocumentPreview DocumentPreviewConfig `yaml:"documentPreview"` // Office 文档预览转换
}

type IPLocator struct {
	IPv4DataPath string `yaml:"ipv4DataPath"` // IPv4 数据文件路径
	IPv6DataPath string `yaml:"ipv6DataPath"` // IPv6 数据文件路径
}

type IDCodecConfig struct {
	Key uint64 `yaml:"key"` // ID 编解码秘钥
}

type LoggerConfig struct {
	Filename   string `yaml:"filename"`   // 日志文件的位置
	MaxSize    int    `yaml:"maxSize"`    // 文件最大尺寸（以MB为单位）
	MaxAge     int    `yaml:"maxAge"`     // 保留旧文件的最大天数
	MaxBackups int    `yaml:"maxBackups"` // 保留的最大旧文件数量
}

type DBConfig struct {
	Type                   string `yaml:"type"` // mysql, sqlite
	Url                    string `yaml:"url"`
	MaxIdleConns           int    `yaml:"maxIdleConns"`
	MaxOpenConns           int    `yaml:"maxOpenConns"`
	ConnMaxIdleTimeSeconds int    `yaml:"connMaxIdleTimeSeconds"`
	ConnMaxLifetimeSeconds int    `yaml:"connMaxLifetimeSeconds"`
	LogLevel               string `yaml:"logLevel"` // silent, error, warn, info
}

type SmtpConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	SSL      bool   `yaml:"ssl"`
}

type SearchConfig struct {
	IndexPath string `yaml:"indexPath"`
}

type CalendarConfig struct {
	BaseURL        string `yaml:"baseUrl"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"`
	CacheSeconds   int    `yaml:"cacheSeconds"`
	FeedToken      string `yaml:"-" json:"-" mapstructure:"-"`
}

type DocumentPreviewConfig struct {
	ConverterURL   string `yaml:"converterUrl"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"`
	MaxOutputMB    int    `yaml:"maxOutputMB"`
}

func ReadConfig() (cfg *Config, exists bool, err error) {
	return readConfig(v)
}

func readConfig(reader *viper.Viper) (cfg *Config, exists bool, err error) {
	exists = true
	if e := reader.ReadInConfig(); e != nil {
		exists = false
		slog.Warn("Config file not found, use default", slog.Any("error", e))
	}

	if exists {
		if e := reader.Unmarshal(&cfg); e != nil {
			err = fmt.Errorf("fatal error unmarshal config: %w", e)
			return
		}
		// 如果配置文件存在但没有语言设置，使用默认语言
		if strs.IsBlank(string(cfg.Language)) {
			cfg.Language = DefaultLanguage
		}
		SetDbDefaults(&cfg.DB)
	} else {
		// default config
		cfg = &Config{
			Language:  DefaultLanguage,
			Port:      8082,
			Installed: false,
			Logger: LoggerConfig{
				Filename:   getLogFilename(),
				MaxSize:    10,
				MaxAge:     10,
				MaxBackups: 10,
			},
			DB: defaultDbConfig(),
		}
	}
	loginRequired, parseErr := parseLoginRequired(reader.GetString("loginRequired"))
	if parseErr != nil {
		return nil, exists, parseErr
	}
	cfg.LoginRequired = loginRequired
	if err = readCalendarConfig(reader, &cfg.Calendar); err != nil {
		return nil, exists, err
	}
	if err = readDocumentPreviewConfig(reader, &cfg.DocumentPreview); err != nil {
		return nil, exists, err
	}

	return cfg, exists, nil
}

func readDocumentPreviewConfig(reader *viper.Viper, preview *DocumentPreviewConfig) error {
	converterURL := strings.TrimRight(strings.TrimSpace(reader.GetString("documentPreview.converterUrl")), "/")
	if converterURL != "" {
		parsedURL, err := url.Parse(converterURL)
		if err != nil || parsedURL == nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" || (parsedURL.Path != "" && parsedURL.Path != "/") {
			return fmt.Errorf("invalid %s: expected an absolute HTTP(S) root URL without credentials, path, query, or fragment", BBSGO_DOCUMENT_CONVERTER_URL)
		}
	}

	timeoutRaw := strings.TrimSpace(reader.GetString("documentPreview.timeoutSeconds"))
	if timeoutRaw == "" {
		timeoutRaw = strconv.Itoa(DefaultDocumentConverterTimeoutSeconds)
	}
	timeoutSeconds, err := parsePositiveConfigInt(timeoutRaw, BBSGO_DOCUMENT_CONVERTER_TIMEOUT_SECONDS, MaxDocumentConverterTimeoutSeconds)
	if err != nil {
		return err
	}
	maxOutputRaw := strings.TrimSpace(reader.GetString("documentPreview.maxOutputMB"))
	if maxOutputRaw == "" {
		maxOutputRaw = strconv.Itoa(DefaultDocumentPreviewMaxOutputMB)
	}
	maxOutputMB, err := parsePositiveConfigInt(maxOutputRaw, BBSGO_DOCUMENT_PREVIEW_MAX_OUTPUT_MB, MaxDocumentPreviewOutputMB)
	if err != nil {
		return err
	}

	preview.ConverterURL = converterURL
	preview.TimeoutSeconds = timeoutSeconds
	preview.MaxOutputMB = maxOutputMB
	return nil
}

func readCalendarConfig(reader *viper.Viper, calendar *CalendarConfig) error {
	baseURL := strings.TrimSpace(reader.GetString("calendar.baseUrl"))
	if baseURL == "" {
		baseURL = DefaultCalendarBaseURL
	}
	feedToken := strings.TrimSpace(os.Getenv(BBSGO_CALENDAR_FEED_TOKEN))
	parsedURL, err := url.Parse(baseURL)
	validScheme := parsedURL != nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https")
	if feedToken != "" {
		validScheme = parsedURL != nil && parsedURL.Scheme == "https"
	}
	if err != nil || parsedURL == nil || !validScheme || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		expectedScheme := "HTTP(S)"
		if feedToken != "" {
			expectedScheme = "HTTPS"
		}
		return fmt.Errorf("invalid %s: expected an absolute %s URL without credentials, query, or fragment", BBSGO_CALENDAR_BASE_URL, expectedScheme)
	}

	timeoutRaw := strings.TrimSpace(reader.GetString("calendar.timeoutSeconds"))
	if timeoutRaw == "" {
		timeoutRaw = strconv.Itoa(DefaultCalendarTimeoutSeconds)
	}
	timeoutSeconds, err := parsePositiveConfigInt(timeoutRaw, BBSGO_CALENDAR_TIMEOUT_SECONDS, MaxCalendarTimeoutSeconds)
	if err != nil {
		return err
	}
	cacheRaw := strings.TrimSpace(reader.GetString("calendar.cacheSeconds"))
	if cacheRaw == "" {
		cacheRaw = strconv.Itoa(DefaultCalendarCacheSeconds)
	}
	cacheSeconds, err := parsePositiveConfigInt(cacheRaw, BBSGO_CALENDAR_CACHE_SECONDS, MaxCalendarCacheSeconds)
	if err != nil {
		return err
	}

	calendar.BaseURL = strings.TrimRight(baseURL, "/")
	calendar.TimeoutSeconds = timeoutSeconds
	calendar.CacheSeconds = cacheSeconds
	calendar.FeedToken = feedToken
	return nil
}

func parsePositiveConfigInt(raw string, envName string, max int) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 || value > max {
		return 0, fmt.Errorf("invalid %s value %q: expected an integer between 1 and %d", envName, raw, max)
	}
	return value, nil
}

func parseLoginRequired(raw string) (bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return DefaultLoginRequired, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s value %q: expected a boolean", BBSGO_LOGIN_REQUIRED, raw)
	}
	return parsed, nil
}

func IsLoginRequired() bool {
	return Instance != nil && Instance.Installed && Instance.LoginRequired
}

func WriteConfig(cfg *Config) error {
	if !writeMx.TryLock() {
		return errors.New("config is being written, please try again later")
	}
	defer writeMx.Unlock()

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	slog.Info("Write config", slog.String("configFile", configFile))

	err = os.WriteFile(configFile, yamlData, 0644)
	if err != nil {
		return err
	}
	return nil
}

func IsProd() bool {
	e := strings.ToLower(GetEnv())
	return e == "prod" || e == "production"
}

func GetEnv() string {
	env := os.Getenv("BBSGO_ENV")
	if strs.IsBlank(env) {
		env = EnvDev
	}
	return env
}

func getConfigFilePath(configName string) string {
	// Always prefer writing next to the working directory, even when the file does not yet exist.
	cwdPath := filepath.Join(".", configName)
	if _, err := os.Stat(cwdPath); err == nil {
		return cwdPath
	}
	// If CWD is accessible but file is missing, still choose CWD so installs do not drift to temp dirs.
	if _, err := os.Stat("."); err == nil {
		return cwdPath
	}

	// Fallbacks: first try beside the executable if reachable, otherwise return the bare name.
	if workDir, err := os.Executable(); err == nil {
		exePath := filepath.Join(filepath.Dir(workDir), configName)
		if _, err := os.Stat(exePath); err == nil {
			return exePath
		}
		return exePath
	}
	return configName
}

func GetConfigDir() string {
	return filepath.Dir(configFile)
}

func getLogFilename() string {
	// workDir, err := os.Getwd()
	// if err != nil {
	// 	slog.Error("Failed to get working directory", slog.Any("error", err))
	// 	return ""
	// }
	return filepath.Join("./", "logs", "bbs-go.log")
}

const (
	DbTypeMySQL       = "mysql"
	DbTypePostgreSQL  = "postgresql"
	DbTypeSQLite      = "sqlite"
	DefaultDBLogLevel = "warn"
)

func SetDbDefaults(c *DBConfig) {
	if c.Type == "" {
		c.Type = DbTypeMySQL
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 50
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 200
	}
	if c.ConnMaxIdleTimeSeconds == 0 {
		c.ConnMaxIdleTimeSeconds = 300
	}
	if c.ConnMaxLifetimeSeconds == 0 {
		c.ConnMaxLifetimeSeconds = 3600
	}
	if strs.IsBlank(c.LogLevel) {
		c.LogLevel = DefaultDBLogLevel
	}
}

func defaultDbConfig() DBConfig {
	return DBConfig{
		Type:                   DbTypeMySQL,
		MaxIdleConns:           50,
		MaxOpenConns:           200,
		ConnMaxIdleTimeSeconds: 300,
		ConnMaxLifetimeSeconds: 3600,
		LogLevel:               DefaultDBLogLevel,
	}
}
