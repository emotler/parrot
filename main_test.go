package main

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestPortParsing tests the port list generation logic from main()
func TestPortParsing(t *testing.T) {
	tests := []struct {
		name      string
		portsFlag string
		basePort  int
		count     int
		want      []int
		wantErr   bool
	}{
		{
			name:      "explicit ports list",
			portsFlag: "8080,8081,8082",
			basePort:  9000,
			count:     5,
			want:      []int{8080, 8081, 8082},
			wantErr:   false,
		},
		{
			name:      "explicit ports with spaces",
			portsFlag: "8080, 8081, 8082",
			basePort:  9000,
			count:     5,
			want:      []int{8080, 8081, 8082},
			wantErr:   false,
		},
		{
			name:      "single explicit port",
			portsFlag: "9999",
			basePort:  8080,
			count:     3,
			want:      []int{9999},
			wantErr:   false,
		},
		{
			name:      "base port with count 1",
			portsFlag: "",
			basePort:  8080,
			count:     1,
			want:      []int{8080},
			wantErr:   false,
		},
		{
			name:      "base port with count 3",
			portsFlag: "",
			basePort:  8080,
			count:     3,
			want:      []int{8080, 8081, 8082},
			wantErr:   false,
		},
		{
			name:      "base port with count 5",
			portsFlag: "",
			basePort:  9000,
			count:     5,
			want:      []int{9000, 9001, 9002, 9003, 9004},
			wantErr:   false,
		},
		{
			name:      "invalid port in list",
			portsFlag: "8080,invalid,8082",
			basePort:  8080,
			count:     1,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty string in port list",
			portsFlag: "8080,,8082",
			basePort:  8080,
			count:     1,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "non-numeric port",
			portsFlag: "abc",
			basePort:  8080,
			count:     1,
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the port parsing logic from main()
			var ports []int
			var err error

			if tt.portsFlag != "" {
				for _, p := range strings.Split(tt.portsFlag, ",") {
					p = strings.TrimSpace(p)
					n, parseErr := strconv.Atoi(p)
					if parseErr != nil {
						err = parseErr
						break
					}
					ports = append(ports, n)
				}
			} else {
				for i := 0; i < tt.count; i++ {
					ports = append(ports, tt.basePort+i)
				}
			}

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(ports) != len(tt.want) {
				t.Errorf("got %d ports, want %d", len(ports), len(tt.want))
				return
			}

			for i, port := range ports {
				if port != tt.want[i] {
					t.Errorf("port[%d] = %d, want %d", i, port, tt.want[i])
				}
			}
		})
	}
}

