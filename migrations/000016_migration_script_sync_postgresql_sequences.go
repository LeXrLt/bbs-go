package migrations

import (
	"bbs-go/internal/pkg/config"

	"github.com/mlogclub/simple/sqls"
)

func migrate_sync_postgresql_sequences() error {
	if config.Instance == nil || config.Instance.DB.Type != config.DbTypePostgreSQL {
		return nil
	}

	return sqls.WithTransaction(func(ctx *sqls.TxContext) error {
		statements := []string{
			"SELECT setval(pg_get_serial_sequence('t_role', 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM t_role",
			"SELECT setval(pg_get_serial_sequence('t_category', 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM t_category",
		}
		for _, statement := range statements {
			if err := ctx.Tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
