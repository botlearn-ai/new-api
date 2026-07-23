package dto

type IntegrationProvisionRequest struct {
	ExternalUserId string  `json:"external_user_id"`
	DisplayName    string  `json:"display_name"`
	InitialQuota   int     `json:"initial_quota"`
	InitialUsd     float64 `json:"initial_usd"`
}

type IntegrationTopUpRequest struct {
	ExternalUserId string  `json:"external_user_id"`
	AmountQuota    int     `json:"amount_quota"`
	AmountUsd      float64 `json:"amount_usd"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type IntegrationTokenView struct {
	Id             int    `json:"id"`
	Name           string `json:"name"`
	ApiKey         string `json:"api_key,omitempty"`
	RemainQuota    int    `json:"remain_quota"`
	UsedQuota      int    `json:"used_quota"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
}

type IntegrationBalanceView struct {
	IntegrationId  string               `json:"integration_id"`
	ExternalUserId string               `json:"external_user_id"`
	UserId         int                  `json:"user_id"`
	Username       string               `json:"username"`
	Quota          int                  `json:"quota"`
	UsedQuota      int                  `json:"used_quota"`
	QuotaPerUsd    float64              `json:"quota_per_usd"`
	BalanceUsd     float64              `json:"balance_usd"`
	UsedUsd        float64              `json:"used_usd"`
	Replayed       bool                 `json:"replayed,omitempty"`
	Token          IntegrationTokenView `json:"token"`
}