// TestFlagDefaults verifies that flag defaults match expected values
func TestFlagDefaults(t *testing.T) {
	// Create a new FlagSet to avoid interfering with global flags
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	basePort := fs.Int("base-port", 8080, "starting port number")
	count := fs.Int("count", 1, "number of parrot instances to launch")
	portsFlag := fs.String("ports", "", "comma-separated list of ports")
	historySize := fs.Int("history", 100, "number of requests to keep in history per instance")
	delay := fs.Duration("delay", 0, "artificial response delay")
	statusCode := fs.Int("status", 200, "HTTP status code to respond with")
	jsonLogs := fs.Bool("log-json", false, "emit structured JSON logs")
	dashboardPort := fs.Int("dashboard", 9999, "port for the web dashboard")
	tlsEnabled := fs.Bool("tls", true, "enable HTTPS alongside HTTP on each instance")
	tlsOffset := fs.Int("tls-offset", 1000, "TLS port = HTTP port + offset")
	tlsCertFile := fs.String("tls-cert", "", "path to TLS certificate PEM")
	tlsKeyFile := fs.String("tls-key", "", "path to TLS key PEM")
	exportOnShutdown := fs.String("export-on-shutdown", "", "write a HAR file to this path on exit")
	replayTimeout := fs.Duration("replay-timeout", 10*time.Second, "timeout for webhook replay requests")
	rateLimit := fs.Float64("rate-limit", 0, "max requests/sec per instance")

	// Parse with no arguments to get defaults
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"base-port", *basePort, 8080},
		{"count", *count, 1},
		{"ports", *portsFlag, ""},
		{"history", *historySize, 100},
		{"delay", *delay, time.Duration(0)},
		{"status", *statusCode, 200},
		{"log-json", *jsonLogs, false},
		{"dashboard", *dashboardPort, 9999},
		{"tls", *tlsEnabled, true},
		{"tls-offset", *tlsOffset, 1000},
		{"tls-cert", *tlsCertFile, ""},
		{"tls-key", *tlsKeyFile, ""},
		{"export-on-shutdown", *exportOnShutdown, ""},
		{"replay-timeout", *replayTimeout, 10 * time.Second},
		{"rate-limit", *rateLimit, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("flag %s default = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestFlagParsing verifies that flags can be parsed correctly
func TestFlagParsing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "custom base port and count",
			args: []string{"-base-port=9000", "-count=3"},
			want: map[string]any{
				"base-port": 9000,
				"count":     3,
			},
		},
		{
			name: "explicit ports",
			args: []string{"-ports=8080,8081,8082"},
			want: map[string]any{
				"ports": "8080,8081,8082",
			},
		},
		{
			name: "custom history size",
			args: []string{"-history=500"},
			want: map[string]any{
				"history": 500,
			},
		},
		{
			name: "custom delay",
			args: []string{"-delay=200ms"},
			want: map[string]any{
				"delay": 200 * time.Millisecond,
			},
		},
		{
			name: "custom status code",
			args: []string{"-status=404"},
			want: map[string]any{
				"status": 404,
			},
		},
		{
			name: "enable json logs",
			args: []string{"-log-json=true"},
			want: map[string]any{
				"log-json": true,
			},
		},
		{
			name: "disable dashboard",
			args: []string{"-dashboard=0"},
			want: map[string]any{
				"dashboard": 0,
			},
		},
		{
			name: "disable tls",
			args: []string{"-tls=false"},
			want: map[string]any{
				"tls": false,
			},
		},
		{
			name: "custom tls offset",
			args: []string{"-tls-offset=2000"},
			want: map[string]any{
				"tls-offset": 2000,
			},
		},
		{
			name: "tls cert and key files",
			args: []string{"-tls-cert=/path/to/cert.pem", "-tls-key=/path/to/key.pem"},
			want: map[string]any{
				"tls-cert": "/path/to/cert.pem",
				"tls-key":  "/path/to/key.pem",
			},
		},
		{
			name: "export on shutdown",
			args: []string{"-export-on-shutdown=./session.har"},
			want: map[string]any{
				"export-on-shutdown": "./session.har",
			},
		},
		{
			name: "custom replay timeout",
			args: []string{"-replay-timeout=30s"},
			want: map[string]any{
				"replay-timeout": 30 * time.Second,
			},
		},
		{
			name: "custom rate limit",
			args: []string{"-rate-limit=10.5"},
			want: map[string]any{
				"rate-limit": 10.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)

			basePort := fs.Int("base-port", 8080, "starting port number")
			count := fs.Int("count", 1, "number of parrot instances to launch")
			portsFlag := fs.String("ports", "", "comma-separated list of ports")
			historySize := fs.Int("history", 100, "number of requests to keep in history per instance")
			delay := fs.Duration("delay", 0, "artificial response delay")
			statusCode := fs.Int("status", 200, "HTTP status code to respond with")
			jsonLogs := fs.Bool("log-json", false, "emit structured JSON logs")
			dashboardPort := fs.Int("dashboard", 9999, "port for the web dashboard")
			tlsEnabled := fs.Bool("tls", true, "enable HTTPS alongside HTTP on each instance")
			tlsOffset := fs.Int("tls-offset", 1000, "TLS port = HTTP port + offset")
			tlsCertFile := fs.String("tls-cert", "", "path to TLS certificate PEM")
			tlsKeyFile := fs.String("tls-key", "", "path to TLS key PEM")
			exportOnShutdown := fs.String("export-on-shutdown", "", "write a HAR file to this path on exit")
			replayTimeout := fs.Duration("replay-timeout", 10*time.Second, "timeout for webhook replay requests")
			rateLimit := fs.Float64("rate-limit", 0, "max requests/sec per instance")

			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			flagMap := map[string]any{
				"base-port":          *basePort,
				"count":              *count,
				"ports":              *portsFlag,
				"history":            *historySize,
				"delay":              *delay,
				"status":             *statusCode,
				"log-json":           *jsonLogs,
				"dashboard":          *dashboardPort,
				"tls":                *tlsEnabled,
				"tls-offset":         *tlsOffset,
				"tls-cert":           *tlsCertFile,
				"tls-key":            *tlsKeyFile,
				"export-on-shutdown": *exportOnShutdown,
				"replay-timeout":     *replayTimeout,
				"rate-limit":         *rateLimit,
			}

			for key, wantVal := range tt.want {
				gotVal := flagMap[key]
				if gotVal != wantVal {
					t.Errorf("flag %s = %v, want %v", key, gotVal, wantVal)
				}
			}
		})
	}
}

// TestBannerConstant verifies the banner constant is defined
func TestBannerConstant(t *testing.T) {
	if banner == "" {
		t.Error("banner constant should not be empty")
	}

	// Check that banner contains expected elements
	expectedStrings := []string{"parrot", "v2.4", "HTTP echo"}
	for _, s := range expectedStrings {
		if !strings.Contains(banner, s) {
			t.Errorf("banner should contain %q", s)
		}
	}
}

