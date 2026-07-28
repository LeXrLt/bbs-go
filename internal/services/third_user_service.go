package services

import (
	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/pkg/bbsurls"
	"bbs-go/internal/pkg/github"
	"bbs-go/internal/pkg/google"
	"bbs-go/internal/pkg/locales"
	"bbs-go/internal/pkg/params"
	"bbs-go/internal/pkg/wx"
	"bbs-go/internal/repositories"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mlogclub/simple/common/dates"
	"github.com/mlogclub/simple/common/jsons"
	"github.com/mlogclub/simple/sqls"
)

var ThirdUserService = newThirdUserService()

func newThirdUserService() *thirdUserService {
	return &thirdUserService{}
}

type thirdUserService struct {
}

func (s *thirdUserService) Get(id int64) *models.ThirdUser {
	return repositories.ThirdUserRepository.Get(sqls.DB(), id)
}

func (s *thirdUserService) Take(where ...interface{}) *models.ThirdUser {
	return repositories.ThirdUserRepository.Take(sqls.DB(), where...)
}

func (s *thirdUserService) Find(cnd *sqls.Cnd) []models.ThirdUser {
	return repositories.ThirdUserRepository.Find(sqls.DB(), cnd)
}

func (s *thirdUserService) FindOne(cnd *sqls.Cnd) *models.ThirdUser {
	return repositories.ThirdUserRepository.FindOne(sqls.DB(), cnd)
}

func (s *thirdUserService) FindPageByParams(params *params.QueryParams) (list []models.ThirdUser, paging *sqls.Paging) {
	return repositories.ThirdUserRepository.FindPageByParams(sqls.DB(), params)
}

func (s *thirdUserService) FindPageByCnd(cnd *sqls.Cnd) (list []models.ThirdUser, paging *sqls.Paging) {
	return repositories.ThirdUserRepository.FindPageByCnd(sqls.DB(), cnd)
}

func (s *thirdUserService) Count(cnd *sqls.Cnd) int64 {
	return repositories.ThirdUserRepository.Count(sqls.DB(), cnd)
}

func (s *thirdUserService) Create(t *models.ThirdUser) error {
	return repositories.ThirdUserRepository.Create(sqls.DB(), t)
}

func (s *thirdUserService) Update(t *models.ThirdUser) error {
	return repositories.ThirdUserRepository.Update(sqls.DB(), t)
}

func (s *thirdUserService) Updates(id int64, columns map[string]interface{}) error {
	return repositories.ThirdUserRepository.Updates(sqls.DB(), id, columns)
}

func (s *thirdUserService) UpdateColumn(id int64, name string, value interface{}) error {
	return repositories.ThirdUserRepository.UpdateColumn(sqls.DB(), id, name, value)
}

func (s *thirdUserService) Delete(id int64) {
	repositories.ThirdUserRepository.Delete(sqls.DB(), id)
}

func (s *thirdUserService) GetByOpenId(openId string, thirdType constants.ThirdType) *models.ThirdUser {
	return repositories.ThirdUserRepository.GetByOpenId(sqls.DB(), openId, thirdType)
}

func (s *thirdUserService) GetByUserId(userId int64, thirdType constants.ThirdType) *models.ThirdUser {
	return repositories.ThirdUserRepository.GetByUserId(sqls.DB(), userId, thirdType)
}

func (s *thirdUserService) loginBoundUser(openId string, thirdType constants.ThirdType) (*models.User, error) {
	thirdUser := s.GetByOpenId(openId, thirdType)
	if thirdUser == nil || thirdUser.UserId <= 0 {
		return nil, errors.New(locales.Get("auth.registration_disabled"))
	}

	user := UserService.Get(thirdUser.UserId)
	if user == nil {
		return nil, errors.New(locales.Get("errors.user_not_found_or_disabled"))
	}
	return user, nil
}

func (s *thirdUserService) LoginWeixin(code, state string) (*models.User, error) {
	loginConfig := SysConfigService.GetLoginConfig()
	oauth := wx.NewOfficialAccount(loginConfig.WeixinLogin.AppId, loginConfig.WeixinLogin.AppSecret).GetOauth()
	accessToken, err := oauth.GetUserAccessToken(code)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	info, err := oauth.GetUserInfo(accessToken.AccessToken, accessToken.OpenID, "cn")
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	return s.loginBoundUser(info.OpenID, constants.ThirdTypeWeixin)
}

