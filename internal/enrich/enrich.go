package enrich

import (
	"fmt"
	"sync"

	"whatsinstalled/internal/store"
)

// Progress represents a single enrichment progress event.
type Progress struct {
	Total   int
	Done    int
	Source  string
	Current string
	Desc    string
}

// ProgressCallback is called during enrichment to report progress.
type ProgressCallback func(total, done int, source, current, desc string)

// Enricher coordinates enrichment from local and remote sources.
type Enricher struct {
	local  *LocalEnricher
	remote *RemoteEnricher
	cache  *Cache
}

// NewEnricher creates an enricher with the given cache.
func NewEnricher(cache *Cache) *Enricher {
	return &Enricher{
		local:  NewLocalEnricher(),
		remote: NewRemoteEnricher(cache, false),
		cache:  cache,
	}
}

// EnrichPackages fetches descriptions for packages that are missing them.
// It tries local sources first, then falls back to remote APIs.
// Progress is reported via the callback.
func (e *Enricher) EnrichPackages(pkgs []store.Package, onProgress ProgressCallback) ([]store.Package, error) {
	// Group packages by source
	bySource := make(map[string][]store.Package)
	for i := range pkgs {
		p := pkgs[i]
		if p.Description == "" {
			bySource[p.Source] = append(bySource[p.Source], p)
		}
	}

	if len(bySource) == 0 {
		return pkgs, nil
	}

	// Track total and progress
	total := 0
	for _, group := range bySource {
		total += len(group)
	}

	done := 0
	var mu sync.Mutex
	updateProgress := func(source, current, desc string) {
		mu.Lock()
		done++
		mu.Unlock()
		if onProgress != nil {
			onProgress(total, done, source, current, desc)
		}
	}

	// Enrich each source concurrently
	var wg sync.WaitGroup
	results := make(map[string]string) // key: "name:source:location" -> description
	var resultsMu sync.Mutex

	for source, group := range bySource {
		wg.Add(1)
		go func(src string, pkgs []store.Package) {
			defer wg.Done()
			e.enrichSource(src, pkgs, results, &resultsMu, updateProgress)
		}(source, group)
	}

	wg.Wait()

	// Count how many descriptions we found
	for i := range pkgs {
		p := pkgs[i]
		key := fmt.Sprintf("%s:%s:%s", p.Name, p.Source, p.Location)
		if desc, ok := results[key]; ok && desc != "" {
			pkgs[i].Description = desc
		}
	}

	return pkgs, nil
}

// enrichSource enriches packages for a single source.
func (e *Enricher) enrichSource(
	source string,
	pkgs []store.Package,
	results map[string]string,
	mu *sync.Mutex,
	onProgress func(source, current, desc string),
) {
	// Extract names
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}

	descMap := e.descMapForSource(source, names)

	for _, p := range pkgs {
		desc := descMap[p.Name]
		key := fmt.Sprintf("%s:%s:%s", p.Name, p.Source, p.Location)
		mu.Lock()
		results[key] = desc
		mu.Unlock()
		onProgress(source, p.Name, desc)
	}
}

// descMapForSource resolves descriptions for a group of package names from a
// single source, trying local commands first and falling back to remote
// registries where one exists. Sources that map to the same registry share a
// resolver (pipx/uv → PyPI, pnpm/yarn → npm, yay → pacman).
func (e *Enricher) descMapForSource(source string, names []string) map[string]string {
	switch source {
	case "bin":
		return e.local.EnrichBin(names)
	case "apt":
		return e.local.EnrichApt(names)
	case "snap":
		return e.local.EnrichSnap(names)
	case "conda":
		return e.local.EnrichConda(names)
	case "brew":
		return e.local.EnrichBrew(names)
	case "pacman", "yay":
		return e.local.EnrichPacman(names)
	case "pip", "pipx", "uv":
		descMap := e.local.EnrichPip(names)
		if remaining := e.remainingNames(names, descMap); len(remaining) > 0 {
			for k, v := range e.remote.EnrichPip(remaining) {
				descMap[k] = v
			}
		}
		return descMap
	case "npm", "pnpm", "yarn":
		descMap := e.local.EnrichNpm(names)
		if remaining := e.remainingNames(names, descMap); len(remaining) > 0 {
			for k, v := range e.remote.EnrichNpm(remaining) {
				descMap[k] = v
			}
		}
		return descMap
	case "cargo":
		return e.remote.EnrichCargo(names)
	case "gem":
		descMap := e.local.EnrichGem(names)
		if remaining := e.remainingNames(names, descMap); len(remaining) > 0 {
			for k, v := range e.remote.EnrichGem(remaining) {
				descMap[k] = v
			}
		}
		return descMap
	default:
		return map[string]string{}
	}
}

// remainingNames returns names that are not in the results map.
func (e *Enricher) remainingNames(names []string, results map[string]string) []string {
	var remaining []string
	for _, name := range names {
		if _, ok := results[name]; !ok {
			remaining = append(remaining, name)
		}
	}
	return remaining
}

// CountMissing returns the number of packages without descriptions.
func CountMissing(pkgs []store.Package) int {
	count := 0
	for _, p := range pkgs {
		if p.Description == "" {
			count++
		}
	}
	return count
}

// FilterMissing returns only packages without descriptions.
func FilterMissing(pkgs []store.Package) []store.Package {
	var missing []store.Package
	for _, p := range pkgs {
		if p.Description == "" {
			missing = append(missing, p)
		}
	}
	return missing
}
