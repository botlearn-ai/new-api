package controller

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const legacyBotcordIntegrationId = "botcord"

// Compatibility aliases keep the existing BotCord controller tests and any
// in-package callers source-compatible while the implementation is generic.
type botcordUserRequest = dto.IntegrationProvisionRequest
type botcordTopUpRequest = dto.IntegrationTopUpRequest
type botcordTokenView = dto.IntegrationTokenView
type botcordBalanceView = dto.IntegrationBalanceView

func requireBotcordInternal(c *gin.Context) bool {
	secret := strings.TrimSpace(common.GetEnvOrDefaultString("BOTCORD_INTERNAL_SECRET", ""))
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "BOTCORD_INTERNAL_SECRET is not configured",
		})
		return false
	}
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") ||
		subtle.ConstantTimeCompare([]byte(parts[1]), []byte(secret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "invalid botcord internal secret",
		})
		return false
	}
	c.Set(middleware.IntegrationIdContextKey, legacyBotcordIntegrationId)
	return true
}

func botcordQuotaFromUsd(usd float64) int {
	quota, _ := service.IntegrationQuotaFromUsd(usd)
	return quota
}

func BotcordProvision(c *gin.Context) {
	if !requireBotcordInternal(c) {
		return
	}
	handleIntegrationProvision(c, 5)
}

func BotcordBalance(c *gin.Context) {
	if !requireBotcordInternal(c) {
		return
	}
	IntegrationBalance(c)
}

func BotcordTopUp(c *gin.Context) {
	if !requireBotcordInternal(c) {
		return
	}
	IntegrationTopUp(c)
}
