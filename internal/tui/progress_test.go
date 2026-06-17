package tui

import (
	"strings"
	"testing"
)

func TestApplyProgress_ScanChecklist(t *testing.T) {
	m := &model{}
	m.applyProgress(scanProgressMsg{source: "scan", count: 3})
	m.applyProgress(scanProgressMsg{source: "apt"})            // apt starts
	m.applyProgress(scanProgressMsg{source: "apt", count: 90}) // apt done
	m.applyProgress(scanProgressMsg{source: "npm"})            // npm starts
	m.applyProgress(scanProgressMsg{source: "npm", count: 14}) // npm done

	if m.scanTotal != 3 {
		t.Errorf("scanTotal = %d, want 3", m.scanTotal)
	}
	if len(m.scanStatus) != 2 {
		t.Fatalf("scanStatus len = %d, want 2", len(m.scanStatus))
	}
	if e := m.scanStatus[0]; e.name != "apt" || e.count != 90 || !e.done {
		t.Errorf("scanStatus[0] = %+v, want apt/90/done", e)
	}
	if m.totalFound != 104 {
		t.Errorf("totalFound = %d, want 104", m.totalFound)
	}
	if !strings.Contains(m.initProgress, "2/3 sources") || !strings.Contains(m.initProgress, "104 packages") {
		t.Errorf("initProgress = %q, want 2/3 sources and 104 packages", m.initProgress)
	}
}

func TestApplyProgress_Phases(t *testing.T) {
	m := &model{}
	m.applyProgress(scanProgressMsg{source: "enrich", count: 40})
	if m.initStep != "enrich" || m.enrichTotal != 40 {
		t.Errorf("enrich header not applied: step=%q total=%d", m.initStep, m.enrichTotal)
	}
	m.applyProgress(scanProgressMsg{source: "enrich:pip", count: 12})
	if !strings.Contains(m.initProgress, "pip 12/40") {
		t.Errorf("initProgress = %q, want pip 12/40", m.initProgress)
	}

	m.applyProgress(scanProgressMsg{source: "embed", count: 50}) // total
	m.applyProgress(scanProgressMsg{source: "embed", count: 25}) // half
	if m.initStep != "embed" || !strings.Contains(m.initProgress, "25/50") || !strings.Contains(m.initProgress, "50%") {
		t.Errorf("embed progress wrong: step=%q msg=%q", m.initStep, m.initProgress)
	}
}

func TestFinishInit(t *testing.T) {
	m := &model{scanning: true, initStep: "embed", totalFound: 100, scanStatus: []scanEntry{{name: "apt"}}}
	m.scanIndex = map[string]int{"apt": 0}
	m.finishInit()
	if m.scanning || m.initStep != "" || m.scanStatus != nil || m.scanIndex != nil || m.totalFound != 0 {
		t.Errorf("finishInit did not clear state: %+v", m)
	}
}

func TestRenderSplash_Smoke(t *testing.T) {
	m := model{
		spinnerFrame: 0,
		initStep:     "scan",
		initProgress: "Scanning · 1/3 sources · 90 packages",
		scanStatus:   []scanEntry{{name: "apt", count: 90, done: true}, {name: "npm"}},
	}
	out := m.renderSplash()
	for _, want := range []string{"whatsinstalled", "Scanning", "apt", "Scan", "Enrich", "Embed"} {
		if !strings.Contains(out, want) {
			t.Errorf("splash missing %q\n%s", want, out)
		}
	}
}
