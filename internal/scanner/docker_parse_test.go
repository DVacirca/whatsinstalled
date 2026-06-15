package scanner

import "testing"

func TestParseDockerSize(t *testing.T) {
	cases := map[string]int64{
		"1.23GB": 1_230_000_000,
		"456MB":  456_000_000,
		"789kB":  789_000,
		"12B":    12,
		"1.5 GB": 1_500_000_000, // podman often inserts a space
		"2TB":    2_000_000_000_000,
	}
	for in, want := range cases {
		got := parseDockerSize(in)
		if got == nil {
			t.Fatalf("parseDockerSize(%q) = nil, want %d", in, want)
		}
		if *got != want {
			t.Fatalf("parseDockerSize(%q) = %d, want %d", in, *got, want)
		}
	}

	for _, bad := range []string{"", "   ", "abc", "GiB"} {
		if got := parseDockerSize(bad); got != nil {
			t.Fatalf("parseDockerSize(%q) = %d, want nil", bad, *got)
		}
	}
}

func TestParseDockerCreated(t *testing.T) {
	if got := parseDockerCreated("2024-01-15 10:30:00 +0000 UTC"); got == nil {
		t.Fatal("expected a parsed time for docker CreatedAt format")
	} else if got.Year() != 2024 || got.Month() != 1 || got.Day() != 15 {
		t.Fatalf("parsed wrong date: %v", got)
	}

	for _, bad := range []string{"", "not a date", "yesterday"} {
		if got := parseDockerCreated(bad); got != nil {
			t.Fatalf("parseDockerCreated(%q) = %v, want nil", bad, got)
		}
	}
}
