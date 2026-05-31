package debugmap

import (
	"bufio"
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Symbol struct {
	Addr uint64
	Size uint64
	Name string
	Kind string
}

type Line struct {
	Addr   uint64 `json:"addr"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type Table struct {
	Symbols []Symbol
	Lines   []Line
	Source  string
}

func Parse(data []byte, source string) (*Table, error) {
	if len(data) >= 4 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		return ParseELF(data, source)
	}
	return ParseSystemMap(data, source)
}

func ParseELF(data []byte, source string) (*Table, error) {
	ef, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer ef.Close()
	syms, err := ef.Symbols()
	if err != nil && len(syms) == 0 {
		syms = nil
	}
	if dyn, derr := ef.DynamicSymbols(); derr == nil {
		syms = append(syms, dyn...)
	}
	t := &Table{Source: source}
	for _, s := range syms {
		if s.Value == 0 || s.Name == "" {
			continue
		}
		bind := elf.ST_BIND(s.Info)
		typ := elf.ST_TYPE(s.Info)
		if bind == elf.STB_LOCAL && typ == elf.STT_NOTYPE && s.Size == 0 {
			continue
		}
		t.Symbols = append(t.Symbols, Symbol{Addr: s.Value, Size: s.Size, Name: s.Name, Kind: typ.String()})
	}
	if dw, derr := ef.DWARF(); derr == nil {
		t.Lines = parseDwarfLines(dw)
	}
	t.normalize()
	if len(t.Symbols) == 0 {
		return nil, fmt.Errorf("no usable ELF symbols found")
	}
	return t, nil
}

func ParseSystemMap(data []byte, source string) (*Table, error) {
	t := &Table{Source: source}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		addr, err := strconv.ParseUint(fields[0], 16, 64)
		if err != nil || addr == 0 {
			continue
		}
		t.Symbols = append(t.Symbols, Symbol{Addr: addr, Name: fields[2], Kind: fields[1]})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	t.normalize()
	if len(t.Symbols) == 0 {
		return nil, fmt.Errorf("no System.map-style symbols found")
	}
	return t, nil
}

func parseDwarfLines(dw *dwarf.Data) []Line {
	if dw == nil {
		return nil
	}
	r := dw.Reader()
	out := make([]Line, 0, 4096)
	for {
		ent, err := r.Next()
		if err != nil || ent == nil {
			break
		}
		if ent.Tag != dwarf.TagCompileUnit {
			if ent.Children {
				r.SkipChildren()
			}
			continue
		}
		lr, err := dw.LineReader(ent)
		if err != nil || lr == nil {
			continue
		}
		var le dwarf.LineEntry
		for {
			err := lr.Next(&le)
			if err != nil {
				break
			}
			if le.Address == 0 || le.File == nil || le.EndSequence {
				continue
			}
			out = append(out, Line{Addr: le.Address, File: le.File.Name, Line: le.Line, Column: le.Column})
			if len(out) >= 200000 {
				return out
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Addr == out[j].Addr {
			if out[i].File == out[j].File {
				return out[i].Line < out[j].Line
			}
			return out[i].File < out[j].File
		}
		return out[i].Addr < out[j].Addr
	})
	uniq := out[:0]
	var last Line
	for i, l := range out {
		if i > 0 && l.Addr == last.Addr && l.File == last.File && l.Line == last.Line {
			continue
		}
		uniq = append(uniq, l)
		last = l
	}
	return uniq
}

func (t *Table) normalize() {
	sort.Slice(t.Symbols, func(i, j int) bool {
		if t.Symbols[i].Addr == t.Symbols[j].Addr {
			return t.Symbols[i].Name < t.Symbols[j].Name
		}
		return t.Symbols[i].Addr < t.Symbols[j].Addr
	})
	out := t.Symbols[:0]
	var lastAddr uint64
	var lastName string
	for i, s := range t.Symbols {
		if i > 0 && s.Addr == lastAddr && s.Name == lastName {
			continue
		}
		out = append(out, s)
		lastAddr, lastName = s.Addr, s.Name
	}
	t.Symbols = out
}

func (t *Table) Count() int {
	if t == nil {
		return 0
	}
	return len(t.Symbols)
}

func (t *Table) Lookup(pc uint64) (Symbol, uint64, bool) {
	if t == nil || len(t.Symbols) == 0 {
		return Symbol{}, 0, false
	}
	i := sort.Search(len(t.Symbols), func(i int) bool { return t.Symbols[i].Addr > pc }) - 1
	if i < 0 {
		return Symbol{}, 0, false
	}
	s := t.Symbols[i]
	return s, pc - s.Addr, true
}

func (t *Table) FormatLookup(pc uint64) string {
	s, off, ok := t.Lookup(pc)
	if !ok {
		return "<no symbol>"
	}
	if off == 0 {
		return fmt.Sprintf("%s [%s] @ %#x", s.Name, s.Kind, s.Addr)
	}
	return fmt.Sprintf("%s+%#x [%s] @ %#x", s.Name, off, s.Kind, s.Addr)
}

func (t *Table) Around(pc uint64, radius int) string {
	if t == nil || len(t.Symbols) == 0 {
		return "<no symbols loaded>\n"
	}
	if radius < 0 {
		radius = 0
	}
	i := sort.Search(len(t.Symbols), func(i int) bool { return t.Symbols[i].Addr > pc }) - 1
	if i < 0 {
		i = 0
	}
	start := i - radius
	if start < 0 {
		start = 0
	}
	end := i + radius + 1
	if end > len(t.Symbols) {
		end = len(t.Symbols)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "symbols: source=%s count=%d pc=%#x -> %s\n", t.Source, len(t.Symbols), pc, t.FormatLookup(pc))
	for j := start; j < end; j++ {
		marker := "  "
		if j == i {
			marker = "> "
		}
		s := t.Symbols[j]
		fmt.Fprintf(&b, "%s%016x %-8s %s\n", marker, s.Addr, s.Kind, s.Name)
	}
	return b.String()
}

func (t *Table) Search(query string, max int) string {
	if t == nil || len(t.Symbols) == 0 {
		return "<no symbols loaded>\n"
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return t.Around(0, 0)
	}
	if max <= 0 || max > 256 {
		max = 32
	}
	var b strings.Builder
	count := 0
	fmt.Fprintf(&b, "symbol search: %q in %s\n", query, t.Source)
	for _, s := range t.Symbols {
		if strings.Contains(strings.ToLower(s.Name), query) {
			fmt.Fprintf(&b, "%016x %-8s %s\n", s.Addr, s.Kind, s.Name)
			count++
			if count >= max {
				break
			}
		}
	}
	if count == 0 {
		fmt.Fprintf(&b, "<no matches>\n")
	}
	return b.String()
}

func (t *Table) LineCount() int {
	if t == nil {
		return 0
	}
	return len(t.Lines)
}

func (t *Table) LookupLine(pc uint64) (Line, uint64, bool) {
	if t == nil || len(t.Lines) == 0 {
		return Line{}, 0, false
	}
	i := sort.Search(len(t.Lines), func(i int) bool { return t.Lines[i].Addr > pc }) - 1
	if i < 0 {
		return Line{}, 0, false
	}
	l := t.Lines[i]
	return l, pc - l.Addr, true
}

func (t *Table) FormatLine(pc uint64) string {
	l, off, ok := t.LookupLine(pc)
	if !ok {
		return "<no DWARF line>"
	}
	if off == 0 {
		return fmt.Sprintf("%s:%d @ %#x", l.File, l.Line, l.Addr)
	}
	return fmt.Sprintf("%s:%d+%#x @ %#x", l.File, l.Line, off, l.Addr)
}

func (t *Table) AroundLine(pc uint64, radius int) string {
	if t == nil || len(t.Lines) == 0 {
		return "<no DWARF line info loaded>\n"
	}
	if radius < 0 {
		radius = 0
	}
	i := sort.Search(len(t.Lines), func(i int) bool { return t.Lines[i].Addr > pc }) - 1
	if i < 0 {
		i = 0
	}
	start := i - radius
	if start < 0 {
		start = 0
	}
	end := i + radius + 1
	if end > len(t.Lines) {
		end = len(t.Lines)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "DWARF lines: source=%s count=%d pc=%#x -> %s\n", t.Source, len(t.Lines), pc, t.FormatLine(pc))
	for j := start; j < end; j++ {
		marker := "  "
		if j == i {
			marker = "> "
		}
		l := t.Lines[j]
		fmt.Fprintf(&b, "%s%016x %s:%d\n", marker, l.Addr, l.File, l.Line)
	}
	return b.String()
}

func (t *Table) FormatDetailed(pc uint64) string {
	if t == nil {
		return "<no symbols loaded>"
	}
	sym := t.FormatLookup(pc)
	line := t.FormatLine(pc)
	if line == "<no DWARF line>" {
		return sym
	}
	return sym + " | " + line
}

var tracePCRe = regexp.MustCompile(`\bpc=([0-9a-fA-F]{8,16})\b`)

func (t *Table) AnnotateTraceText(trace string, max int) string {
	if t == nil || len(t.Symbols) == 0 {
		return "<no symbols loaded>\n"
	}
	if max <= 0 || max > 2048 {
		max = 256
	}
	lines := strings.Split(strings.TrimSuffix(trace, "\n"), "\n")
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		if m := tracePCRe.FindStringSubmatch(line); len(m) == 2 {
			if pc, err := strconv.ParseUint(m[1], 16, 64); err == nil {
				b.WriteString("  => ")
				b.WriteString(t.FormatDetailed(pc))
			}
		}
		b.WriteByte('\n')
	}
	if len(lines) == 0 {
		b.WriteString("<empty trace>\n")
	}
	return b.String()
}

func (t *Table) LineSummary(maxFiles int) string {
	if t == nil || len(t.Lines) == 0 {
		return "<no DWARF line info loaded>\n"
	}
	if maxFiles <= 0 || maxFiles > 256 {
		maxFiles = 32
	}
	counts := map[string]int{}
	for _, l := range t.Lines {
		counts[l.File]++
	}
	type row struct {
		file  string
		count int
	}
	rows := make([]row, 0, len(counts))
	for f, c := range counts {
		rows = append(rows, row{f, c})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count == rows[j].count {
			return rows[i].file < rows[j].file
		}
		return rows[i].count > rows[j].count
	})
	if len(rows) > maxFiles {
		rows = rows[:maxFiles]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "DWARF line files: source=%s lines=%d files=%d\n", t.Source, len(t.Lines), len(counts))
	for _, r := range rows {
		fmt.Fprintf(&b, "%6d %s\n", r.count, r.file)
	}
	return b.String()
}

// AnalyzeLog scans panic/oops style text for hexadecimal program-counter-like
// addresses and resolves them through the table. It keeps first-seen order and
// de-duplicates addresses so browser-side log paste analysis stays readable.
func (t *Table) AnalyzeLog(text string, max int) string {
	if t == nil || len(t.Symbols) == 0 {
		return "<no symbols loaded>\n"
	}
	if max <= 0 || max > 256 {
		max = 64
	}
	var b strings.Builder
	fmt.Fprintf(&b, "log symbol analysis: source=%s\n", t.Source)
	seen := map[uint64]bool{}
	count := 0
	fields := splitAddressCandidates(text)
	for _, f := range fields {
		addr, ok := parseAddressCandidate(f)
		if !ok || seen[addr] {
			continue
		}
		seen[addr] = true
		fmt.Fprintf(&b, "%016x -> %s\n", addr, t.FormatLookup(addr))
		count++
		if count >= max {
			break
		}
	}
	if count == 0 {
		b.WriteString("<no hexadecimal addresses found>\n")
	}
	return b.String()
}

func splitAddressCandidates(text string) []string {
	isHex := func(r rune) bool {
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') || r == 'x' || r == 'X'
	}
	return strings.FieldsFunc(text, func(r rune) bool { return !isHex(r) })
}

func parseAddressCandidate(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 8 {
		return 0, false
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(s) < 8 || len(s) > 16 {
		return 0, false
	}
	addr, err := strconv.ParseUint(s, 16, 64)
	return addr, err == nil
}
