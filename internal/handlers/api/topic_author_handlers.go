package api

import (
	"fmt"
	"strings"

	"bbs-go/internal/handlers/render"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/pkg/idcodec"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/pkg/params"
	"bbs-go/internal/services"

	"github.com/gin-gonic/gin"
)

func parseTopicUserIds(value string) ([]int64, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > 100 {
		return nil, fmt.Errorf("userIds must contain at most 100 users")
	}
	ids := make([]int64, 0, len(parts))
	seen := make(map[int64]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 || len(part) > 19 {
			return nil, fmt.Errorf("invalid userIds")
		}
		id := idcodec.Decode(part)
		if id <= 0 {
			return nil, fmt.Errorf("invalid userIds")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func TopicAuthors(ctx *gin.Context) {
	if _, err := common.CheckLogin(ctx); err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	categoryId := params.FormValueInt64Default(ctx, "categoryId", 0)
	category := services.CategoryService.Get(categoryId)
	if categoryId <= 0 || category == nil || category.Status != constants.StatusOk {
		ginx.WriteJSON(ctx, ginx.ErrorMessage(locales.Get("common.not_found")))
		return
	}
	users, err := services.TopicService.GetCategoryAuthors(categoryId)
	if err != nil {
		ginx.WriteJSON(ctx, err)
		return
	}
	type author struct {
		Id          string `json:"id"`
		Nickname    string `json:"nickname"`
		Avatar      string `json:"avatar"`
		SmallAvatar string `json:"smallAvatar"`
	}
	result := make([]author, 0, len(users))
	for _, user := range users {
		info := render.BuildUserInfo(&user)
		result = append(result, author{
			Id: info.Id, Nickname: info.Nickname, Avatar: info.Avatar, SmallAvatar: info.SmallAvatar,
		})
	}
	ginx.WriteJSON(ctx, result)
}
