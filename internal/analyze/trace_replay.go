package analyze

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

type TraceEvent struct {
	Line  int    `json:"line"`
	Cycle uint64 `json:"cycle,omitempty"`
	Kind  string `json:"kind"`
	Mode  string `json:"mode,omitempty"`
	PC    uint64 `json:"pc,omitempty"`
	Next  uint64 `json:"next,omitempty"`
	Inst  string `json:"inst,omitempty"`
	ASM   string `json:"asm,omitempty"`
	Cause string `json:"cause,omitempty"`
	Raw   string `json:"raw"`
}

type TraceStats struct {
	Lines      int            `json:"lines"`
	Steps      int            `json:"steps"`
	Traps      int            `json:"traps"`
	Ecalls     int            `json:"ecalls"`
	SBIShim    int            `json:"sbi_shim"`
	FirstPC    uint64         `json:"first_pc,omitempty"`
	LastPC     uint64         `json:"last_pc,omitempty"`
	TopASM     map[string]int `json:"top_asm,omitempty"`
	TrapCauses map[string]int `json:"trap_causes,omitempty"`
}

type TraceMismatch struct {
	Index int        `json:"index"`
	A     TraceEvent `json:"a"`
	B     TraceEvent `json:"b"`
	Why   string     `json:"why"`
}

var (
	reCycle = regexp.MustCompile(`\bcycle=(\d+)`)
	reMode  = regexp.MustCompile(`\bmode=([0-9]+)`)
	rePC    = regexp.MustCompile(`\bpc=([0-9a-fA-F]{8,16})\b`)
	reNext  = regexp.MustCompile(`\bnext=([0-9a-fA-F]{8,16})\b`)
	reInst  = regexp.MustCompile(`\binst=([0-9a-fA-F]+)\b`)
	reASM   = regexp.MustCompile(`\basm="([^"]*)"`)
	reCause = regexp.MustCompile(`\bcause=([^\s]+)`)
)

func ParseTraceText(text string, max int) []TraceEvent {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if max <= 0 || max > 65536 {
		max = 65536
	}
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	out := make([]TraceEvent, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ev := TraceEvent{Line: i + 1, Raw: line, Kind: "step"}
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, " trap ") || strings.HasPrefix(lower, "trap ") || strings.Contains(lower, "trap interrupt="):
			ev.Kind = "trap"
		case strings.Contains(lower, " ecall ") || strings.HasPrefix(lower, "ecall "):
			ev.Kind = "ecall"
		case strings.Contains(lower, "sbi-shim"):
			ev.Kind = "sbi-shim"
		}
		ev.Cycle = matchUint(reCycle, line, 10)
		if m := reMode.FindStringSubmatch(line); len(m) == 2 {
			ev.Mode = m[1]
		}
		ev.PC = matchUint(rePC, line, 16)
		ev.Next = matchUint(reNext, line, 16)
		if m := reInst.FindStringSubmatch(line); len(m) == 2 {
			ev.Inst = m[1]
		}
		if m := reASM.FindStringSubmatch(line); len(m) == 2 {
			ev.ASM = m[1]
		}
		if m := reCause.FindStringSubmatch(line); len(m) == 2 {
			ev.Cause = m[1]
		}
		out = append(out, ev)
	}
	return out
}

func matchUint(re *regexp.Regexp, line string, base int) uint64 {
	m := re.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0
	}
	v, _ := strconv.ParseUint(m[1], base, 64)
	return v
}

func TraceStatsForText(text string) TraceStats {
	events := ParseTraceText(text, 0)
	st := TraceStats{Lines: len(events), TopASM: map[string]int{}, TrapCauses: map[string]int{}}
	for _, ev := range events {
		switch ev.Kind {
		case "step":
			st.Steps++
		case "trap":
			st.Traps++
			if ev.Cause != "" {
				st.TrapCauses[ev.Cause]++
			}
		case "ecall":
			st.Ecalls++
		case "sbi-shim":
			st.SBIShim++
		}
		if ev.PC != 0 {
			if st.FirstPC == 0 {
				st.FirstPC = ev.PC
			}
			st.LastPC = ev.PC
		}
		if ev.ASM != "" {
			op := strings.Fields(ev.ASM)
			if len(op) > 0 {
				st.TopASM[op[0]]++
			}
		}
	}
	return st
}

