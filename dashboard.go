package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Dashboard handles both terminal rendering and the web UI.
type Dashboard struct {
	store *Store
	ports []int
	flags []string // non-default flags, shown in both UIs
}

func NewDashboard(store *Store, ports []int, flags []string) *Dashboard {
	return &Dashboard{store: store, ports: ports, flags: flags}
}

// Render draws the live terminal dashboard using ANSI escape codes.
func (d *Dashboard) Render() {
	// Move the cursor to the top-left and clear the screen
	fmt.Print("\033[H\033[2J")

	uptime := d.store.Uptime().Round(time.Second)

	fmt.Printf("\033[1;36m🦜 parrot\033[0m  \033[90muptime: %s\033[0m\n", uptime)
	if len(d.flags) > 0 {
		fmt.Printf("\033[90m  flags: %s\033[0m\n", strings.Join(d.flags, "  "))
	}
	fmt.Println(strings.Repeat("─", 70))

	// Per-port stats table
	fmt.Printf("\033[1m  %-8s  %-12s  %-12s  %-20s\033[0m\n",
		"PORT", "REQUESTS", "AVG (ms)", "LAST REQUEST")
	fmt.Println(strings.Repeat("─", 70))

	sortedPorts := make([]int, len(d.ports))
	copy(sortedPorts, d.ports)
	sort.Ints(sortedPorts)

	for _, port := range sortedPorts {
		count, avgMs := d.store.Stats(port)
		history := d.store.GetHistory(port)

		lastReq := "\033[90mnone yet\033[0m"
		if len(history) > 0 {
			last := history[len(history)-1]
			ago := time.Since(last.Timestamp).Round(time.Millisecond)
			tlsTag := ""
			if last.TLS {
				tlsTag = " \033[36m[tls]\033[0m"
			}
			lastReq = fmt.Sprintf("\033[33m%s %s\033[0m%s (%s ago)", last.Method, last.Path, tlsTag, ago)
		}

		fmt.Printf("  \033[32m%-8d\033[0m  %-12d  %-12s  %s\n",
			port,
			count,
			fmt.Sprintf("%.2f", avgMs),
			lastReq,
		)
	}

	fmt.Println(strings.Repeat("─", 70))

	// Recent requests across all ports
	fmt.Printf("\033[1m  Recent Requests\033[0m\n\n")

	type entry struct {
		r    EchoResponse
		port int
	}
	var all []entry
	for _, port := range sortedPorts {
		for _, r := range d.store.GetHistory(port) {
			all = append(all, entry{r, port})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].r.Timestamp.After(all[j].r.Timestamp)
	})
	if len(all) > 8 {
		all = all[:8]
	}

	for _, e := range all {
		methodColor := methodColor(e.r.Method)
		ago := time.Since(e.r.Timestamp).Round(time.Millisecond)
		scheme := "http "
		schemeColor := "\033[90m"
		if e.r.TLS {
			scheme = "https"
			schemeColor = "\033[36m"
		}
		fmt.Printf("  \033[90m:%d\033[0m  %s%-5s\033[0m  %s%s\033[0m  %-28s  \033[90m%.2fms  %s ago\033[0m\n",
			e.port,
			methodColor,
			e.r.Method,
			schemeColor,
			scheme,
			truncate(e.r.Path, 28),
			e.r.DurationMs,
			ago,
		)
	}

	if len(all) == 0 {
		fmt.Printf("  \033[90mwaiting for requests...\033[0m\n")
	}

	fmt.Println()
	fmt.Printf("\033[90m  /_parrot/history  /_parrot/health  dashboard: http://localhost:9999\033[0m\n")
}

