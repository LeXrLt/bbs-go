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

func migrate_add_daily_review_site_nav() error {
	return sqls.WithTransaction(func(ctx *sqls.TxContext) error {
		var siteNavs models.SysConfig
		if err := ctx.Tx.Where("key = ?", constants.SysConfigSiteNavs).Take(&siteNavs).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				value, _, err := addDailyReviewSiteNav("", dailyReviewSiteNavTitle())
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

		value, changed, err := addDailyReviewSiteNav(siteNavs.Value, dailyReviewSiteNavTitle())
		if err != nil {
			return fmt.Errorf("add daily review to site navigation: %w", err)
		}
		if !changed {
			return nil
		}
		return updateSiteNavsValue(ctx.Tx, &siteNavs, value)
	})
}

func addDailyReviewSiteNav(value, title string) (string, bool, error) {
	navs := make([]rawSiteNav, 0)
	if strings.TrimSpace(value) != "" {
		if err := json.Unmarshal([]byte(value), &navs); err != nil || navs == nil {
			return value, false, errInvalidSiteNavs
		}
	}

	calendarIndex := -1
	articlesIndex := -1
	topicsIndex := -1
	for index, nav := range navs {
		if nav == nil {
			return value, false, errInvalidSiteNavs
		}
		containsDailyReview, err := rawSiteNavContainsURL(nav, "/daily-review")
		if err != nil {
			return value, false, errInvalidSiteNavs
		}
		if containsDailyReview {
			return value, false, nil
		}
		url, err := rawSiteNavURL(nav)
		if err != nil {
			return value, false, errInvalidSiteNavs
		}
		switch url {
		case "/calendar":
			if calendarIndex == -1 {
				calendarIndex = index
			}
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
	if calendarIndex >= 0 {
		insertAt = calendarIndex + 1
	} else if articlesIndex >= 0 {
		insertAt = articlesIndex + 1
	} else if topicsIndex >= 0 {
		insertAt = topicsIndex + 1
	}

	titleJSON, err := json.Marshal(title)
	if err != nil {
		return value, false, err
	}
	nav := rawSiteNav{
		"title": titleJSON,
		"url":   json.RawMessage(`"/daily-review"`),
	}
	navs = append(navs, nil)
	copy(navs[insertAt+1:], navs[insertAt:])
	navs[insertAt] = nav

	updated, err := json.Marshal(navs)
	if err != nil {
		return value, false, err
	}
	return string(updated), true, nil
}

func rawSiteNavContainsURL(nav rawSiteNav, target string) (bool, error) {
	url, err := rawSiteNavURL(nav)
	if err != nil {
		return false, err
	}
	if url == target {
		return true, nil
	}

	rawChildren, ok := nav["children"]
	if !ok || strings.TrimSpace(string(rawChildren)) == "null" {
		return false, nil
	}
	var children []rawSiteNav
	if err := json.Unmarshal(rawChildren, &children); err != nil || children == nil {
		return false, errInvalidSiteNavs
	}
	for _, child := range children {
		if child == nil {
			return false, errInvalidSiteNavs
		}
		found, err := rawSiteNavContainsURL(child, target)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func dailyReviewSiteNavTitle() string {
	if siteNavLanguage() == config.LanguageZhCN {
		return "每日复盘"
	}
	return "Daily Review"
}
