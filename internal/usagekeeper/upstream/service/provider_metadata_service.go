package service

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/models"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/repository"
	"gorm.io/gorm"
)

type ProviderMetadataProvider interface {
	ListProviderMetadata(context.Context) ([]models.ProviderMetadata, error)
}

type providerMetadataService struct {
	db *gorm.DB
}

func NewProviderMetadataService(db *gorm.DB) ProviderMetadataProvider {
	return &providerMetadataService{db: db}
}

func (s *providerMetadataService) ListProviderMetadata(context.Context) ([]models.ProviderMetadata, error) {
	return repository.ListProviderMetadata(s.db)
}
