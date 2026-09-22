package migrations

import (
	"bbs-go/internal/models"

	"github.com/mlogclub/simple/sqls"
)

func migrate_topic_reads() error {
	return sqls.DB().AutoMigrate(&models.TopicRead{})
}
