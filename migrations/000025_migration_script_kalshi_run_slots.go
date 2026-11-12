package migrations

import (
	"bbs-go/internal/models"
	"encoding/json"
	"regexp"

	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
)

var legacyKalshiManifest = regexp.MustCompile("(?ms)^```json manifest\\.json\\r?\\n(.*?)^```\\r?$")

// Old documents without an explicit run_slot remain unclassified, never guessed.
func migrate_kalshi_run_slots() error {
	return sqls.DB().Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasColumn(&models.KalshiReport{}, "slot") {
			if err := tx.Migrator().AddColumn(&models.KalshiReport{}, "slot"); err != nil {
				return err
			}
		}
		var reports []models.KalshiReport
		if err := tx.Where("slot = '' OR slot IS NULL").Find(&reports).Error; err != nil {
			return err
		}
		for _, report := range reports {
			matches := legacyKalshiManifest.FindAllStringSubmatch(report.Markdown, -1)
			if len(matches) != 1 {
				continue
			}
			var manifest struct {
				Metadata struct {
					RunSlot string `json:"run_slot"`
				} `json:"metadata"`
			}
			if json.Unmarshal([]byte(matches[0][1]), &manifest) != nil || (manifest.Metadata.RunSlot != "morning" && manifest.Metadata.RunSlot != "evening") {
				continue
			}
			if err := tx.Model(&models.KalshiReport{}).Where("id = ?", report.Id).Update("slot", manifest.Metadata.RunSlot).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasIndex(&models.KalshiReport{}, "uk_kalshi_report_date") {
			if err := tx.Migrator().DropIndex(&models.KalshiReport{}, "uk_kalshi_report_date"); err != nil {
				return err
			}
		}
		return tx.AutoMigrate(&models.KalshiReport{})
	})
}