func TraceStatsString(text string) string {
	st := TraceStatsForText(text)
	var b strings.Builder
	fmt.Fprintf(&b, "lines=%d steps=%d traps=%d ecalls=%d sbi_shim=%d first_pc=%#x last_pc=%#x\n", st.Lines, st.Steps, st.Traps, st.Ecalls, st.SBIShim, st.FirstPC, st.LastPC)
	writeTopMap(&b, "top asm", st.TopASM, 12)
	writeTopMap(&b, "trap causes", st.TrapCauses, 12)
	return b.String()
}

func CompareTraceText(a, b string, maxDiffs int) []TraceMismatch {
	if maxDiffs <= 0 || maxDiffs > 256 {
		maxDiffs = 32
	}
	aa := ParseTraceText(a, 0)
	bb := ParseTraceText(b, 0)
	n := len(aa)
	if len(bb) < n {
		n = len(bb)
	}
	out := []TraceMismatch{}
	for i := 0; i < n && len(out) < maxDiffs; i++ {
		why := ""
		if aa[i].Kind != bb[i].Kind {
			why = fmt.Sprintf("kind %s != %s", aa[i].Kind, bb[i].Kind)
		} else if aa[i].PC != bb[i].PC {
			why = fmt.Sprintf("pc %#x != %#x", aa[i].PC, bb[i].PC)
		} else if aa[i].Inst != "" && bb[i].Inst != "" && aa[i].Inst != bb[i].Inst {
			why = fmt.Sprintf("inst %s != %s", aa[i].Inst, bb[i].Inst)
		} else if aa[i].Cause != bb[i].Cause {
			why = fmt.Sprintf("cause %s != %s", aa[i].Cause, bb[i].Cause)
		}
		if why != "" {
			out = append(out, TraceMismatch{Index: i, A: aa[i], B: bb[i], Why: why})
		}
	}
	if len(out) < maxDiffs && len(aa) != len(bb) {
		out = append(out, TraceMismatch{Index: n, Why: fmt.Sprintf("length %d != %d", len(aa), len(bb))})
	}
	return out
}

func CompareTraceTextString(a, b string, maxDiffs int) string {
	diffs := CompareTraceText(a, b, maxDiffs)
	if len(diffs) == 0 {
		return "<no trace mismatches>\n"
	}
	var sb strings.Builder
	for _, d := range diffs {
		fmt.Fprintf(&sb, "#%d %s\n", d.Index, d.Why)
		if d.A.Raw != "" {
			fmt.Fprintf(&sb, "  A: %s\n", d.A.Raw)
		}
		if d.B.Raw != "" {
			fmt.Fprintf(&sb, "  B: %s\n", d.B.Raw)
		}
	}
	return sb.String()
}

type BootRegressionReport struct {
	Status          string             `json:"status"`
	BootEvents      []BootEvent        `json:"boot_events"`
	DeviceProbe     []DeviceProbe      `json:"device_probe"`
	Virtqueues      []VirtqueueState   `json:"virtqueues"`
	MemoryCounts    map[string]int     `json:"memory_counts"`
	InitcallCounts  map[string]int     `json:"initcall_counts"`
	TraceStats      TraceStats         `json:"trace_stats"`
	AccessHistogram []mem.AccessBucket `json:"access_histogram,omitempty"`
}

func BuildBootRegressionReport(status, console, trace string, b *mem.Bus, events []mem.AccessEvent, hist []mem.AccessBucket) BootRegressionReport {
	return BootRegressionReport{
		Status:          status,
		BootEvents:      BootTimeline(console, events, 128),
		DeviceProbe:     DeviceProbeSummary(events),
		Virtqueues:      VirtqueueSummary(events),
		MemoryCounts:    MemoryObjectTypeCounts(ScanMemoryObjects(b, 12)),
		InitcallCounts:  InitcallCategoryCounts(console),
		TraceStats:      TraceStatsForText(trace),
		AccessHistogram: hist,
	}
}

