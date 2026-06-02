package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const botcordTokenName = "BotCord Cloud Agent"

type botcordUserRequest struct {
	ExternalUserId string  `json:"external_user_id"`
	DisplayName    string  `json:"display_name"`
	InitialQuota   int     `json:"initial_quota"`
	InitialUsd     float64 `json:"initial_usd"`
}

type botcordTopUpRequest struct {
	ExternalUserId string  `json:"external_user_id"`
	AmountQuota    int     `json:"amount_quota"`
	AmountUsd      float64 `json:"amount_usd"`
}

type botcordTokenView struct {
	Id             int    `json:"id"`
	Name           string `json:"name"`
	ApiKey         string `json:"api_key,omitempty"`
	RemainQuota    int    `json:"remain_quota"`
	UsedQuota      int    `json:"used_quota"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
}

type botcordBalanceView struct {
	ExternalUserId string           `json:"external_user_id"`
	UserId         int              `json:"user_id"`
	Username       string           `json:"username"`
	Quota          int              `json:"quota"`
	UsedQuota      int              `json:"used_quota"`
	QuotaPerUsd    float64          `json:"quota_per_usd"`
	BalanceUsd     float64          `json:"balance_usd"`
	UsedUsd        float64          `json:"used_usd"`
	Token          botcordTokenView `json:"token"`
}

func requireBotcordInternal(c *gin.Context) bool {
	secret := strings.TrimSpace(common.GetEnvOrDefaultString("BOTCORD_INTERNAL_SECRET", ""))
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "BOTCORD_INTERNAL_SECRET is not configured",
		})
		return false
	}

	got := strings.TrimSpace(c.GetHeader("Authorization"))
	got = strings.TrimSpace(strings.TrimPrefix(got, "Bearer "))
	if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "invalid botcord internal secret",
		})
		return false
	}
	return true
}

func botcordUsername(externalUserId string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(externalUserId)))
	return "bc_" + hex.EncodeToString(sum[:])[:16]
}

func botcordQuotaFromUsd(usd float64) int {
	if usd <= 0 {
		return 0
	}
	return int(math.Round(usd * common.QuotaPerUnit))
}

func botcordInitialQuota(req botcordUserRequest) int {
	if req.InitialQuota > 0 {
		return req.InitialQuota
	}
	if req.InitialUsd > 0 {
		return botcordQuotaFromUsd(req.InitialUsd)
	}
	return botcordQuotaFromUsd(5)
}

func botcordTopUpQuota(req botcordTopUpRequest) int {
	if req.AmountQuota > 0 {
		return req.AmountQuota
	}
	return botcordQuotaFromUsd(req.AmountUsd)
}

func botcordDisplayName(externalUserId string, displayName string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "BotCord " + strings.TrimSpace(externalUserId)
	}
	runes := []rune(name)
	if len(runes) > model.UserNameMaxLength {
		name = string(runes[:model.UserNameMaxLength])
	}
	return name
}

func botcordCreateToken(tx *gorm.DB, user *model.User, remainQuota int) (*model.Token, error) {
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	token := &model.Token{
		UserId:             user.Id,
		Name:               botcordTokenName,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        -1,
		RemainQuota:        max(remainQuota, 0),
		UnlimitedQuota:     false,
		ModelLimitsEnabled: false,
	}
	if setting.DefaultUseAutoGroup {
		token.Group = "auto"
	}
	if err := tx.Create(token).Error; err != nil {
		return nil, err
	}
	return token, nil
}

func botcordEnsureUserAndToken(req botcordUserRequest) (*model.User, *model.Token, bool, error) {
	externalUserId := strings.TrimSpace(req.ExternalUserId)
	if externalUserId == "" {
		return nil, nil, false, errors.New("external_user_id is required")
	}

	username := botcordUsername(externalUserId)
	initialQuota := botcordInitialQuota(req)
	created := false
	var user model.User
	var token model.Token

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("username = ?", username).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			password, keyErr := common.GenerateRandomKey(20)
			if keyErr != nil {
				return keyErr
			}
			passwordHash, hashErr := common.Password2Hash(password)
			if hashErr != nil {
				return hashErr
			}
			user = model.User{
				Username:    username,
				Password:    passwordHash,
				DisplayName: botcordDisplayName(externalUserId, req.DisplayName),
				Role:        common.RoleCommonUser,
				Status:      common.UserStatusEnabled,
				Quota:       initialQuota,
				AffCode:     common.GetRandomString(4),
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
			created = true
		} else if err != nil {
			return err
		}

		err = tx.Where("user_id = ? AND name = ?", user.Id, botcordTokenName).First(&token).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			createdToken, createErr := botcordCreateToken(tx, &user, user.Quota)
			if createErr != nil {
				return createErr
			}
			token = *createdToken
		} else if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	return &user, &token, created, nil
}

func botcordFindUserAndToken(externalUserId string) (*model.User, *model.Token, error) {
	externalUserId = strings.TrimSpace(externalUserId)
	if externalUserId == "" {
		return nil, nil, errors.New("external_user_id is required")
	}
	var user model.User
	if err := model.DB.Where("username = ?", botcordUsername(externalUserId)).First(&user).Error; err != nil {
		return nil, nil, err
	}
	var token model.Token
	if err := model.DB.Where("user_id = ? AND name = ?", user.Id, botcordTokenName).First(&token).Error; err != nil {
		return nil, nil, err
	}
	return &user, &token, nil
}

func botcordBuildBalance(externalUserId string, user *model.User, token *model.Token, includeKey bool) botcordBalanceView {
	view := botcordBalanceView{
		ExternalUserId: externalUserId,
		UserId:         user.Id,
		Username:       user.Username,
		Quota:          user.Quota,
		UsedQuota:      user.UsedQuota,
		QuotaPerUsd:    common.QuotaPerUnit,
		BalanceUsd:     float64(user.Quota) / common.QuotaPerUnit,
		UsedUsd:        float64(user.UsedQuota) / common.QuotaPerUnit,
		Token: botcordTokenView{
			Id:             token.Id,
			Name:           token.Name,
			RemainQuota:    token.RemainQuota,
			UsedQuota:      token.UsedQuota,
			UnlimitedQuota: token.UnlimitedQuota,
		},
	}
	if includeKey {
		view.Token.ApiKey = "sk-" + token.GetFullKey()
	}
	return view
}

func BotcordProvision(c *gin.Context) {
	if !requireBotcordInternal(c) {
		return
	}
	var req botcordUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	user, token, _, err := botcordEnsureUserAndToken(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.InvalidateUserTokensCache(user.Id); err != nil {
		common.SysLog(fmt.Sprintf("botcord provision failed to invalidate token cache: %s", err.Error()))
	}
	common.ApiSuccess(c, botcordBuildBalance(req.ExternalUserId, user, token, true))
}

func BotcordBalance(c *gin.Context) {
	if !requireBotcordInternal(c) {
		return
	}
	var req botcordUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	user, token, err := botcordFindUserAndToken(req.ExternalUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, botcordBuildBalance(req.ExternalUserId, user, token, false))
}

func BotcordTopUp(c *gin.Context) {
	if !requireBotcordInternal(c) {
		return
	}
	var req botcordTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	quota := botcordTopUpQuota(req)
	if quota <= 0 {
		common.ApiError(c, errors.New("amount_usd or amount_quota must be positive"))
		return
	}

	user, token, err := botcordFindUserAndToken(req.ExternalUserId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "botcord user or token not found",
			})
			return
		}
		common.ApiError(c, err)
		return
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", gorm.Expr("quota + ?", quota)).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Token{}).Where("id = ?", token.Id).Update("remain_quota", gorm.Expr("remain_quota + ?", quota)).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.InvalidateUserCache(user.Id); err != nil {
		common.SysLog(fmt.Sprintf("botcord topup failed to invalidate user cache: %s", err.Error()))
	}
	if err := model.InvalidateUserTokensCache(user.Id); err != nil {
		common.SysLog(fmt.Sprintf("botcord topup failed to invalidate token cache: %s", err.Error()))
	}

	user, token, err = botcordFindUserAndToken(req.ExternalUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, botcordBuildBalance(req.ExternalUserId, user, token, false))
}
