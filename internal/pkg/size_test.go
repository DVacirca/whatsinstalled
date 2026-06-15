package pkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathSizeFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := PathSize(f)
	if got == nil || *got != 5 {
		t.Fatalf("PathSize(file) = %v, want 5", got)
	}
}

func TestPathSizeDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	_ = os.Mkdir(sub, 0o755)
	if err := os.WriteFile(filepath.Join(sub, "b"), []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := PathSize(dir)
	if got == nil || *got != 7 { // 3 + 4, recursive
		t.Fatalf("PathSize(dir) = %v, want 7", got)
	}
}

func TestPathSizeMissing(t *testing.T) {
	if got := PathSize(filepath.Join(t.TempDir(), "nope")); got != nil {
		t.Fatalf("PathSize(missing) = %v, want nil", got)
	}
}

func TestGetModTime(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GetModTime(f); got == nil {
		t.Fatal("GetModTime(file) = nil, want a time")
	}
	if got := GetModTime(filepath.Join(dir, "nope")); got != nil {
		t.Fatalf("GetModTime(missing) = %v, want nil", got)
	}
}