func BootRegressionReportString(r BootRegressionReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	fmt.Fprintf(&b, "trace: lines=%d steps=%d traps=%d ecalls=%d first_pc=%#x last_pc=%#x\n", r.TraceStats.Lines, r.TraceStats.Steps, r.TraceStats.Traps, r.TraceStats.Ecalls, r.TraceStats.FirstPC, r.TraceStats.LastPC)
	fmt.Fprintf(&b, "boot_events=%d devices=%d virtqueues=%d\n", len(r.BootEvents), len(r.DeviceProbe), len(r.Virtqueues))
	b.WriteString("memory objects:\n")
	writeSortedCounts(&b, r.MemoryCounts)
	b.WriteString("initcall categories:\n")
	writeSortedCounts(&b, r.InitcallCounts)
	if len(r.BootEvents) != 0 {
		b.WriteString("recent boot events:\n")
		start := len(r.BootEvents) - 12
		if start < 0 {
			start = 0
		}
		for _, ev := range r.BootEvents[start:] {
			fmt.Fprintf(&b, "  %09d %-8s %-20s %s\n", ev.Seq, ev.Source, ev.Phase, ev.Detail)
		}
	}
	return b.String()
}

type VirtqueueSnapshot struct {
	Summary []VirtqueueState `json:"summary"`
	Chains  []VirtqueueChain `json:"chains"`
}

func BuildVirtqueueSnapshot(b *mem.Bus, events []mem.AccessEvent, maxHeads int) VirtqueueSnapshot {
	return VirtqueueSnapshot{Summary: VirtqueueSummary(events), Chains: VirtqueueChains(b, events, maxHeads)}
}

func VirtqueueSnapshotString(b *mem.Bus, events []mem.AccessEvent, maxHeads int) string {
	ss := BuildVirtqueueSnapshot(b, events, maxHeads)
	var out strings.Builder
	out.WriteString("[summary]\n")
	if len(ss.Summary) == 0 {
		out.WriteString("<no virtqueue state>\n")
	}
	for _, s := range ss.Summary {
		fmt.Fprintf(&out, "%s q=%d num=%d ready=%v desc=%#x driver=%#x device=%#x notifySeq=%d\n", s.Device, s.Queue, s.Num, s.Ready, s.Desc, s.Driver, s.DeviceArea, s.LastNotifyAt)
	}
	out.WriteString("[chains]\n")
	out.WriteString(VirtqueueChainsString(b, events, maxHeads))
	return out.String()
}

func DiffMemoryObjects(before, after []MemoryObject) []string {
	key := func(o MemoryObject) string { return fmt.Sprintf("%s@%#x:%#x:%s", o.Type, o.Addr, o.Size, o.Detail) }
	bm := map[string]MemoryObject{}
	am := map[string]MemoryObject{}
	for _, o := range before {
		bm[key(o)] = o
	}
	for _, o := range after {
		am[key(o)] = o
	}
	lines := []string{}
	for k, o := range am {
		if _, ok := bm[k]; !ok {
			lines = append(lines, fmt.Sprintf("+ %#016x %-22s size=%#x %s", o.Addr, o.Type, o.Size, o.Detail))
		}
	}
	for k, o := range bm {
		if _, ok := am[k]; !ok {
			lines = append(lines, fmt.Sprintf("- %#016x %-22s size=%#x %s", o.Addr, o.Type, o.Size, o.Detail))
		}
	}
	sort.Strings(lines)
	return lines
}

func DiffMemoryObjectsString(before, after []MemoryObject) string {
	lines := DiffMemoryObjects(before, after)
	if len(lines) == 0 {
		return "<no memory scan changes>\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

func writeTopMap(b *strings.Builder, title string, m map[string]int, limit int) {
	if len(m) == 0 {
		return
	}
	type kv struct {
		k string
		v int
	}
	rows := make([]kv, 0, len(m))
	for k, v := range m {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].v == rows[j].v {
			return rows[i].k < rows[j].k
		}
		return rows[i].v > rows[j].v
	})
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	fmt.Fprintf(b, "%s:\n", title)
	for _, r := range rows[:limit] {
		fmt.Fprintf(b, "  %-18s %d\n", r.k, r.v)
	}
}

func writeSortedCounts(b *strings.Builder, m map[string]int) {
	if len(m) == 0 {
		b.WriteString("  <none>\n")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "  %-22s %d\n", k, m[k])
	}
}
