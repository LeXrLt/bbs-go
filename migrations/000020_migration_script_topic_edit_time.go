package migrations

import (
	"bbs-go/internal/models/constants"

	"github.com/mlogclub/simple/sqls"
)

func migrate_topic_edit_time() error {
	// Older versions only recorded edits in the operation log. Topics without
	// an edit record use their publication time; preserve already recorded edits.
	return sqls.DB().Exec(`
		UPDATE t_topic
		SET edit_time = COALESCE((
			SELECT MAX(create_time)
			FROM t_operate_log
			WHERE data_type = ? AND op_type = ? AND data_id = t_topic.id
				AND create_time >= t_topic.create_time
		), create_time, 0)
		WHERE edit_time IS NULL OR edit_time = 0
	`, constants.EntityTopic, constants.OpTypeUpdate).Error
}
