package migrations

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
)

var errInvalidSiteNavs = errors.New("invalid site navigation data")
var errSiteNavsUpdateConflict = errors.New("site navigation config was modified concurrently")

type rawSiteNav map[string]json.RawMessage

func migrate_add_calendar_site_nav() error {
	return sqls.WithTransaction(func(ctx *sqls.TxContext) error {
		var siteNavs models.SysConfig
		if err := ctx.Tx.Where("key = ?", constants.SysConfigSiteNavs).Take(&siteNavs).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				value, _, err := addCalendarSiteNav("", calendarSiteNavTitle())
				if err != nil {
					return fmt.Errorf("build default site navigation: %w", err)
				}
				name, description := calendarSiteNavMeta()
				now := dates.NowTimestamp()
				if err := ctx.Tx.Create(&models.SysConfig{
					Key:         constants.SysConfigSiteNavs,
					Value:       value,
					Name:        name,
					Description: description,
					CreateTime:  now,
					UpdateTime:  now,
				}).Error; err != nil {
					return fmt.Errorf("create site navigation config: %w", err)
				}
				return nil
			}
			return fmt.Errorf("query site navigation config: %w", err)
		}

		value, changed, err := addCalendarSiteNav(siteNavs.Value, calendarSiteNavTitle())
		if err != nil {
			return fmt.Errorf("add calendar to site navigation: %w", err)
		}
		if !changed {
			return nil
		}

		return updateSiteNavsValue(ctx.Tx, &siteNavs, value)
	})
}

func updateSiteNavsValue(db *gorm.DB, siteNavs *models.SysConfig, value string) error {
	result := db.Model(&models.SysConfig{}).
		Where("id = ? AND key = ? AND value = ? AND update_time = ?",
			siteNavs.Id, siteNavs.Key, siteNavs.Value, siteNavs.UpdateTime).
		Updates(map[string]any{
			"value":       value,
			"update_time": dates.NowTimestamp(),
		})
	if result.Error != nil {
		return fmt.Errorf("update site navigation config: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: key %q", errSiteNavsUpdateConflict, siteNavs.Key)
	}
	return nil
}

func addCalendarSiteNav(value, title string) (string, bool, error) {
	navs := make([]rawSiteNav, 0)
	if strings.TrimSpace(value) != "" {
		if err := json.Unmarshal([]byte(value), &navs); err != nil || navs == nil {
			return value, false, errInvalidSiteNavs
		}
	}

	articlesIndex := -1
	topicsIndex := -1
	for index, nav := range navs {
		if nav == nil {
			return value, false, errInvalidSiteNavs
		}
		url, err := rawSiteNavURL(nav)
		if err != nil {
			return value, false, errInvalidSiteNavs
		}
		switch url {
		case "/calendar":
			return value, false, nil
		case "/articles":
			if articlesIndex == -1 {
				articlesIndex = index
			}
		case "/topics":
			if topicsIndex == -1 {
				topicsIndex = index
			}
		}
	}

	insertAt := len(navs)
	if articlesIndex >= 0 {
		insertAt = articlesIndex + 1
	} else if topicsIndex >= 0 {
		insertAt = topicsIndex + 1
	}

	titleJSON, err := json.Marshal(title)
	if err != nil {
		return value, false, err
	}
	calendarNav := rawSiteNav{
		"title": titleJSON,
		"url":   json.RawMessage(`"/calendar"`),
	}
	navs = append(navs, nil)
	copy(navs[insertAt+1:], navs[insertAt:])
	navs[insertAt] = calendarNav

	updated, err := json.Marshal(navs)
	if err != nil {
		return value, false, err
	}
	return string(updated), true, nil
}

func rawSiteNavURL(nav rawSiteNav) (string, error) {
	rawURL, ok := nav["url"]
	if !ok {
		return "", nil
	}
	var url *string
	if err := json.Unmarshal(rawURL, &url); err != nil {
		return "", err
	}
	if url == nil {
		return "", errInvalidSiteNavs
	}
	return strings.TrimSpace(*url), nil
}

func calendarSiteNavTitle() string {
	if siteNavLanguage() == config.LanguageZhCN {
		return "日历"
	}
	return "Calendar"
}

func calendarSiteNavMeta() (name, description string) {
	if siteNavLanguage() == config.LanguageZhCN {
		return "站点导航", "站点导航"
	}
	return "Site Navigation", "Site Navigation"
}

func siteNavLanguage() config.Language {
	language := config.DefaultLanguage
	if config.Instance != nil && config.Instance.Language.IsValid() {
		language = config.Instance.Language
	}
	return language
}
