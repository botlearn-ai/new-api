package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func setupBotcordControllerTestDB(t *testing.T) {
	t.Helper()

	db := openTokenControllerTestDB(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.IntegrationAccount{},
		&model.IntegrationOperation{},
	); err != nil {
		t.Fatalf("failed to migrate botcord controller test tables: %v", err)
	}
}

func postBotcordProvision(t *testing.T, req botcordUserRequest) botcordBalanceView {
	t.Helper()

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/botcord/provision", req, 0)
	ctx.Request.Header.Set("Authorization", "Bearer secret")

	BotcordProvision(ctx)

	return decodeBotcordBalance(t, recorder)
}

func decodeBotcordBalance(t *testing.T, recorder *httptest.ResponseRecorder) botcordBalanceView {
	t.Helper()

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", recorder.Code, recorder.Body.String())
	}
	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected successful response, got %s", response.Message)
	}
	var view botcordBalanceView
	if err := common.Unmarshal(response.Data, &view); err != nil {
		t.Fatalf("failed to decode botcord balance: %v", err)
	}
	return view
}

func TestBotcordProvisionRejectsInvalidSecret(t *testing.T) {
	setupBotcordControllerTestDB(t)
	t.Setenv("BOTCORD_INTERNAL_SECRET", "secret")

	ctx, recorder := newAuthenticatedContext(
		t,
		http.MethodPost,
		"/api/botcord/provision",
		botcordUserRequest{
			ExternalUserId: "user-1",
			InitialUsd:     5,
		},
		0,
	)
	ctx.Request.Header.Set("Authorization", "Bearer wrong")

	BotcordProvision(ctx)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d with body %s", recorder.Code, recorder.Body.String())
	}
	response := decodeAPIResponse(t, recorder)
	if response.Success {
		t.Fatalf("expected unsuccessful response")
	}

	var userCount int64
	if err := model.DB.Model(&model.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected invalid secret not to create users, got %d", userCount)
	}
}

