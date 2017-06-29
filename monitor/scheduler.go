package monitor

import (
	"context"
	"sync"
	"time"
)

// Scheduler is an object which performs availability polling once every interval
// and writes the availability statuses into a channel.
type Scheduler struct {
	// The Poller object which will be used to do polling.
	Poller *Poller
	// Source of lists of targets.
	Targets TargetsGetter
	// Time interval between targets polling.
	Interval time.Duration
	// Maximum number of parallel http requests.
	ParallelPolls uint
	// The channel into which the scheduler will write the polling results.
	Statuses chan TargetStatus
	// The optional channel into which the scheduler should write errors of
	// TargetsGetter. If this field is set to nil, the scheduler will ignore
	// TargetsGetter errors.
	Errors chan error
}

// NewScheduler constructs a new Scheduler with given TargetsGetter and default
// fields values.
func NewScheduler(targets TargetsGetter) *Scheduler {
	return &Scheduler{
		Poller:        NewPoller(),
		Targets:       targets,
		Interval:      5 * time.Second,
		ParallelPolls: 5,
		Statuses:      make(chan TargetStatus, 1),
		Errors:        nil,
	}
}

// Run start infinite looop of targets polling in foreground. Call this method
// in a goroutine to do polling in background.
// If the context argument is not nil, the scheduler will stop the loop when
// it receives a signal from context.Done().
func (s *Scheduler) Run(context context.Context) {
	var done <-chan struct{}
	if context != nil {
		done = context.Done()
	}

	ticker := time.NewTicker(s.Interval)

	for {
		select {
		case <-ticker.C:
			s.PollTargets()
		case <-done:
			return
		}
	}
}

// PollTargets does single cycle of targets polling in foreground, which includes:
// - getting the targets list from s.Targets;
// - polling each target with s.Poller;
// - writing results into s.Statuses channel.
// If the s.Targets returns an error, which method will no perform polling
// and will attempt to send the error to s.Errors channel, if it's not nil.
func (s *Scheduler) PollTargets() {
	targets, err := s.Targets.GetTargets()
	if err != nil {
		if s.Errors != nil {
			s.Errors <- err
		}
		return
	}

	// This chanell is used to limit number of workers working at the same time.
	workersPool := make(chan struct{}, s.ParallelPolls)

	// This WaitGroup is used to wait for all workers to finish their job.
	var workersDone sync.WaitGroup

	for _, target := range targets {
		target := target
		workersDone.Add(1)
		workersPool <- struct{}{}
		go func() {
			status := s.Poller.PollService(target.URL)
			if s.Statuses != nil {
				s.Statuses <- TargetStatus{target, status}
			}
			<-workersPool
			workersDone.Done()
		}()
	}

	workersDone.Wait()
}