func (s *thirdUserService) BindWeixin(userId int64, code, state string) error {
	if temp := s.GetByUserId(userId, constants.ThirdTypeWeixin); temp != nil {
		return errors.New(locales.Getf("auth.wechat_already_bound", temp.Nickname))
	}

	loginConfig := SysConfigService.GetLoginConfig()
	oauth := wx.NewOfficialAccount(loginConfig.WeixinLogin.AppId, loginConfig.WeixinLogin.AppSecret).GetOauth()
	accessToken, err := oauth.GetUserAccessToken(code)
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	info, err := oauth.GetUserInfo(accessToken.AccessToken, accessToken.OpenID, "cn")
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	if temp := s.GetByOpenId(info.OpenID, constants.ThirdTypeWeixin); temp != nil && temp.Id != userId {
		return errors.New(locales.Get("auth.wechat_bound_to_other"))
	}

	return s.Create(&models.ThirdUser{
		UserId:     userId,
		OpenId:     info.OpenID,
		ThirdType:  constants.ThirdTypeWeixin,
		Nickname:   info.Nickname,
		Avatar:     info.HeadImgURL,
		ExtraData:  jsons.ToJsonStr(info),
		CreateTime: dates.NowTimestamp(),
	})
}

func (s *thirdUserService) UnbindWeixin(userId int64) {
	thirdUser := s.GetByUserId(userId, constants.ThirdTypeWeixin)
	if thirdUser == nil {
		return
	}
	repositories.ThirdUserRepository.Delete(sqls.DB(), thirdUser.Id)
}

func (s *thirdUserService) LoginGoogle(code, state string) (*models.User, error) {
	loginConfig := SysConfigService.GetLoginConfig()
	if !loginConfig.GoogleLogin.Enabled {
		return nil, errors.New(locales.Get("auth.google_login_disabled"))
	}

	// 使用与授权时相同的 redirectURI（必须完全一致）
	redirectURI := bbsurls.AbsUrl(google.CallbackPathLogin)
	oauth := google.NewGoogleOAuth(loginConfig.GoogleLogin.ClientId, loginConfig.GoogleLogin.ClientSecret, redirectURI)

	ctx := context.Background()
	info, err := oauth.GetUserInfo(ctx, code)
	if err != nil {
		slog.Error("Google登录获取用户信息失败", slog.Any("err", err))
		return nil, err
	}

	return s.loginBoundUser(info.ID, constants.ThirdTypeGoogle)
}

func (s *thirdUserService) LoginGoogleOneTap(credential string) (*models.User, error) {
	loginConfig := SysConfigService.GetLoginConfig()
	if !loginConfig.GoogleLogin.Enabled {
		return nil, errors.New(locales.Get("auth.google_login_disabled"))
	}

	// 使用 Google API 验证 JWT
	ctx := context.Background()
	info, err := google.VerifyJWTWithGoogleAPI(ctx, credential)
	if err != nil {
		slog.Error("Google OneTap JWT验证失败", slog.Any("err", err))
		return nil, err
	}

	// 验证 Client ID（从 JWT 中提取，但我们已经通过 Google API 验证了）
	// 这里可以额外验证 info 是否匹配我们的 Client ID

	return s.loginBoundUser(info.ID, constants.ThirdTypeGoogle)
}

func (s *thirdUserService) LoginGithub(code, state string) (*models.User, error) {
	loginConfig := SysConfigService.GetLoginConfig()
	if !loginConfig.GithubLogin.Enabled {
		return nil, errors.New(locales.Get("auth.github_login_disabled"))
	}

	redirectURI := bbsurls.AbsUrl(github.AuthorizationCallbackURL)
	oauth := github.NewGithubOAuth(loginConfig.GithubLogin.ClientId, loginConfig.GithubLogin.ClientSecret, redirectURI)

	ctx := context.Background()
	info, err := oauth.GetUserInfo(ctx, code)
	if err != nil {
		slog.Error("GitHub登录获取用户信息失败", slog.Any("err", err))
		return nil, err
	}

	openId := fmt.Sprintf("%d", info.ID)
	return s.loginBoundUser(openId, constants.ThirdTypeGithub)
}

