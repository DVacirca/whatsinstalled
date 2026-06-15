package enrich

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RemoteEnricher fetches descriptions from package registries via HTTP.
type RemoteEnricher struct {
	client  *http.Client
	cache   *Cache
	verbose bool
}

// NewRemoteEnricher creates a remote enricher with the given cache.
func NewRemoteEnricher(cache *Cache, verbose bool) *RemoteEnricher {
	return &RemoteEnricher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache:   cache,
		verbose: verbose,
	}
}

// EnrichPip fetches descriptions from PyPI for pip packages.
// Returns a map of package name -> description.
func (re *RemoteEnricher) EnrichPip(names []string) map[string]string {
	results := make(map[string]string)
	for _, name := range names {
		// Check cache first
		if desc, ok := re.cache.Get(name, "pip", 30*24*time.Hour); ok {
			results[name] = desc
			continue
		}

		desc := re.fetchPyPI(name)
		if desc != "" {
			results[name] = desc
			_ = re.cache.Set(name, "pip", desc)
		}
		// Small delay to be polite to PyPI
		time.Sleep(100 * time.Millisecond)
	}
	return results
}

// EnrichNpm fetches descriptions from npm registry for npm packages.
// Returns a map of package name -> description.
func (re *RemoteEnricher) EnrichNpm(names []string) map[string]string {
	results := make(map[string]string)
	for _, name := range names {
		// Check cache first
		if desc, ok := re.cache.Get(name, "npm", 30*24*time.Hour); ok {
			results[name] = desc
			continue
		}

		desc := re.fetchNpm(name)
		if desc != "" {
			results[name] = desc
			_ = re.cache.Set(name, "npm", desc)
		}
		// Small delay to be polite
		time.Sleep(100 * time.Millisecond)
	}
	return results
}

// EnrichCargo fetches descriptions from crates.io for cargo packages.
func (re *RemoteEnricher) EnrichCargo(names []string) map[string]string {
	results := make(map[string]string)
	for _, name := range names {
		if desc, ok := re.cache.Get(name, "cargo", 30*24*time.Hour); ok {
			results[name] = desc
			continue
		}
		desc := re.fetchCrates(name)
		if desc != "" {
			results[name] = desc
			_ = re.cache.Set(name, "cargo", desc)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return results
}

// EnrichGem fetches descriptions from rubygems.org for gem packages.
func (re *RemoteEnricher) EnrichGem(names []string) map[string]string {
	results := make(map[string]string)
	for _, name := range names {
		if desc, ok := re.cache.Get(name, "gem", 30*24*time.Hour); ok {
			results[name] = desc
			continue
		}
		desc := re.fetchRubyGems(name)
		if desc != "" {
			results[name] = desc
			_ = re.cache.Set(name, "gem", desc)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return results
}

// fetchJSON GETs a URL and unmarshals the body into v. crates.io requires a
// descriptive User-Agent, so we always set one.
func (re *RemoteEnricher) fetchJSON(url string, v any) bool {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "installr (https://github.com/dantevacirca/installr)")
	resp, err := re.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	return json.Unmarshal(body, v) == nil
}

// fetchCrates queries the crates.io API for a crate description.
func (re *RemoteEnricher) fetchCrates(name string) string {
	var data struct {
		Crate struct {
			Description string `json:"description"`
		} `json:"crate"`
	}
	if !re.fetchJSON(fmt.Sprintf("https://crates.io/api/v1/crates/%s", name), &data) {
		return ""
	}
	return data.Crate.Description
}

// fetchRubyGems queries the rubygems.org API for a gem description.
func (re *RemoteEnricher) fetchRubyGems(name string) string {
	var data struct {
		Info    string `json:"info"`
		Summary string `json:"summary"`
	}
	if !re.fetchJSON(fmt.Sprintf("https://rubygems.org/api/v1/gems/%s.json", name), &data) {
		return ""
	}
	if data.Summary != "" {
		return data.Summary
	}
	return data.Info
}

// fetchPyPI queries the PyPI JSON API for a package description.
func (re *RemoteEnricher) fetchPyPI(name string) string {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", name)
	resp, err := re.client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var data struct {
		Info struct {
			Summary string `json:"summary"`
		} `json:"info"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	return data.Info.Summary
}

// fetchNpm queries the npm registry for a package description.
func (re *RemoteEnricher) fetchNpm(name string) string {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", name)
	resp, err := re.client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var data struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	return data.Description
}
