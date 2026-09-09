package api

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/idcodec"

	"github.com/gin-gonic/gin"
	"github.com/mlogclub/simple/sqls"
)

func TestParseTopicUserIds(t *testing.T) {
	previous := idcodec.Instance
	idcodec.Init(1)
	t.Cleanup(func() { idcodec.Instance = previous })
	for _, value := range []string{"1,2,1", idcodec.Encode(1) + ", " + idcodec.Encode(2)} {
		ids, err := parseTopicUserIds(value)
		if err != nil || !reflect.DeepEqual(ids, []int64{1, 2}) {
			t.Fatalf("%q: %v, %v", value, ids, err)
		}
	}
	if ids, err := parseTopicUserIds(""); err != nil || len(ids) != 0 {
		t.Fatal("empty filter must allow all users")
	}
	for _, value := range []string{"0", "-1", "invalid!", "1,", ",2", "1,,2", strings.Repeat("1,", 100) + "1", strings.Repeat("9", 20)} {
		if _, err := parseTopicUserIds(value); err == nil {
			t.Fatalf("accepted invalid userIds %q", value)
		}
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("GET", "/api/topic/topics?userIds="+value, nil)
		TopicTopics(ctx)
		var response struct{ Success bool }
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Success {
			t.Fatalf("invalid filters must fail: %s", w.Body.String())
		}
	}
}

func TestTopicAuthorsRequiresLoginAndReturnsOnlyDisplayFields(t *testing.T) {
	previousCodec, previousConfig := idcodec.Instance, config.Instance
	idcodec.Init(1)
	config.Instance = &config.Config{Language: config.LanguageEnUS}
	t.Cleanup(func() { idcodec.Instance, config.Instance = previousCodec, previousConfig })
	db := setupTopicHandlerCategoryTestDB(t)
	if err := db.AutoMigrate(&models.User{}, &models.LevelConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Category{Model: models.Model{Id: 1}, Name: "daily"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{Model: models.Model{Id: 1}, Nickname: "Reporter", Username: sqls.SqlNullString("private-login"), Email: sqls.SqlNullString("private@example.test"), Password: "test-only-hash"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Topic{CategoryId: 1, UserId: 1}).Error; err != nil {
		t.Fatal(err)
	}
	for _, loggedIn := range []bool{false, true} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("GET", "/api/topic/authors?categoryId=1", nil)
		if loggedIn {
			common.SetCurrentUser(ctx, &models.User{Model: models.Model{Id: 1}})
		}
		TopicAuthors(ctx)
		var response struct {
			Success bool
			Data    []map[string]any
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Success != loggedIn {
			t.Fatalf("unexpected login result: %s", w.Body.String())
		}
		if loggedIn {
			if len(response.Data) != 1 || len(response.Data[0]) != 4 || response.Data[0]["id"] != idcodec.Encode(1) || response.Data[0]["nickname"] != "Reporter" {
				t.Fatalf("unexpected author response: %s", w.Body.String())
			}
		}
	}
}
