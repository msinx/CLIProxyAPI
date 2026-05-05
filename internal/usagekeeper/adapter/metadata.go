package adapter

import (
	"context"

	"gorm.io/gorm"
)

type MetadataRefresher interface {
	RefreshUsageKeeperMetadata(context.Context, *gorm.DB) error
}
