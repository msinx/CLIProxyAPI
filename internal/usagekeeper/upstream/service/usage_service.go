package service

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/cpa"
)

type UsageProvider interface {
	GetUsageWithFilter(context.Context, UsageFilter) (*cpa.StatisticsSnapshot, error)
	GetUsageOverview(context.Context, UsageFilter) (*UsageOverviewSnapshot, error)
	ListUsageEvents(context.Context, UsageFilter) (*UsageEventsPage, error)
	ListUsageEventFilterOptions(context.Context, UsageFilter) (*UsageEventFilterOptions, error)
	ListUsageCredentialStats(context.Context, UsageFilter) ([]UsageCredentialStat, error)
	GetUsageAnalysis(context.Context, UsageFilter) (*UsageAnalysisSnapshot, error)
}
