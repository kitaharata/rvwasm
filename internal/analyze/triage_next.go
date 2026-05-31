package analyze

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

type StopCauseCandidate struct {
	Rank       int      `json:"rank"`
	Severity   string   `json:"severity"`
	Score      int      `json:"score"`
	Category   string   `json:"category"`
	Summary    string   `json:"summary"`
	Evidence   []string `json:"evidence,omitempty"`
	NextAction string   `json:"next_action,omitempty"`
}

type TriageDashboard struct {
	Status         string               `json:"status"`
	Phase          string               `json:"phase"`
	TopCandidates  []StopCauseCandidate `json:"top_candidates"`
	AnomalyCounts  map[string]int       `json:"anomaly_counts,omitempty"`
	DeviceProbe    []DeviceProbe        `json:"device_probe,omitempty"`
	MemoryCounts   map[string]int       `json:"memory_counts,omitempty"`
	QueryBookmarks []QueryBookmark      `json:"query_bookmarks,omitempty"`
}

func BuildTriageDashboard(status, console, trace, csrTrace string, b *mem.Bus, events []mem.AccessEvent, hist []mem.AccessBucket, query string) TriageDashboard {
	report := BuildBootRegressionReport(status, console, trace, b, events, hist)
	anomalies := VirtqueueAnomalies(b, events, 16)
	idx := BuildMemoryIndex(b, 24, 0x20000)
	presets := DiagnosticQueryPresetResults(console, trace, csrTrace, events, anomalies, idx, 8)
	phase := "unknown"
	if len(report.BootEvents) != 0 {
		phase = report.BootEvents[len(report.BootEvents)-1].Phase
	}
	return TriageDashboard{
		Status:         status,
		Phase:          phase,
		TopCandidates:  RankStopCauses(status, console, trace, report, anomalies, 8),
		AnomalyCounts:  VirtqueueAnomalyTriage(anomalies).Counts,
		DeviceProbe:    report.DeviceProbe,
		MemoryCounts:   report.MemoryCounts,
		QueryBookmarks: QueryBookmarks(presets, 2),
	}
}

func TriageDashboardString(d TriageDashboard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "triage dashboard phase=%s\nstatus: %s\n", d.Phase, d.Status)
	if len(d.TopCandidates) == 0 {
		b.WriteString("\n<no ranked stop-cause candidates yet>\n")
	} else {
		b.WriteString("\nranked stop-cause candidates\n")
		for _, c := range d.TopCandidates {
			fmt.Fprintf(&b, "%2d %-8s score=%-3d %-18s %s\n", c.Rank, c.Severity, c.Score, c.Category, c.Summary)
			if c.NextAction != "" {
				fmt.Fprintf(&b, "   next: %s\n", c.NextAction)
			}
			for _, ev := range c.Evidence {
				fmt.Fprintf(&b, "   - %s\n", ev)
			}
		}
	}
	if len(d.AnomalyCounts) != 0 {
		fmt.Fprintf(&b, "\nanomalies: critical=%d error=%d warn=%d info=%d\n", d.AnomalyCounts["critical"], d.AnomalyCounts["error"], d.AnomalyCounts["warn"], d.AnomalyCounts["info"])
	}
	if len(d.DeviceProbe) != 0 {
		b.WriteString("\ndevices\n")
		for _, p := range d.DeviceProbe {
			fmt.Fprintf(&b, "  %-15s R=%d W=%d id=%d ready=%d notify=%d last=%s:%#x\n", p.Name, p.Reads, p.Writes, p.IdentityReads, p.QueueReady, p.QueueNotify, p.LastReg, p.LastValue)
		}
	}
	if len(d.QueryBookmarks) != 0 {
		b.WriteString("\nbookmarks\n")
		for _, bm := range d.QueryBookmarks {
			loc := bm.Source
			if bm.Line != 0 {
				loc = fmt.Sprintf("%s:%d", bm.Source, bm.Line)
			}
			fmt.Fprintf(&b, "  [%s] %-18s %s\n", bm.Label, loc, bm.Text)
		}
	}
	return b.String()
}

