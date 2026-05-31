package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
)

// SmokeMatrixDiff summarizes how a smoke matrix changed relative to a saved baseline.
type SmokeMatrixDiff struct {
	Preset         string `json:"preset"`
	Kind           string `json:"kind"`
	BeforePhase    string `json:"before_phase,omitempty"`
	AfterPhase     string `json:"after_phase,omitempty"`
	BeforeTopCause string `json:"before_top_cause,omitempty"`
	AfterTopCause  string `json:"after_top_cause,omitempty"`
	BeforeRan      int    `json:"before_ran,omitempty"`
	AfterRan       int    `json:"after_ran,omitempty"`
	DeltaRan       int    `json:"delta_ran,omitempty"`
}

func CompareSmokeSummaries(current, baseline []SmokeSummary) []SmokeMatrixDiff {
	byBase := map[string]SmokeSummary{}
	seen := map[string]bool{}
	for _, b := range baseline {
		byBase[b.Preset] = b
	}
	rows := []SmokeMatrixDiff{}
	for _, c := range current {
		seen[c.Preset] = true
		b, ok := byBase[c.Preset]
		if !ok {
			rows = append(rows, SmokeMatrixDiff{Preset: c.Preset, Kind: "added", AfterPhase: c.Phase, AfterTopCause: c.TopCause, AfterRan: c.Ran})
			continue
		}
		kind := "unchanged"
		if b.Phase != c.Phase || b.TopCause != c.TopCause || b.Ran != c.Ran {
			kind = "changed"
		}
		rows = append(rows, SmokeMatrixDiff{Preset: c.Preset, Kind: kind, BeforePhase: b.Phase, AfterPhase: c.Phase, BeforeTopCause: b.TopCause, AfterTopCause: c.TopCause, BeforeRan: b.Ran, AfterRan: c.Ran, DeltaRan: c.Ran - b.Ran})
	}
	for _, b := range baseline {
		if !seen[b.Preset] {
			rows = append(rows, SmokeMatrixDiff{Preset: b.Preset, Kind: "removed", BeforePhase: b.Phase, BeforeTopCause: b.TopCause, BeforeRan: b.Ran})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		order := map[string]int{"changed": 0, "added": 1, "removed": 2, "unchanged": 3}
		if order[rows[i].Kind] == order[rows[j].Kind] {
			return rows[i].Preset < rows[j].Preset
		}
		return order[rows[i].Kind] < order[rows[j].Kind]
	})
	return rows
}

func SmokeMatrixDiffString(rows []SmokeMatrixDiff) string {
	if len(rows) == 0 {
		return "<no smoke matrix diff>\n"
	}
	var b strings.Builder
	b.WriteString("smoke matrix baseline diff\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%-12s %-9s ran=%d->%d (%+d) phase=%s->%s\n", r.Preset, r.Kind, r.BeforeRan, r.AfterRan, r.DeltaRan, firstNonEmpty(r.BeforePhase, "-"), firstNonEmpty(r.AfterPhase, "-"))
		if r.BeforeTopCause != r.AfterTopCause {
			fmt.Fprintf(&b, "  cause: %s -> %s\n", firstNonEmpty(r.BeforeTopCause, "-"), firstNonEmpty(r.AfterTopCause, "-"))
		}
	}
	return b.String()
}

func SmokeSummaryMarkdown(rows []SmokeSummary) string {
	var b strings.Builder
	b.WriteString("# rvwasm smoke matrix\n\n")
	if len(rows) == 0 {
		b.WriteString("_no smoke results_\n")
		return b.String()
	}
	b.WriteString("| Preset | Ran | Requested | Phase | Top cause | Status |\n|---|---:|---:|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %d | %d | %s | %s | %s |\n", mdCell(r.Preset), r.Ran, r.Requested, mdCell(r.Phase), mdCell(r.TopCause), mdCell(truncate(r.Status, 160)))
	}
	return b.String()
}

