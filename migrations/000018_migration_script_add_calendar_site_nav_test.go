package migrations

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestMigrateAddCalendarSiteNavPlacementAndLanguage(t *testing.T) {
	tests := []struct {
		name          string
		language      config.Language
		value         string
		wantURLs      []string
		wantTitle     string
		calendarIndex int
	}{
		{
			name:          "english after articles",
			language:      config.LanguageEnUS,
			value:         `[{"title":"Topics","url":"/topics"},{"title":"Articles","url":"/articles"},{"title":"Custom","url":"/custom"}]`,
			wantURLs:      []string{"/topics", "/articles", "/calendar", "/custom"},
			wantTitle:     "Calendar",
			calendarIndex: 2,
		},
		{
			name:          "chinese after topics when articles missing",
			language:      config.LanguageZhCN,
			value:         `[{"title":"话题","url":"/topics"},{"title":"自定义","url":"/custom"}]`,
			wantURLs:      []string{"/topics", "/calendar", "/custom"},
			wantTitle:     "日历",
			calendarIndex: 1,
		},
		{
			name:          "append when standard anchors missing",
			language:      config.LanguageZhCN,
			value:         `[{"title":"自定义","url":"/custom"}]`,
			wantURLs:      []string{"/custom", "/calendar"},
			wantTitle:     "日历",
			calendarIndex: 1,
		},
		{
			name:          "valid empty array",
			language:      config.LanguageEnUS,
			value:         `[]`,
			wantURLs:      []string{"/calendar"},
			wantTitle:     "Calendar",
			calendarIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSiteNavMigrationTestDB(t)
			setSiteNavMigrationLanguage(t, tt.language)
			createSiteNavConfig(t, db, tt.value)

			if err := migrate_add_calendar_site_nav(); err != nil {
				t.Fatalf("migrate calendar site navigation: %v", err)
			}

			navs := decodeSiteNavs(t, readSiteNavConfig(t, db).Value)
			if got := siteNavURLs(t, navs); !reflect.DeepEqual(got, tt.wantURLs) {
				t.Fatalf("unexpected navigation URLs: got %v, want %v", got, tt.wantURLs)
			}
			if got := navs[tt.calendarIndex]["title"]; got != tt.wantTitle {
				t.Fatalf("unexpected calendar title: got %#v, want %q", got, tt.wantTitle)
			}
		})
	}
}

func TestMigrateAddCalendarSiteNavPreservesCustomDataAndAddsTopLevelEntry(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageEnUS)
	value := `[
		{"title":"Topics","url":"/topics","openInNewWindow":true,"badge":{"color":"red"}},
		{"title":"Group","children":[{"title":"Nested calendar","url":"/calendar","openInNewWindow":true}],"customOrder":7},
		{"title":"Articles","url":"/articles","extra":["one",2]},
		{"title":"Tail","url":"/tail"}
	]`
	createSiteNavConfig(t, db, value)
	before := decodeSiteNavs(t, value)

	if err := migrate_add_calendar_site_nav(); err != nil {
		t.Fatalf("migrate calendar site navigation: %v", err)
	}

	after := decodeSiteNavs(t, readSiteNavConfig(t, db).Value)
	want := make([]map[string]any, 0, len(before)+1)
	want = append(want, before[:3]...)
	want = append(want, map[string]any{"title": "Calendar", "url": "/calendar"})
	want = append(want, before[3:]...)
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("migration changed existing navigation data:\n got: %#v\nwant: %#v", after, want)
	}
}

func TestMigrateAddCalendarSiteNavIsIdempotentWhenTopLevelEntryExists(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageZhCN)
	value := ` [ {"title":"自定义日历","url":"/calendar","openInNewWindow":true,"custom":{"keep":true}}, {"title":"话题","url":"/topics"} ] `
	createSiteNavConfig(t, db, value)

	for run := 1; run <= 2; run++ {
		if err := migrate_add_calendar_site_nav(); err != nil {
			t.Fatalf("migration run %d: %v", run, err)
		}
		configRow := readSiteNavConfig(t, db)
		if configRow.Value != value {
			t.Fatalf("migration run %d changed existing calendar navigation: got %q, want %q", run, configRow.Value, value)
		}
		if configRow.UpdateTime != 456 {
			t.Fatalf("migration run %d changed update time: got %d, want 456", run, configRow.UpdateTime)
		}
	}
}

