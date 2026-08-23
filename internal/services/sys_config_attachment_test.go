package services

import (
	"testing"

	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/dto"
)

func TestNormalizeAttachmentConfigSizeLimit(t *testing.T) {
	for _, test := range []struct {
		name string
		got  int
		want int
	}{
		{name: "missing uses default", got: 0, want: constants.AttachmentMaxSizeMB},
		{name: "negative uses default", got: -1, want: constants.AttachmentMaxSizeMB},
		{name: "smaller configured limit is preserved", got: 64, want: 64},
		{name: "maximum is accepted", got: constants.AttachmentMaxSizeMB, want: constants.AttachmentMaxSizeMB},
		{name: "oversized config is capped", got: constants.AttachmentMaxSizeMB + 1, want: constants.AttachmentMaxSizeMB},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := normalizeAttachmentConfig(dto.AttachmentConfig{MaxSizeMB: test.got})
			if cfg.MaxSizeMB != test.want {
				t.Fatalf("maxSizeMB=%d want %d", cfg.MaxSizeMB, test.want)
			}
		})
	}
}

func TestValidateAttachmentConfigSizeLimit(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "minimum", value: `{"maxSizeMB":1}`},
		{name: "maximum", value: `{"maxSizeMB":256}`},
		{name: "zero", value: `{"maxSizeMB":0}`, wantErr: true},
		{name: "above maximum", value: `{"maxSizeMB":257}`, wantErr: true},
		{name: "fraction", value: `{"maxSizeMB":1.5}`, wantErr: true},
		{name: "missing uses server default", value: `{}`},
		{name: "malformed", value: `{`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateAttachmentConfig(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateAttachmentConfig() error=%v wantErr=%t", err, test.wantErr)
			}
		})
	}
}
