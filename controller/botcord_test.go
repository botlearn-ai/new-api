package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func setupBotcordControllerTestDB(t *testing.T) {
	t.Helper()

	db := openTokenControllerTestDB(t)
	if err := db.AutoMigrate(&model.User{}, &model.Token{}); err != nil {
		t.Fatalf("failed to migrate botcord controller test tables: %v", err)
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