func RankStopCauses(status, console, trace string, report BootRegressionReport, anomalies []VirtqueueAnomaly, limit int) []StopCauseCandidate {
	if limit <= 0 || limit > 32 {
		limit = 8
	}
	add := func(rows *[]StopCauseCandidate, sev string, score int, cat, summary, next string, evidence ...string) {
		*rows = append(*rows, StopCauseCandidate{Severity: sev, Score: score, Category: cat, Summary: summary, NextAction: next, Evidence: compactEvidence(evidence, 4)})
	}
	rows := []StopCauseCandidate{}
	lowerConsole := strings.ToLower(console)
	lowerStatus := strings.ToLower(status)
	lowerTrace := strings.ToLower(trace)
	if strings.Contains(lowerConsole, "kernel panic") || strings.Contains(lowerConsole, "panic:") {
		add(&rows, "critical", 100, "kernel-panic", "console contains kernel panic", "Open Panic summary and symbol-resolve the first faulting PC", extractLines(console, []string{"kernel panic", "panic:"}, 4)...)
	}
	if strings.Contains(lowerConsole, "oops") || strings.Contains(lowerConsole, "unable to handle") {
		add(&rows, "critical", 92, "kernel-oops", "console contains oops/fault text", "Use Panic summary, DWARF source context, and annotated trace", extractLines(console, []string{"oops", "unable to handle", "call trace"}, 5)...)
	}
	if strings.Contains(lowerStatus, "illegal") || strings.Contains(lowerTrace, "illegal") {
		add(&rows, "error", 88, "illegal-instruction", "last status/trace suggests illegal instruction", "Check trace decode around last PC and verify missing ISA/CSR", status)
	}
	if strings.Contains(lowerStatus, "page fault") || strings.Contains(lowerTrace, "page fault") || strings.Contains(lowerTrace, "load page") || strings.Contains(lowerTrace, "inst page") {
		add(&rows, "error", 84, "page-fault", "page fault or translation fault detected", "Inspect satp/CSR trace, MMU state, and faulting virtual address", status)
	}
	if strings.Contains(lowerStatus, "access fault") || strings.Contains(lowerTrace, "access fault") {
		add(&rows, "error", 80, "access-fault", "physical/PMP/access fault detected", "Check PMP settings and memory map / device address", status)
	}
	triage := VirtqueueAnomalyTriage(anomalies)
	for _, it := range triage.Items {
		base := 75 - it.Priority*10
		sev := it.Severity
		add(&rows, sev, base, "virtqueue", fmt.Sprintf("%s q=%d: %s", it.Device, it.Queue, it.Detail), it.Hint)
	}
	for _, p := range report.DeviceProbe {
		if strings.HasPrefix(p.Name, "virtio-") && p.IdentityReads != 0 && p.QueueReady == 0 && p.QueueNotify == 0 {
			add(&rows, "warn", 45, "device-probe", fmt.Sprintf("%s was probed but no queue became ready", p.Name), "Check feature negotiation and driver availability", fmt.Sprintf("R=%d W=%d last=%s:%#x", p.Reads, p.Writes, p.LastReg, p.LastValue))
		}
		if strings.HasPrefix(p.Name, "virtio-") && p.QueueReady != 0 && p.QueueNotify == 0 {
			add(&rows, "info", 30, "device-idle", fmt.Sprintf("%s queue ready but not notified yet", p.Name), "Continue execution or inspect driver probe logs")
		}
	}
	if len(report.TraceStats.TrapCauses) != 0 {
		keys := sortedKeys(report.TraceStats.TrapCauses)
		ev := []string{}
		for _, k := range keys {
			ev = append(ev, fmt.Sprintf("%s=%d", k, report.TraceStats.TrapCauses[k]))
		}
		add(&rows, "info", 25, "trap-summary", "trace contains traps", "Use trace filter: trap", ev...)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score == rows[j].Score {
			return rows[i].Category < rows[j].Category
		}
		return rows[i].Score > rows[j].Score
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}

func compactEvidence(in []string, max int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, truncate(s, 220))
		if len(out) >= max {
			break
		}
	}
	return out
}