func SmokeSummaryHTML(rows []SmokeSummary) string {
	md := SmokeSummaryMarkdown(rows)
	j, _ := json.MarshalIndent(rows, "", "  ")
	return "<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm smoke matrix</title>" +
		"<style>body{font-family:system-ui,sans-serif;margin:24px;background:#101216;color:#eee}pre{white-space:pre-wrap;background:#05070a;border:1px solid #333;padding:16px;border-radius:8px;overflow:auto}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>" +
		"<h1>rvwasm smoke matrix</h1><pre>" + html.EscapeString(md) + "</pre><h2>JSON</h2><pre>" + html.EscapeString(string(j)) + "</pre>"
}

type ChecklistItem struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Action   string `json:"action"`
	Query    string `json:"query,omitempty"`
	Done     bool   `json:"done"`
}

func StopCauseChecklist(rows []StopCauseCandidate) []ChecklistItem {
	out := []ChecklistItem{}
	add := func(c StopCauseCandidate, action, query string) {
		out = append(out, ChecklistItem{Category: c.Category, Severity: c.Severity, Action: action, Query: query})
	}
	for _, c := range rows {
		switch c.Category {
		case "kernel-panic", "kernel-oops":
			add(c, "Open Panic summary and symbol-resolve the first faulting address", "panic oops call trace")
			add(c, "Open DWARF source context for the current PC and first Call Trace PC", "pc= call trace")
		case "illegal-instruction":
			add(c, "Inspect annotated trace around the illegal instruction PC", "illegal asm=")
			add(c, "Verify ISA extension and CSR legality for the decoded instruction", "CSR illegal")
		case "page-fault":
			add(c, "Compare satp/stval/scause and check the page-table memory range", "satp stval page fault")
			add(c, "Dump memory around the faulting virtual/physical address candidate", "page fault")
		case "access-fault":
			add(c, "Check PMP entries and physical memory map for the faulting address", "access fault PMP")
			add(c, "Add a read/write watchpoint around the failing physical range", "watchpoint access")
		case "virtqueue":
			add(c, "Open Virtqueue anomaly triage and descriptor chain DOT for the failing queue", "virtqueue descriptor")
			add(c, "Check QueueReady, QueueNum, QueueDesc, QueueDriver, QueueDevice, and QueueNotify order", "QueueReady QueueNotify")
		case "device-probe", "device-idle":
			add(c, "Open Device probe and Decoded MMIO to verify feature negotiation", "DeviceID Status QueueReady")
		default:
			add(c, firstNonEmpty(c.NextAction, "Inspect trace, CSR trace, and decoded MMIO around the stop point"), strings.Join(SuggestedQueriesForStopCause(c), " "))
		}
	}
	// Deduplicate while preserving order.
	seen := map[string]bool{}
	compact := out[:0]
	for _, it := range out {
		key := it.Category + "\x00" + it.Action
		if seen[key] {
			continue
		}
		seen[key] = true
		compact = append(compact, it)
	}
	return compact
}

func StopCauseChecklistString(rows []StopCauseCandidate) string {
	items := StopCauseChecklist(rows)
	if len(items) == 0 {
		return "<no stop-cause checklist>\n"
	}
	var b strings.Builder
	b.WriteString("stop-cause checklist\n")
	for _, it := range items {
		fmt.Fprintf(&b, "[ ] %-8s %-18s %s\n", it.Severity, it.Category, it.Action)
		if it.Query != "" {
			fmt.Fprintf(&b, "    query: %s\n", it.Query)
		}
	}
	return b.String()
}

type QueryBookmarkSet struct {
	Query     string     `json:"query"`
	CSRHits   []QueryHit `json:"csr_hits,omitempty"`
	MMIOHits  []QueryHit `json:"mmio_hits,omitempty"`
	TraceHits []QueryHit `json:"trace_hits,omitempty"`
}

