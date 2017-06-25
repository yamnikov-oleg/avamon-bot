package monitor

import (
	"net/http"
	"time"
)

type Poller struct {
	Timeout time.Duration
}

func NewPoller() *Poller {
	return &Poller{
		Timeout: 3 * time.Second,
	}
}

func (p *Poller) PollService(url string) *Status {
	client := &http.Client{}
	client.Timeout = p.Timeout

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return newURLParsingErrorStatus(err, 0)
	}

	reqStart := time.Now()
	resp, err := client.Do(req)
	reqEnd := time.Now()
	dur := reqEnd.Sub(reqStart)

	if err != nil {
		return netErrToStatus(err, dur)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newHTTPErrorStatus(resp, dur)
	}

	return newSuccessStatus(resp, dur)
}
