package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- Panel state-machine tests ----

func TestConfigyOKFalseBeforeFetch(t *testing.T) {
	p := newServiceListPanel("", "test")
	if p.ConfigyOK() {
		t.Error("ConfigyOK should be false before any fetch")
	}
}

func TestServiceNotUnavailableOnFirstFailure(t *testing.T) {
	p := newServiceListPanel("", "test")
	entry := &ServiceEntry{Domain: "test.l42.eu"}
	p.recordFailure(entry)
	if entry.Unavailable {
		t.Error("should not be unavailable immediately after first failure")
	}
}

func TestServiceBecomesUnavailableAfterThreshold(t *testing.T) {
	p := newServiceListPanel("", "test")
	entry := &ServiceEntry{
		Domain:       "test.l42.eu",
		failingSince: time.Now().Add(-2 * unavailableAfter), // well past threshold
	}
	p.recordFailure(entry)
	if !entry.Unavailable {
		t.Error("should be unavailable after threshold exceeded")
	}
}

func TestServiceUnavailableFlagNotSetBeforeThreshold(t *testing.T) {
	p := newServiceListPanel("", "test")
	entry := &ServiceEntry{
		Domain:       "test.l42.eu",
		failingSince: time.Now().Add(-30 * time.Minute), // under 1-hour threshold
	}
	p.recordFailure(entry)
	if entry.Unavailable {
		t.Error("should not be unavailable before 1-hour threshold")
	}
}

// ---- fetchInfo integration test (plain HTTP mock) ----

func TestFetchInfoUpdatesEntry(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_info" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"title":"Test","icon":"/icon.png","start_url":"/app","network_only":false,"show_on_homepage":true}`)) //nolint:errcheck
	}))
	defer mock.Close()

	p := newServiceListPanel("", "test")
	p.scheme = "http" // point at plain-HTTP mock
	p.httpClient = mock.Client()

	domain := strings.TrimPrefix(mock.URL, "http://")
	entry := &ServiceEntry{Domain: domain}

	p.fetchInfo(t.Context(), entry)

	if entry.Info == nil {
		t.Fatal("entry.Info should be set after successful fetch")
	}
	if entry.Info.Title != "Test" {
		t.Errorf("title = %q, want Test", entry.Info.Title)
	}
	if !entry.Info.ShowOnHomepage {
		t.Error("show_on_homepage should be true")
	}
	if entry.IconURL != "http://"+domain+"/icon.png" {
		t.Errorf("IconURL = %q, unexpected value", entry.IconURL)
	}
	if entry.PageURL != "http://"+domain+"/app" {
		t.Errorf("PageURL = %q, unexpected value", entry.PageURL)
	}
	if entry.Unavailable {
		t.Error("entry should not be unavailable after successful fetch")
	}
}

func TestFetchInfoRecordsFailureOnError(t *testing.T) {
	// Point at a server that immediately closes the connection
	p := newServiceListPanel("", "test")
	p.scheme = "http"
	entry := &ServiceEntry{Domain: "127.0.0.1:1"} // port 1 — should fail to connect

	p.fetchInfo(t.Context(), entry)

	if entry.Info != nil {
		t.Error("Info should remain nil after failed fetch")
	}
	if entry.failingSince.IsZero() {
		t.Error("failingSince should be set after failure")
	}
}

// ---- Handler tests ----

func newTestServer(t *testing.T) *Server {
	t.Helper()
	p := newServiceListPanel("", "test_system")
	srv, err := newServer(p, "test_system")
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	return srv
}

func TestInfoHandlerShape(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/_info", nil)
	rec := httptest.NewRecorder()
	srv.infoHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp InfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode /_info: %v", err)
	}
	if resp.System != "test_system" {
		t.Errorf("system = %q, want test_system", resp.System)
	}
	if resp.Title == "" {
		t.Error("title should not be empty (Tier 2 field)")
	}
	if resp.ShowOnHomepage {
		t.Error("lucos_root should not appear on its own homepage")
	}
	check, ok := resp.Checks["configy-reachable"]
	if !ok {
		t.Fatal("missing configy-reachable check")
	}
	if check.OK {
		t.Error("configy-reachable should be false before any configy fetch")
	}
	if check.TechDetail == "" {
		t.Error("configy-reachable check should have a techDetail")
	}
}

func TestHomepageOnlyShowsHomepageServices(t *testing.T) {
	srv := newTestServer(t)

	srv.panel.mu.Lock()
	srv.panel.entries = []*ServiceEntry{
		{
			Domain:  "visible.l42.eu",
			Info:    &ServiceInfo{Title: "Visible", ShowOnHomepage: true, Icon: "/icon.png", StartURL: "/"},
			IconURL: "https://visible.l42.eu/icon.png",
			PageURL: "https://visible.l42.eu/",
		},
		{
			Domain: "hidden.l42.eu",
			Info:   &ServiceInfo{Title: "Hidden", ShowOnHomepage: false},
		},
		{
			Domain: "noinfo.l42.eu",
			// Info deliberately nil: not yet fetched
		},
	}
	srv.panel.mu.Unlock()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.indexHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "visible.l42.eu") {
		t.Error("homepage should show visible service")
	}
	if strings.Contains(body, "hidden.l42.eu") {
		t.Error("homepage should not show service with show_on_homepage=false")
	}
	if strings.Contains(body, "noinfo.l42.eu") {
		t.Error("homepage should not show service with no Info yet")
	}
}

func TestHomepageMarksUnavailableService(t *testing.T) {
	srv := newTestServer(t)

	srv.panel.mu.Lock()
	srv.panel.entries = []*ServiceEntry{
		{
			Domain:      "down.l42.eu",
			Info:        &ServiceInfo{Title: "Down Service", ShowOnHomepage: true, Icon: "/icon.png", StartURL: "/"},
			IconURL:     "https://down.l42.eu/icon.png",
			PageURL:     "https://down.l42.eu/",
			Unavailable: true,
		},
	}
	srv.panel.mu.Unlock()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.indexHandler(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "unavailable") {
		t.Error("homepage should mark unavailable service with 'unavailable' in HTML")
	}
}

func TestServiceWorkerContainsPreamble(t *testing.T) {
	srv := newTestServer(t)

	srv.panel.mu.Lock()
	srv.panel.entries = []*ServiceEntry{
		{
			Domain:  "test.l42.eu",
			Info:    &ServiceInfo{Title: "Test", ShowOnHomepage: true, Icon: "/icon.png"},
			IconURL: "https://test.l42.eu/icon.png",
		},
	}
	srv.panel.mu.Unlock()

	req := httptest.NewRequest("GET", "/serviceworker.js", nil)
	rec := httptest.NewRecorder()
	srv.serviceWorkerHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "const localUrls") {
		t.Error("serviceworker.js should contain localUrls declaration")
	}
	if !strings.Contains(body, "const iconUrls") {
		t.Error("serviceworker.js should contain iconUrls declaration")
	}
	if !strings.Contains(body, "https://test.l42.eu/icon.png") {
		t.Error("serviceworker.js should include injected icon URL")
	}
}
