package migrations

import (
	"errors"
	"reflect"
	"testing"

	"bbs-go/internal/pkg/config"
)

func TestMigrateAddDailyReviewSiteNavPlacementAndLanguage(t *testing.T) {
	tests := []struct {
		name       string
		language   config.Language
		value      string
		wantURLs   []string
		wantTitle  string
		entryIndex int
	}{
		{
			name:       "english after calendar",
			language:   config.LanguageEnUS,
			value:      `[{"title":"Topics","url":"/topics"},{"title":"Articles","url":"/articles"},{"title":"Calendar","url":"/calendar"},{"title":"Custom","url":"/custom"}]`,
			wantURLs:   []string{"/topics", "/articles", "/calendar", "/daily-review", "/custom"},
			wantTitle:  "Daily Review",
			entryIndex: 3,
		},
		{
			name:       "chinese after articles when calendar missing",
			language:   config.LanguageZhCN,
			value:      `[{"title":"话题","url":"/topics"},{"title":"文章","url":"/articles"},{"title":"自定义","url":"/custom"}]`,
			wantURLs:   []string{"/topics", "/articles", "/daily-review", "/custom"},
			wantTitle:  "每日复盘",
			entryIndex: 2,
		},
		{
			name:       "append when standard anchors missing",
			language:   config.LanguageZhCN,
			value:      `[{"title":"自定义","url":"/custom"}]`,
			wantURLs:   []string{"/custom", "/daily-review"},
			wantTitle:  "每日复盘",
			entryIndex: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSiteNavMigrationTestDB(t)
			setSiteNavMigrationLanguage(t, tt.language)
			createSiteNavConfig(t, db, tt.value)

			if err := migrate_add_daily_review_site_nav(); err != nil {
				t.Fatalf("migrate daily review site navigation: %v", err)
			}

			navs := decodeSiteNavs(t, readSiteNavConfig(t, db).Value)
			if got := siteNavURLs(t, navs); !reflect.DeepEqual(got, tt.wantURLs) {
				t.Fatalf("unexpected navigation URLs: got %v, want %v", got, tt.wantURLs)
			}
			if got := navs[tt.entryIndex]["title"]; got != tt.wantTitle {
				t.Fatalf("unexpected daily review title: got %#v, want %q", got, tt.wantTitle)
			}
		})
	}
}

func TestMigrateAddDailyReviewSiteNavPreservesCustomData(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageEnUS)
	value := `[
		{"title":"Topics","url":"/topics","openInNewWindow":true,"badge":{"color":"red"}},
		{"title":"Calendar","url":"/calendar","customOrder":7},
		{"title":"Tail","url":"/tail","extra":["one",2]}
	]`
	createSiteNavConfig(t, db, value)
	before := decodeSiteNavs(t, value)

	if err := migrate_add_daily_review_site_nav(); err != nil {
		t.Fatalf("migrate daily review site navigation: %v", err)
	}

	after := decodeSiteNavs(t, readSiteNavConfig(t, db).Value)
	want := make([]map[string]any, 0, len(before)+1)
	want = append(want, before[:2]...)
	want = append(want, map[string]any{"title": "Daily Review", "url": "/daily-review"})
	want = append(want, before[2:]...)
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("migration changed existing navigation data:\n got: %#v\nwant: %#v", after, want)
	}
}

func TestMigrateAddDailyReviewSiteNavIsIdempotent(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageZhCN)
	value := ` [{"title":"自定义复盘","url":"/daily-review","openInNewWindow":true},{"title":"话题","url":"/topics"}] `
	createSiteNavConfig(t, db, value)

	for run := 1; run <= 2; run++ {
		if err := migrate_add_daily_review_site_nav(); err != nil {
			t.Fatalf("migration run %d: %v", run, err)
		}
		configRow := readSiteNavConfig(t, db)
		if configRow.Value != value || configRow.UpdateTime != 456 {
			t.Fatalf("migration run %d changed existing navigation: %#v", run, configRow)
		}
	}
}

func TestMigrateAddDailyReviewSiteNavPreservesNestedEntry(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageZhCN)
	value := ` [{"title":"复盘分组","children":[{"title":"自定义复盘","url":"/daily-review","openInNewWindow":true,"custom":{"keep":true}}],"badge":{"color":"green"}},{"title":"话题","url":"/topics"}] `
	createSiteNavConfig(t, db, value)

	for run := 1; run <= 2; run++ {
		if err := migrate_add_daily_review_site_nav(); err != nil {
			t.Fatalf("migration run %d: %v", run, err)
		}
		configRow := readSiteNavConfig(t, db)
		if configRow.Value != value || configRow.UpdateTime != 456 {
			t.Fatalf("migration run %d changed nested navigation: %#v", run, configRow)
		}
	}
}

func TestMigrateAddDailyReviewSiteNavCreatesMissingConfig(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageZhCN)

	if err := migrate_add_daily_review_site_nav(); err != nil {
		t.Fatalf("migrate missing navigation config: %v", err)
	}

	configRow := readSiteNavConfig(t, db)
	want := []map[string]any{{"title": "每日复盘", "url": "/daily-review"}}
	if got := decodeSiteNavs(t, configRow.Value); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected default navigation: got %#v, want %#v", got, want)
	}
	if configRow.CreateTime == 0 || configRow.UpdateTime != configRow.CreateTime {
		t.Fatalf("unexpected timestamps: create=%d update=%d", configRow.CreateTime, configRow.UpdateTime)
	}
}

func TestMigrateAddDailyReviewSiteNavRejectsInvalidConfig(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	setSiteNavMigrationLanguage(t, config.LanguageEnUS)
	value := `{}`
	createSiteNavConfig(t, db, value)

	err := migrate_add_daily_review_site_nav()
	if !errors.Is(err, errInvalidSiteNavs) {
		t.Fatalf("migration error = %v, want %v", err, errInvalidSiteNavs)
	}
	configRow := readSiteNavConfig(t, db)
	if configRow.Value != value || configRow.UpdateTime != 456 {
		t.Fatalf("migration changed invalid navigation config: %#v", configRow)
	}
}
