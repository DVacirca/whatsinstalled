package enrich

import "testing"

func TestParseBrewJSON(t *testing.T) {
	out := []byte(`{
		"formulae": [
			{"name": "ripgrep", "desc": "Search tool like grep"},
			{"name": "noinfo", "desc": ""}
		],
		"casks": [
			{"token": "firefox", "desc": "Web browser"}
		]
	}`)

	got := parseBrewJSON(out)
	if got["ripgrep"] != "Search tool like grep" {
		t.Fatalf("ripgrep = %q", got["ripgrep"])
	}
	if got["firefox"] != "Web browser" {
		t.Fatalf("firefox (cask) = %q", got["firefox"])
	}
	if _, ok := got["noinfo"]; ok {
		t.Fatalf("empty desc should be skipped: %v", got)
	}
}

func TestParseBrewJSONInvalid(t *testing.T) {
	if got := parseBrewJSON([]byte("not json")); len(got) != 0 {
		t.Fatalf("invalid json should yield empty map, got %v", got)
	}
}

func TestParsePacmanInfo(t *testing.T) {
	out := []byte(`Name            : bash
Version         : 5.2.015-1
Description     : The GNU Bourne Again shell

Name            : curl
Version         : 8.1.2-1
Description     : command line tool for transferring data with URLs
`)

	got := parsePacmanInfo(out)
	if got["bash"] != "The GNU Bourne Again shell" {
		t.Fatalf("bash = %q", got["bash"])
	}
	if got["curl"] != "command line tool for transferring data with URLs" {
		t.Fatalf("curl = %q", got["curl"])
	}
}

func TestParsePacmanInfoEmpty(t *testing.T) {
	if got := parsePacmanInfo(nil); len(got) != 0 {
		t.Fatalf("nil input should yield empty map, got %v", got)
	}
}

// TestDescMapUnknownSource verifies sources without an enrichment route return an
// empty map rather than panicking.
func TestDescMapUnknownSource(t *testing.T) {
	e := NewEnricher(nil)
	for _, src := range []string{"docker", "podman", "appimage", "nix", "go", "totally-unknown"} {
		got := e.descMapForSource(src, []string{"x"})
		if len(got) != 0 {
			t.Fatalf("source %q should have no route, got %v", src, got)
		}
	}
}
