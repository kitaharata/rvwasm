package cpu

import (
	"fmt"
	"sort"
	"strings"
)

const traceRingSize = 512

func (h *Hart) traceStep(oldpc uint64, raw uint32, width int) {
	if !h.Trace {
		return
	}
	if len(h.traceRing) != traceRingSize {
		h.traceRing = make([]string, traceRingSize)
	}
	line := fmt.Sprintf("cycle=%d mode=%d pc=%016x inst=%0*x asm=\"%s\" next=%016x", h.Cycle, h.Mode, oldpc, width/4, raw, disasm(raw, width), h.PC)
	h.traceRing[h.tracePos%traceRingSize] = line
	h.tracePos++
	if h.traceCount < traceRingSize {
		h.traceCount++
	}
}

func (h *Hart) traceTrap(cause, tval uint64, interrupt bool, target PrivMode) {
	if !h.Trace {
		return
	}
	if len(h.traceRing) != traceRingSize {
		h.traceRing = make([]string, traceRingSize)
	}
	line := fmt.Sprintf("cycle=%d trap interrupt=%v cause=%d tval=%#x target=%d pc=%016x", h.Cycle, interrupt, cause, tval, target, h.PC)
	h.traceRing[h.tracePos%traceRingSize] = line
	h.tracePos++
	if h.traceCount < traceRingSize {
		h.traceCount++
	}
}

func (h *Hart) observeEcall() {
	h.LastEcallMode = h.Mode
	h.LastEcallExt = h.X[17]
	h.LastEcallFunc = h.X[16]
	h.LastEcallArgs = [6]uint64{h.X[10], h.X[11], h.X[12], h.X[13], h.X[14], h.X[15]}
	h.LastEcallCycle = h.Cycle
	h.classifySBI()
	h.traceEcall()
}

func (h *Hart) classifySBI() {
	if h.Mode != PrivS {
		h.LastSBIClass = ""
		return
	}
	switch h.X[17] {
	case 0x10:
		h.LastSBIClass = "BASE"
		h.SBIBaseCount++
	case 0x54494d45: // TIME
		h.LastSBIClass = "TIME"
		h.SBITimeCount++
	case 0x735049: // sPI
		h.LastSBIClass = "IPI"
		h.SBIIPICount++
	case 0x52464e43: // RFNC
		h.LastSBIClass = "RFENCE"
		h.SBIRFenceCount++
	case 0x48534d: // HSM
		h.LastSBIClass = "HSM"
		h.SBIHSMCount++
	case 0x53525354: // SRST
		h.LastSBIClass = "SRST"
		h.SBISRSTCount++
	case 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8:
		h.LastSBIClass = "LEGACY"
		h.SBILegacyCount++
	default:
		h.LastSBIClass = "OTHER"
		h.SBIOtherCount++
	}
}

func (h *Hart) SBIObservationString() string {
	return fmt.Sprintf("last=%s base=%d time=%d ipi=%d rfence=%d hsm=%d srst=%d legacy=%d other=%d",
		h.LastSBIClass, h.SBIBaseCount, h.SBITimeCount, h.SBIIPICount, h.SBIRFenceCount, h.SBIHSMCount, h.SBISRSTCount, h.SBILegacyCount, h.SBIOtherCount)
}

func (h *Hart) traceEcall() {
	if !h.Trace {
		return
	}
	if len(h.traceRing) != traceRingSize {
		h.traceRing = make([]string, traceRingSize)
	}
	line := fmt.Sprintf("cycle=%d ecall mode=%d sbi=%s ext=%#x func=%#x a0=%#x a1=%#x a2=%#x a3=%#x", h.Cycle, h.Mode, h.LastSBIClass, h.X[17], h.X[16], h.X[10], h.X[11], h.X[12], h.X[13])
	h.traceRing[h.tracePos%traceRingSize] = line
	h.tracePos++
	if h.traceCount < traceRingSize {
		h.traceCount++
	}
}

