package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"

	"github.com/mlogclub/simple/sqls"
)

func (s *topicService) GetCategoryAuthors(categoryId int64) ([]models.User, error) {
	users := make([]models.User, 0)
	categoryIds := CategoryService.GetCategoryIdsForList(categoryId)
	if len(categoryIds) == 0 {
		return users, nil
	}
	authors := sqls.DB().Model(&models.Topic{}).Select("user_id").
		Where("status = ? AND category_id IN ?", constants.StatusOk, categoryIds)
	err := sqls.DB().Model(&models.User{}).
		Where("status = ? AND id IN (?)", constants.StatusOk, authors).
		Order("nickname ASC").Order("id ASC").Find(&users).Error
	return users, err
}
