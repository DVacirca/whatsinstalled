package enrich

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"whatsinstalled/internal/store"
)

// newTestEnricher returns a RemoteEnricher whose crates/rubygems base URLs point
// at the given test server, backed by a real (temp) cache.
func newTestEnricher(t *testing.T, baseURL string) *RemoteEnricher {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := InitCacheTable(db.GetEnrichmentCache()); err != nil {
		t.Fatal(err)
	}
	re := NewRemoteEnricher(NewCache(db.GetEnrichmentCache()), false)
	re.cratesURL = baseURL + "/crates"
	re.rubygemsURL = baseURL + "/gems"
	return re
}

func TestEnrichCargo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/crates/ripgrep":
			w.Write([]byte(`{"crate":{"description":"recursively search directories"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	re := newTestEnricher(t, srv.URL)
	got := re.EnrichCargo([]string{"ripgrep", "does-not-exist"})

	if got["ripgrep"] != "recursively search directories" {
		t.Fatalf("ripgrep desc = %q", got["ripgrep"])
	}
	if _, ok := got["does-not-exist"]; ok {
		t.Fatalf("missing crate should not be in results: %v", got)
	}

	// Second call must be served from cache (server returns 500 to prove it).
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	})
	cached := re.EnrichCargo([]string{"ripgrep"})
	if cached["ripgrep"] != "recursively search directories" {
		t.Fatalf("cache miss: %q", cached["ripgrep"])
	}
}

func TestEnrichGemPrefersSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gems/rails.json":
			w.Write([]byte(`{"summary":"web framework","info":"longer info text"}`))
		case "/gems/rake.json":
			w.Write([]byte(`{"summary":"","info":"build tool"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	re := newTestEnricher(t, srv.URL)
	got := re.EnrichGem([]string{"rails", "rake"})

	if got["rails"] != "web framework" {
		t.Fatalf("rails should prefer summary, got %q", got["rails"])
	}
	if got["rake"] != "build tool" {
		t.Fatalf("rake should fall back to info, got %q", got["rake"])
	}
}

func TestFetchJSONNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", 404)
	}))
	defer srv.Close()

	re := NewRemoteEnricher(nil, false)
	var v map[string]any
	if re.fetchJSON(srv.URL, &v) {
		t.Fatal("fetchJSON should return false on non-200")
	}
}

func TestFetchJSONSetsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	re := NewRemoteEnricher(nil, false)
	var v map[string]any
	if !re.fetchJSON(srv.URL, &v) {
		t.Fatal("fetchJSON should succeed")
	}
	if gotUA == "" {
		t.Fatal("User-Agent header was not set (crates.io requires one)")
	}
}

// Guard the default registry URLs so an accidental edit can't silently point the
// enricher at the wrong host.
func TestDefaultRegistryURLs(t *testing.T) {
	re := NewRemoteEnricher(nil, false)
	if re.cratesURL != "https://crates.io/api/v1/crates" {
		t.Fatalf("cratesURL = %q", re.cratesURL)
	}
	if re.rubygemsURL != "https://rubygems.org/api/v1/gems" {
		t.Fatalf("rubygemsURL = %q", re.rubygemsURL)
	}
}
