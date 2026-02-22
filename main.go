package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const banner = `
  _  _
 (o)(o)   parrot v2.4 — HTTP echo & observability server
  (  )
  /\/\
`

func main() {
	basePort := flag.Int("base-port", 8080, "starting port number")
	count := flag.Int("count", 1, "number of parrot instances to launch")
	portsFlag := flag.String("ports", "", "comma-separated list of ports (overrides -base-port and -count)")
	historySize := flag.Int("history", 100, "number of requests to keep in history per instance")
	delay := flag.Duration("delay", 0, "artificial response delay (e.g. 200ms, 1s)")
	statusCode := flag.Int("status", 200, "HTTP status code to respond with")
	jsonLogs := flag.Bool("log-json", false, "emit structured JSON logs")
	dashboardPort := flag.Int("dashboard", 9999, "port for the web dashboard (0 to disable)")

	// TLS flags
	tlsEnabled := flag.Bool("tls", true, "enable HTTPS alongside HTTP on each instance")
	tlsOffset := flag.Int("tls-offset", 1000, "TLS port = HTTP port + offset (e.g. 8080 → 9080)")
	tlsCertFile := flag.String("tls-cert", "", "path to TLS certificate PEM (auto-generated if empty)")
	tlsKeyFile := flag.String("tls-key", "", "path to TLS key PEM (auto-generated if empty)")

	// HAR export flag
	exportOnShutdown := flag.String("export-on-shutdown", "", "write a HAR file to this path on exit (e.g. ./session.har)")

	// Replay flag
	replayTimeout := flag.Duration("replay-timeout", 10*time.Second, "timeout for webhook replay requests (e.g. 5s, 30s)")

	// Rate limit flag
	rateLimit := flag.Float64("rate-limit", 0, "max requests/sec per instance, 0 = unlimited (e.g. 10, 0.5)")

	flag.Parse()

	var ports []int
	if *portsFlag != "" {
		for _, p := range strings.Split(*portsFlag, ",") {
			p = strings.TrimSpace(p)
			n, err := strconv.Atoi(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid port %q: %v\n", p, err)
				os.Exit(1)
			}
			ports = append(ports, n)
		}
	} else {
		for i := 0; i < *count; i++ {
			ports = append(ports, *basePort+i)
		}
	}

	fmt.Print(banner)

	// Collect non-default flags for display in dashboards
	var activeFlags []string
	if *portsFlag != "" {
		activeFlags = append(activeFlags, "--ports="+*portsFlag)
	} else if *basePort != 8080 {
		activeFlags = append(activeFlags, fmt.Sprintf("--base-port=%d", *basePort))
	}
	if *count != 1 {
		activeFlags = append(activeFlags, fmt.Sprintf("--count=%d", *count))
	}
	if *historySize != 100 {
		activeFlags = append(activeFlags, fmt.Sprintf("--history=%d", *historySize))
	}
	if *delay != 0 {
		activeFlags = append(activeFlags, "--delay="+delay.String())
	}
	if *statusCode != 200 {
		activeFlags = append(activeFlags, fmt.Sprintf("--status=%d", *statusCode))
	}
	if *jsonLogs {
		activeFlags = append(activeFlags, "--log-json")
	}
	if !*tlsEnabled {
		activeFlags = append(activeFlags, "--tls=false")
	}
	if *tlsEnabled && *tlsOffset != 1000 {
		activeFlags = append(activeFlags, fmt.Sprintf("--tls-offset=%d", *tlsOffset))
	}
	if *tlsCertFile != "" {
		activeFlags = append(activeFlags, "--tls-cert="+*tlsCertFile)
	}
	if *exportOnShutdown != "" {
		activeFlags = append(activeFlags, "--export-on-shutdown="+*exportOnShutdown)
	}
	if *replayTimeout != 10*time.Second {
		activeFlags = append(activeFlags, "--replay-timeout="+replayTimeout.String())
	}
	if *rateLimit != 0 {
		activeFlags = append(activeFlags, fmt.Sprintf("--rate-limit=%.4g", *rateLimit))
	}
	if *dashboardPort != 9999 {
		activeFlags = append(activeFlags, fmt.Sprintf("--dashboard=%d", *dashboardPort))
	}

	store := NewStore(*historySize)
	dash := NewDashboard(store, ports, activeFlags)

	cfg := Config{
		Delay:         *delay,
		StatusCode:    *statusCode,
		JSONLogs:      *jsonLogs,
		ReplayTimeout: *replayTimeout,
		RateLimit:     *rateLimit,
	}

	// Build TLS config once and share across all instances
	var tlsCfg *tls.Config
	if *tlsEnabled {
		var err error
		tlsCfg, err = buildTLSConfig(*tlsCertFile, *tlsKeyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TLS setup failed: %v\n", err)
			os.Exit(1)
		}
	}

	var wg sync.WaitGroup
	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			startParrot(p, store, cfg, ports)
		}(port)

		if *tlsEnabled {
			tlsPort := port + *tlsOffset
			wg.Add(1)
			go func(p, tp int) {
				defer wg.Done()
				startTLSParrot(p, tp, tlsCfg, store, cfg, ports)
			}(port, tlsPort)
		}
	}

	if *dashboardPort > 0 {
		go dash.Serve(*dashboardPort)
		go func() {
			for {
				dash.Render()
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// HAR export on shutdown
	if *exportOnShutdown != "" {
		fmt.Printf("\n  writing HAR to %s ... ", *exportOnShutdown)
		if err := exportHARToFile(*exportOnShutdown, store, ports); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			fmt.Println("done ✓")
		}
	}

	fmt.Println("\nall parrots have flown away. goodbye!")
	os.Exit(0)
}
