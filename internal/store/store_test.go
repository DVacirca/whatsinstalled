package store

import (
	"testing"
	"time"
)

func TestOpen(t *testing.T) {
	db := t.TempDir() + "/test.db"
	s, err := Open(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()
}

func TestUpsertAndList(t *testing.T) {
	db := t.TempDir() + "/test.db"
	s, err := Open(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	p := Package{
		Name:        "nginx",
		Version:     "1.24.0",
		Source:      "apt",
		Location:    "system",
		Description: "web server",
		AutoInstalled: false,
	}
	if err := s.Upsert(p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	pkgs, err := s.List("", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Name != "nginx" {
		t.Errorf("expected nginx, got %s", pkgs[0].Name)
	}
}

func TestSearch(t *testing.T) {
	db := t.TempDir() + "/test.db"
	s, err := Open(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	_ = s.Upsert(Package{Name: "nginx", Source: "apt", Location: "system"})
	_ = s.Upsert(Package{Name: "node", Source: "snap", Location: "system"})
	_ = s.Upsert(Package{Name: "lodash", Source: "npm", Location: "~/proj"})

	pkgs, err := s.Search("ng", "", false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "nginx" {
		t.Fatalf("expected nginx, got %+v", pkgs)
	}

	pkgs, err = s.Search("n", "apt", false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "nginx" {
		t.Fatalf("expected nginx in apt, got %+v", pkgs)
	}
}

func TestDelete(t *testing.T) {
	db := t.TempDir() + "/test.db"
	s, err := Open(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	_ = s.Upsert(Package{Name: "nginx", Source: "apt", Location: "system"})
	if err := s.Delete("nginx", "apt", "system"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	pkgs, _ := s.List("", false)
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages after delete, got %d", len(pkgs))
	}
}

func TestPurgeStale(t *testing.T) {
	db := t.TempDir() + "/test.db"
	s, err := Open(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	_ = s.Upsert(Package{Name: "old", Source: "apt", Location: "system"})
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()
	_ = s.Upsert(Package{Name: "new", Source: "apt", Location: "system"})
	if err := s.PurgeStale(cutoff); err != nil {
		t.Fatalf("purge stale: %v", err)
	}
	pkgs, _ := s.List("", false)
	if len(pkgs) != 1 || pkgs[0].Name != "new" {
		t.Fatalf("expected only 'new' after purge, got %+v", pkgs)
	}
}

func TestCountBySource(t *testing.T) {
	db := t.TempDir() + "/test.db"
	s, err := Open(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	_ = s.Upsert(Package{Name: "a", Source: "apt", Location: "system"})
	_ = s.Upsert(Package{Name: "b", Source: "apt", Location: "system"})
	_ = s.Upsert(Package{Name: "c", Source: "npm", Location: "~"})

	counts, total, err := s.CountBySource(false)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if counts["apt"] != 2 {
		t.Errorf("expected apt=2, got %d", counts["apt"])
	}
	if counts["npm"] != 1 {
		t.Errorf("expected npm=1, got %d", counts["npm"])
	}
}