func extractLines(text string, needles []string, max int) []string {
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		for _, n := range needles {
			if strings.Contains(lower, strings.ToLower(n)) {
				out = append(out, strings.TrimSpace(line))
				break
			}
		}
		if len(out) >= max {
			break
		}
	}
	return out
}

// ---- query preset comparison ----

type QueryPresetComparison struct {
	Name           string `json:"name"`
	Query          string `json:"query"`
	CurrentHits    int    `json:"current_hits"`
	BaselineHits   int    `json:"baseline_hits"`
	Delta          int    `json:"delta"`
	Status         string `json:"status"`
	CurrentSample  string `json:"current_sample,omitempty"`
	BaselineSample string `json:"baseline_sample,omitempty"`
}

func CompareDiagnosticQueryPresets(current, baseline []DiagnosticQueryPresetResult) []QueryPresetComparison {
	by := map[string]DiagnosticQueryPresetResult{}
	for _, b := range baseline {
		by[b.Preset.Name] = b
	}
	out := make([]QueryPresetComparison, 0, len(current))
	for _, c := range current {
		b := by[c.Preset.Name]
		delta := len(c.Hits) - len(b.Hits)
		status := "unchanged"
		switch {
		case len(b.Hits) == 0 && len(c.Hits) != 0:
			status = "new-hit"
		case len(b.Hits) != 0 && len(c.Hits) == 0:
			status = "cleared"
		case delta > 0:
			status = "more"
		case delta < 0:
			status = "less"
		}
		out = append(out, QueryPresetComparison{Name: c.Preset.Name, Query: c.Preset.Query, CurrentHits: len(c.Hits), BaselineHits: len(b.Hits), Delta: delta, Status: status, CurrentSample: firstHitText(c.Hits), BaselineSample: firstHitText(b.Hits)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Status == out[j].Status {
			return out[i].Name < out[j].Name
		}
		order := map[string]int{"new-hit": 0, "more": 1, "less": 2, "cleared": 3, "unchanged": 4}
		return order[out[i].Status] < order[out[j].Status]
	})
	return out
}

func QueryPresetComparisonString(rows []QueryPresetComparison) string {
	if len(rows) == 0 {
		return "<no preset comparison rows>\n"
	}
	var b strings.Builder
	b.WriteString("diagnostic preset comparison\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%-10s delta=%+d current=%d baseline=%d %-18s query=%q\n", r.Status, r.Delta, r.CurrentHits, r.BaselineHits, r.Name, r.Query)
		if r.CurrentSample != "" {
			fmt.Fprintf(&b, "  current:  %s\n", r.CurrentSample)
		}
		if r.BaselineSample != "" && r.BaselineSample != r.CurrentSample {
			fmt.Fprintf(&b, "  baseline: %s\n", r.BaselineSample)
		}
	}
	return b.String()
}

func firstHitText(h []QueryHit) string {
	if len(h) == 0 {
		return ""
	}
	return truncate(h[0].Text, 180)
}

// ---- memory object dump helpers ----

type MemoryDump struct {
	Addr   uint64 `json:"addr"`
	End    uint64 `json:"end"`
	Type   string `json:"type,omitempty"`
	Detail string `json:"detail,omitempty"`
	Hex    string `json:"hex"`
	ASCII  string `json:"ascii"`
}

