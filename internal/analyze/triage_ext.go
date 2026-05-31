package analyze

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

// StopCauseExplanation makes the stop-cause ranking auditable.  The ranking
// itself is intentionally heuristic, so every exported candidate should carry a
// human-readable explanation of why it was placed where it was.
type StopCauseExplanation struct {
	Rank             int      `json:"rank"`
	Category         string   `json:"category"`
	Severity         string   `json:"severity"`
	Score            int      `json:"score"`
	Confidence       string   `json:"confidence"`
	Summary          string   `json:"summary"`
	ScoreBreakdown   []string `json:"score_breakdown"`
	Evidence         []string `json:"evidence,omitempty"`
	NextAction       string   `json:"next_action,omitempty"`
	SuggestedQueries []string `json:"suggested_queries,omitempty"`
}

func ExplainStopCauses(rows []StopCauseCandidate) []StopCauseExplanation {
	out := make([]StopCauseExplanation, 0, len(rows))
	for _, c := range rows {
		sevWeight := map[string]int{"critical": 90, "error": 75, "warn": 45, "info": 20}[c.Severity]
		confidence := "low"
		switch {
		case c.Score >= 90:
			confidence = "high"
		case c.Score >= 70:
			confidence = "medium"
		case c.Score >= 40:
			confidence = "medium-low"
		}
		breakdown := []string{
			fmt.Sprintf("severity=%s baseline≈%d", c.Severity, sevWeight),
			fmt.Sprintf("heuristic score=%d", c.Score),
		}
		if len(c.Evidence) != 0 {
			breakdown = append(breakdown, fmt.Sprintf("evidence lines=%d", len(c.Evidence)))
		}
		if c.NextAction != "" {
			breakdown = append(breakdown, "next action available")
		}
		out = append(out, StopCauseExplanation{Rank: c.Rank, Category: c.Category, Severity: c.Severity, Score: c.Score, Confidence: confidence, Summary: c.Summary, ScoreBreakdown: breakdown, Evidence: c.Evidence, NextAction: c.NextAction, SuggestedQueries: SuggestedQueriesForStopCause(c)})
	}
	return out
}

