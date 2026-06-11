package scanner

import "testing"

func TestParseUvToolList(t *testing.T) {
	out := "pytest v9.0.2\n- py.test\n- pytest\nwsl-profiler v0.1.0\n- wsl-profiler\n"
	pkgs := parseUvToolList(out)
	if len(pkgs) != 2 {
		t.Fatalf("want 2 tools, got %d: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "pytest" || pkgs[0].Version != "9.0.2" {
		t.Errorf("tool[0] = %q %q, want pytest 9.0.2", pkgs[0].Name, pkgs[0].Version)
	}
	if pkgs[1].Name != "wsl-profiler" || pkgs[1].Version != "0.1.0" {
		t.Errorf("tool[1] = %q %q, want wsl-profiler 0.1.0", pkgs[1].Name, pkgs[1].Version)
	}
	if pkgs[0].Source != "uv" {
		t.Errorf("source = %q, want uv", pkgs[0].Source)
	}
}

func TestParseGemList(t *testing.T) {
	out := "abbrev (default: 0.1.1)\nrake (13.0.6)\njson (2.6.3, 2.5.1)\n\n"
	pkgs := parseGemList(out, "/var/lib/gems/3.2.0")
	if len(pkgs) != 3 {
		t.Fatalf("want 3 gems, got %d: %+v", len(pkgs), pkgs)
	}
	want := map[string]string{"abbrev": "0.1.1", "rake": "13.0.6", "json": "2.6.3"}
	for _, p := range pkgs {
		if want[p.Name] != p.Version {
			t.Errorf("gem %q = %q, want %q", p.Name, p.Version, want[p.Name])
		}
		if p.Location != "/var/lib/gems/3.2.0" {
			t.Errorf("location = %q", p.Location)
		}
	}
}

func TestParseYarnGlobalList(t *testing.T) {
	out := "yarn global v1.22.22\n" +
		"info \"left-pad@1.3.0\" has binaries:\n" +
		"   - left-pad\n" +
		"info \"@scope/tool@2.1.0\" has binaries:\n" +
		"Done in 0.05s.\n"
	pkgs := parseYarnGlobalList(out)
	if len(pkgs) != 2 {
		t.Fatalf("want 2 pkgs, got %d: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "left-pad" || pkgs[0].Version != "1.3.0" {
		t.Errorf("pkg[0] = %q %q", pkgs[0].Name, pkgs[0].Version)
	}
	if pkgs[1].Name != "@scope/tool" || pkgs[1].Version != "2.1.0" {
		t.Errorf("pkg[1] = %q %q, want @scope/tool 2.1.0", pkgs[1].Name, pkgs[1].Version)
	}
}

func TestSplitAppImageName(t *testing.T) {
	cases := []struct{ in, name, ver string }{
		{"GitKraken-9.10.0.AppImage", "GitKraken", "9.10.0"},
		{"nvim.appimage", "nvim", ""},
		{"Cursor-1.2.3.AppImage", "Cursor", "1.2.3"},
		{"balenaEtcher.AppImage", "balenaEtcher", ""},
	}
	for _, c := range cases {
		name, ver := splitAppImageName(c.in)
		if name != c.name || ver != c.ver {
			t.Errorf("splitAppImageName(%q) = (%q, %q), want (%q, %q)", c.in, name, ver, c.name, c.ver)
		}
	}
}

func TestParsePnpmList(t *testing.T) {
	out := []byte(`[{"dependencies":{"typescript":{"version":"5.4.5"},"eslint":{"version":"9.1.0"}}}]`)
	pkgs := parsePnpmList(out, "global")
	if len(pkgs) != 2 {
		t.Fatalf("want 2 pkgs, got %d: %+v", len(pkgs), pkgs)
	}
	got := map[string]string{}
	for _, p := range pkgs {
		got[p.Name] = p.Version
		if p.Source != "pnpm" || p.Location != "global" {
			t.Errorf("pkg %q has source=%q location=%q", p.Name, p.Source, p.Location)
		}
	}
	if got["typescript"] != "5.4.5" || got["eslint"] != "9.1.0" {
		t.Errorf("versions = %+v", got)
	}
}
