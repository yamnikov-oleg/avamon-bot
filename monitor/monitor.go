package monitor

import (
	"context"
	"time"
)

type Monitor struct {
	Scheduler      *Scheduler
	StatusStore    StatusStore
	ExpirationTime time.Duration
	Updates        chan TargetStatus

	errors chan error
}

func New(targets TargetsGetter) *Monitor {
	return &Monitor{
		Scheduler:      NewScheduler(targets),
		StatusStore:    SimpleStore{},
		ExpirationTime: 30 * time.Second,
		Updates:        make(chan TargetStatus),

		errors: nil,
	}
}

func (m *Monitor) Errors() chan error {
	if m.errors == nil {
		m.errors = make(chan error, 10)
		go func() {
			for err := range m.Scheduler.Errors() {
				m.errors <- err
			}
		}()
	}
	return m.errors
}

func (m *Monitor) Run(ctx context.Context) {
	go m.Scheduler.Run(ctx)

	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}

	var ts TargetStatus
	for {
		select {
		case ts = <-m.Scheduler.Statuses:
			m.applyNewStatus(ts.Target, ts.Status)
		case <-done:
			return
		}
	}
}

func (m *Monitor) applyNewStatus(t Target, s Status) {
	oldStatus, ok, err := m.StatusStore.GetStatus(t)
	if err != nil && m.errors != nil {
		m.errors <- err
		// Just to guarantee that ok is false when error has occured.
		ok = false
	}

	if !ok || oldStatus.Type != s.Type {
		m.Updates <- TargetStatus{t, s}
	}
	m.StatusStore.SetStatus(t, s, m.ExpirationTime)
}
