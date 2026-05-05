package api

import (
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/models"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/service"
)

func loadUsageResolutionData(
	c *gin.Context,
	usageIdentityProvider service.UsageIdentityProvider,
) ([]models.UsageIdentity, error) {
	if usageIdentityProvider == nil {
		return []models.UsageIdentity{}, nil
	}
	return usageIdentityProvider.ListUsageIdentities(c.Request.Context())
}