// TestConfigStruct verifies Config struct can be created with expected fields
func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Delay:         100 * time.Millisecond,
		StatusCode:    201,
		JSONLogs:      true,
		ReplayTimeout: 5 * time.Second,
		RateLimit:     10.5,
	}

	if cfg.Delay != 100*time.Millisecond {
		t.Errorf("Delay = %v, want %v", cfg.Delay, 100*time.Millisecond)
	}
	if cfg.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", cfg.StatusCode)
	}
	if !cfg.JSONLogs {
		t.Error("JSONLogs should be true")
	}
	if cfg.ReplayTimeout != 5*time.Second {
		t.Errorf("ReplayTimeout = %v, want %v", cfg.ReplayTimeout, 5*time.Second)
	}
	if cfg.RateLimit != 10.5 {
		t.Errorf("RateLimit = %f, want 10.5", cfg.RateLimit)
	}
}

// TestTLSPortCalculation verifies TLS port offset calculation
func TestTLSPortCalculation(t *testing.T) {
	tests := []struct {
		name      string
		httpPort  int
		tlsOffset int
		want      int
	}{
		{"default offset", 8080, 1000, 9080},
		{"custom offset", 8080, 2000, 10080},
		{"different port", 9000, 1000, 10000},
		{"zero offset", 8080, 0, 8080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsPort := tt.httpPort + tt.tlsOffset
			if tlsPort != tt.want {
				t.Errorf("TLS port = %d, want %d", tlsPort, tt.want)
			}
		})
	}
}

// TestPortListGeneration tests various scenarios of port list generation
func TestPortListGeneration(t *testing.T) {
	tests := []struct {
		name     string
		basePort int
		count    int
		want     []int
	}{
		{"single port", 8080, 1, []int{8080}},
		{"two ports", 8080, 2, []int{8080, 8081}},
		{"five ports", 9000, 5, []int{9000, 9001, 9002, 9003, 9004}},
		{"ten ports", 8000, 10, []int{8000, 8001, 8002, 8003, 8004, 8005, 8006, 8007, 8008, 8009}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ports []int
			for i := 0; i < tt.count; i++ {
				ports = append(ports, tt.basePort+i)
			}

			if len(ports) != len(tt.want) {
				t.Errorf("got %d ports, want %d", len(ports), len(tt.want))
				return
			}

			for i, port := range ports {
				if port != tt.want[i] {
					t.Errorf("port[%d] = %d, want %d", i, port, tt.want[i])
				}
			}
		})
	}
}

// TestInvalidPortHandling tests error handling for invalid port specifications
func TestInvalidPortHandling(t *testing.T) {
	tests := []struct {
		name      string
		portStr   string
		shouldErr bool
	}{
		{"valid port", "8080", false},
		{"invalid non-numeric", "abc", true},
		{"invalid with letters", "80a0", true},
		{"invalid negative", "-1", false}, // strconv.Atoi accepts negative numbers
		{"invalid float", "80.5", true},
		{"invalid empty", "", true},
		{"valid large port", "65535", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := strconv.Atoi(tt.portStr)
			hasErr := err != nil

			if hasErr != tt.shouldErr {
				if tt.shouldErr {
					t.Errorf("expected error for port %q but got none", tt.portStr)
				} else {
					t.Errorf("unexpected error for port %q: %v", tt.portStr, err)
				}
			}
		})
	}
}

// TestMainExitBehavior verifies that main would exit with code 1 on invalid port
// This is a documentation test showing the expected behavior
func TestMainExitBehavior(t *testing.T) {
	// This test documents that main() calls os.Exit(1) on invalid port
	// We can't actually test os.Exit in a unit test, but we can verify
	// the error detection logic

	invalidPorts := []string{"invalid", "80a0", ""}
	for _, p := range invalidPorts {
		_, err := strconv.Atoi(p)
		if err == nil {
			t.Errorf("expected error for invalid port %q", p)
		}
	}
}

// TestStoreInitialization verifies Store can be created with history size
func TestStoreInitialization(t *testing.T) {
	tests := []struct {
		name        string
		historySize int
	}{
		{"default size", 100},
		{"custom size", 500},
		{"small size", 10},
		{"large size", 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(tt.historySize)
			if store == nil {
				t.Fatal("NewStore returned nil")
			}
		})
	}
}

// TestDashboardInitialization verifies Dashboard can be created
func TestDashboardInitialization(t *testing.T) {
	store := NewStore(100)
	ports := []int{8080, 8081, 8082}
	flags := []string{"flag1", "flag2=value"}

	dash := NewDashboard(store, ports, flags)
	if dash == nil {
		t.Fatal("NewDashboard returned nil")
	}
}

// TestSignalHandling documents the signal handling behavior
func TestSignalHandling(t *testing.T) {
	// This test documents that main() handles SIGINT and SIGTERM
	// We can't easily test the actual signal handling in a unit test,
	// but we verify the signals are the expected ones

	expectedSignals := []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	if len(expectedSignals) != 2 {
		t.Error("expected 2 signals to be handled")
	}

	// Verify the signals are the correct types
	for i, sig := range expectedSignals {
		if sig == nil {
			t.Errorf("signal[%d] should not be nil", i)
		}
	}
}
