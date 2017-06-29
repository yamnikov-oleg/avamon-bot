package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/yamnikov-oleg/avamon-bot/monitor"
)

type quotedSplitter struct {
	sections []string
	buffer   []rune
	quote    *rune
}

func (s *quotedSplitter) next(rn rune) {
	switch {
	case s.quote != nil && rn == *s.quote:
		s.quote = nil
	case s.quote == nil && (rn == '\'' || rn == '"'):
		s.quote = &rn
	case s.quote == nil && rn == ' ':
		if s.buffer != nil {
			s.sections = append(s.sections, string(s.buffer))
			s.buffer = nil
		}
	default:
		s.buffer = append(s.buffer, rn)
	}
}

func (s *quotedSplitter) finish() []string {
	if s.buffer != nil {
		s.sections = append(s.sections, string(s.buffer))
	}
	return s.sections
}

func splitWithQuotes(s string) []string {
	s = strings.TrimSpace(s)
	qs := quotedSplitter{}
	rns := []rune(s)
	for _, rn := range rns {
		qs.next(rn)
	}
	return qs.finish()
}

func parseTarget(args []string) (monitor.Target, error) {
	if len(args) != 3 {
		return monitor.Target{}, errors.New("Target parsing requires 3 arguments")
	}

	var target monitor.Target

	_, err := fmt.Sscan(args[0], &target.ID)
	if err != nil {
		return monitor.Target{}, errors.Wrap(err, "could not parse target id")
	}

	target.Title = args[1]
	target.URL = args[2]

	return target, nil
}

func parseStatus(args []string) (monitor.Status, error) {
	if len(args) != 4 {
		return monitor.Status{}, errors.New("Status parsing requires 4 arguments")
	}

	var status monitor.Status

	stype, ok := monitor.ScanStatusTypeSoft(args[0])
	if !ok {
		return monitor.Status{}, fmt.Errorf("Unknown status type: %v", args[0])
	}
	status.Type = stype

	status.Err = errors.New(args[1])

	rt, err := time.ParseDuration(args[2])
	if err != nil {
		return monitor.Status{}, errors.Wrap(err, "could not parse response time")
	}
	status.ResponseTime = rt

	_, err = fmt.Sscan(args[3], &status.HTTPStatusCode)
	if err != nil {
		return monitor.Status{}, errors.Wrap(err, "could not parse http status code")
	}

	return status, nil
}

func main() {
	host := flag.String("h", "localhost", "Host of redis server")
	port := flag.Uint("p", 6379, "Port of redis server")
	pwd := flag.String("pwd", "", "Password of redis server")
	db := flag.Int("db", 0, "Database to select")

	flag.Parse()

	// targets := monitor.TargetsSlice{}
	rs := monitor.NewRedisStore(monitor.RedisOptions{
		Host:     *host,
		Port:     *port,
		Password: *pwd,
		DB:       *db,
	})

	if err := rs.Ping(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Connected to redis.")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		cmdline := splitWithQuotes(line)
		if len(cmdline) == 0 {
			continue
		}

		cmd := cmdline[0]
		args := cmdline[1:]
		switch cmd {
		case "help":
			fmt.Println("Usage:")
			fmt.Println("  help      - Print help.")
			fmt.Println("  ping      - Ping redis server.")
			fmt.Println("  exit      - Exit this CLI.")
			fmt.Println("  scan      - Scan redis database for statuses.")
			fmt.Println("  getstatus - Get status of a target.")
			fmt.Println("              Arguments:")
			fmt.Println("                <tid>   - Target ID")
			fmt.Println("                <title> - Target Title")
			fmt.Println("                <url>   - Target URL")
			fmt.Println("  setstatus - Set status of a target.")
			fmt.Println("              Arguments:")
			fmt.Println("                <tid>   - Target ID")
			fmt.Println("                <title> - Target Title")
			fmt.Println("                <url>   - Target URL")
			fmt.Println("                <type>  - Status type (e.g. 'ok' or 'generic error')")
			fmt.Println("                <err>   - Error message")
			fmt.Println("                <time>  - Response time (e.g. 200ms)")
			fmt.Println("                <http>  - HTTP status code (e.g. 200) or 0")
			fmt.Println("                <exp>   - Expiration time (e.g. 5s)")

		case "exit":
			fmt.Println("Bye")
			os.Exit(0)

		case "ping":
			err := rs.Ping()
			if err != nil {
				fmt.Println(err)
				continue
			}
			fmt.Println("Pong")

		case "scan":
			tarstats, err := rs.Scan()
			if err != nil {
				fmt.Println(err)
				continue
			}

			fmt.Printf("Found %v statuses\n", len(tarstats))
			for _, ts := range tarstats {
				fmt.Println(ts)
			}

		case "getstatus":
			if len(args) != 3 {
				fmt.Println("This command requires exactly 3 arguments:")
				fmt.Println("getstatus <tid> <title> <url>")
				continue
			}

			target, err := parseTarget(args[0:3])
			if err != nil {
				fmt.Println(err)
				continue
			}

			status, ok, err := rs.GetStatus(target)
			if err != nil {
				fmt.Println(err)
				continue
			}
			if !ok {
				fmt.Println("N/A")
				continue
			}
			fmt.Println(status.ExpandedString())

		case "setstatus":
			if len(args) != 8 {
				fmt.Println("This command requires exactly 8 arguments:")
				fmt.Println("setstatus <tid> <title> <url> <type> <err> <time> <http> <exp>")
				continue
			}

			target, err := parseTarget(args[0:3])
			if err != nil {
				fmt.Println(err)
				continue
			}
			status, err := parseStatus(args[3:7])
			if err != nil {
				fmt.Println(err)
				continue
			}

			exp, err := time.ParseDuration(args[7])
			if err != nil {
				fmt.Printf("Could not parse expiration time: %v\n", err)
				continue
			}

			err = rs.SetStatus(target, status, exp)
			if err != nil {
				fmt.Println(err)
				continue
			}

		default:
			fmt.Println("Unknown command. Print 'help' for list of available commands.")
		}
	}
}
