package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

// DiagnosticQueryPreset is a saved high-signal search used by the browser UI
// and share reports. Presets deliberately use plain token queries so they can
// run offline against the same QueryDiagnostics index as ad-hoc searches.
type DiagnosticQueryPreset struct {
	Name        string `json:"name"`
	Query       string `json:"query"`
	Description string `json:"description,omitempty"`
}

type DiagnosticQueryPresetResult struct {
	Preset DiagnosticQueryPreset `json:"preset"`
	Hits   []QueryHit            `json:"hits"`
}

func DefaultDiagnosticQueryPresets() []DiagnosticQueryPreset {
	return []DiagnosticQueryPreset{
		{Name: "panic/oops", Query: "panic", Description: "kernel panic and fatal failure lines"},
		{Name: "virtio negotiation", Query: "virtio status", Description: "virtio status and feature negotiation"},
		{Name: "queue setup", Query: "QueueReady", Description: "virtqueue setup and readiness"},
		{Name: "queue notify", Query: "QueueNotify", Description: "guest queue notifications"},
		{Name: "CSR satp", Query: "satp", Description: "address translation mode changes"},
		{Name: "CSR mstatus", Query: "mstatus", Description: "privilege and interrupt state changes"},
		{Name: "trap", Query: "trap", Description: "guest exceptions and interrupts"},
		{Name: "init/rootfs", Query: "root=", Description: "kernel command line / rootfs hints"},
	}
}

func DiagnosticQueryPresetResults(console string, trace string, csrTrace string, events []mem.AccessEvent, anomalies []VirtqueueAnomaly, idx MemoryIndex, perPresetLimit int) []DiagnosticQueryPresetResult {
	if perPresetLimit <= 0 || perPresetLimit > 128 {
		perPresetLimit = 24
	}
	presets := DefaultDiagnosticQueryPresets()
	out := make([]DiagnosticQueryPresetResult, 0, len(presets))
	for _, p := range presets {
		hits := QueryDiagnostics(p.Query, console, trace, csrTrace, events, anomalies, idx, perPresetLimit)
		out = append(out, DiagnosticQueryPresetResult{Preset: p, Hits: hits})
	}
	return out
}

func DiagnosticQueryPresetResultsString(console string, trace string, csrTrace string, events []mem.AccessEvent, anomalies []VirtqueueAnomaly, idx MemoryIndex, perPresetLimit int) string {
	rows := DiagnosticQueryPresetResults(console, trace, csrTrace, events, anomalies, idx, perPresetLimit)
	var b strings.Builder
	b.WriteString("diagnostic query presets\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "\n[%s] query=%q hits=%d\n", r.Preset.Name, r.Preset.Query, len(r.Hits))
		if r.Preset.Description != "" {
			fmt.Fprintf(&b, "  %s\n", r.Preset.Description)
		}
		limit := len(r.Hits)
		if limit > 6 {
			limit = 6
		}
		for _, h := range r.Hits[:limit] {
			loc := h.Source
			if h.Line != 0 {
				loc = fmt.Sprintf("%s:%d", h.Source, h.Line)
			}
			fmt.Fprintf(&b, "  %3d %-22s %s\n", h.Score, loc, h.Text)
		}
		if len(r.Hits) > limit {
			fmt.Fprintf(&b, "  ... %d more hits\n", len(r.Hits)-limit)
		}
	}
	return b.String()
}

// QueryBookmark summarizes a hit in a stable, report-friendly form.
type QueryBookmark struct {
	Label  string `json:"label"`
	Source string `json:"source"`
	Line   int    `json:"line,omitempty"`
	Text   string `json:"text"`
}

func QueryBookmarks(results []DiagnosticQueryPresetResult, maxPerPreset int) []QueryBookmark {
	if maxPerPreset <= 0 || maxPerPreset > 16 {
		maxPerPreset = 4
	}
	out := []QueryBookmark{}
	for _, r := range results {
		limit := len(r.Hits)
		if limit > maxPerPreset {
			limit = maxPerPreset
		}
		for _, h := range r.Hits[:limit] {
			out = append(out, QueryBookmark{Label: r.Preset.Name, Source: h.Source, Line: h.Line, Text: h.Text})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Label == out[j].Label {
			if out[i].Source == out[j].Source {
				return out[i].Line < out[j].Line
			}
			return out[i].Source < out[j].Source
		}
		return out[i].Label < out[j].Label
	})
	return out
}
