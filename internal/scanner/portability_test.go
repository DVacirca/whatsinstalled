package scanner

import (
	"strings"
	"testing"
)

func TestBinDirCandidates_EnvVars(t *testing.T) {
	env := map[string]string{
		"GOBIN":       "/custom/gobin",
		"GOPATH":      "/gp1:/gp2",
		"CARGO_HOME":  "/cargo",
		"BUN_INSTALL": "/bun",
		"PATH":        "/some/path/dir:/usr/bin",
	}
	got := binDirCandidates("/home/u", func(k string) string { return env[k] })

	want := []string{
		"/custom/gobin",
		"/gp1/bin", "/gp2/bin",
		"/cargo/bin",
		"/bun/bin",
		"/some/path/dir",
		"/home/u/.local/bin",
		"/usr/local/bin",
		"/usr/bin", // core OS dir scanned (ownership filter removes managed)
	}
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("missing candidate %q in %v", w, got)
		}
	}
}

func TestBinDirCandidates_GlobsAndDefaults(t *testing.T) {
	// No env at all: still produce home + system + core dirs and shim globs.
	got := binDirCandidates("/home/u", func(string) string { return "" })
	for _, w := range []string{
		"/home/u/.nvm/versions/node/*/bin",
		"/home/u/.sdkman/candidates/*/current/bin",
		"/opt/homebrew/bin",
		"/snap/bin",
		"/sbin",
	} {
		if !contains(got, w) {
			t.Errorf("missing default candidate %q", w)
		}
	}
}

func TestIsStoreManaged(t *testing.T) {
	managed := []string{
		"/opt/homebrew/Cellar/wget/1.21/bin/wget",
		"/nix/store/abc-hello/bin/hello",
		"/var/lib/snapd/snap/core/123/usr/bin/foo", // contains /snap/
	}
	for _, p := range managed {
		if !isStoreManaged(p) {
			t.Errorf("%q should be store-managed", p)
		}
	}
	for _, p := range []string{"/usr/local/bin/mytool", "/home/u/.local/bin/script"} {
		if isStoreManaged(p) {
			t.Errorf("%q should NOT be store-managed", p)
		}
	}
}

func TestParseDpkgSearch(t *testing.T) {
	out := strings.Join([]string{
		"coreutils: /usr/bin/ls",
		"adduser, passwd: /usr/sbin/adduser",
		"diversion by foo from: /usr/bin/bar",
		"garbage line",
		"",
	}, "\n")
	got := parseDpkgSearch(out)
	for _, w := range []string{"/usr/bin/ls", "/usr/sbin/adduser", "/usr/bin/bar"} {
		if !contains(got, w) {
			t.Errorf("missing owned path %q in %v", w, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d paths, want 3: %v", len(got), got)
	}
}

func TestLinesToSet(t *testing.T) {
	set := linesToSet("/usr/bin/ls\n/usr/lib/\n  /usr/bin/cat  \nrelative/skip\n")
	for _, w := range []string{"/usr/bin/ls", "/usr/lib", "/usr/bin/cat"} {
		if !set[w] {
			t.Errorf("expected %q in set", w)
		}
	}
	if set["relative/skip"] {
		t.Error("relative path should be excluded")
	}
}

func TestExcludedBinDir(t *testing.T) {
	for _, p := range []string{"/mnt", "/mnt/c/Windows/System32", "/mnt/d/tools"} {
		if !excludedBinDir(p) {
			t.Errorf("%q should be excluded", p)
		}
	}
	for _, p := range []string{"/usr/bin", "/home/u/.local/bin", "/mntfoo/bin"} {
		if excludedBinDir(p) {
			t.Errorf("%q should NOT be excluded", p)
		}
	}
}

func TestIsLibraryFile(t *testing.T) {
	for _, n := range []string{"libd3d12.so", "libfoo.so.1.2", "libbar.dylib", "x.dll", "y.a", "z.o"} {
		if !isLibraryFile(n) {
			t.Errorf("%q should be a library file", n)
		}
	}
	for _, n := range []string{"kubectl", "gh", "soffice", "go", "node"} {
		if isLibraryFile(n) {
			t.Errorf("%q should NOT be a library file", n)
		}
	}
}

func TestPathAliases(t *testing.T) {
	cases := map[string]string{
		"/usr/sbin/bridge": "/sbin/bridge",
		"/sbin/bridge":     "/usr/sbin/bridge",
		"/usr/bin/ls":      "/bin/ls",
		"/bin/ls":          "/usr/bin/ls",
	}
	for in, want := range cases {
		got := pathAliases(in)
		if len(got) != 1 || got[0] != want {
			t.Errorf("pathAliases(%q) = %v, want [%q]", in, got, want)
		}
	}
	if a := pathAliases("/home/u/.local/bin/tool"); a != nil {
		t.Errorf("non-system path should have no alias, got %v", a)
	}
}

func TestOwnedAny(t *testing.T) {
	set := map[string]bool{"/sbin/bridge": true}
	if !ownedAny(set, "/usr/sbin/bridge") {
		t.Error("usr-merge alias of an owned path should count as owned")
	}
	if ownedAny(set, "/usr/bin/btrfs") {
		t.Error("unrelated path should not be owned")
	}
}

func TestChunkStrings(t *testing.T) {
	got := chunkStrings([]string{"a", "b", "c", "d", "e"}, 2)
	if len(got) != 3 || len(got[0]) != 2 || len(got[2]) != 1 {
		t.Fatalf("unexpected chunks: %v", got)
	}
}

func TestParsePipLocation(t *testing.T) {
	out := "Name: pip\nVersion: 24.0\nLocation: /usr/lib/python3/dist-packages\nRequires:\n"
	if got := parsePipLocation(out); got != "/usr/lib/python3/dist-packages" {
		t.Errorf("parsePipLocation = %q, want /usr/lib/python3/dist-packages", got)
	}
	if got := parsePipLocation("Name: pip\nVersion: 24.0\n"); got != "" {
		t.Errorf("missing Location should yield empty, got %q", got)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
