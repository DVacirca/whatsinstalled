package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParsePyMetadata checks header extraction stops at the blank line that
// precedes the long description (so description text can't spoof headers).
func TestParsePyMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "METADATA")
	content := "Metadata-Version: 2.1\n" +
		"Name: requests\n" +
		"Version: 2.31.0\n" +
		"Summary: Python HTTP for Humans.\n" +
		"\n" +
		"Name: not-a-real-package\n" + // in the body — must be ignored
		"Long description here.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	name, version, summary := parsePyMetadata(path)
	if name != "requests" {
		t.Fatalf("name = %q, want requests", name)
	}
	if version != "2.31.0" {
		t.Fatalf("version = %q, want 2.31.0", version)
	}
	if summary != "Python HTTP for Humans." {
		t.Fatalf("summary = %q", summary)
	}
}

// TestScanVenvMetadataDoesNotExecutePip is the security regression for the
// arbitrary-code-execution finding: a venv carries a hostile bin/pip, and the
// scanner must read package metadata from disk WITHOUT running it.
func TestScanVenvMetadataDoesNotExecutePip(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "proj")
	venv := filepath.Join(proj, ".venv")
	site := filepath.Join(venv, "lib", "python3.11", "site-packages")
	if err := os.MkdirAll(filepath.Join(site, "requests-2.31.0.dist-info"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := "Name: requests\nVersion: 2.31.0\nSummary: HTTP for Humans.\n\nbody\n"
	if err := os.WriteFile(filepath.Join(site, "requests-2.31.0.dist-info", "METADATA"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}

	// A malicious pip that would drop a marker file if it were ever executed.
	marker := filepath.Join(tmp, "pwned")
	if err := os.MkdirAll(filepath.Join(venv, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	evil := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(venv, "bin", "pip"), []byte(evil), 0o777); err != nil {
		t.Fatal(err)
	}

	pkgs := PipScanner{}.scanVenvMetadata(venv, proj)

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("malicious venv pip was executed — scanner must read metadata only")
	}
	if len(pkgs) != 1 {
		t.Fatalf("got %d packages, want 1", len(pkgs))
	}
	p := pkgs[0]
	if p.Name != "requests" || p.Version != "2.31.0" || p.Source != "pip" || p.Location != proj {
		t.Fatalf("unexpected package: %+v", p)
	}
	if p.Description != "HTTP for Humans." {
		t.Fatalf("description = %q", p.Description)
	}
}

// TestScanVenvMetadataIgnoresNonVenv ensures a plain directory yields nothing.
func TestScanVenvMetadataIgnoresNonVenv(t *testing.T) {
	dir := t.TempDir()
	if pkgs := (PipScanner{}).scanVenvMetadata(dir, dir); pkgs != nil {
		t.Fatalf("expected nil for non-venv dir, got %d packages", len(pkgs))
	}
}
