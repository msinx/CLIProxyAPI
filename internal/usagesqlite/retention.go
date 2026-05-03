package usagesqlite

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

func StartRetentionCleaner(ctx context.Context, store *Store, retentionDays int, interval time.Duration) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil || retentionDays <= 0 {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	clean := func() {
		cutoff := time.Now().AddDate(0, 0, -retentionDays)
		deleted, err := store.DeleteBefore(context.Background(), cutoff)
		if err != nil {
			log.WithError(err).Warn("failed to clean old sqlite usage events")
			return
		}
		if deleted > 0 {
			log.WithField("deleted", deleted).Info("cleaned old sqlite usage events")
		}
	}
	go func() {
		clean()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				clean()
			}
		}
	}()
}
