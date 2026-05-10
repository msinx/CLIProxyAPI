package adapter

import (
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/poller"
)

type StatusProvider struct {
	timezone string
	mu       sync.RWMutex
	status   poller.Status
}

func NewStatusProvider(timezone string) *StatusProvider {
	if timezone == "" || timezone == defaultTimezone {
		timezone = time.Local.String()
	}
	return &StatusProvider{
		timezone: timezone,
		status: poller.Status{
			Running:    true,
			LastStatus: "completed",
		},
	}
}

func (p *StatusProvider) Status() poller.Status {
	if p == nil {
		return poller.Status{LastStatus: "completed"}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	status := p.status
	if status.LastStatus == "" {
		status.LastStatus = "completed"
	}
	return status
}

func (p *StatusProvider) MarkSyncCompleted() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.Running = true
	p.status.SyncRunning = false
	p.status.LastRunAt = time.Now().UTC()
	p.status.LastError = ""
	p.status.LastWarning = ""
	p.status.LastStatus = "completed"
}

func (p *StatusProvider) MarkSyncWarning(err error) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.Running = true
	p.status.SyncRunning = false
	p.status.LastRunAt = time.Now().UTC()
	p.status.LastWarning = err.Error()
	p.status.LastError = ""
	p.status.LastStatus = "completed_with_warnings"
}
