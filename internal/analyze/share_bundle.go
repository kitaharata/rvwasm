package analyze

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

// ShareBundle is a compact text bundle meant for bug reports and collaborator handoff.
type ShareBundle struct {
	Report    BootRegressionReport          `json:"report"`
	Anomalies []VirtqueueAnomaly            `json:"anomalies"`
	Triage    VirtqueueTriageReport         `json:"triage"`
	Memory    MemoryIndex                   `json:"memory"`
	JumpHints []MemoryJumpHint              `json:"memory_jump_hints,omitempty"`
	Presets   []DiagnosticQueryPresetResult `json:"query_presets,omitempty"`
	Bookmarks []QueryBookmark               `json:"bookmarks,omitempty"`
	Query     string                        `json:"query,omitempty"`
	QueryHits []QueryHit                    `json:"query_hits,omitempty"`
}

func BuildShareBundle(status, console, trace, csrTrace string, b *mem.Bus, events []mem.AccessEvent, hist []mem.AccessBucket, query string) ShareBundle {
	report := BuildBootRegressionReport(status, console, trace, b, events, hist)
	anomalies := VirtqueueAnomalies(b, events, 16)
	memory := BuildMemoryIndex(b, 16, 0x20000)
	var hits []QueryHit
	if strings.TrimSpace(query) != "" {
		hits = QueryDiagnostics(query, console, trace, csrTrace, events, anomalies, memory, 48)
	}
	presets := DiagnosticQueryPresetResults(console, trace, csrTrace, events, anomalies, memory, 12)
	return ShareBundle{Report: report, Anomalies: anomalies, Triage: VirtqueueAnomalyTriage(anomalies), Memory: memory, JumpHints: MemoryJumpHints(memory, 12), Presets: presets, Bookmarks: QueryBookmarks(presets, 3), Query: strings.TrimSpace(query), QueryHits: hits}
}

func ShareBundleMarkdown(bundle ShareBundle) string {
	var b strings.Builder
	b.WriteString(BootRegressionReportMarkdown(bundle.Report))
	b.WriteString("\n## Virtqueue anomaly hints\n\n")
	b.WriteString("```text\n")
	b.WriteString(VirtqueueAnomalyHintsString(bundle.Anomalies))
	b.WriteString("```\n")

	b.WriteString("\n## Virtqueue anomaly triage\n\n")
	b.WriteString("```text\n")
	b.WriteString(VirtqueueAnomalyTriageString(bundle.Anomalies))
	b.WriteString("```\n")

	b.WriteString("\n## Guest memory index\n\n")
	fmt.Fprintf(&b, "- DRAM: `%#x..%#x`\n", bundle.Memory.DRAMBase, bundle.Memory.DRAMBase+bundle.Memory.DRAMSize)
	fmt.Fprintf(&b, "- Indexed ranges: `%d`\n\n", len(bundle.Memory.Ranges))
	if len(bundle.Memory.Counts) != 0 {
		writeMarkdownCounts(&b, bundle.Memory.Counts)
	}
	if len(bundle.JumpHints) != 0 {
		b.WriteString("\n## Memory jump hints\n\n")
		b.WriteString("| Address | Label | Reason |\n|---:|---|---|\n")
		for _, h := range bundle.JumpHints {
			fmt.Fprintf(&b, "| `%#x` | %s | %s |\n", h.Address, mdCell(h.Label), mdCell(h.Reason))
		}
	}
	if len(bundle.Bookmarks) != 0 {
		b.WriteString("\n## Query preset bookmarks\n\n")
		b.WriteString("| Preset | Source | Text |\n|---|---|---|\n")
		for _, bm := range bundle.Bookmarks {
			src := bm.Source
			if bm.Line != 0 {
				src = fmt.Sprintf("%s:%d", bm.Source, bm.Line)
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", mdCell(bm.Label), mdCell(src), mdCell(bm.Text))
		}
	}
	if len(bundle.QueryHits) != 0 {
		b.WriteString("\n## Diagnostic query hits\n\n")
		fmt.Fprintf(&b, "Query: `%s`\n\n", mdCell(bundle.Query))
		b.WriteString("| Score | Source | Text |\n|---:|---|---|\n")
		for _, h := range bundle.QueryHits {
			src := h.Source
			if h.Line != 0 {
				src = fmt.Sprintf("%s:%d", h.Source, h.Line)
			}
			fmt.Fprintf(&b, "| %d | %s | %s |\n", h.Score, mdCell(src), mdCell(h.Text))
		}
	}
	return b.String()
}

// ShareBundleHTML renders a self-contained, dependency-free HTML bundle. It
// embeds the JSON payload in a script tag so a collaborator can save one file
// and still recover the raw machine-readable report.
func ShareBundleHTML(bundle ShareBundle) string {
	md := ShareBundleMarkdown(bundle)
	j, _ := json.MarshalIndent(bundle, "", "  ")
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm share report</title>")
	b.WriteString("<style>body{font-family:system-ui,sans-serif;margin:24px;background:#101216;color:#e8e8e8}pre{white-space:pre-wrap;background:#05070a;border:1px solid #333;padding:16px;border-radius:8px;overflow:auto}button{margin:4px;padding:6px 10px}code{color:#9cf}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	b.WriteString("<h1>rvwasm share report</h1>")
	b.WriteString("<p>Self-contained report: Markdown is shown below, and JSON is embedded for copy/export.</p>")
	b.WriteString("<button onclick=\"navigator.clipboard&&navigator.clipboard.writeText(document.getElementById('md').textContent)\">Copy Markdown</button>")
	b.WriteString("<button onclick=\"navigator.clipboard&&navigator.clipboard.writeText(document.getElementById('json').textContent)\">Copy JSON</button>")
	b.WriteString("<h2>Markdown</h2><pre id=\"md\">")
	b.WriteString(html.EscapeString(md))
	b.WriteString("</pre><h2>JSON</h2><pre id=\"json\">")
	b.WriteString(html.EscapeString(string(j)))
	b.WriteString("</pre><script type=\"application/json\" id=\"rvwasm-share-bundle\">")
	b.WriteString(html.EscapeString(string(j)))
	b.WriteString("</script>")
	return b.String()
}
