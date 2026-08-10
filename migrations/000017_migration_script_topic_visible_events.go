package migrations

import (
	"bbs-go/internal/models/constants"

	"github.com/mlogclub/simple/sqls"
)

func migrate_topic_visible_events() error {
	return sqls.WithTransaction(func(ctx *sqls.TxContext) error {
		return ctx.Tx.Exec(`
			INSERT INTO t_topic_visible_event (topic_id, create_time)
			SELECT topic.id, topic.create_time
			FROM t_topic AS topic
			WHERE topic.status = ?
				AND NOT EXISTS (
					SELECT 1
					FROM t_topic_visible_event AS visible_event
					WHERE visible_event.topic_id = topic.id
				)
			ORDER BY topic.id
		`, constants.StatusOk).Error
	})
}