func StopCauseEvidenceString(rows []StopCauseCandidate) string {
	expl := ExplainStopCauses(rows)
	if len(expl) == 0 {
		return "<no stop-cause evidence available>\n"
	}
	var b strings.Builder
	for _, e := range expl {
		fmt.Fprintf(&b, "#%d %-8s confidence=%-10s score=%d category=%s\n", e.Rank, e.Severity, e.Confidence, e.Score, e.Category)
		fmt.Fprintf(&b, "summary: %s\n", e.Summary)
		if len(e.ScoreBreakdown) != 0 {
			b.WriteString("score basis:\n")
			for _, s := range e.ScoreBreakdown {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
		}
		if len(e.Evidence) != 0 {
			b.WriteString("evidence:\n")
			for _, s := range e.Evidence {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
		}
		if len(e.SuggestedQueries) != 0 {
			fmt.Fprintf(&b, "queries: %s\n", strings.Join(e.SuggestedQueries, ", "))
		}
		if e.NextAction != "" {
			fmt.Fprintf(&b, "next: %s\n", e.NextAction)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func SuggestedQueriesForStopCause(c StopCauseCandidate) []string {
	switch c.Category {
	case "kernel-panic", "kernel-oops":
		return []string{"panic", "oops", "call trace"}
	case "illegal-instruction":
		return []string{"illegal", "asm=", "CSR"}
	case "page-fault":
		return []string{"page fault", "satp", "stval"}
	case "access-fault":
		return []string{"access fault", "PMP", "watchpoint"}
	case "virtqueue":
		return []string{"virtqueue", "QueueReady", "QueueNotify", "descriptor"}
	case "device-probe", "device-idle":
		return []string{"DeviceID", "Status", "QueueReady"}
	default:
		return []string{"trap", c.Category}
	}
}

// RedactionOptionsFromJSON lets the browser UI edit redaction behavior without
// recompiling. Missing fields keep the supplied defaults. The JSON shape matches
// RedactionOptions, e.g. {"replace_long_hex":true}.
func RedactionOptionsFromJSON(spec string, def RedactionOptions) RedactionOptions {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return def
	}
	var raw map[string]bool
	if err := json.Unmarshal([]byte(spec), &raw); err != nil {
		return def
	}
	if v, ok := raw["replace_ips"]; ok {
		def.ReplaceIPs = v
	}
	if v, ok := raw["replace_macs"]; ok {
		def.ReplaceMACs = v
	}
	if v, ok := raw["replace_emails"]; ok {
		def.ReplaceEmails = v
	}
	if v, ok := raw["replace_long_hex"]; ok {
		def.ReplaceLongHex = v
	}
	return def
}

func RedactionOptionsString(opt RedactionOptions) string {
	b, _ := json.MarshalIndent(opt, "", "  ")
	return string(b)
}

type MemoryRangeLine struct {
	Addr  uint64 `json:"addr"`
	Hex   string `json:"hex"`
	ASCII string `json:"ascii"`
}

type MemoryRangeDump struct {
	Addr   uint64            `json:"addr"`
	End    uint64            `json:"end"`
	Length int               `json:"length"`
	Lines  []MemoryRangeLine `json:"lines"`
}

func DumpMemoryRange(b *mem.Bus, addr uint64, length int) MemoryRangeDump {
	if length <= 0 || length > 65536 {
		length = 256
	}
	out := MemoryRangeDump{Addr: addr, Length: length}
	if b == nil || !b.InDRAM(addr, 1) {
		out.End = addr
		return out
	}
	max := int(uint64(len(b.DRAM)) - (addr - b.DRAMBase))
	if length > max {
		length = max
	}
	out.Length = length
	out.End = addr + uint64(length) - 1
	data := b.DRAM[addr-b.DRAMBase : addr-b.DRAMBase+uint64(length)]
	for off := 0; off < len(data); off += 16 {
		end := off + 16
		if end > len(data) {
			end = len(data)
		}
		chunk := data[off:end]
		out.Lines = append(out.Lines, MemoryRangeLine{Addr: addr + uint64(off), Hex: spacedHex(chunk), ASCII: printableASCII(chunk)})
	}
	return out
}

func DumpMemoryRangeString(b *mem.Bus, addr uint64, length int) string {
	d := DumpMemoryRange(b, addr, length)
	if len(d.Lines) == 0 {
		return fmt.Sprintf("<address %#x is outside DRAM or range is empty>\n", addr)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "memory range %#016x..%#016x len=%d\n", d.Addr, d.End, d.Length)
	for _, line := range d.Lines {
		fmt.Fprintf(&sb, "  %#016x  %-47s  |%s|\n", line.Addr, line.Hex, line.ASCII)
	}
	return sb.String()
}

type TriageDiff struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Kind   string `json:"kind"`
}

func DiffTriageDashboardJSON(baselineJSON string, current TriageDashboard) []TriageDiff {
	baselineJSON = strings.TrimSpace(baselineJSON)
	if baselineJSON == "" {
		return []TriageDiff{{Path: "$", Kind: "missing-baseline", After: "current dashboard only"}}
	}
	var before TriageDashboard
	if err := json.Unmarshal([]byte(baselineJSON), &before); err != nil {
		return []TriageDiff{{Path: "$", Kind: "parse-error", Before: err.Error()}}
	}
	var rows []TriageDiff
	add := func(path, before, after string) {
		if before != after {
			rows = append(rows, TriageDiff{Path: path, Before: before, After: after, Kind: "changed"})
		}
	}
	add("status", before.Status, current.Status)
	add("phase", before.Phase, current.Phase)
	add("top_candidate", firstCandidateKey(before.TopCandidates), firstCandidateKey(current.TopCandidates))
	diffCounts("anomaly_counts", before.AnomalyCounts, current.AnomalyCounts, &rows)
	diffCounts("memory_counts", before.MemoryCounts, current.MemoryCounts, &rows)
	diffDevices(before.DeviceProbe, current.DeviceProbe, &rows)
	if len(rows) == 0 {
		rows = append(rows, TriageDiff{Path: "$", Kind: "unchanged"})
	}
	return rows
}

func TriageDashboardDiffString(baselineJSON string, current TriageDashboard) string {
	rows := DiffTriageDashboardJSON(baselineJSON, current)
	var b strings.Builder
	for _, r := range rows {
		if r.Kind == "unchanged" || r.Kind == "missing-baseline" || r.Kind == "parse-error" {
			fmt.Fprintf(&b, "%s: %s %s\n", r.Kind, r.Path, firstNonEmpty(r.After, r.Before))
			continue
		}
		fmt.Fprintf(&b, "%s: %s -> %s\n", r.Path, r.Before, r.After)
	}
	return b.String()
}

func diffCounts(prefix string, a, b map[string]int, rows *[]TriageDiff) {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if a[k] != b[k] {
			*rows = append(*rows, TriageDiff{Path: prefix + "." + k, Before: strconv.Itoa(a[k]), After: strconv.Itoa(b[k]), Kind: "changed"})
		}
	}
}

func diffDevices(a, b []DeviceProbe, rows *[]TriageDiff) {
	ma := map[string]DeviceProbe{}
	mb := map[string]DeviceProbe{}
	for _, p := range a {
		ma[p.Name] = p
	}
	for _, p := range b {
		mb[p.Name] = p
	}
	keys := map[string]bool{}
	for k := range ma {
		keys[k] = true
	}
	for k := range mb {
		keys[k] = true
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		pa, oka := ma[k]
		pb, okb := mb[k]
		switch {
		case !oka:
			*rows = append(*rows, TriageDiff{Path: "device." + k, Kind: "added", After: fmt.Sprintf("R=%d W=%d", pb.Reads, pb.Writes)})
		case !okb:
			*rows = append(*rows, TriageDiff{Path: "device." + k, Kind: "removed", Before: fmt.Sprintf("R=%d W=%d", pa.Reads, pa.Writes)})
		default:
			before := fmt.Sprintf("R=%d W=%d ready=%d notify=%d last=%s:%#x", pa.Reads, pa.Writes, pa.QueueReady, pa.QueueNotify, pa.LastReg, pa.LastValue)
			after := fmt.Sprintf("R=%d W=%d ready=%d notify=%d last=%s:%#x", pb.Reads, pb.Writes, pb.QueueReady, pb.QueueNotify, pb.LastReg, pb.LastValue)
			if before != after {
				*rows = append(*rows, TriageDiff{Path: "device." + k, Kind: "changed", Before: before, After: after})
			}
		}
	}
}

func firstCandidateKey(rows []StopCauseCandidate) string {
	if len(rows) == 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%d", rows[0].Category, rows[0].Severity, rows[0].Score)
}

type SmokeSummary struct {
	Preset    string `json:"preset"`
	Ran       int    `json:"ran"`
	Requested int    `json:"requested"`
	Status    string `json:"status"`
	Phase     string `json:"phase,omitempty"`
	TopCause  string `json:"top_cause,omitempty"`
}

func SmokeSummaryString(rows []SmokeSummary) string {
	if len(rows) == 0 {
		return "<no smoke results>\n"
	}
	var b strings.Builder
	b.WriteString("smoke matrix summary\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%-12s ran=%-8d requested=%-8d phase=%-18s cause=%s\n", r.Preset, r.Ran, r.Requested, r.Phase, firstNonEmpty(r.TopCause, "-"))
		if r.Status != "" {
			fmt.Fprintf(&b, "  status: %s\n", truncate(r.Status, 180))
		}
	}
	return b.String()
}

func MarshalMemoryRangeDump(d MemoryRangeDump) string {
	j, _ := json.MarshalIndent(d, "", "  ")
	return string(j)
}

func HexToUint64(s string, def uint64) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return def
	}
	return v
}

func BytesHex(data []byte) string { return hex.EncodeToString(data) }
