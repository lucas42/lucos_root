package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	configyURL       = "https://configy.l42.eu/systems/http"
	configyCacheTTL  = 5 * time.Minute
	refreshInterval  = 60 * time.Second
	unavailableAfter = time.Hour
	fetchTimeout     = 10 * time.Second
)

//go:embed public templates
var assets embed.FS

// ---- Domain types ----

// ServiceInfo is the subset of /_info fields relevant to the homepage.
type ServiceInfo struct {
	Title          string `json:"title"`
	Icon           string `json:"icon"`
	NetworkOnly    bool   `json:"network_only"`
	StartURL       string `json:"start_url"`
	ShowOnHomepage bool   `json:"show_on_homepage"`
}

// ServiceEntry holds the in-memory state for one service.
type ServiceEntry struct {
	Domain      string
	Info        *ServiceInfo // nil until first successful fetch
	IconURL     string       // absolute: https://domain + icon path
	PageURL     string       // absolute: https://domain + start_url

	Unavailable bool
	// failingSince is the time of the first consecutive failure; zero when healthy.
	failingSince time.Time
	// notified tracks whether Loganne has been told about the current unavailable state.
	notified bool
}

// ---- Panel abstraction ----

// Panel is a named source of data for the homepage.
// Today only ServiceListPanel implements it; future panels (monitoring status,
// Loganne feed, etc.) will implement this interface so they share the same
// refresh machinery without a rearchitect.
type Panel interface {
	Name() string
	Refresh(ctx context.Context) error
}

// ---- ServiceListPanel ----

// ServiceListPanel fetches the service list from configy and polls /_info
// for each service. It is the sole Panel implementation for issue #126.
type ServiceListPanel struct {
	mu        sync.RWMutex
	entries   []*ServiceEntry
	configyAt time.Time

	// httpClient is cookie-jar-capable and can carry auth headers/cookies for
	// future authenticated panels (see architect's seam notes, lucos_root#126).
	httpClient *http.Client
	loganne    string
	system     string
	// scheme is the URL scheme used when fetching /_info endpoints.
	// Defaults to "https"; overridden to "http" in unit tests.
	scheme string
}

func newServiceListPanel(loganne, system string) *ServiceListPanel {
	return &ServiceListPanel{
		loganne: loganne,
		system:  system,
		scheme:  "https",
		httpClient: &http.Client{
			Timeout: fetchTimeout,
			// Jar: nil → no cookie jar yet; easy to add for future auth panels.
		},
	}
}

func (p *ServiceListPanel) Name() string { return "services" }

// ConfigyOK reports whether the service list has been successfully fetched at least once.
func (p *ServiceListPanel) ConfigyOK() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.configyAt.IsZero()
}

// fetchDomains fetches the service list from configy (JSON format).
func (p *ServiceListPanel) fetchDomains(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", configyURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.system)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("configy returned HTTP %d", resp.StatusCode)
	}

	var systems []struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&systems); err != nil {
		return nil, fmt.Errorf("configy response parse: %w", err)
	}

	domains := make([]string, 0, len(systems))
	for _, s := range systems {
		domains = append(domains, s.Domain)
	}
	return domains, nil
}

// fetchInfo fetches /_info for one service and updates its entry.
// It never returns an error; failures are recorded and logged.
func (p *ServiceListPanel) fetchInfo(ctx context.Context, entry *ServiceEntry) {
	url := p.scheme + "://" + entry.Domain + "/_info"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("WARN: build request for %s: %v", entry.Domain, err)
		p.recordFailure(entry)
		return
	}
	req.Header.Set("User-Agent", p.system)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("WARN: fetch /_info %s: %v", entry.Domain, err)
		p.recordFailure(entry)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("WARN: /_info %s returned HTTP %d", entry.Domain, resp.StatusCode)
		p.recordFailure(entry)
		return
	}

	var info ServiceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		log.Printf("WARN: /_info %s parse: %v", entry.Domain, err)
		p.recordFailure(entry)
		return
	}

	// Success — update entry.
	p.mu.Lock()
	defer p.mu.Unlock()

	entry.Info = &info

	iconPath := info.Icon
	if iconPath == "" {
		iconPath = "/icon.png"
	}
	entry.IconURL = p.scheme + "://" + entry.Domain + iconPath

	startPath := info.StartURL
	if startPath == "" {
		startPath = "/"
	}
	entry.PageURL = p.scheme + "://" + entry.Domain + startPath

	if entry.Unavailable {
		// Recovery: service was unavailable and is now back.
		entry.Unavailable = false
		if entry.notified {
			name := info.Title
			if name == "" {
				name = entry.Domain
			}
			p.emitLoganneLocked("serviceRecovered", name+" ("+entry.Domain+") is back on the homepage")
			entry.notified = false
		}
	}
	entry.failingSince = time.Time{} // reset failure clock
}

