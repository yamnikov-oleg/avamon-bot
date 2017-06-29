package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/yamnikov-oleg/avamon-bot/monitor"
)

func main() {
	timeout := flag.Duration("timeout", 3*time.Second, "Timeout for network request")

	flag.Parse()

	poller := monitor.NewPoller()
	poller.Timeout = *timeout

	urls := flag.Args()
	for _, url := range urls {
		fmt.Printf("Requesting %q\n", url)
		status := poller.PollService(url)
		fmt.Println(status.ExpandedString())
	}
}
