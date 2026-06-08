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
