package migrations

import (
	"bbs-go/internal/models"

	"github.com/mlogclub/simple/sqls"
)

func migrate_topic_unread_baselines() error {
	return sqls.DB().AutoMigrate(&models.TopicUnreadBaseline{})
}
