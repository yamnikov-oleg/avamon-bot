package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yamnikov-oleg/avamon-bot/monitor"
)

func main() {
	interval := flag.Duration("int", 5*time.Second, "interval between targets poll")
	maxParallel := flag.Uint("par", 3, "maximum parallel network requests")

	flag.Parse()

	urls := flag.Args()

	if len(urls) == 0 {
		fmt.Println("Please, specify at least one url")
		os.Exit(1)
	}

	targets := monitor.NewTargetsSliceFromUrls(urls)
	scheduler := monitor.NewScheduler(targets)
	scheduler.Interval = *interval
	scheduler.ParallelPolls = *maxParallel

	go scheduler.Run(nil)

	for ts := range scheduler.Statuses {
		fmt.Println(ts)
	}
}