func BuildQueryBookmarkSet(query string, hits []QueryHit, perSource int) QueryBookmarkSet {
	if perSource <= 0 || perSource > 32 {
		perSource = 8
	}
	out := QueryBookmarkSet{Query: strings.TrimSpace(query)}
	add := func(dst *[]QueryHit, h QueryHit) {
		if len(*dst) < perSource {
			*dst = append(*dst, h)
		}
	}
	for _, h := range hits {
		switch h.Source {
		case "csr":
			add(&out.CSRHits, h)
		case "mmio":
			add(&out.MMIOHits, h)
		case "trace":
			add(&out.TraceHits, h)
		}
	}
	return out
}

func QueryBookmarkSetString(s QueryBookmarkSet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "query bookmarks for %q\n", s.Query)
	writeHits := func(title string, rows []QueryHit) {
		fmt.Fprintf(&b, "\n[%s]\n", title)
		if len(rows) == 0 {
			b.WriteString("  <none>\n")
			return
		}
		for _, h := range rows {
			loc := h.Source
			if h.Line != 0 {
				loc = fmt.Sprintf("%s:%d", h.Source, h.Line)
			}
			fmt.Fprintf(&b, "  %3d %-16s %s\n", h.Score, loc, truncate(h.Text, 180))
		}
	}
	writeHits("CSR", s.CSRHits)
	writeHits("MMIO", s.MMIOHits)
	writeHits("TRACE", s.TraceHits)
	return b.String()
}

type ArtifactEntry struct {
	Role     string `json:"role"`
	Bytes    int    `json:"bytes"`
	LoadAddr uint64 `json:"load_addr,omitempty"`
	EndAddr  uint64 `json:"end_addr,omitempty"`
	Entry    uint64 `json:"entry,omitempty"`
	ELF      bool   `json:"elf,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Note     string `json:"note,omitempty"`
}

type ArtifactManifest struct {
	BootArgs        string          `json:"boot_args"`
	HartCount       int             `json:"hart_count"`
	NextAddr        uint64          `json:"next_addr"`
	DTBAddr         uint64          `json:"dtb_addr"`
	DynamicInfoAddr uint64          `json:"dynamic_info_addr"`
	Artifacts       []ArtifactEntry `json:"artifacts"`
}

func NewArtifactEntry(role string, data []byte, loadAddr, entry uint64, elf bool, note string) ArtifactEntry {
	sum := sha256.Sum256(data)
	end := loadAddr
	if len(data) != 0 {
		end = loadAddr + uint64(len(data)) - 1
	}
	return ArtifactEntry{Role: role, Bytes: len(data), LoadAddr: loadAddr, EndAddr: end, Entry: entry, ELF: elf, SHA256: hex.EncodeToString(sum[:]), Note: note}
}

func ArtifactManifestString(m ArtifactManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "boot artifact manifest harts=%d next=%#x dtb=%#x dynamic=%#x\n", m.HartCount, m.NextAddr, m.DTBAddr, m.DynamicInfoAddr)
	fmt.Fprintf(&b, "bootargs: %s\n", m.BootArgs)
	if len(m.Artifacts) == 0 {
		b.WriteString("<no loaded artifacts recorded>\n")
		return b.String()
	}
	for _, a := range m.Artifacts {
		fmt.Fprintf(&b, "%-10s bytes=%-10d range=%#x..%#x entry=%#x elf=%v sha256=%s", a.Role, a.Bytes, a.LoadAddr, a.EndAddr, a.Entry, a.ELF, shortHash(a.SHA256))
		if a.Note != "" {
			fmt.Fprintf(&b, " %s", a.Note)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func shortHash(s string) string {
	if len(s) > 16 {
		return s[:16]
	}
	return s
}

func ArtifactManifestJSON(m ArtifactManifest) string {
	b, _ := json.MarshalIndent(m, "", "  ")
	return string(b)
}
