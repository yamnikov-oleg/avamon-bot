package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yamnikov-oleg/avamon-bot/monitor"
)

func main() {
	interval := flag.Duration("int", 5*time.Second, "Interval between targets poll")
	maxParallel := flag.Uint("par", 3, "Maximum parallel network requests")

	redis := flag.Bool("redis", false, "Store statuses to redis db")
	host := flag.String("h", "localhost", "Host of redis server")
	port := flag.Uint("p", 6379, "Port of redis server")
	pwd := flag.String("pwd", "", "Password of redis server")
	db := flag.Int("db", 0, "Database to select")

	flag.Parse()

	targets := monitor.NewTargetsSliceFromUrls(flag.Args())

	mon := monitor.New(targets)
	mon.Scheduler.Interval = *interval
	mon.Scheduler.ParallelPolls = *maxParallel

	if *redis {
		ropts := monitor.RedisOptions{
			Host:     *host,
			Port:     *port,
			Password: *pwd,
			DB:       *db,
		}
		rs := monitor.NewRedisStore(ropts)
		if err := rs.Ping(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		mon.StatusStore = rs
	}

	go func() {
		for upd := range mon.Updates {
			if upd.Status.Type == monitor.StatusOK {
				fmt.Printf("%v is UP:\n", upd.Target)
			} else {
				fmt.Printf("%v is DOWN:\n", upd.Target)
			}
			fmt.Println(upd.Status.ExpandedString())
			fmt.Println()
		}
	}()

	go func() {
		for err := range mon.Errors() {
			fmt.Println(err)
		}
	}()

	mon.Run(nil)
}
