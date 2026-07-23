package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIntegrationAuthAcceptsConfiguredService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTEGRATION_CLIENTS", `{"partner-a":"secret-a","partner-b":"secret-b"}`)

	router := gin.New()
	router.Use(IntegrationAuth())
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"integration_id": c.GetString(IntegrationIdContextKey),
		})
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Integration-ID", "PARTNER-A")
	request.Header.Set("Authorization", "Bearer secret-a")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != `{"integration_id":"partner-a"}` {
		t.Fatalf("unexpected response body %s", recorder.Body.String())
	}
}

func TestIntegrationAuthRejectsAnotherServicesSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTEGRATION_CLIENTS", `{"partner-a":"secret-a","partner-b":"secret-b"}`)

	router := gin.New()
	router.Use(IntegrationAuth())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Integration-ID", "partner-a")
	request.Header.Set("Authorization", "Bearer secret-b")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d with body %s", recorder.Code, recorder.Body.String())
	}
}