// Serve runs the web dashboard on the given port.
func (d *Dashboard) Serve(port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := w.Write([]byte(dashboardHTML))
		if err != nil {
			fmt.Println("error writing dashboard HTML:", err)
			return
		}
	})

	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		type PortStat struct {
			Port    int            `json:"port"`
			Count   int64          `json:"count"`
			AvgMs   float64        `json:"avg_ms"`
			History []EchoResponse `json:"history"`
		}

		stats := []PortStat{}
		sortedPorts := make([]int, len(d.ports))
		copy(sortedPorts, d.ports)
		sort.Ints(sortedPorts)

		for _, p := range sortedPorts {
			count, avgMs := d.store.Stats(p)
			history := d.store.GetHistory(p)
			// Return last 20 for web UI
			if len(history) > 20 {
				history = history[len(history)-20:]
			}
			stats = append(stats, PortStat{
				Port:    p,
				Count:   count,
				AvgMs:   avgMs,
				History: history,
			})
		}

		err := json.NewEncoder(w).Encode(map[string]any{
			"uptime_seconds": d.store.Uptime().Seconds(),
			"ports":          stats,
			"flags":          d.flags,
		})
		if err != nil {
			fmt.Println("error encoding stats JSON:", err)
			return
		}
	})

	mux.HandleFunc("/api/replay", replayHandler(d.store, 10*time.Second))

	mux.HandleFunc("/api/clear", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		portsParam := r.URL.Query().Get("ports")
		targets := resolveExportPorts(portsParam, 0, d.ports)
		// resolveExportPorts returns ownPort=0 when empty — default to all instead
		if portsParam == "" {
			targets = make([]int, len(d.ports))
			copy(targets, d.ports)
		}
		for _, p := range targets {
			d.store.Clear(p)
		}
		err := json.NewEncoder(w).Encode(map[string]any{"cleared": targets})
		if err != nil {
			fmt.Println("error encoding clear response JSON:", err)
			return
		}
	})

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("\033[90m  web dashboard → http://localhost%s\033[0m\n", addr)
	err := http.ListenAndServe(addr, mux)
	if err != nil {
		fmt.Println("error starting dashboard server:", err)
		return
	}
}

