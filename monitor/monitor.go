package monitor

import (
	"context"
	"time"
)

// Monitor is a wrapper around Scheduler, which saves statuses into a StatusStore
// and sends status changes into Updates channel.
type Monitor struct {
	// The scheduler object used to perform regular availability checks.
	Scheduler *Scheduler
	// Interface to the status storage. By default Monitor will use in-memory
	// internal storage (SimpleStore), but you might want to use some other
	// backend like Redis with RedisStore.
	StatusStore StatusStore
	// Expiration time for status values in the store.
	ExpirationTime time.Duration
	// If this flag describes how the monitor reacts to new statuses.
	// A new status is a status of a target, which has not been checked out into
	// status store yet.
	// If this flag is set to false, monitor will NOT send new OK statuses
	// into Updates chanel, but will send new error statuses.
	// If this flag is set to true, monitor will send any new status to Updates.
	NotifyFirstOK bool
	// Channel by which the monitor will send all status changes.
	// Whenever a type of a target's status (Status.Type) changes, monitor will
	// send the target and its _new_ status into this channel.
	// If the caller ignores this channel and does not read values from it,
	// the monitor (and its scheduler) will clobber and stop doing status checks.
	Updates chan TargetStatus

	errors chan error
}

// New creates a Monitor with given targets getter and default field values.
func New(targets TargetsGetter) *Monitor {
	return &Monitor{
		Scheduler:      NewScheduler(targets),
		StatusStore:    SimpleStore{},
		ExpirationTime: 30 * time.Second,
		Updates:        make(chan TargetStatus),

		errors: nil,
	}
}

// Errors returns a channel through which the monitor will send all errors,
// appearing in the process of its work.
// The caller may ignore this channel, its optional.
// It may be useful to log the monitor errors.
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

// Run starts the monitoring in the foreground. Call this method in another
// goroutine if you want to do background monitoring.
// If `ctx` is not nil, the monitor will listen to ctx.Done() and stop monitoring
// when it recieves the signal.
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

func (m *Monitor) isStatusNew(oldStatus Status, oldOk bool, newStatus Status) bool {
	if oldOk {
		return oldStatus.Type != newStatus.Type
	}

	// oldOk = false
	if newStatus.Type != StatusOK {
		return true
	}
	if newStatus.Type == StatusOK && m.NotifyFirstOK {
		return true
	}
	return false
}

func (m *Monitor) applyNewStatus(t Target, s Status) {
	oldStatus, ok, err := m.StatusStore.GetStatus(t)
	if err != nil && m.errors != nil {
		m.errors <- err
		// Just to guarantee that ok is false when error has occured.
		ok = false
	}

	if m.isStatusNew(oldStatus, ok, s) {
		m.Updates <- TargetStatus{t, s}
	}
	m.StatusStore.SetStatus(t, s, m.ExpirationTime)
}
