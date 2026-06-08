package enrich

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"
	"sync"
)

// LocalEnricher fetches descriptions from local system sources.
type LocalEnricher struct {
	// dpkgCache maps binary paths to apt package names.
	dpkgCache map[string]string
	dpkgOnce  sync.Once
}

// NewLocalEnricher creates a local enricher.
func NewLocalEnricher() *LocalEnricher {
	return &LocalEnricher{
		dpkgCache: make(map[string]string),
	}
}

// EnrichBin fetches descriptions for bin packages using whatis and dpkg -S.
// Returns a map of package name -> description.
func (le *LocalEnricher) EnrichBin(names []string) map[string]string {
	results := make(map[string]string)

	// First pass: whatis (bulk, fast)
	whatisResults := le.whatisBatch(names)
	for name, desc := range whatisResults {
		if desc != "" {
			results[name] = desc
		}
	}

	// Second pass: dpkg -S for remaining
	remaining := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := results[name]; !ok {
			remaining = append(remaining, name)
		}
	}

	if len(remaining) > 0 {
		dpkgResults := le.dpkgBatch(remaining)
		for name, desc := range dpkgResults {
			if desc != "" {
				results[name] = desc
			}
		}
	}

	return results
}

// EnrichPip fetches descriptions for pip packages using pip show.
// Returns a map of package name -> description.
func (le *LocalEnricher) EnrichPip(names []string) map[string]string {
	results := make(map[string]string)

	// pip show supports multiple packages at once
	if len(names) == 0 {
		return results
	}

	args := append([]string{"show"}, names...)
	cmd := exec.Command("pip", args...)
	out, err := cmd.Output()
	if err != nil {
		return results
	}

	var currentName, currentDesc string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Name: ") {
			if currentName != "" && currentDesc != "" {
				results[currentName] = currentDesc
			}
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "Name: "))
			currentDesc = ""
		} else if strings.HasPrefix(line, "Summary: ") {
			currentDesc = strings.TrimSpace(strings.TrimPrefix(line, "Summary: "))
		}
	}
	if currentName != "" && currentDesc != "" {
		results[currentName] = currentDesc
	}

	return results
}

// whatisBatch runs whatis for multiple names and returns name -> description.
func (le *LocalEnricher) whatisBatch(names []string) map[string]string {
	results := make(map[string]string)
	if len(names) == 0 {
		return results
	}

	cmd := exec.Command("whatis", names...)
	out, err := cmd.Output()
	if err != nil {
		return results
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "name (section)  - description"
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		// Extract name before " ("
		namePart := parts[0]
		idx := strings.Index(namePart, " (")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(namePart[:idx])
		desc := strings.TrimSpace(parts[1])
		if desc != "" && desc != "nothing appropriate." {
			results[name] = desc
		}
	}

	return results
}

// dpkgBatch maps binary names to apt packages via dpkg -S, then fetches descriptions.
func (le *LocalEnricher) dpkgBatch(names []string) map[string]string {
	results := make(map[string]string)
	if len(names) == 0 {
		return results
	}

	// Build paths for /usr/bin and /usr/local/bin
	paths := make([]string, 0, len(names)*2)
	for _, name := range names {
		paths = append(paths, "/usr/bin/"+name)
		paths = append(paths, "/usr/local/bin/"+name)
	}

	// Run dpkg -S for all paths
	cmd := exec.Command("dpkg", append([]string{"-S"}, paths...)...)
	out, err := cmd.Output()
	if err != nil {
		return results
	}

	// Parse output: "package: /path/to/binary"
	binToPkg := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		pkgName := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		// Extract binary name from path
		binName := path[strings.LastIndex(path, "/")+1:]
		binToPkg[binName] = pkgName
	}

	// Get descriptions for found packages
	if len(binToPkg) == 0 {
		return results
	}

	pkgNames := make([]string, 0, len(binToPkg))
	seen := make(map[string]bool)
	for _, pkg := range binToPkg {
		if !seen[pkg] {
			seen[pkg] = true
			pkgNames = append(pkgNames, pkg)
		}
	}

	// Query dpkg for descriptions
	cmd = exec.Command("dpkg-query", "-W", "-f=${Package}\t${Description}\n")
	cmd.Args = append(cmd.Args, pkgNames...)
	out, err = cmd.Output()
	if err != nil {
		return results
	}

	pkgToDesc := make(map[string]string)
	scanner = bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 2)
		if len(fields) == 2 {
			pkgToDesc[fields[0]] = strings.TrimSpace(fields[1])
		}
	}

	// Map back to binary names
	for binName, pkgName := range binToPkg {
		if desc, ok := pkgToDesc[pkgName]; ok {
			results[binName] = desc
		}
	}

	return results
}

// EnrichSnap gets descriptions for snap packages using snap info.
func (le *LocalEnricher) EnrichSnap(names []string) map[string]string {
	results := make(map[string]string)
	for _, name := range names {
		cmd := exec.Command("snap", "info", name)
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "summary:") {
				results[name] = strings.TrimSpace(strings.TrimPrefix(line, "summary:"))
				break
			}
		}
	}
	return results
}

// EnrichNpm gets descriptions for npm packages using npm info.
func (le *LocalEnricher) EnrichNpm(names []string) map[string]string {
	results := make(map[string]string)
	for _, name := range names {
		cmd := exec.Command("npm", "info", name, "--json")
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		// Extract description from JSON output
		if idx := strings.Index(string(out), `"description":"`); idx >= 0 {
			start := idx + len(`"description":"`)
			end := strings.Index(string(out[start:]), `"`)
			if end >= 0 {
				results[name] = string(out[start : start+end])
			}
		}
	}
	return results
}

// EnrichApt gets descriptions for apt packages using apt show.
func (le *LocalEnricher) EnrichApt(names []string) map[string]string {
	results := make(map[string]string)
	if len(names) == 0 {
		return results
	}

	cmd := exec.Command("apt", append([]string{"show"}, names...)...)
	out, err := cmd.Output()
	if err != nil {
		return results
	}

	var currentName, currentDesc string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Package: ") {
			if currentName != "" && currentDesc != "" {
				results[currentName] = currentDesc
			}
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
			currentDesc = ""
		} else if strings.HasPrefix(line, "Description: ") {
			currentDesc = strings.TrimSpace(strings.TrimPrefix(line, "Description: "))
		}
	}
	if currentName != "" && currentDesc != "" {
		results[currentName] = currentDesc
	}

	return results
}

// EnrichConda gets descriptions for conda packages using conda search.
func (le *LocalEnricher) EnrichConda(names []string) map[string]string {
	// Conda packages already have descriptions from the scanner.
	// This is a fallback for any missing ones.
	results := make(map[string]string)
	for _, name := range names {
		cmd := exec.Command("conda", "search", "--json", name)
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		// Extract summary from JSON if available
		if idx := strings.Index(string(out), `"summary":"`); idx >= 0 {
			start := idx + len(`"summary":"`)
			end := strings.Index(string(out[start:]), `"`)
			if end >= 0 {
				results[name] = string(out[start : start+end])
			}
		}
	}
	return results
}