func (h *Hart) traceSBIShim(errorCode int64, value uint64) {
	if !h.Trace {
		return
	}
	if len(h.traceRing) != traceRingSize {
		h.traceRing = make([]string, traceRingSize)
	}
	line := fmt.Sprintf("cycle=%d sbi-shim class=%s ext=%#x func=%#x err=%d value=%#x", h.Cycle, h.LastSBIClass, h.LastEcallExt, h.LastEcallFunc, errorCode, value)
	h.traceRing[h.tracePos%traceRingSize] = line
	h.tracePos++
	if h.traceCount < traceRingSize {
		h.traceCount++
	}
}

func (h *Hart) TraceString(n int) string {
	if n <= 0 || n > h.traceCount {
		n = h.traceCount
	}
	if n == 0 {
		return ""
	}
	start := h.tracePos - n
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx := (start + i) % traceRingSize
		if idx < 0 {
			idx += traceRingSize
		}
		b.WriteString(h.traceRing[idx])
		b.WriteByte('\n')
	}
	return b.String()
}

func (h *Hart) TraceStringFiltered(n int, filter string) string {
	if filter == "" {
		return h.TraceString(n)
	}
	if h.traceCount == 0 {
		return ""
	}
	filter = strings.ToLower(filter)
	if n <= 0 || n > h.traceCount {
		n = h.traceCount
	}
	start := h.tracePos - h.traceCount
	matches := make([]string, 0, n)
	for i := 0; i < h.traceCount; i++ {
		idx := (start + i) % traceRingSize
		if idx < 0 {
			idx += traceRingSize
		}
		line := h.traceRing[idx]
		if strings.Contains(strings.ToLower(line), filter) {
			matches = append(matches, line)
			if len(matches) > n {
				copy(matches, matches[1:])
				matches = matches[:n]
			}
		}
	}
	return strings.Join(matches, "\n") + trailingNewline(len(matches))
}

func (h *Hart) TraceLinesFiltered(n int, filter string) []string {
	text := h.TraceStringFiltered(n, filter)
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (h *Hart) TraceCompactString(n int, filter string) string {
	lines := h.TraceLinesFiltered(n, filter)
	if len(lines) == 0 {
		return ""
	}
	type group struct {
		first string
		last  string
		key   string
		count int
	}
	keyOf := func(line string) string {
		for _, marker := range []string{" trap ", " ecall ", " sbi-shim "} {
			if strings.Contains(line, marker) {
				return strings.TrimSpace(marker)
			}
		}
		if i := strings.Index(line, " asm="); i >= 0 {
			return line[i:]
		}
		return line
	}
	var groups []group
	cur := group{}
	for _, line := range lines {
		k := keyOf(line)
		if cur.count == 0 {
			cur = group{first: line, last: line, key: k, count: 1}
			continue
		}
		if cur.key == k {
			cur.last = line
			cur.count++
		} else {
			groups = append(groups, cur)
			cur = group{first: line, last: line, key: k, count: 1}
		}
	}
	if cur.count != 0 {
		groups = append(groups, cur)
	}
	var b strings.Builder
	for _, g := range groups {
		if g.count == 1 {
			b.WriteString(g.first)
			b.WriteByte('\n')
		} else {
			fmt.Fprintf(&b, "repeat=%d key=%s\n  first: %s\n  last:  %s\n", g.count, g.key, g.first, g.last)
		}
	}
	return b.String()
}

func (h *Hart) notePCProfile(pc uint64) {
	if !h.Profile {
		return
	}
	if h.pcProfile == nil {
		h.pcProfile = map[uint64]uint64{}
	}
	h.pcProfile[pc]++
}

func (h *Hart) SetProfile(on bool) { h.Profile = on }

func (h *Hart) ClearProfile() { h.pcProfile = nil }

type PCProfileEntry struct {
	PC    uint64 `json:"pc"`
	Count uint64 `json:"count"`
}

func (h *Hart) PCProfileTop(n int) []PCProfileEntry {
	if len(h.pcProfile) == 0 {
		return nil
	}
	out := make([]PCProfileEntry, 0, len(h.pcProfile))
	for pc, count := range h.pcProfile {
		out = append(out, PCProfileEntry{PC: pc, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].PC < out[j].PC
		}
		return out[i].Count > out[j].Count
	})
	if n <= 0 || n > len(out) {
		n = len(out)
	}
	return out[:n]
}

