package usagesqlite

import (
	"context"
	"testing"
	"time"
)

func TestDeleteBeforeRemovesOnlyOldEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	oldTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	for _, event := range []Event{
		{EventKey: "old", Timestamp: oldTime, CreatedAt: oldTime},
		{EventKey: "new", Timestamp: newTime, CreatedAt: newTime},
	} {
		if err := store.InsertEvent(ctx, event); err != nil {
			t.Fatalf("InsertEvent(%s) error = %v", event.EventKey, err)
		}
	}

	deleted, err := store.DeleteBefore(ctx, time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeleteBefore() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if page.TotalCount != 1 || page.Events[0].Timestamp != newTime {
		t.Fatalf("remaining events = %+v, total=%d", page.Events, page.TotalCount)
	}
}
