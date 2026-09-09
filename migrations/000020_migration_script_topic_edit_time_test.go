package migrations

import (
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestMigrateTopicEditTime(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:topic_edit_time_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)
	if err := db.AutoMigrate(&models.Topic{}, &models.OperateLog{}); err != nil {
		t.Fatal(err)
	}
	for _, topic := range []models.Topic{
		{Model: models.Model{Id: 1}, CreateTime: 100},
		{Model: models.Model{Id: 2}, CreateTime: 200},
		{Model: models.Model{Id: 3}, CreateTime: 300, EditTime: 350},
		{Model: models.Model{Id: 4}, CreateTime: 400, Status: constants.StatusReview},
		{Model: models.Model{Id: 5}, CreateTime: 500, Status: constants.StatusDeleted},
	} {
		if err := db.Create(&topic).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, log := range []models.OperateLog{
		{DataType: constants.EntityTopic, DataId: 1, OpType: constants.OpTypeUpdate, CreateTime: 50},
		{DataType: constants.EntityTopic, DataId: 1, OpType: constants.OpTypeCreate, CreateTime: 900},
		{DataType: constants.EntityArticle, DataId: 1, OpType: constants.OpTypeUpdate, CreateTime: 900},
		{DataType: constants.EntityTopic, DataId: 2, OpType: constants.OpTypeUpdate, CreateTime: 250},
		{DataType: constants.EntityTopic, DataId: 2, OpType: constants.OpTypeUpdate, CreateTime: 450},
		{DataType: constants.EntityTopic, DataId: 3, OpType: constants.OpTypeUpdate, CreateTime: 600},
		{DataType: constants.EntityTopic, DataId: 4, OpType: constants.OpTypeUpdate, CreateTime: 550},
	} {
		if err := db.Create(&log).Error; err != nil {
			t.Fatal(err)
		}
	}
	for run := 0; run < 2; run++ {
		if err := migrate_topic_edit_time(); err != nil {
			t.Fatal(err)
		}
		for id, want := range map[int64]int64{1: 100, 2: 450, 3: 350, 4: 550, 5: 500} {
			var topic models.Topic
			if err := db.First(&topic, id).Error; err != nil {
				t.Fatal(err)
			}
			if topic.EditTime != want {
				t.Fatalf("run %d, topic %d: edit time = %d, want %d", run, id, topic.EditTime, want)
			}
		}
	}
}
