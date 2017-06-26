package monitor

type Target struct {
	ID    uint
	Title string
	URL   string
}

type TargetsGetter interface {
	GetTargets() ([]Target, error)
}