func TestMigrateAddCalendarSiteNavCreatesMissingLocalizedConfig(t *testing.T) {
	tests := []struct {
		name            string
		language        config.Language
		wantTitle       string
		wantName        string
		wantDescription string
	}{
		{
			name:            "english",
			language:        config.LanguageEnUS,
			wantTitle:       "Calendar",
			wantName:        "Site Navigation",
			wantDescription: "Site Navigation",
		},
		{
			name:            "chinese",
			language:        config.LanguageZhCN,
			wantTitle:       "日历",
			wantName:        "站点导航",
			wantDescription: "站点导航",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSiteNavMigrationTestDB(t)
			setSiteNavMigrationLanguage(t, tt.language)

			if err := migrate_add_calendar_site_nav(); err != nil {
				t.Fatalf("migrate missing navigation config: %v", err)
			}

			configRow := readSiteNavConfig(t, db)
			wantNavs := []map[string]any{{"title": tt.wantTitle, "url": "/calendar"}}
			if got := decodeSiteNavs(t, configRow.Value); !reflect.DeepEqual(got, wantNavs) {
				t.Fatalf("unexpected default navigation: got %#v, want %#v", got, wantNavs)
			}
			if configRow.Name != tt.wantName || configRow.Description != tt.wantDescription {
				t.Fatalf("unexpected metadata: got (%q, %q), want (%q, %q)",
					configRow.Name, configRow.Description, tt.wantName, tt.wantDescription)
			}
			if configRow.CreateTime == 0 || configRow.UpdateTime != configRow.CreateTime {
				t.Fatalf("unexpected timestamps: create=%d update=%d", configRow.CreateTime, configRow.UpdateTime)
			}
		})
	}
}

func TestMigrateAddCalendarSiteNavPopulatesBlankConfig(t *testing.T) {
	tests := []struct {
		name      string
		language  config.Language
		value     string
		wantTitle string
	}{
		{name: "empty english", language: config.LanguageEnUS, value: "", wantTitle: "Calendar"},
		{name: "whitespace chinese", language: config.LanguageZhCN, value: " \n\t ", wantTitle: "日历"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSiteNavMigrationTestDB(t)
			setSiteNavMigrationLanguage(t, tt.language)
			createSiteNavConfig(t, db, tt.value)

			if err := migrate_add_calendar_site_nav(); err != nil {
				t.Fatalf("migrate blank navigation config: %v", err)
			}

			configRow := readSiteNavConfig(t, db)
			wantNavs := []map[string]any{{"title": tt.wantTitle, "url": "/calendar"}}
			if got := decodeSiteNavs(t, configRow.Value); !reflect.DeepEqual(got, wantNavs) {
				t.Fatalf("unexpected navigation: got %#v, want %#v", got, wantNavs)
			}
			if configRow.Name != "custom navigation" || configRow.Description != "keep this metadata" || configRow.CreateTime != 123 {
				t.Fatalf("migration replaced existing metadata: %#v", configRow)
			}
			if configRow.UpdateTime == 456 {
				t.Fatal("migration did not update the blank config timestamp")
			}
		})
	}
}

func TestMigrateAddCalendarSiteNavRejectsInvalidConfig(t *testing.T) {
	for _, value := range []string{"{not-json", `{}`, `null`, `[null]`, `[{"title":"broken","url":null}]`, `[{"title":"broken","url":7}]`} {
		t.Run(fmt.Sprintf("value_%q", value), func(t *testing.T) {
			db := newSiteNavMigrationTestDB(t)
			setSiteNavMigrationLanguage(t, config.LanguageEnUS)
			createSiteNavConfig(t, db, value)

			err := migrate_add_calendar_site_nav()
			if !errors.Is(err, errInvalidSiteNavs) {
				t.Fatalf("migrate invalid navigation config error = %v, want %v", err, errInvalidSiteNavs)
			}
			configRow := readSiteNavConfig(t, db)
			if configRow.Value != value {
				t.Fatalf("migration changed invalid navigation config: got %q, want %q", configRow.Value, value)
			}
			if configRow.UpdateTime != 456 {
				t.Fatalf("migration changed update time: got %d, want 456", configRow.UpdateTime)
			}
		})
	}
}