func methodColor(method string) string {
	switch method {
	case "GET":
		return "\033[32m"
	case "POST":
		return "\033[34m"
	case "PUT":
		return "\033[33m"
	case "DELETE":
		return "\033[31m"
	case "PATCH":
		return "\033[35m"
	default:
		return "\033[37m"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>🦜 parrot dashboard</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #0f1117; color: #e2e8f0; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; }
  header { padding: 20px 24px 12px; border-bottom: 1px solid #1e2535; display: flex; align-items: center; gap: 12px; }
  header h1 { font-size: 18px; font-weight: 700; color: #7dd3fc; }
  header span { color: #475569; font-size: 12px; }
  .export-bar { display: flex; align-items: center; gap: 8px; margin-left: auto; }
  .export-bar select { background: #161b27; color: #cbd5e1; border: 1px solid #1e2535; border-radius: 5px; padding: 5px 8px; font-size: 12px; font-family: inherit; cursor: pointer; }
  .export-bar button { background: #1e3a5f; color: #7dd3fc; border: 1px solid #2563eb44; border-radius: 5px; padding: 5px 12px; font-size: 12px; font-family: inherit; cursor: pointer; transition: background 0.15s; }
  .export-bar button:hover { background: #1d4ed8; color: #fff; }
  .btn-clear { background: #2d1515 !important; color: #f87171 !important; border: 1px solid #991b1b44 !important; border-radius: 5px; padding: 5px 12px; font-size: 12px; font-family: inherit; cursor: pointer; transition: background 0.15s; }
  .btn-clear:hover { background: #7f1d1d !important; color: #fff !important; }
  .flag-bar { padding: 6px 24px; background: #0d1117; border-bottom: 1px solid #1e2535; display: flex; flex-wrap: wrap; gap: 6px; min-height: 32px; align-items: center; }
  .flag-bar.hidden { display: none; }
  .flag-pill { background: #161b27; border: 1px solid #1e2535; border-radius: 4px; padding: 2px 8px; font-size: 11px; color: #94a3b8; white-space: nowrap; }
  .flag-pill span { color: #7dd3fc; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; padding: 20px 24px; }
  .card { background: #161b27; border: 1px solid #1e2535; border-radius: 8px; padding: 16px; }
  .card-title { color: #7dd3fc; font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; margin-bottom: 12px; }
  .stat { font-size: 28px; font-weight: 700; color: #f1f5f9; }
  .stat-label { color: #475569; font-size: 11px; margin-top: 2px; }
  .section { padding: 0 24px 24px; }
  .section h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.08em; color: #475569; margin-bottom: 12px; }
  table { width: 100%; border-collapse: collapse; }
  th { text-align: left; color: #475569; font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; padding: 6px 10px; border-bottom: 1px solid #1e2535; }
  td { padding: 8px 10px; border-bottom: 1px solid #111827; font-size: 12px; }
  tr:hover td { background: #161b27; }
  .method { display: inline-block; padding: 2px 6px; border-radius: 3px; font-size: 11px; font-weight: 700; }
  .GET    { background: #052e16; color: #4ade80; }
  .POST   { background: #0c1a3a; color: #60a5fa; }
  .PUT    { background: #2d1b00; color: #fbbf24; }
  .DELETE { background: #2d0a0a; color: #f87171; }
  .PATCH  { background: #1e0a2e; color: #c084fc; }
  .path   { color: #cbd5e1; }
  .muted  { color: #475569; }
  .port-badge { background: #1e2535; color: #7dd3fc; padding: 2px 7px; border-radius: 4px; font-size: 11px; }
  .scheme-http  { color: #475569; font-size: 11px; }
  .scheme-https { color: #22d3ee; font-size: 11px; font-weight: 600; }
  #uptime { color: #4ade80; }
  .replay-bar { padding: 12px 24px; background: #0d1117; border-bottom: 1px solid #1e2535; display: flex; align-items: center; gap: 8px; }
  .replay-bar label { color: #475569; font-size: 11px; white-space: nowrap; }
  .replay-bar input { flex: 1; background: #161b27; color: #e2e8f0; border: 1px solid #1e2535; border-radius: 5px; padding: 5px 10px; font-size: 12px; font-family: inherit; }
  .replay-bar input:focus { outline: none; border-color: #2563eb; }
  .replay-bar input::placeholder { color: #334155; }
  .btn-replay { background: #1a2e1a; color: #4ade80; border: 1px solid #16a34a44; border-radius: 4px; padding: 2px 8px; font-size: 11px; font-family: inherit; cursor: pointer; white-space: nowrap; transition: background 0.15s; }
  .btn-replay:hover { background: #14532d; }
  .btn-replay:disabled { opacity: 0.4; cursor: default; }
  .replay-result { font-size: 11px; padding: 8px 12px; border-radius: 4px; margin-top: 6px; }
  .replay-ok   { background: #052e16; color: #4ade80; border: 1px solid #166534; }
  .replay-err  { background: #2d0a0a; color: #f87171; border: 1px solid #991b1b; }
  .replay-detail { padding: 10px 24px 0; }
  .replay-detail pre { background: #0d1117; border: 1px solid #1e2535; border-radius: 4px; padding: 10px; font-size: 11px; color: #94a3b8; overflow-x: auto; max-height: 200px; white-space: pre-wrap; word-break: break-all; }
</style>
</head>
<body>
<header>
  <h1>🦜 parrot</h1>
  <span>uptime: <span id="uptime">—</span></span>
  <div class="export-bar">
    <select id="export-scope">
      <option value="all">all ports</option>
    </select>
    <button onclick="exportHAR()">⬇ Export HAR</button>
    <button class="btn-clear" onclick="clearHistory()">✕ Clear</button>
  </div>
</header>

<div id="flag-bar" class="flag-bar hidden"></div>

<div class="replay-bar">
  <label>Replay target →</label>
  <input id="replay-target" type="text" placeholder="http://localhost:3000/webhook" />
  <span style="color:#475569;font-size:11px">Strip headers:</span>
  <input id="replay-strip" type="text" placeholder="X-Stripe-Signature, X-Hub-Signature" style="flex:0.6" />
</div>

<div id="replay-detail" class="replay-detail" style="display:none">
  <div id="replay-result-badge"></div>
  <pre id="replay-result-body"></pre>
</div>

<div class="grid" id="port-cards"></div>

<div class="section">
  <h2>Recent Requests</h2>
  <table>
    <thead>
      <tr>
        <th>Port</th><th>Method</th><th>Scheme</th><th>Path</th><th>Duration</th><th>Time</th><th>Remote</th><th></th>
      </tr>
    </thead>
    <tbody id="request-table"></tbody>
  </table>
</div>

<script>
function fmt(seconds) {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  return [h,m,s].map((v,i) => v > 0 || i > 0 ? String(v).padStart(2,'0') : null).filter(Boolean).join(':') || '0s';
}

function ago(ts) {
  const diff = (Date.now() - new Date(ts)) / 1000;
  if (diff < 60) return diff.toFixed(1) + 's ago';
  if (diff < 3600) return Math.floor(diff/60) + 'm ago';
  return Math.floor(diff/3600) + 'h ago';
}

async function refresh() {
  try {
    const res = await fetch('/api/stats');
    const data = await res.json();

    document.getElementById('uptime').textContent = fmt(data.uptime_seconds);

    // Flag pills
    const flagBar = document.getElementById('flag-bar');
    if (data.flags && data.flags.length > 0) {
      flagBar.className = 'flag-bar';
      flagBar.innerHTML = data.flags.map(f => {
        const eq = f.indexOf('=');
        if (eq === -1) return ` + "`" + `<span class="flag-pill"><span>${f}</span></span>` + "`" + `;
        const key = f.slice(0, eq);
        const val = f.slice(eq + 1);
        return ` + "`" + `<span class="flag-pill">${key}=<span>${val}</span></span>` + "`" + `;
      }).join('');
    } else {
      flagBar.className = 'flag-bar hidden';
    }

    window._parrotPorts = data.ports.map(p => p.port);

    // Populate export scope selector
    const sel = document.getElementById('export-scope');
    const prev = sel.value;
    sel.innerHTML = '<option value="all">all ports</option>' +
      data.ports.map(p => ` + "`" + `<option value="${p.port}">port ${p.port}</option>` + "`" + `).join('');
    sel.value = prev && [...sel.options].some(o => o.value === prev) ? prev : 'all';

    // Port cards
    const cards = document.getElementById('port-cards');
    cards.innerHTML = data.ports.map(p => ` + "`" + `
      <div class="card">
        <div class="card-title">Port ${p.port}</div>
        <div class="stat">${p.count}</div>
        <div class="stat-label">total requests</div>
        <div style="margin-top:12px;color:#94a3b8">avg <strong style="color:#f1f5f9">${p.avg_ms.toFixed(2)}ms</strong></div>
        <div style="margin-top:8px;font-size:11px;color:#475569">
          http &nbsp;
          <a href="http://localhost:${p.port}/_parrot/history" style="color:#7dd3fc;text-decoration:none">history ↗</a>
          &nbsp;·&nbsp;
          <a href="http://localhost:${p.port}/_parrot/health" style="color:#7dd3fc;text-decoration:none">health ↗</a>
        </div>
        <div style="margin-top:4px;font-size:11px;color:#475569">
          https
          <a href="https://localhost:${p.port+1000}/_parrot/history" style="color:#22d3ee;text-decoration:none">history ↗</a>
          &nbsp;·&nbsp;
          <a href="https://localhost:${p.port+1000}/_parrot/health" style="color:#22d3ee;text-decoration:none">health ↗</a>
        </div>
      </div>` + "`" + `).join('');

    // Recent requests table
    const all = data.ports.flatMap(p => p.history.map(r => ({...r, _port: p.port})));
    all.sort((a,b) => new Date(b.timestamp) - new Date(a.timestamp));
    const recent = all.slice(0, 30);

    const tbody = document.getElementById('request-table');
    if (recent.length === 0) {
      tbody.innerHTML = '<tr><td colspan="8" class="muted" style="text-align:center;padding:20px">waiting for requests...</td></tr>';
    } else {
      tbody.innerHTML = recent.map(r => ` + "`" + `
        <tr>
          <td><span class="port-badge">${r._port}</span></td>
          <td><span class="method ${r.method}">${r.method}</span></td>
          <td><span class="${r.tls ? 'scheme-https' : 'scheme-http'}">${r.tls ? 'https' : 'http'}</span></td>
          <td class="path">${r.path}${r.url.includes('?') ? '<span class="muted">?' + r.url.split('?')[1] + '</span>' : ''}</td>
          <td class="muted">${r.duration_ms.toFixed(2)}ms</td>
          <td class="muted">${ago(r.timestamp)}</td>
          <td class="muted">${r.remote_addr}</td>
          <td><button class="btn-replay" onclick="replayRequest('${r.id}')">↺ Replay</button></td>
        </tr>` + "`" + `).join('');
    }
  } catch(e) {
    console.error('fetch failed', e);
  }
}

async function clearHistory() {
  try {
    await fetch('/api/clear?ports=all', { method: 'DELETE' });
    document.getElementById('replay-detail').style.display = 'none';
  } catch(e) {
    console.error('clear failed', e);
  }
}

async function replayRequest(id) {
  const target = document.getElementById('replay-target').value.trim();
  if (!target) {
    alert('Set a replay target URL first (the bar at the top).');
    return;
  }

  const stripRaw = document.getElementById('replay-strip').value.trim();
  const stripHeaders = stripRaw ? stripRaw.split(',').map(s => s.trim()).filter(Boolean) : [];

  // Disable all replay buttons while in flight
  document.querySelectorAll('.btn-replay').forEach(b => b.disabled = true);

  const detail = document.getElementById('replay-detail');
  const badge  = document.getElementById('replay-result-badge');
  const body   = document.getElementById('replay-result-body');
  detail.style.display = 'block';
  badge.innerHTML = '<span style="color:#475569">replaying...</span>';
  body.textContent = '';

  try {
    const res = await fetch('/api/replay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, target, strip_headers: stripHeaders })
    });
    const data = await res.json();

    if (data.ok) {
      badge.innerHTML = ` + "`" + `<div class="replay-result replay-ok">✓ ${data.method} ${data.target} → <strong>${data.status_code}</strong> in ${data.duration_ms.toFixed(2)}ms${data.stripped_headers?.length ? ' · stripped: ' + data.stripped_headers.join(', ') : ''}</div>` + "`" + `;
      body.textContent = data.response_body || '(empty body)';
    } else {
      badge.innerHTML = ` + "`" + `<div class="replay-result replay-err">✗ ${data.error}</div>` + "`" + `;
      body.textContent = '';
    }
  } catch (e) {
    badge.innerHTML = ` + "`" + `<div class="replay-result replay-err">✗ fetch failed: ${e.message}</div>` + "`" + `;
    body.textContent = '';
  } finally {
    document.querySelectorAll('.btn-replay').forEach(b => b.disabled = false);
  }
}

function exportHAR() {
  const scope = document.getElementById('export-scope').value;
  // Find the first available port to proxy the request through
  const firstPort = window._parrotPorts && window._parrotPorts[0];
  if (!firstPort) { alert('No ports available yet.'); return; }
  const portsParam = scope === 'all' ? 'all' : scope;
  window.location.href = ` + "`" + `http://localhost:${firstPort}/_parrot/export.har?ports=${portsParam}` + "`" + `;
}

refresh();
setInterval(refresh, 1000);
</script>
</body>
</html>`
