package monitor

import "time"

// StatusStore is an interface of storage of availability statuses.
// Standart implementation of this interface (RedisStore) uses Redis as backend,
// but some user may reimplement interface to store data in a RDB or in memory.
type StatusStore interface {
	GetStatus(t Target) (Status, bool, error)
	SetStatus(t Target, s Status, exp time.Duration) error
}
