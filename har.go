package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── HAR 1.2 structs ────────────────────────────────────────────────────────

type HAR struct {
	Log HARLog `json:"log"`
}

type HARLog struct {
	Version string     `json:"version"`
	Creator HARCreator `json:"creator"`
	Comment string     `json:"comment,omitempty"`
	Entries []HAREntry `json:"entries"`
}

type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type HAREntry struct {
	StartedDateTime string      `json:"startedDateTime"` // ISO 8601
	Time            float64     `json:"time"`            // ms
	Request         HARRequest  `json:"request"`
	Response        HARResponse `json:"response"`
	ServerIPAddress string      `json:"serverIPAddress,omitempty"`
	Comment         string      `json:"comment,omitempty"`
}

type HARRequest struct {
	Method      string       `json:"method"`
	URL         string       `json:"url"`
	HTTPVersion string       `json:"httpVersion"`
	Headers     []HARNameVal `json:"headers"`
	QueryString []HARNameVal `json:"queryString"`
	PostData    *HARPostData `json:"postData,omitempty"`
	BodySize    int          `json:"bodySize"`
	HeadersSize int          `json:"headersSize"`
}

type HARResponse struct {
	Status      int          `json:"status"`
	StatusText  string       `json:"statusText"`
	HTTPVersion string       `json:"httpVersion"`
	Headers     []HARNameVal `json:"headers"`
	Content     HARContent   `json:"content"`
	BodySize    int          `json:"bodySize"`
	HeadersSize int          `json:"headersSize"`
	RedirectURL string       `json:"redirectURL"`
}

type HARContent struct {
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
}

type HARPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type HARNameVal struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ── Conversion ─────────────────────────────────────────────────────────────

func echoResponsesToHAR(entries []EchoResponse, comment string) HAR {
	harEntries := make([]HAREntry, 0, len(entries))

	for _, e := range entries {
		scheme := "http"
		if e.TLS {
			scheme = "https"
		}

		// Reconstruct full URL
		host := fmt.Sprintf("localhost:%d", e.Port)
		fullURL := fmt.Sprintf("%s://%s%s", scheme, host, e.URL)

		// Headers → HAR name/value pairs
		reqHeaders := make([]HARNameVal, 0, len(e.Headers))
		for k, v := range e.Headers {
			reqHeaders = append(reqHeaders, HARNameVal{Name: k, Value: v})
		}
		sort.Slice(reqHeaders, func(i, j int) bool {
			return reqHeaders[i].Name < reqHeaders[j].Name
		})

		// Query string
		queryString := make([]HARNameVal, 0, len(e.Query))
		for k, v := range e.Query {
			queryString = append(queryString, HARNameVal{Name: k, Value: v})
		}
		sort.Slice(queryString, func(i, j int) bool {
			return queryString[i].Name < queryString[j].Name
		})

		// Post data
		var postData *HARPostData
		if e.Body != "" {
			mimeType := "application/octet-stream"
			if ct, ok := e.Headers["Content-Type"]; ok {
				mimeType = ct
			}
			postData = &HARPostData{MimeType: mimeType, Text: e.Body}
		}

		// Response content — parrot echoes back JSON
		respText, _ := json.Marshal(e)
		respContent := HARContent{
			Size:     len(respText),
			MimeType: "application/json",
			Text:     string(respText),
		}

		// Response headers parrot would have sent
		respHeaders := []HARNameVal{
			{Name: "Content-Type", Value: "application/json"},
			{Name: "X-Parrot-Port", Value: strconv.Itoa(e.Port)},
			{Name: "X-Parrot-TLS", Value: strconv.FormatBool(e.TLS)},
			{Name: "X-Parrot-Duration-Ms", Value: fmt.Sprintf("%.2f", e.DurationMs)},
		}

		statusText := http.StatusText(e.StatusCode)
		if statusText == "" {
			statusText = "Unknown"
		}

		harEntries = append(harEntries, HAREntry{
			StartedDateTime: e.Timestamp.UTC().Format(time.RFC3339Nano),
			Time:            e.DurationMs,
			ServerIPAddress: "127.0.0.1",
			Comment:         fmt.Sprintf("port:%d tls:%v", e.Port, e.TLS),
			Request: HARRequest{
				Method:      e.Method,
				URL:         fullURL,
				HTTPVersion: "HTTP/1.1",
				Headers:     reqHeaders,
				QueryString: queryString,
				PostData:    postData,
				BodySize:    e.BodyBytes,
				HeadersSize: -1, // not tracked
			},
			Response: HARResponse{
				Status:      e.StatusCode,
				StatusText:  statusText,
				HTTPVersion: "HTTP/1.1",
				Headers:     respHeaders,
				Content:     respContent,
				BodySize:    len(respText),
				HeadersSize: -1,
				RedirectURL: "",
			},
		})
	}

	// Sort chronologically
	sort.Slice(harEntries, func(i, j int) bool {
		return harEntries[i].StartedDateTime < harEntries[j].StartedDateTime
	})

	return HAR{
		Log: HARLog{
			Version: "1.2",
			Creator: HARCreator{Name: "parrot", Version: "2.2"},
			Comment: comment,
			Entries: harEntries,
		},
	}
}

// ── Port resolution helper ──────────────────────────────────────────────────

// resolveExportPorts parses the ?ports= query param against the known ports.
// Accepts: "all", empty (defaults to ownPort only), or "8080,8081".
func resolveExportPorts(portsParam string, ownPort int, knownPorts []int) []int {
	portsParam = strings.TrimSpace(portsParam)

	if portsParam == "" {
		return []int{ownPort}
	}

	if strings.EqualFold(portsParam, "all") {
		out := make([]int, len(knownPorts))
		copy(out, knownPorts)
		sort.Ints(out)
		return out
	}

	seen := map[int]bool{}
	var out []int
	for _, p := range strings.Split(portsParam, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// ── HTTP handler ────────────────────────────────────────────────────────────

// harHandler returns an http.HandlerFunc that serves a HAR export.
func harHandler(ownPort int, store *Store, knownPorts []int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ports := resolveExportPorts(r.URL.Query().Get("ports"), ownPort, knownPorts)

		var entries []EchoResponse
		for _, p := range ports {
			entries = append(entries, store.GetHistory(p)...)
		}

		portStrs := make([]string, len(ports))
		for i, p := range ports {
			portStrs[i] = strconv.Itoa(p)
		}
		comment := fmt.Sprintf("exported by parrot from port(s): %s", strings.Join(portStrs, ", "))
		har := echoResponsesToHAR(entries, comment)

		filename := fmt.Sprintf("parrot-%s.har", time.Now().Format("20060102-150405"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(har)
	}
}

// ── File export (used on shutdown) ─────────────────────────────────────────

// exportHARToFile writes all history across all ports to a file.
func exportHARToFile(path string, store *Store, knownPorts []int) error {
	var entries []EchoResponse
	for _, p := range knownPorts {
		entries = append(entries, store.GetHistory(p)...)
	}

	comment := fmt.Sprintf("parrot shutdown export — %s", time.Now().UTC().Format(time.RFC3339))
	har := echoResponsesToHAR(entries, comment)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating HAR file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(har)
}