func DumpMemoryIndexHits(b *mem.Bus, idx MemoryIndex, query string, maxHits int, maxBytes int) []MemoryDump {
	if b == nil {
		return nil
	}
	if maxHits <= 0 || maxHits > 16 {
		maxHits = 8
	}
	if maxBytes <= 0 || maxBytes > 4096 {
		maxBytes = 256
	}
	hits := SearchMemoryIndex(idx, query, maxHits)
	out := []MemoryDump{}
	for _, h := range hits {
		addr := h.Addr
		n := maxBytes
		if h.End > h.Addr && h.End-h.Addr+1 < uint64(n) {
			n = int(h.End - h.Addr + 1)
		}
		if !b.InDRAM(addr, n) {
			continue
		}
		off := addr - b.DRAMBase
		data := b.DRAM[off : off+uint64(n)]
		out = append(out, MemoryDump{Addr: addr, End: addr + uint64(n) - 1, Type: h.Type, Detail: firstNonEmpty(h.Detail, h.Label), Hex: hex.EncodeToString(data), ASCII: printableASCII(data)})
	}
	return out
}

func DumpMemoryIndexHitsString(b *mem.Bus, idx MemoryIndex, query string, maxHits int, maxBytes int) string {
	rows := DumpMemoryIndexHits(b, idx, query, maxHits, maxBytes)
	if len(rows) == 0 {
		return fmt.Sprintf("<no dumpable memory hits for %q>\n", strings.TrimSpace(query))
	}
	var sb strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&sb, "dump %#016x..%#016x %-22s %s\n", r.Addr, r.End, r.Type, r.Detail)
		data, _ := hex.DecodeString(r.Hex)
		for off := 0; off < len(data); off += 16 {
			end := off + 16
			if end > len(data) {
				end = len(data)
			}
			chunk := data[off:end]
			fmt.Fprintf(&sb, "  %#016x  %-47s  |%s|\n", r.Addr+uint64(off), spacedHex(chunk), printableASCII(chunk))
		}
	}
	return sb.String()
}

func printableASCII(data []byte) string {
	out := make([]byte, len(data))
	for i, c := range data {
		if c >= 32 && c < 127 {
			out[i] = c
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func spacedHex(data []byte) string {
	parts := make([]string, len(data))
	for i, c := range data {
		parts[i] = fmt.Sprintf("%02x", c)
	}
	return strings.Join(parts, " ")
}
func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// ---- redaction helpers ----

type RedactionOptions struct {
	ReplaceIPs     bool `json:"replace_ips"`
	ReplaceMACs    bool `json:"replace_macs"`
	ReplaceEmails  bool `json:"replace_emails"`
	ReplaceLongHex bool `json:"replace_long_hex"`
}

func DefaultRedactionOptions() RedactionOptions {
	return RedactionOptions{ReplaceIPs: true, ReplaceMACs: true, ReplaceEmails: true, ReplaceLongHex: false}
}

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
var macRe = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(:[0-9a-f]{2}){5}\b`)
var longHexRe = regexp.MustCompile(`\b0x[0-9a-fA-F]{13,}\b`)

func RedactSensitiveText(s string, opt RedactionOptions) string {
	if opt.ReplaceEmails {
		s = emailRe.ReplaceAllString(s, "<email>")
	}
	if opt.ReplaceMACs {
		s = macRe.ReplaceAllString(s, "<mac>")
	}
	if opt.ReplaceIPs {
		fields := strings.FieldsFunc(s, func(r rune) bool { return !(r == '.' || (r >= '0' && r <= '9')) })
		for _, f := range fields {
			ip := net.ParseIP(f)
			if ip != nil && strings.Contains(f, ".") {
				s = strings.ReplaceAll(s, f, "<ipv4>")
			}
		}
	}
	if opt.ReplaceLongHex {
		s = longHexRe.ReplaceAllString(s, "<hexaddr>")
	}
	return s
}

func RedactedShareBundleMarkdown(bundle ShareBundle, opt RedactionOptions) string {
	return RedactSensitiveText(ShareBundleMarkdown(bundle), opt)
}

func RedactedShareBundleJSON(bundle ShareBundle, opt RedactionOptions) string {
	j, _ := json.MarshalIndent(bundle, "", "  ")
	return RedactSensitiveText(string(j), opt)
}