func (s *thirdUserService) BindGithub(userId int64, code, state string) error {
	if temp := s.GetByUserId(userId, constants.ThirdTypeGithub); temp != nil {
		return errors.New(locales.Getf("auth.github_already_bound", temp.Nickname))
	}

	loginConfig := SysConfigService.GetLoginConfig()
	if !loginConfig.GithubLogin.Enabled {
		return errors.New(locales.Get("auth.github_login_disabled"))
	}

	// GitHub 只允许配置一个回调地址，这里必须与发起授权时使用的 redirectURI 保持完全一致
	// 统一使用登录回调路径 CallbackPathLogin
	redirectURI := bbsurls.AbsUrl(github.AuthorizationCallbackURL)
	oauth := github.NewGithubOAuth(loginConfig.GithubLogin.ClientId, loginConfig.GithubLogin.ClientSecret, redirectURI)

	ctx := context.Background()
	info, err := oauth.GetUserInfo(ctx, code)
	if err != nil {
		slog.Error("GitHub绑定获取用户信息失败", slog.Any("err", err))
		return err
	}

	openId := fmt.Sprintf("%d", info.ID)
	if temp := s.GetByOpenId(openId, constants.ThirdTypeGithub); temp != nil && temp.UserId != userId {
		return errors.New(locales.Get("auth.github_bound_to_other"))
	}

	nickname := info.Name
	if nickname == "" {
		nickname = info.Login
		if nickname == "" {
			nickname = locales.Get("auth.github_default_nickname")
		}
	}

	return s.Create(&models.ThirdUser{
		UserId:     userId,
		OpenId:     openId,
		ThirdType:  constants.ThirdTypeGithub,
		Nickname:   nickname,
		Avatar:     info.AvatarURL,
		ExtraData:  jsons.ToJsonStr(info),
		CreateTime: dates.NowTimestamp(),
		UpdateTime: dates.NowTimestamp(),
	})
}

func (s *thirdUserService) UnbindGithub(userId int64) {
	thirdUser := s.GetByUserId(userId, constants.ThirdTypeGithub)
	if thirdUser == nil {
		return
	}
	repositories.ThirdUserRepository.Delete(sqls.DB(), thirdUser.Id)
}

func (s *thirdUserService) BindGoogle(userId int64, code, state string) error {
	if temp := s.GetByUserId(userId, constants.ThirdTypeGoogle); temp != nil {
		return errors.New(locales.Getf("auth.google_already_bound", temp.Nickname))
	}

	loginConfig := SysConfigService.GetLoginConfig()
	if !loginConfig.GoogleLogin.Enabled {
		return errors.New(locales.Get("auth.google_login_disabled"))
	}

	// 使用与授权时相同的 redirectURI（必须完全一致）
	redirectURI := bbsurls.AbsUrl(google.CallbackPathBind)
	oauth := google.NewGoogleOAuth(loginConfig.GoogleLogin.ClientId, loginConfig.GoogleLogin.ClientSecret, redirectURI)

	ctx := context.Background()
	info, err := oauth.GetUserInfo(ctx, code)
	if err != nil {
		slog.Error("Google绑定获取用户信息失败", slog.Any("err", err))
		return err
	}

	if temp := s.GetByOpenId(info.ID, constants.ThirdTypeGoogle); temp != nil && temp.UserId != userId {
		return errors.New(locales.Get("auth.google_bound_to_other"))
	}

	nickname := info.Name
	if nickname == "" {
		nickname = info.Email
		if nickname == "" {
			nickname = locales.Get("auth.google_default_nickname")
		}
	}

	return s.Create(&models.ThirdUser{
		UserId:     userId,
		OpenId:     info.ID,
		ThirdType:  constants.ThirdTypeGoogle,
		Nickname:   nickname,
		Avatar:     info.Picture,
		ExtraData:  jsons.ToJsonStr(info),
		CreateTime: dates.NowTimestamp(),
		UpdateTime: dates.NowTimestamp(),
	})
}

func (s *thirdUserService) UnbindGoogle(userId int64) {
	thirdUser := s.GetByUserId(userId, constants.ThirdTypeGoogle)
	if thirdUser == nil {
		return
	}
	repositories.ThirdUserRepository.Delete(sqls.DB(), thirdUser.Id)
}
