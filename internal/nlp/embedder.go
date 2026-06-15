package nlp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nlpodyssey/cybertron/pkg/models/bert"
	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
	"github.com/rs/zerolog"
	"whatsinstalled/internal/pkg"
)

var (
	defaultModel = "sentence-transformers/all-MiniLM-L6-v2"
	modelsDir    string
)

func init() {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
}

// Embedder loads a small sentence-transformer model and encodes text.
type Embedder struct {
	model textencoding.Interface
	mu    sync.RWMutex
}

// LoadEmbedder loads (or downloads) the embedding model. Safe to call
// multiple times — the model is cached on disk.
func LoadEmbedder() (*Embedder, error) {
	home := pkg.HomeDir()
	modelsDir = filepath.Join(home, ".whatsinstalled", "models")

	// Ensure models directory exists
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create models dir: %w", err)
	}

	m, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir: modelsDir,
		ModelName: defaultModel,
	})
	if err != nil {
		return nil, fmt.Errorf("load embedding model: %w", err)
	}

	return &Embedder{model: m}, nil
}

// Encode returns a 384-dimensional embedding vector for the given text.
func (e *Embedder) Encode(ctx context.Context, text string) ([]float64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result, err := e.model.Encode(ctx, text, int(bert.MeanPooling))
	if err != nil {
		return nil, err
	}

	vec := result.Vector.Data().F64()
	return vec, nil
}

// CosineSimilarity returns a score between -1 and 1.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// PackageText returns a rich text representation of a package for embedding.
// It includes source-specific context to help the model understand what kind
// of package this is (e.g., "python package" for pip, "node package" for npm).
func PackageText(name, source, description string) string {
	var sourceContext string
	switch source {
	case "apt":
		sourceContext = "debian system package manager"
	case "snap":
		sourceContext = "ubuntu snap system package"
	case "npm":
		sourceContext = "node javascript package"
	case "pip":
		sourceContext = "python package"
	case "conda":
		sourceContext = "conda python environment package"
	case "bin":
		sourceContext = "user binary tool"
	default:
		sourceContext = "package"
	}

	parts := []string{name, sourceContext}
	if description != "" {
		parts = append(parts, description)
	}
	parts = append(parts, "source:", source)
	return strings.Join(parts, " ")
}

// ToJSON serializes a float64 slice to JSON.
func ToJSON(v []float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// FromJSON deserializes a JSON string to a float64 slice.
func FromJSON(s string) ([]float64, error) {
	var v []float64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}
