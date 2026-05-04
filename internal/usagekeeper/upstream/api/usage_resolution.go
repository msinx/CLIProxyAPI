package api

import (
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/models"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/service"
	"github.com/gin-gonic/gin"
)

func loadUsageResolutionData(
	c *gin.Context,
	authFileProvider service.AuthFileProvider,
	providerMetadataProvider service.ProviderMetadataProvider,
) ([]models.AuthFile, []models.ProviderMetadata, error) {
	authFiles := []models.AuthFile{}
	providerMetadata := []models.ProviderMetadata{}
	var err error
	if authFileProvider != nil {
		authFiles, err = authFileProvider.ListAuthFiles(c.Request.Context())
		if err != nil {
			return nil, nil, err
		}
	}
	if providerMetadataProvider != nil {
		providerMetadata, err = providerMetadataProvider.ListProviderMetadata(c.Request.Context())
		if err != nil {
			return nil, nil, err
		}
	}
	return authFiles, providerMetadata, nil
}
