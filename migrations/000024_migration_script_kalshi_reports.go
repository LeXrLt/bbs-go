package migrations

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/config"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
)

func migrate_add_kalshi_reports() error {
	if err := sqls.DB().AutoMigrate(&models.KalshiReport{}, &models.KalshiReportRevision{}); err != nil {
		return err
	}
	return sqls.WithTransaction(func(ctx *sqls.TxContext) error {
		var nav models.SysConfig
		err := ctx.Tx.Where("key = ?", constants.SysConfigSiteNavs).Take(&nav).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			value, _, err := addKalshiSiteNav("")
			if err != nil {
				return err
			}
			name, description := calendarSiteNavMeta()
			now := dates.NowTimestamp()
			return ctx.Tx.Create(&models.SysConfig{Key: constants.SysConfigSiteNavs, Value: value, Name: name, Description: description, CreateTime: now, UpdateTime: now}).Error
		}
		if err != nil {
			return err
		}
		value, changed, err := addKalshiSiteNav(nav.Value)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return updateSiteNavsValue(ctx.Tx, &nav, value)
	})
}

func addKalshiSiteNav(value string) (string, bool, error) {
	navs := make([]rawSiteNav, 0)
	if strings.TrimSpace(value) != "" {
		if err := json.Unmarshal([]byte(value), &navs); err != nil || navs == nil {
			return value, false, errInvalidSiteNavs
		}
	}
	for _, nav := range navs {
		if nav == nil {
			return value, false, errInvalidSiteNavs
		}
		found, err := rawSiteNavContainsURL(nav, "/kalshi")
		if err != nil {
			return value, false, errInvalidSiteNavs
		}
		if found {
			return value, false, nil
		}
	}
	title := "Kalshi 合约"
	if siteNavLanguage() != config.LanguageZhCN {
		title = "Kalshi Markets"
	}
	titleJSON, err := json.Marshal(title)
	if err != nil {
		return value, false, err
	}
	entry := rawSiteNav{"title": titleJSON, "url": json.RawMessage(`"/kalshi"`)}
	index := len(navs)
	for i, nav := range navs {
		url, err := rawSiteNavURL(nav)
		if err != nil {
			return value, false, errInvalidSiteNavs
		}
		if url == "/daily-review" {
			index = i + 1
			break
		}
	}
	navs = append(navs, nil)
	copy(navs[index+1:], navs[index:])
	navs[index] = entry
	result, err := json.Marshal(navs)
	return string(result), true, err
}
