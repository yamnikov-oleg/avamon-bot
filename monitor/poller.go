package monitor

import (
	"net/http"
	"strings"
	"time"
)

// Poller makes HTTP request to some URL to return its availability status.
type Poller struct {
	// Timeout of network request.
	Timeout time.Duration
	// How many times should poller repeat the request if all previous ones
	// ended in timeout.
	TimeoutRetries int
}

// NewPoller constructs a new Poller with default fields.
func NewPoller() *Poller {
	return &Poller{
		Timeout:        3 * time.Second,
		TimeoutRetries: 2,
	}
}

// PollService makes HTTP GET request to the URL and returns its availability status.
// If there was an error during request, the returned Status structure will
// contain information about the error.
func (p *Poller) PollService(url string) Status {
	retries := p.TimeoutRetries
	for {
		stat := p.pollServiceOnce(url)
		if stat.Type != StatusTimeout {
			return stat
		}
		if retries <= 0 {
			return stat
		}
		retries--
	}
}

func (p *Poller) pollServiceOnce(url string) Status {
	client := &http.Client{}
	client.Timeout = p.Timeout

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

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
