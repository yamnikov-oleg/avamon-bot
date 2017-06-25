package monitor

type Target struct {
	Title string
	URL   string
}

type TargetsGetter interface {
	GetTargets() ([]Target, error)
}