// recordFailure records a fetch failure and crosses into unavailable state
// when the threshold is exceeded. Emits Loganne on first transition.
func (p *ServiceListPanel) recordFailure(entry *ServiceEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry.failingSince.IsZero() {
		entry.failingSince = time.Now()
	}
	if !entry.Unavailable && time.Since(entry.failingSince) >= unavailableAfter {
		entry.Unavailable = true
		if !entry.notified {
			name := entry.Domain
			if entry.Info != nil && entry.Info.Title != "" {
				name = entry.Info.Title
			}
			p.emitLoganneLocked("serviceUnavailable", name+" ("+entry.Domain+") is unavailable on the homepage")
			entry.notified = true
		}
	}
}

// emitLoganneLocked posts a Loganne event asynchronously.
// Must be called with p.mu held (it fires a goroutine and returns immediately).
func (p *ServiceListPanel) emitLoganneLocked(eventType, humanReadable string) {
	if p.loganne == "" {
		return
	}
	endpoint := p.loganne
	system := p.system
	go func() {
		body := fmt.Sprintf(`{"type":%q,"humanReadable":%q,"source":%q}`, eventType, humanReadable, system)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(body))
		if err != nil {
			log.Printf("WARN: build loganne request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", system)
		resp, err := p.httpClient.Do(req)
		if err != nil {
			log.Printf("WARN: loganne post: %v", err)
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body) //nolint:errcheck
		if resp.StatusCode >= 300 {
			log.Printf("WARN: loganne returned HTTP %d", resp.StatusCode)
		}
	}()
}

// Refresh fetches configy (if stale) then /_info for all services sequentially.
// It implements Panel.
func (p *ServiceListPanel) Refresh(ctx context.Context) error {
	p.mu.RLock()
	stale := time.Since(p.configyAt) > configyCacheTTL
	p.mu.RUnlock()

	if stale {
		domains, err := p.fetchDomains(ctx)
		if err != nil {
			log.Printf("WARN: configy fetch: %v", err)
			// Continue with existing entries if we already have them.
		} else {
			p.mu.Lock()
			existing := make(map[string]*ServiceEntry, len(p.entries))
			for _, e := range p.entries {
				existing[e.Domain] = e
			}
			updated := make([]*ServiceEntry, 0, len(domains))
			for _, d := range domains {
				if e, ok := existing[d]; ok {
					updated = append(updated, e) // preserve cached state
				} else {
					updated = append(updated, &ServiceEntry{Domain: d})
				}
			}
			p.entries = updated
			p.configyAt = time.Now()
			p.mu.Unlock()
		}
	}

	// Snapshot entry list for sequential /_info fetches.
	p.mu.RLock()
	snapshot := make([]*ServiceEntry, len(p.entries))
	copy(snapshot, p.entries)
	p.mu.RUnlock()

	for _, entry := range snapshot {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		p.fetchInfo(fetchCtx, entry)
		cancel()
	}
	return nil
}

// Entries returns a read-only snapshot of the current service entries.
func (p *ServiceListPanel) Entries() []*ServiceEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*ServiceEntry, len(p.entries))
	copy(out, p.entries)
	return out
}

// IconURLs returns absolute icon URLs for all homepage-listed services.
// Used by the service worker to pre-cache icons.
func (p *ServiceListPanel) IconURLs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var urls []string
	for _, e := range p.entries {
		if e.Info != nil && e.Info.ShowOnHomepage && e.IconURL != "" {
			urls = append(urls, e.IconURL)
		}
	}
	return urls
}

// ---- HTTP server ----

// TemplateData is passed to the index template.
type TemplateData struct {
	Services []*ServiceEntry
}

// InfoResponse is the /_info JSON payload for lucos_root itself.
type InfoResponse struct {
	System         string            `json:"system"`
	Checks         map[string]Check  `json:"checks"`
	Metrics        map[string]Metric `json:"metrics"`
	CI             map[string]string `json:"ci"`
	Title          string            `json:"title,omitempty"`
	ShowOnHomepage bool              `json:"show_on_homepage"`
}

