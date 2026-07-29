package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/req"
	"bbs-go/internal/permissions"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/repositories"
	"testing"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/sqls"
)

func setupCommentServiceTestDB(t *testing.T) {
	t.Helper()
	config.Instance = &config.Config{Language: config.DefaultLanguage}
	db := setupTestDB(t)
	if err := db.AutoMigrate(&models.Comment{}, &models.Role{}, &models.UserRole{}, &models.Permission{}, &models.RolePermission{}); err != nil {
		t.Fatalf("auto migrate comment: %v", err)
	}
	PermissionService.ClearCache()
}

func mustCreateComment(t *testing.T, comment *models.Comment) *models.Comment {
	t.Helper()
	if comment.Status == 0 {
		comment.Status = constants.StatusOk
	}
	if err := repositories.CommentRepository.Create(sqls.DB(), comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	return comment
}

func TestCommentServicePublishDefaultsToText(t *testing.T) {
	setupCommentServiceTestDB(t)

	comment, err := CommentService.Publish(0, req.CreateCommentReq{
		EntityType: constants.EntityArticle,
		EntityId:   "1",
		Content:    "  plain **text**  ",
	})
	if err != nil {
		t.Fatalf("publish comment: %v", err)
	}
	if comment.ContentType != constants.ContentTypeText {
		t.Fatalf("expected default content type %q, got %q", constants.ContentTypeText, comment.ContentType)
	}
	if comment.Content != "plain **text**" {
		t.Fatalf("expected trimmed source content, got %q", comment.Content)
	}

	stored := CommentService.Get(comment.Id)
	if stored == nil {
		t.Fatal("expected persisted comment")
	}
	if stored.ContentType != constants.ContentTypeText || stored.Content != comment.Content {
		t.Fatalf("unexpected persisted comment: %#v", stored)
	}
}

func TestCommentServicePublishAcceptsExplicitText(t *testing.T) {
	setupCommentServiceTestDB(t)

	comment, err := CommentService.Publish(0, req.CreateCommentReq{
		EntityType:  constants.EntityArticle,
		EntityId:    "1",
		Content:     "plain text",
		ContentType: constants.ContentTypeText,
	})
	if err != nil {
		t.Fatalf("publish text comment: %v", err)
	}
	if comment.ContentType != constants.ContentTypeText {
		t.Fatalf("expected text content type, got %q", comment.ContentType)
	}
}

func TestCommentServicePublishStoresMarkdownSource(t *testing.T) {
	setupCommentServiceTestDB(t)
	source := "## Heading\n\n**bold**"

	comment, err := CommentService.Publish(0, req.CreateCommentReq{
		EntityType:  constants.EntityArticle,
		EntityId:    "1",
		Content:     source,
		ContentType: constants.ContentTypeMarkdown,
	})
	if err != nil {
		t.Fatalf("publish markdown comment: %v", err)
	}
	if comment.ContentType != constants.ContentTypeMarkdown {
		t.Fatalf("expected markdown content type, got %q", comment.ContentType)
	}
	if comment.Content != source {
		t.Fatalf("expected markdown source to be preserved, got %q", comment.Content)
	}

	stored := CommentService.Get(comment.Id)
	if stored == nil || stored.ContentType != constants.ContentTypeMarkdown || stored.Content != source {
		t.Fatalf("unexpected persisted markdown comment: %#v", stored)
	}
}

func TestCommentServicePublishRejectsUnsupportedContentTypes(t *testing.T) {
	setupCommentServiceTestDB(t)

	for _, contentType := range []constants.ContentType{constants.ContentTypeHtml, "unknown"} {
		t.Run(string(contentType), func(t *testing.T) {
			comment, err := CommentService.Publish(0, req.CreateCommentReq{
				EntityType:  constants.EntityArticle,
				EntityId:    "1",
				Content:     "content",
				ContentType: contentType,
			})
			if err == nil {
				t.Fatalf("expected content type %q to be rejected", contentType)
			}
			if err.Error() != locales.Get("comment.content_type_invalid") {
				t.Fatalf("unexpected error: %v", err)
			}
			if comment != nil {
				t.Fatalf("expected no comment, got %#v", comment)
			}
		})
	}

	if count := CommentService.Count(sqls.NewCnd()); count != 0 {
		t.Fatalf("expected rejected comments not to be persisted, got %d", count)
	}
}

func TestCommentService_DeleteByUserRejectsNonAuthorWithoutPermission(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityTopic,
		EntityId:    20,
		Content:     "hello",
		ContentType: constants.ContentTypeText,
	})

	regularUser := &models.User{Roles: ""}
	if err := CommentService.DeleteByUser(regularUser, comment.Id); err == nil {
		t.Fatalf("expected permission error for regular user")
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusOk {
		t.Fatalf("expected comment status ok, got %d", got.Status)
	}
}

func TestCommentService_DeleteByUserAllowsAuthor(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityTopic,
		EntityId:    20,
		Content:     "hello",
		ContentType: constants.ContentTypeText,
	})

	author := &models.User{Model: models.Model{Id: 10}}
	if err := CommentService.DeleteByUser(author, comment.Id); err != nil {
		t.Fatalf("delete by author: %v", err)
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusDeleted {
		t.Fatalf("expected comment status deleted, got %d", got.Status)
	}
}

func TestCommentService_DeleteByUserAllowsCommentDeletePermission(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityTopic,
		EntityId:    20,
		Content:     "hello",
		ContentType: constants.ContentTypeText,
	})
	now := dates.NowTimestamp()
	moderator := mustCreateUser(t, now)
	role := mustCreateRole(t, "comment-moderator", constants.StatusOk)
	permission := mustCreatePermission(t, permissions.PermissionCommentDelete.Code, constants.StatusOk)
	mustAssignRole(t, moderator, role)
	mustGrantPermission(t, role, permission)

	if err := CommentService.DeleteByUser(moderator, comment.Id); err != nil {
		t.Fatalf("delete with comment permission: %v", err)
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusDeleted {
		t.Fatalf("expected comment status deleted, got %d", got.Status)
	}
}

func TestCommentService_DeleteByUserAllowsOwner(t *testing.T) {
	setupCommentServiceTestDB(t)
	comment := mustCreateComment(t, &models.Comment{
		UserId:      10,
		EntityType:  constants.EntityComment,
		EntityId:    20,
		Content:     "reply",
		ContentType: constants.ContentTypeText,
	})

	ownerUser := &models.User{Roles: constants.RoleOwner}
	if err := CommentService.DeleteByUser(ownerUser, comment.Id); err != nil {
		t.Fatalf("delete by owner: %v", err)
	}

	got := CommentService.Get(comment.Id)
	if got == nil {
		t.Fatalf("expected comment to still exist")
	}
	if got.Status != constants.StatusDeleted {
		t.Fatalf("expected comment status deleted, got %d", got.Status)
	}
}
