package models

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestAttachmentFileTypeColumnFitsOfficeMIMETypes(t *testing.T) {
	field, ok := reflect.TypeOf(Attachment{}).FieldByName("FileType")
	if !ok {
		t.Fatal("Attachment.FileType field is missing")
	}

	columnSize := 0
	for _, option := range strings.Split(field.Tag.Get("gorm"), ";") {
		value, found := strings.CutPrefix(option, "size:")
		if !found {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("parse Attachment.FileType size %q: %v", value, err)
		}
		columnSize = parsed
		break
	}

	for _, contentType := range []string{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
	} {
		if columnSize < len(contentType) {
			t.Fatalf("Attachment.FileType size %d does not fit %q (%d bytes)", columnSize, contentType, len(contentType))
		}
	}
}