// Check is a single /_info health check entry.
type Check struct {
	OK         bool   `json:"ok"`
	TechDetail string `json:"techDetail"`
}

// Metric is a single /_info metric entry.
type Metric struct {
	Value      float64 `json:"value"`
	TechDetail string  `json:"techDetail"`
}

// Server holds all HTTP handler dependencies.
// Using a struct (rather than closures or globals) keeps auth-middleware
// insertion straightforward: wrap srv.routes() with any middleware chain.
type Server struct {
	panel     *ServiceListPanel
	templates *template.Template
	publicFS  fs.FS
	system    string
}

func newServer(panel *ServiceListPanel, system string) (*Server, error) {
	tmpl, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	pubFS, err := fs.Sub(assets, "public")
	if err != nil {
		return nil, fmt.Errorf("public sub-FS: %w", err)
	}
	return &Server{
		panel:     panel,
		templates: tmpl,
		publicFS:  pubFS,
		system:    system,
	}, nil
}

// routes returns the mux. Wrap it with middleware as needed.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	staticServer := http.FileServer(http.FS(s.publicFS))

	mux.HandleFunc("/_info", s.infoHandler)
	mux.HandleFunc("/serviceworker.js", s.serviceWorkerHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			s.indexHandler(w, r)
			return
		}
		staticServer.ServeHTTP(w, r)
	})
	return mux
}

func (s *Server) infoHandler(w http.ResponseWriter, r *http.Request) {
	resp := InfoResponse{
		System: s.system,
		Title:  "LucOS Root",
		Checks: map[string]Check{
			"configy-reachable": {
				OK:         s.panel.ConfigyOK(),
				TechDetail: "Whether the configy service list has been fetched at least once",
			},
		},
		Metrics:        map[string]Metric{},
		CI:             map[string]string{"circle": "gh/lucas42/lucos_root"},
		ShowOnHomepage: false,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	data := TemplateData{Services: s.panel.Entries()}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "index", data); err != nil {
		log.Printf("ERROR: render index: %v", err)
	}
}

func (s *Server) serviceWorkerHandler(w http.ResponseWriter, r *http.Request) {
	var sb strings.Builder

	// localUrls: all static asset paths served from public/
	sb.WriteString("const localUrls = [\n\t\"/\",\n")
	entries, _ := fs.ReadDir(s.publicFS, ".")
	for _, e := range entries {
		fmt.Fprintf(&sb, "\t\"/%s\",\n", e.Name())
	}
	sb.WriteString("];\n\n")

	// iconUrls: absolute icon URLs for homepage-listed services
	sb.WriteString("const iconUrls = [\n")
	for _, url := range s.panel.IconURLs() {
		fmt.Fprintf(&sb, "\t%q,\n", url)
	}
	sb.WriteString("];\n\n")

	// Append the service worker logic (static JS template)
	swContent, err := assets.ReadFile("templates/service-worker.js")
	if err != nil {
		http.Error(w, "service worker unavailable", http.StatusInternalServerError)
		return
	}
	sb.Write(swContent)

	w.Header().Set("Content-Type", "application/javascript")
	io.WriteString(w, sb.String()) //nolint:errcheck
}

// ---- main ----

func main() {
	// Healthcheck mode: make a quick HTTP GET to /_info and exit.
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8003"
		}
		resp, err := http.Get("http://127.0.0.1:" + port + "/_info")
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			fmt.Fprintf(os.Stderr, "healthcheck: HTTP %d\n", resp.StatusCode)
			os.Exit(1)
		}
		os.Exit(0)
	}

	system := os.Getenv("SYSTEM")
	if system == "" {
		system = "lucos_root"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8003"
	}
	loganne := os.Getenv("LOGANNE_ENDPOINT")

	panel := newServiceListPanel(loganne, system)

	log.Printf("Starting %s — performing initial service sweep...", system)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if err := panel.Refresh(ctx); err != nil {
		log.Printf("Initial sweep incomplete: %v", err)
	}
	cancel()

	// Background refresh goroutine
	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), refreshInterval)
			if err := panel.Refresh(ctx); err != nil {
				log.Printf("Refresh error: %v", err)
			}
			cancel()
		}
	}()

	srv, err := newServer(panel, system)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	log.Printf("Listening on :%s", port)
	if err := http.ListenAndServe(":"+port, srv.routes()); err != nil {
		log.Fatalf("HTTP server: %v", err)
	}
}
