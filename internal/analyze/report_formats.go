package analyze

import (
	"fmt"
	"html"
	"strings"
)

// BootRegressionReportMarkdown renders a portable Markdown report that can be
// pasted into issues or saved next to a trace baseline.
func BootRegressionReportMarkdown(r BootRegressionReport) string {
	var b strings.Builder
	b.WriteString("# rvwasm boot regression report\n\n")
	fmt.Fprintf(&b, "- Status: `%s`\n", escapeBackticks(r.Status))
	fmt.Fprintf(&b, "- Trace: lines=%d steps=%d traps=%d ecalls=%d first_pc=`%#x` last_pc=`%#x`\n", r.TraceStats.Lines, r.TraceStats.Steps, r.TraceStats.Traps, r.TraceStats.Ecalls, r.TraceStats.FirstPC, r.TraceStats.LastPC)
	fmt.Fprintf(&b, "- Boot events: %d\n", len(r.BootEvents))
	fmt.Fprintf(&b, "- Device probes: %d\n", len(r.DeviceProbe))
	fmt.Fprintf(&b, "- Virtqueues: %d\n\n", len(r.Virtqueues))

	b.WriteString("## Memory objects\n\n")
	writeMarkdownCounts(&b, r.MemoryCounts)
	b.WriteString("\n## Initcall categories\n\n")
	writeMarkdownCounts(&b, r.InitcallCounts)

	if len(r.TraceStats.TrapCauses) != 0 {
		b.WriteString("\n## Trap causes\n\n")
		writeMarkdownCounts(&b, r.TraceStats.TrapCauses)
	}
	if len(r.TraceStats.TopASM) != 0 {
		b.WriteString("\n## Hot mnemonics\n\n")
		writeMarkdownCounts(&b, r.TraceStats.TopASM)
	}

	if len(r.BootEvents) != 0 {
		b.WriteString("\n## Recent boot events\n\n")
		b.WriteString("| Seq | Source | Phase | Detail |\n|---:|---|---|---|\n")
		start := len(r.BootEvents) - 24
		if start < 0 {
			start = 0
		}
		for _, ev := range r.BootEvents[start:] {
			fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", ev.Seq, mdCell(ev.Source), mdCell(ev.Phase), mdCell(ev.Detail))
		}
	}

	if len(r.DeviceProbe) != 0 {
		b.WriteString("\n## Device probe summary\n\n")
		b.WriteString("| Device | Reads | Writes | Identity reads | Queue ready | Queue notify | Last register |\n|---|---:|---:|---:|---:|---:|---|\n")
		for _, p := range r.DeviceProbe {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | `%s=%#x` |\n", mdCell(p.Name), p.Reads, p.Writes, p.IdentityReads, p.QueueReady, p.QueueNotify, mdCell(p.LastReg), p.LastValue)
		}
	}

	if len(r.Virtqueues) != 0 {
		b.WriteString("\n## Virtqueue state\n\n")
		b.WriteString("| Device | Queue | Ready | Num | Desc | Driver | Device | Notify count |\n|---|---:|---|---:|---:|---:|---:|---:|\n")
		for _, q := range r.Virtqueues {
			fmt.Fprintf(&b, "| %s | %d | %v | %d | `%#x` | `%#x` | `%#x` | %d |\n", mdCell(q.Device), q.Queue, q.Ready, q.Num, q.Desc, q.Driver, q.DeviceArea, q.NotifyCount)
		}
	}
	return b.String()
}

// BootRegressionReportHTML renders a standalone, dependency-free HTML report.
func BootRegressionReportHTML(r BootRegressionReport) string {
	md := BootRegressionReportMarkdown(r)
	// Keep this deliberately simple: preformatted Markdown inside HTML is more
	// robust than a partial Markdown renderer and works offline.
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm boot regression report</title>")
	b.WriteString("<style>body{font-family:system-ui,sans-serif;margin:24px;background:#111;color:#eee}pre{white-space:pre-wrap;background:#050505;border:1px solid #333;padding:16px;border-radius:8px}code{color:#9cf}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	b.WriteString("<h1>rvwasm boot regression report</h1><pre>")
	b.WriteString(html.EscapeString(md))
	b.WriteString("</pre>")
	return b.String()
}

func escapeBackticks(s string) string { return strings.ReplaceAll(s, "`", "\\`") }

func writeMarkdownCounts(b *strings.Builder, m map[string]int) {
	if len(m) == 0 {
		b.WriteString("_none_\n")
		return
	}
	b.WriteString("| Name | Count |\n|---|---:|\n")
	keys := sortedKeys(m)
	for _, k := range keys {
		fmt.Fprintf(b, "| %s | %d |\n", mdCell(k), m[k])
	}
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// small local insertion sort avoids another dependency in this file.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
