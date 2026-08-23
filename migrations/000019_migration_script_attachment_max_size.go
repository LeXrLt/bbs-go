package migrations

import (
	"encoding/json"
	"errors"
	"fmt"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/dto"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
)

const legacyAttachmentMaxSizeMB = 10

func migrate_attachment_max_size() error {
	return sqls.WithTransaction(func(ctx *sqls.TxContext) error {
		var row models.SysConfig
		if err := ctx.Tx.Where("key = ?", constants.SysConfigAttachmentConfig).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return createAttachmentConfigWithCurrentLimit(ctx.Tx)
			}
			return fmt.Errorf("query attachment config: %w", err)
		}

		value, changed, err := setAttachmentMaxSize(row.Value)
		if err != nil {
			return fmt.Errorf("update attachment config: %w", err)
		}
		if !changed {
			return nil
		}

		result := ctx.Tx.Model(&models.SysConfig{}).
			Where("id = ? AND key = ? AND value = ? AND update_time = ?", row.Id, row.Key, row.Value, row.UpdateTime).
			Updates(map[string]any{
				"value":       value,
				"update_time": dates.NowTimestamp(),
			})
		if result.Error != nil {
			return fmt.Errorf("persist attachment config: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return errors.New("attachment config was modified concurrently")
		}
		return nil
	})
}

func createAttachmentConfigWithCurrentLimit(db *gorm.DB) error {
	cfg := dto.AttachmentConfig{
		Enabled:      true,
		AllowedTypes: []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".md", ".csv", ".zip", ".rar", ".7z", ".tar", ".gz"},
		MaxSizeMB:    constants.AttachmentMaxSizeMB,
		MaxCount:     5,
	}
	value, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode attachment config: %w", err)
	}
	name, description := attachmentConfigMetaByLanguage()
	now := dates.NowTimestamp()
	if err := db.Create(&models.SysConfig{
		Key:         constants.SysConfigAttachmentConfig,
		Value:       string(value),
		Name:        name,
		Description: description,
		CreateTime:  now,
		UpdateTime:  now,
	}).Error; err != nil {
		return fmt.Errorf("create attachment config: %w", err)
	}
	return nil
}

func setAttachmentMaxSize(value string) (string, bool, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &config); err != nil || config == nil {
		return value, false, errors.New("invalid attachment config JSON")
	}

	if raw, exists := config["maxSizeMB"]; exists {
		var current int
		if err := json.Unmarshal(raw, &current); err == nil {
			if current == constants.AttachmentMaxSizeMB || (current > 0 && current != legacyAttachmentMaxSizeMB && current < constants.AttachmentMaxSizeMB) {
				return value, false, nil
			}
		}
	}
	config["maxSizeMB"] = json.RawMessage(fmt.Sprintf("%d", constants.AttachmentMaxSizeMB))

	updated, err := json.Marshal(config)
	if err != nil {
		return value, false, fmt.Errorf("encode attachment config JSON: %w", err)
	}
	return string(updated), true, nil
}
