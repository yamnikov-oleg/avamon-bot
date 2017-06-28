package monitor

import "fmt"

// Target is a URL, which has to be polled for availability.
type Target struct {
	// Unique identifier of this target. Targets' IDs cannot intercept. Target's
	// ID must be constant between GetTargets() calls.
	ID uint
	// User-supplied target title, used purely for display.
	Title string
	// The HTTP URL to poll.
	URL string
}

func (t Target) String() string {
	return fmt.Sprintf("Target %v { %q, %q }", t.ID, t.Title, t.URL)
}

// TargetStatus simply connects target and its status in one structure.
type TargetStatus struct {
	Target Target
	Status Status
}

func (ts TargetStatus) String() string {
	return fmt.Sprintf("%v : %v", ts.Target, ts.Status)
}

// TargetsGetter is an interface of targets source. Monitor uses it to retrieve
// list targets on every polling iteration. External frontend may implement
// this interface to store targets in a DB or in a configuration file.
type TargetsGetter interface {
	GetTargets() ([]Target, error)
}