func (h *Hart) PCProfileString(n int, lookup func(uint64) string) string {
	entries := h.PCProfileTop(n)
	if len(entries) == 0 {
		return "<empty profile>\n"
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%016x count=%d", e.PC, e.Count)
		if lookup != nil {
			if s := lookup(e.PC); s != "" {
				fmt.Fprintf(&b, " %s", s)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type CSRAccessCount struct {
	Reads  uint64 `json:"reads"`
	Writes uint64 `json:"writes"`
}

const csrTraceRingSize = 256

func (h *Hart) traceCSR(op string, addr uint16, old, val uint64, write bool) {
	if !h.CSRTrace {
		return
	}
	if h.csrCounts == nil {
		h.csrCounts = map[uint16]CSRAccessCount{}
	}
	c := h.csrCounts[addr]
	if write {
		c.Writes++
	} else {
		c.Reads++
	}
	h.csrCounts[addr] = c
	if len(h.csrRing) != csrTraceRingSize {
		h.csrRing = make([]string, csrTraceRingSize)
	}
	line := ""
	if write {
		line = fmt.Sprintf("cycle=%d csr-%s mode=%d csr=%#03x old=%#x new=%#x pc=%016x", h.Cycle, op, h.Mode, addr, old, val, h.InstPC)
	} else {
		line = fmt.Sprintf("cycle=%d csr-%s mode=%d csr=%#03x val=%#x pc=%016x", h.Cycle, op, h.Mode, addr, old, h.InstPC)
	}
	h.csrRing[h.csrPos%csrTraceRingSize] = line
	h.csrPos++
	if h.csrCount < csrTraceRingSize {
		h.csrCount++
	}
}

func (h *Hart) CSRTraceString(n int) string {
	if n <= 0 || n > h.csrCount {
		n = h.csrCount
	}
	if n == 0 {
		return ""
	}
	start := h.csrPos - n
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx := (start + i) % csrTraceRingSize
		if idx < 0 {
			idx += csrTraceRingSize
		}
		b.WriteString(h.csrRing[idx])
		b.WriteByte('\n')
	}
	return b.String()
}

func (h *Hart) SetCSRTrace(on bool) { h.CSRTrace = on }

func (h *Hart) CSRAccessCounts() map[uint16]CSRAccessCount {
	out := make(map[uint16]CSRAccessCount, len(h.csrCounts))
	for k, v := range h.csrCounts {
		out[k] = v
	}
	return out
}

func (h *Hart) ClearCSRTrace() {
	h.csrRing = nil
	h.csrPos, h.csrCount = 0, 0
	h.csrCounts = nil
}

func (h *Hart) CSRAccessSummaryString(limit int) string {
	if len(h.csrCounts) == 0 {
		return "<empty CSR access summary>\n"
	}
	type row struct {
		addr uint16
		c    CSRAccessCount
	}
	rows := make([]row, 0, len(h.csrCounts))
	for addr, c := range h.csrCounts {
		rows = append(rows, row{addr: addr, c: c})
	}
	sort.Slice(rows, func(i, j int) bool {
		ti := rows[i].c.Reads + rows[i].c.Writes
		tj := rows[j].c.Reads + rows[j].c.Writes
		if ti == tj {
			return rows[i].addr < rows[j].addr
		}
		return ti > tj
	})
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	var b strings.Builder
	for _, r := range rows[:limit] {
		fmt.Fprintf(&b, "csr=%#03x reads=%d writes=%d total=%d\n", r.addr, r.c.Reads, r.c.Writes, r.c.Reads+r.c.Writes)
	}
	return b.String()
}

func trailingNewline(n int) string {
	if n == 0 {
		return ""
	}
	return "\n"
}
