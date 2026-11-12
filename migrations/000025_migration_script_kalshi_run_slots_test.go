package migrations

import (
	"fmt"
	"testing"
	"time"

	"bbs-go/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestMigrateKalshiRunSlots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:kalshi_slots_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true}})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqls.SetDB(db)
	if err := db.Exec("CREATE TABLE t_kalshi_report (id integer PRIMARY KEY, report_date date NOT NULL, as_of datetime NOT NULL, markdown text NOT NULL, content_hash varchar(64) NOT NULL, create_time bigint NOT NULL DEFAULT 0, update_time bigint NOT NULL DEFAULT 0)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX uk_kalshi_report_date ON t_kalshi_report (report_date)").Error; err != nil {
		t.Fatal(err)
	}
	for i, source := range []string{
		"```json manifest.json\n{\"metadata\":{\"run_slot\":\"morning\"}}\n```\n",
		"```json manifest.json\n{\"metadata\":{}}\n```\n",
	} {
		if err := db.Exec("INSERT INTO t_kalshi_report (id, report_date, as_of, markdown, content_hash) VALUES (?, ?, ?, ?, ?)", i+1, fmt.Sprintf("2026-11-%02d", i+9), "2026-11-09T08:00:00+08:00", source, "hash").Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrate_kalshi_run_slots(); err != nil {
			t.Fatalf("migration %d: %v", i, err)
		}
	}
	var reports []models.KalshiReport
	if err := db.Order("id").Find(&reports).Error; err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 || reports[0].Slot != "morning" || reports[1].Slot != "" {
		t.Fatalf("slots: %+v", reports)
	}
	if db.Migrator().HasIndex(&models.KalshiReport{}, "uk_kalshi_report_date") || !db.Migrator().HasIndex(&models.KalshiReport{}, "uk_kalshi_report_date_slot") {
		t.Fatal("old unique index still present or new index missing")
	}
	if err := db.Exec("INSERT INTO t_kalshi_report (report_date, slot, as_of, markdown, content_hash) VALUES (?, ?, ?, ?, ?)", "2026-11-09", "evening", "2026-11-09T19:00:00+08:00", "test", "hash").Error; err != nil {
		t.Fatalf("second slot: %v", err)
	}
	if err := db.Exec("INSERT INTO t_kalshi_report (report_date, slot, as_of, markdown, content_hash) VALUES (?, ?, ?, ?, ?)", "2026-11-09", "morning", "2026-11-09T19:00:00+08:00", "duplicate", "hash").Error; err == nil {
		t.Fatal("duplicate slot accepted")
	}
}
