package monitor

import (
	"time"

	"github.com/pkg/errors"
)

// StatusStore is an interface of storage of availability statuses.
// Standart implementation of this interface (RedisStore) uses Redis as backend,
// but some user may reimplement interface to store data in a RDB or in memory.
type StatusStore interface {
	GetStatus(t Target) (Status, bool, error)
	SetStatus(t Target, s Status, exp time.Duration) error
}

type simpleStoreRecord struct {
	Target    Target
	Status    Status
	Expirated time.Time
}

// SimpleStore is the basic implementation of StatusStore which uses a map as
// a storage backend.
type SimpleStore map[uint]simpleStoreRecord

// GetStatus returns status of a target if it's set and not expired.
func (ss SimpleStore) GetStatus(t Target) (Status, bool, error) {
	rec, ok := ss[t.ID]
	if !ok {
		return Status{}, false, nil
	}

	if rec.Expirated.Before(time.Now()) {
		return Status{}, false, nil
	}

	if rec.Target.ID != t.ID || rec.Target.URL != t.URL {
		return Status{}, false, errors.Errorf(
			"target validation failed: (actual != expected) %v != %v", rec.Target, t)
	}

	return rec.Status, true, nil
}

// SetStatus saves the status of a target and makes it expire after `exp`
// amount of time.
func (ss SimpleStore) SetStatus(t Target, s Status, exp time.Duration) error {
	rec := simpleStoreRecord{
		Target:    t,
		Status:    s,
		Expirated: time.Now().Add(exp),
	}
	ss[t.ID] = rec
	return nil
}
