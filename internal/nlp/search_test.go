package nlp

import (
	"strings"
	"testing"

	"whatsinstalled/internal/store"
)

func TestExpandQuery(t *testing.T) {
	tests := []struct {
		query    string
		contains []string
	}{
		{"networking tools", []string{"curl", "wget", "dig", "openssl"}},
		{"python packages", []string{"pip", "numpy", "pandas"}},
		{"web development", []string{"nodejs", "react", "webpack"}},
		{"database tools", []string{"postgres", "redis", "mongodb"}},
		{"container orchestration", []string{"docker", "kubernetes", "kubectl"}},
		{"security audit", []string{"openssl", "nmap", "metasploit"}},
		{"git workflow", []string{"git", "github", "gitlab"}},
		{"build system", []string{"cmake", "bazel", "gradle"}},
		{"text editor", []string{"vim", "emacs", "vscode"}},
		{"unit testing", []string{"pytest", "jest", "cypress"}},
	}

	for _, tt := range tests {
		expanded := ExpandQuery(tt.query)
		for _, term := range tt.contains {
			if !containsWord(expanded, term) {
				t.Errorf("ExpandQuery(%q) missing %q", tt.query, term)
			}
		}
	}
}

func containsWord(s, word string) bool {
	return len(s) > 0 && len(word) > 0 &&
		(s == word ||
			len(s) > len(word) &&
				(s[:len(word)+1] == word+" " ||
					s[len(s)-len(word)-1:] == " "+word ||
					strings.Contains(s, " "+word+" ") ||
					strings.HasSuffix(s, " "+word)))
}

func TestKeywordScore(t *testing.T) {
	// dig should get a high score for "networking tools"
	dig := store.Package{Name: "dig", Source: "apt", Description: "DNS lookup utility"}
	scoreDig := KeywordScore("networking tools", dig)
	if scoreDig < 0.2 {
		t.Errorf("Expected dig to score > 0.2 for 'networking tools', got %.2f", scoreDig)
	}

	// curl should also get a high score
	curl := store.Package{Name: "curl", Source: "apt", Description: "transfer data with URLs"}
	scoreCurl := KeywordScore("networking tools", curl)
	if scoreCurl < 0.2 {
		t.Errorf("Expected curl to score > 0.2 for 'networking tools', got %.2f", scoreCurl)
	}

	// openssl should also get a high score
	openssl := store.Package{Name: "openssl", Source: "apt", Description: "Secure Sockets Layer toolkit"}
	scoreOpenssl := KeywordScore("networking tools", openssl)
	if scoreOpenssl < 0.2 {
		t.Errorf("Expected openssl to score > 0.2 for 'networking tools', got %.2f", scoreOpenssl)
	}

	// pip should NOT get a high score for "networking tools"
	pip := store.Package{Name: "pip", Source: "pip", Description: "python package installer"}
	scorePip := KeywordScore("networking tools", pip)
	if scorePip > 0.1 {
		t.Errorf("Expected pip to score <= 0.1 for 'networking tools', got %.2f", scorePip)
	}

	// pip SHOULD get a high score for "python tools"
	scorePipPython := KeywordScore("python tools", pip)
	if scorePipPython < 0.2 {
		t.Errorf("Expected pip to score > 0.2 for 'python tools', got %.2f", scorePipPython)
	}
}

func TestKeywordScoreExactMatch(t *testing.T) {
	pkg := store.Package{Name: "curl", Source: "apt", Description: "transfer data"}
	score := KeywordScore("curl", pkg)
	if score < 0.5 {
		t.Errorf("Expected exact name match to score >= 0.5, got %.2f", score)
	}
}
