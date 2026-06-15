package nlp

import (
	"context"
	"testing"
	"time"

	"whatsinstalled/internal/store"
)

func TestEmbedder(t *testing.T) {
	emb, err := LoadEmbedder()
	if err != nil {
		t.Skipf("embedder not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec, err := emb.Encode(ctx, "python package manager")
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if len(vec) == 0 {
		t.Fatal("empty vector")
	}
	if len(vec) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(vec))
	}
	t.Logf("vector length: %d, first 5: %.4f", len(vec), vec[:5])
}

func TestSemanticSearch(t *testing.T) {
	emb, err := LoadEmbedder()
	if err != nil {
		t.Skipf("embedder not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fake packages
	pkgs := []store.Package{
		{Name: "pip", Source: "pip", Description: "python package installer"},
		{Name: "uv", Source: "pip", Description: "fast python package manager"},
		{Name: "npm", Source: "npm", Description: "node package manager"},
		{Name: "curl", Source: "apt", Description: "transfer data with URLs"},
		{Name: "docker", Source: "apt", Description: "container platform"},
	}

	// Embed query
	queryVec, err := emb.Encode(ctx, "python based tools")
	if err != nil {
		t.Fatalf("encode query: %v", err)
	}

	// Embed packages and score
	var best store.Package
	var bestScore float64
	for _, p := range pkgs {
		text := PackageText(p.Name, p.Source, p.Description)
		vec, err := emb.Encode(ctx, text)
		if err != nil {
			t.Fatalf("encode %s: %v", p.Name, err)
		}
		score := CosineSimilarity(queryVec, vec)
		t.Logf("%s: %.4f", p.Name, score)
		if score > bestScore {
			bestScore = score
			best = p
		}
	}

	if best.Name != "pip" && best.Name != "uv" {
		t.Fatalf("expected python tool, got %s (score %.4f)", best.Name, bestScore)
	}
	t.Logf("best match: %s (%.4f)", best.Name, bestScore)
}
