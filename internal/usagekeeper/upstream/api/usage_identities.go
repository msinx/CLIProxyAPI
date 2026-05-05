package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/service"
)

const (
	usageIdentityAuthTypeOAuth  = 1
	usageIdentityAuthTypeAPIKey = 2
)

type usageIdentitiesResponse struct {
	Identities []usageIdentityPayload `json:"identities"`
}

type usageIdentityPayload struct {
	ID                         int64   `json:"id"`
	Name                       string  `json:"name"`
	AuthType                   int     `json:"auth_type"`
	AuthTypeName               string  `json:"auth_type_name"`
	Identity                   string  `json:"identity"`
	Type                       string  `json:"type"`
	Provider                   string  `json:"provider"`
	TotalRequests              int64   `json:"total_requests"`
	SuccessCount               int64   `json:"success_count"`
	FailureCount               int64   `json:"failure_count"`
	InputTokens                int64   `json:"input_tokens"`
	OutputTokens               int64   `json:"output_tokens"`
	ReasoningTokens            int64   `json:"reasoning_tokens"`
	CachedTokens               int64   `json:"cached_tokens"`
	TotalTokens                int64   `json:"total_tokens"`
	LastAggregatedUsageEventID int64   `json:"last_aggregated_usage_event_id"`
	FirstUsedAt                *string `json:"first_used_at,omitempty"`
	LastUsedAt                 *string `json:"last_used_at,omitempty"`
	StatsUpdatedAt             *string `json:"stats_updated_at,omitempty"`
	IsDeleted                  bool    `json:"is_deleted"`
	CreatedAt                  string  `json:"created_at"`
	UpdatedAt                  string  `json:"updated_at"`
	DeletedAt                  *string `json:"deleted_at,omitempty"`
}

func registerUsageIdentitiesRoute(
	router gin.IRoutes,
	usageProvider service.UsageProvider,
	authFileProvider service.AuthFileProvider,
	providerMetadataProvider service.ProviderMetadataProvider,
) {
	router.GET("/usage/identities", func(c *gin.Context) {
		if usageProvider == nil {
			c.JSON(http.StatusOK, usageIdentitiesResponse{Identities: []usageIdentityPayload{}})
			return
		}

		filter, err := parseUsageFilterQuery(c.Request, time.Now().UTC())
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		rows, err := usageProvider.ListUsageCredentialStats(c.Request.Context(), filter)
		if err != nil {
			writeInternalError(c, "list usage identity stats failed", err)
			return
		}

		authFiles, providerMetadata, err := loadUsageResolutionData(c, authFileProvider, providerMetadataProvider)
		if err != nil {
			writeInternalError(c, "load usage resolution data failed", err)
			return
		}
		resolver := newUsageSourceResolver(authFiles, providerMetadata)
		c.JSON(http.StatusOK, usageIdentitiesResponse{Identities: buildUsageIdentitiesPayload(rows, resolver)})
	})
}

func buildUsageIdentitiesPayload(rows []service.UsageCredentialStat, resolver usageSourceResolver) []usageIdentityPayload {
	if len(rows) == 0 {
		return []usageIdentityPayload{}
	}

	credentials := buildUsageCredentialsPayload(rows, resolver)
	now := time.Now().UTC().Format(time.RFC3339)
	identities := make([]usageIdentityPayload, 0, len(credentials))
	for index, credential := range credentials {
		authType, authTypeName := usageIdentityAuthFields(credential.SourceType, credential.SourceKey)
		identities = append(identities, usageIdentityPayload{
			ID:            int64(index + 1),
			Name:          credential.Source,
			AuthType:      authType,
			AuthTypeName:  authTypeName,
			Identity:      credential.Source,
			Type:          credential.SourceType,
			Provider:      credential.SourceType,
			TotalRequests: credential.TotalCount,
			SuccessCount:  credential.SuccessCount,
			FailureCount:  credential.FailureCount,
			IsDeleted:     false,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	return identities
}

func usageIdentityAuthFields(sourceType, sourceKey string) (int, string) {
	if strings.HasPrefix(sourceKey, "auth:") {
		return usageIdentityAuthTypeOAuth, "oauth"
	}
	return usageIdentityAuthTypeAPIKey, "apikey"
}