func TestBotcordProvisionIsIdempotent(t *testing.T) {
	setupBotcordControllerTestDB(t)
	t.Setenv("BOTCORD_INTERNAL_SECRET", "secret")

	req := botcordUserRequest{
		ExternalUserId: "user-1",
		DisplayName:    "Ada",
		InitialUsd:     5,
	}
	first := postBotcordProvision(t, req)
	second := postBotcordProvision(t, req)

	if first.UserId != second.UserId {
		t.Fatalf("expected same user id, got %d and %d", first.UserId, second.UserId)
	}
	if first.Token.Id != second.Token.Id {
		t.Fatalf("expected same token id, got %d and %d", first.Token.Id, second.Token.Id)
	}
	if first.Token.ApiKey == "" {
		t.Fatal("expected API key on initial provision")
	}
	if second.Token.ApiKey != "" {
		t.Fatalf("expected repeated provision not to reveal API key, got %q", second.Token.ApiKey)
	}

	var userCount int64
	if err := model.DB.Model(&model.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected one user, got %d", userCount)
	}

	var tokenCount int64
	if err := model.DB.Model(&model.Token{}).Count(&tokenCount).Error; err != nil {
		t.Fatalf("failed to count tokens: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("expected one token, got %d", tokenCount)
	}

	var user model.User
	if err := model.DB.First(&user, first.UserId).Error; err != nil {
		t.Fatalf("failed to load user: %v", err)
	}
	if user.Quota != botcordQuotaFromUsd(5) {
		t.Fatalf("expected initial quota once, got %d", user.Quota)
	}
}

func TestBotcordProvisionAdoptsLegacyAccount(t *testing.T) {
	setupBotcordControllerTestDB(t)
	t.Setenv("BOTCORD_INTERNAL_SECRET", "secret")

	externalUserId := "legacy-user"
	sum := sha256.Sum256([]byte(externalUserId))
	user := model.User{
		Username:    "bc_" + hex.EncodeToString(sum[:])[:16],
		Password:    "unused-password-hash",
		DisplayName: "Legacy User",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Quota:       botcordQuotaFromUsd(5),
		AffCode:     "lgcy",
	}
	if err := model.DB.Create(&user).Error; err != nil {
		t.Fatalf("failed to create legacy user: %v", err)
	}
	token := model.Token{
		UserId:         user.Id,
		Name:           "BotCord Cloud Agent",
		Key:            "legacy-integration-key",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    -1,
		RemainQuota:    botcordQuotaFromUsd(5),
		UnlimitedQuota: false,
	}
	if err := model.DB.Create(&token).Error; err != nil {
		t.Fatalf("failed to create legacy token: %v", err)
	}

	ctx, recorder := newAuthenticatedContext(
		t,
		http.MethodPost,
		"/api/botcord/balance",
		botcordUserRequest{ExternalUserId: externalUserId},
		0,
	)
	ctx.Request.Header.Set("Authorization", "Bearer secret")
	BotcordBalance(ctx)
	balance := decodeBotcordBalance(t, recorder)
	if balance.UserId != user.Id || balance.Token.Id != token.Id {
		t.Fatalf(
			"expected legacy balance account %d/%d, got %d/%d",
			user.Id,
			token.Id,
			balance.UserId,
			balance.Token.Id,
		)
	}

	provisioned := postBotcordProvision(t, botcordUserRequest{
		ExternalUserId: externalUserId,
	})
	if provisioned.UserId != user.Id || provisioned.Token.Id != token.Id {
		t.Fatalf(
			"expected legacy account %d/%d, got %d/%d",
			user.Id,
			token.Id,
			provisioned.UserId,
			provisioned.Token.Id,
		)
	}
	if provisioned.Token.ApiKey != "" {
		t.Fatalf("expected adopted account not to reveal existing key, got %q", provisioned.Token.ApiKey)
	}
}

func TestBotcordBalanceUsesTokenQuota(t *testing.T) {
	setupBotcordControllerTestDB(t)
	t.Setenv("BOTCORD_INTERNAL_SECRET", "secret")

	provisioned := postBotcordProvision(t, botcordUserRequest{
		ExternalUserId: "user-1",
		InitialUsd:     5,
	})
	if err := model.DB.Model(&model.User{}).
		Where("id = ?", provisioned.UserId).
		Update("quota", botcordQuotaFromUsd(18)).Error; err != nil {
		t.Fatalf("failed to update user quota: %v", err)
	}
	if err := model.DB.Model(&model.Token{}).
		Where("id = ?", provisioned.Token.Id).
		Updates(map[string]any{
			"remain_quota": botcordQuotaFromUsd(5),
			"used_quota":   botcordQuotaFromUsd(2),
		}).Error; err != nil {
		t.Fatalf("failed to update token quota: %v", err)
	}

	ctx, recorder := newAuthenticatedContext(
		t,
		http.MethodPost,
		"/api/botcord/balance",
		botcordUserRequest{ExternalUserId: "user-1"},
		0,
	)
	ctx.Request.Header.Set("Authorization", "Bearer secret")

	BotcordBalance(ctx)

	view := decodeBotcordBalance(t, recorder)
	if view.Quota != botcordQuotaFromUsd(18) {
		t.Fatalf("expected user quota to remain visible, got %d", view.Quota)
	}
	if view.Token.RemainQuota != botcordQuotaFromUsd(5) {
		t.Fatalf("expected token remain quota, got %d", view.Token.RemainQuota)
	}
	if view.BalanceUsd != 5 {
		t.Fatalf("expected balance_usd from token quota, got %f", view.BalanceUsd)
	}
	if view.UsedUsd != 2 {
		t.Fatalf("expected used_usd from token usage, got %f", view.UsedUsd)
	}
}

func TestBotcordTopUpDoesNotProvisionMissingUser(t *testing.T) {
	setupBotcordControllerTestDB(t)
	t.Setenv("BOTCORD_INTERNAL_SECRET", "secret")

	ctx, recorder := newAuthenticatedContext(
		t,
		http.MethodPost,
		"/api/botcord/topup",
		botcordTopUpRequest{
			ExternalUserId: "missing-user",
			AmountUsd:      1,
			IdempotencyKey: "missing-user-topup",
		},
		0,
	)
	ctx.Request.Header.Set("Authorization", "Bearer secret")

	BotcordTopUp(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d with body %s", recorder.Code, recorder.Body.String())
	}
	response := decodeAPIResponse(t, recorder)
	if response.Success {
		t.Fatalf("expected unsuccessful response")
	}

	var userCount int64
	if err := model.DB.Model(&model.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected topup not to create users, got %d", userCount)
	}

	var tokenCount int64
	if err := model.DB.Model(&model.Token{}).Count(&tokenCount).Error; err != nil {
		t.Fatalf("failed to count tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("expected topup not to create tokens, got %d", tokenCount)
	}
}
