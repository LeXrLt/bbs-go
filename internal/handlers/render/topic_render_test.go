package render

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"bbs-go/internal/cache"
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/idcodec"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mlogclub/simple/sqls"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestBuildSimpleTopicsBatchesCategoriesAndTopicTags(t *testing.T) {
	dsn := fmt.Sprintf("file:topic_batch_render_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "t_", SingularTable: true},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.LevelConfig{}, &models.Category{}, &models.TopicTag{}, &models.Tag{}); err != nil {
		t.Fatalf("auto migrate render models: %v", err)
	}
	sqls.SetDB(db)
	idcodec.Init(1)
	if err := db.Create(&models.Category{Model: models.Model{Id: 11}, Name: "Category"}).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := db.Create(&models.Tag{Model: models.Model{Id: 21}, Name: "Tag"}).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := db.Create(&[]models.TopicTag{{TopicId: 101, TagId: 21}, {TopicId: 102, TagId: 21}}).Error; err != nil {
		t.Fatalf("create topic tags: %v", err)
	}
	cache.TagCache.Invalidate(21)

	queries := map[string]int{}
	if err := db.Callback().Query().Before("gorm:query").Register("count_topic_render_relations", func(tx *gorm.DB) {
		queries[tx.Statement.Table]++
	}); err != nil {
		t.Fatalf("register query counter: %v", err)
	}
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	topics := []models.Topic{
		{Model: models.Model{Id: 101}, CategoryId: 11, Title: "First"},
		{Model: models.Model{Id: 102}, CategoryId: 11, Title: "Second"},
		{Model: models.Model{Id: 103}, CategoryId: 0, Title: "No category"},
	}
	responses := BuildSimpleTopics(ctx, topics)
	if queries["t_category"] != 1 || queries["t_topic_tag"] != 1 {
		t.Fatalf("relation queries = %#v, want one category and one topic tag query", queries)
	}
	if len(responses) != 3 || responses[0].Category == nil || responses[1].Category == nil || responses[2].Category != nil {
		t.Fatalf("unexpected category responses: %#v", responses)
	}
	if responses[0].Tags == nil || len(*responses[0].Tags) != 1 || (*responses[0].Tags)[0].Name != "Tag" ||
		responses[1].Tags == nil || len(*responses[1].Tags) != 1 || responses[2].Tags != nil {
		t.Fatalf("unexpected tag responses: %#v", responses)
	}
}

func TestBuildSimpleTopicSummaryUsesParagraphPreviewForRegularTopics(t *testing.T) {
	first := strings.Repeat("首", 199)
	topic := &models.Topic{
		Type:        constants.TopicTypeTopic,
		ContentType: constants.ContentTypeMarkdown,
		Content:     first + "\n\n第二段完整内容\n\n不应展示的第三段",
	}

	want := first + "\n第二段完整内容..."
	if got := buildSimpleTopicSummary(topic); got != want {
		t.Fatalf("buildSimpleTopicSummary() = %q, want %q", got, want)
	}
}

func TestBuildSimpleTopicSummaryCapsRegularTopicPreviewAt800Runes(t *testing.T) {
	topic := &models.Topic{
		Type:        constants.TopicTypeTopic,
		ContentType: constants.ContentTypeHtml,
		Content:     "<p>" + strings.Repeat("界", 900) + "</p>",
	}

	got := buildSimpleTopicSummary(topic)
	if utf8.RuneCountInString(got) != topicPreviewMaxLength {
		t.Fatalf("summary length = %d, want %d", utf8.RuneCountInString(got), topicPreviewMaxLength)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated summary should end with ellipsis: %q", got[len(got)-3:])
	}
}

func TestBuildSimpleTopicSummaryKeepsQuestionPreviewRule(t *testing.T) {
	topic := &models.Topic{
		Type:        constants.TopicTypeQA,
		ContentType: constants.ContentTypeMarkdown,
		Content:     strings.Repeat("问", topicSummaryLength+1),
	}

	want := strings.Repeat("问", topicSummaryLength) + "..."
	if got := buildSimpleTopicSummary(topic); got != want {
		t.Fatalf("buildSimpleTopicSummary() = %q, want legacy question summary", got)
	}
}