func TestMigrateAddCalendarSiteNavReturnsQueryError(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageEnUS)
	if err := db.Migrator().DropTable(&models.SysConfig{}); err != nil {
		t.Fatalf("drop sys config table: %v", err)
	}

	err := migrate_add_calendar_site_nav()
	if err == nil || !strings.Contains(err.Error(), "query site navigation config") {
		t.Fatalf("migration query error = %v, want contextual database error", err)
	}
}

func TestUpdateSiteNavsValueDetectsOptimisticConflict(t *testing.T) {
	tests := []struct {
		name       string
		concurrent map[string]any
	}{
		{
			name: "value changed",
			concurrent: map[string]any{
				"value":       `[{"title":"Concurrent","url":"/concurrent"}]`,
				"update_time": int64(456),
			},
		},
		{
			name: "timestamp changed",
			concurrent: map[string]any{
				"update_time": int64(457),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSiteNavMigrationTestDB(t)
			original := `[{"title":"Topics","url":"/topics"}]`
			createSiteNavConfig(t, db, original)
			stale := readSiteNavConfig(t, db)
			if err := db.Model(&models.SysConfig{}).
				Where("id = ?", stale.Id).
				Updates(tt.concurrent).Error; err != nil {
				t.Fatalf("apply concurrent update: %v", err)
			}

			err := updateSiteNavsValue(db, &stale, `[{"title":"Calendar","url":"/calendar"}]`)
			if !errors.Is(err, errSiteNavsUpdateConflict) {
				t.Fatalf("optimistic update error = %v, want %v", err, errSiteNavsUpdateConflict)
			}
		})
	}
}

func newSiteNavMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:calendar_site_nav_migration_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "t_",
			SingularTable: true,
		},
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SysConfig{}); err != nil {
		t.Fatalf("auto migrate sys config: %v", err)
	}
	sqls.SetDB(db)
	return db
}

func setSiteNavMigrationLanguage(t *testing.T, language config.Language) {
	t.Helper()
	previous := config.Instance
	config.Instance = &config.Config{Language: language}
	t.Cleanup(func() { config.Instance = previous })
}

func createSiteNavConfig(t *testing.T, db *gorm.DB, value string) {
	t.Helper()
	configRow := &models.SysConfig{
		Key:         constants.SysConfigSiteNavs,
		Value:       value,
		Name:        "custom navigation",
		Description: "keep this metadata",
		CreateTime:  123,
		UpdateTime:  456,
	}
	if err := db.Create(configRow).Error; err != nil {
		t.Fatalf("create site navigation config: %v", err)
	}
}

func readSiteNavConfig(t *testing.T, db *gorm.DB) models.SysConfig {
	t.Helper()
	var configRow models.SysConfig
	if err := db.Where("key = ?", constants.SysConfigSiteNavs).Take(&configRow).Error; err != nil {
		t.Fatalf("read site navigation config: %v", err)
	}
	return configRow
}

func decodeSiteNavs(t *testing.T, value string) []map[string]any {
	t.Helper()
	var navs []map[string]any
	if err := json.Unmarshal([]byte(value), &navs); err != nil {
		t.Fatalf("decode site navigation: %v", err)
	}
	return navs
}

func siteNavURLs(t *testing.T, navs []map[string]any) []string {
	t.Helper()
	urls := make([]string, 0, len(navs))
	for index, nav := range navs {
		url, ok := nav["url"].(string)
		if !ok {
			t.Fatalf("navigation item %d has non-string URL: %#v", index, nav["url"])
		}
		urls = append(urls, url)
	}
	return urls
}
