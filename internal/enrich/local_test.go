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

func TestParseGemDetails(t *testing.T) {
	out := []byte(`abbrev (0.1.1)
    Author: Akinori MUSHO
    Homepage: https://github.com/ruby/abbrev
    Licenses: Ruby, BSD-2-Clause
    Installed at (default): /usr/lib/ruby/gems/3.2.0

    Calculates a set of unique abbreviations for a given set of strings

base64 (0.1.1)
    Author: Yusuke Endoh
    Homepage: https://github.com/ruby/base64
    Licenses: Ruby, BSD-2-Clause
    Installed at (default): /usr/lib/ruby/gems/3.2.0

    Support for encoding and decoding binary data using a Base64 representation

rake (13.0.6)
    Authors: Hiroshi SHIBATA, Eric Hodel, Jim Weirich
    Homepage: https://github.com/ruby/rake
    Installed at: /usr/lib/ruby/gems/3.2.0

    Rake is a Make-like program implemented in Ruby
`)

	got := parseGemDetails(out)
	if got["abbrev"] != "Calculates a set of unique abbreviations for a given set of strings" {
		t.Fatalf("abbrev = %q", got["abbrev"])
	}
	if got["base64"] != "Support for encoding and decoding binary data using a Base64 representation" {
		t.Fatalf("base64 = %q", got["base64"])
	}
	if got["rake"] != "Rake is a Make-like program implemented in Ruby" {
		t.Fatalf("rake = %q", got["rake"])
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 gems, got %d: %v", len(got), got)
	}
}

func TestParseGemDetailsEmpty(t *testing.T) {
	if got := parseGemDetails([]byte("")); len(got) != 0 {
		t.Fatalf("empty input = %v", got)
	}
	if got := parseGemDetails([]byte("no gems here\n")); len(got) != 0 {
		t.Fatalf("no-match input = %v", got)
	}
}
