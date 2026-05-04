package poller

import (
	"errors"
	"time"
)

var ErrSyncAlreadyRunning = errors.New("sync already running")
var ErrSyncCompletedWithWarnings = errors.New("sync completed with warnings")

type Status struct {
	Running     bool
	LastRunAt   time.Time
	LastError   string
	LastWarning string
	LastStatus  string
	SyncRunning bool
}
