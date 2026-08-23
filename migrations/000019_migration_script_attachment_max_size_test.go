package migrations

import (
	"encoding/json"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
)

func TestSetAttachmentMaxSize(t *testing.T) {
	original := `{"enabled":true,"maxSizeMB":10,"maxCount":3,"future":{"keep":true}}`
	updated, changed, err := setAttachmentMaxSize(original)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("10 MB config must be updated")
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(updated), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["maxSizeMB"] != float64(constants.AttachmentMaxSizeMB) {
		t.Fatalf("maxSizeMB=%v", decoded["maxSizeMB"])
	}
	if decoded["maxCount"] != float64(3) || decoded["future"].(map[string]any)["keep"] != true {
		t.Fatalf("unrelated attachment settings were not preserved: %#v", decoded)
	}

	unchanged, changed, err := setAttachmentMaxSize(updated)
	if err != nil || changed || unchanged != updated {
		t.Fatalf("256 MB config should be idempotent: changed=%t err=%v", changed, err)
	}
	if _, _, err := setAttachmentMaxSize(`not-json`); err == nil {
		t.Fatal("malformed config must fail closed")
	}
	custom := `{"enabled":true,"maxSizeMB":64,"maxCount":5}`
	preserved, changed, err := setAttachmentMaxSize(custom)
	if err != nil || changed || preserved != custom {
		t.Fatalf("custom lower limit should be preserved: changed=%t err=%v", changed, err)
	}
}

func TestMigrateAttachmentMaxSizeUpdatesExistingConfig(t *testing.T) {
	db := newSiteNavMigrationTestDB(t)
	row := &models.SysConfig{
		Key:         constants.SysConfigAttachmentConfig,
		Value:       `{"enabled":true,"maxSizeMB":10,"maxCount":5}`,
		Name:        "custom attachment config",
		Description: "preserve metadata",
		CreateTime:  100,
		UpdateTime:  200,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrate_attachment_max_size(); err != nil {
		t.Fatal(err)
	}

	var got models.SysConfig
	if err := db.Where("key = ?", constants.SysConfigAttachmentConfig).Take(&got).Error; err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		MaxSizeMB int `json:"maxSizeMB"`
	}
	if err := json.Unmarshal([]byte(got.Value), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.MaxSizeMB != constants.AttachmentMaxSizeMB {
		t.Fatalf("maxSizeMB=%d want %d", cfg.MaxSizeMB, constants.AttachmentMaxSizeMB)
	}
	if got.Name != row.Name || got.Description != row.Description || got.CreateTime != row.CreateTime {
		t.Fatalf("migration changed unrelated metadata: %#v", got)
	}
}
