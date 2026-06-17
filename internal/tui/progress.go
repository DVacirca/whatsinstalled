package tui

import (
	"fmt"
	"strings"
)

// applyProgress folds one streamed scanProgressMsg into the splash state:
// the active phase, the per-source scan checklist, and the header line.
func (m *model) applyProgress(msg scanProgressMsg) {
	// Keep the background corner indicator fed (used during a cached refresh).
	m.scanSource = msg.source
	m.scanCount = msg.count

	switch {
	case msg.source == "scan":
		m.initStep = "scan"
		m.scanTotal = msg.count
		m.initProgress = "Scanning packages…"
	case msg.source == "enrich":
		m.initStep = "enrich"
		m.enrichTotal = msg.count
		m.initProgress = fmt.Sprintf("Enriching descriptions… %d packages", msg.count)
	case strings.HasPrefix(msg.source, "enrich:"):
		m.initStep = "enrich"
		src := strings.TrimPrefix(msg.source, "enrich:")
		if m.enrichTotal > 0 {
			m.initProgress = fmt.Sprintf("Enriching · %s %d/%d", src, msg.count, m.enrichTotal)
		} else {
			m.initProgress = fmt.Sprintf("Enriching · %s %d", src, msg.count)
		}
	case msg.source == "embed":
		m.initStep = "embed"
		if m.embedTotal == 0 {
			m.embedTotal = msg.count
			m.initProgress = fmt.Sprintf("Computing embeddings… %d packages", msg.count)
		} else {
			pct := float64(msg.count) / float64(m.embedTotal) * 100
			m.initProgress = fmt.Sprintf("Computing embeddings · %d/%d (%.0f%%)", msg.count, m.embedTotal, pct)
		}
	case msg.source != "":
		m.recordScan(msg.source, msg.count)
	case msg.count > 0:
		m.initProgress = fmt.Sprintf("Using cached data… %d packages", msg.count)
	}
}

// recordScan updates the per-source checklist. The first message for a source
// marks it in-progress; the second carries its final package count.
func (m *model) recordScan(name string, count int) {
	m.initStep = "scan"
	if m.scanIndex == nil {
		m.scanIndex = make(map[string]int)
	}
	if idx, ok := m.scanIndex[name]; ok {
		m.scanStatus[idx].count = count
		m.scanStatus[idx].done = true
		m.totalFound += count
	} else {
		m.scanIndex[name] = len(m.scanStatus)
		m.scanStatus = append(m.scanStatus, scanEntry{name: name})
	}
	done := 0
	for _, e := range m.scanStatus {
		if e.done {
			done++
		}
	}
	m.initProgress = fmt.Sprintf("Scanning · %d/%d sources · %d packages", done, m.scanTotal, m.totalFound)
}

// finishInit clears all transient init state once the pipeline completes.
func (m *model) finishInit() {
	m.scanning = false
	m.bgUpdating = false
	m.scanSource = ""
	m.scanCount = 0
	m.totalFound = 0
	m.scanTotal = 0
	m.scanStatus = nil
	m.scanIndex = nil
	m.enrichTotal = 0
	m.embedTotal = 0
	m.initStep = ""
	m.initProgress = ""
	m.initCh = nil
}
