package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

// QueryHit is a small search result that can point at any diagnostic stream.
type QueryHit struct {
	Source string `json:"source"`
	Line   int    `json:"line,omitempty"`
	Score  int    `json:"score"`
	Text   string `json:"text"`
}

// QueryDiagnostics searches console/dmesg, trace, CSR trace, MMIO timeline,
// virtqueue anomalies and the guest-memory index with the same token query.
// It intentionally uses simple substring/token scoring so it works offline in
// the browser without extra indexes.
func QueryDiagnostics(query string, console string, trace string, csrTrace string, events []mem.AccessEvent, anomalies []VirtqueueAnomaly, idx MemoryIndex, limit int) []QueryHit {
	if limit <= 0 || limit > 512 {
		limit = 80
	}
	toks := queryTokens(query)
	if len(toks) == 0 {
		return nil
	}
	hits := []QueryHit{}
	addLines := func(source, text string) {
		for i, line := range strings.Split(text, "\n") {
			s := scoreLine(line, toks)
			if s > 0 {
				hits = append(hits, QueryHit{Source: source, Line: i + 1, Score: s, Text: strings.TrimRight(line, "\r")})
			}
		}
	}
	addLines("console", console)
	addLines("trace", trace)
	addLines("csr", csrTrace)
	for _, ev := range events {
		op := "R"
		if ev.Write {
			op = "W"
		}
		line := fmt.Sprintf("%06d %s %-15s %-22s addr=%#x size=%d val=%#x", ev.Seq, op, ev.Name, ev.Reg, ev.Addr, ev.Size, ev.Value)
		if s := scoreLine(line, toks); s > 0 {
			hits = append(hits, QueryHit{Source: "mmio", Line: int(ev.Seq), Score: s, Text: line})
		}
	}
	for _, a := range anomalies {
		line := fmt.Sprintf("%s %s q=%d head=%d %s", a.Severity, a.Device, a.Queue, a.Head, a.Detail)
		if s := scoreLine(line, toks); s > 0 {
			hits = append(hits, QueryHit{Source: "virtqueue-anomaly", Score: s, Text: line})
		}
	}
	for _, r := range idx.Ranges {
		line := fmt.Sprintf("%#x..%#x %s objects=%d", r.Start, r.End, r.Label, len(r.Objects))
		if s := scoreLine(line, toks); s > 0 {
			hits = append(hits, QueryHit{Source: "memory-index", Score: s, Text: line})
		}
		for _, o := range r.Objects {
			line := fmt.Sprintf("%#x %-22s size=%#x %s", o.Addr, o.Type, o.Size, o.Detail)
			if s := scoreLine(line, toks); s > 0 {
				hits = append(hits, QueryHit{Source: "memory-object", Score: s, Text: line})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			if hits[i].Source == hits[j].Source {
				return hits[i].Line < hits[j].Line
			}
			return hits[i].Source < hits[j].Source
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func QueryDiagnosticsString(query string, console string, trace string, csrTrace string, events []mem.AccessEvent, anomalies []VirtqueueAnomaly, idx MemoryIndex, limit int) string {
	hits := QueryDiagnostics(query, console, trace, csrTrace, events, anomalies, idx, limit)
	if len(hits) == 0 {
		return fmt.Sprintf("<no diagnostic hits for %q>\n", strings.TrimSpace(query))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diagnostic query %q hits=%d\n", strings.TrimSpace(query), len(hits))
	for _, h := range hits {
		loc := h.Source
		if h.Line != 0 {
			loc = fmt.Sprintf("%s:%d", h.Source, h.Line)
		}
		fmt.Fprintf(&b, "%3d %-22s %s\n", h.Score, loc, h.Text)
	}
	return b.String()
}

func queryTokens(q string) []string {
	raw := strings.Fields(strings.ToLower(strings.TrimSpace(q)))
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.Trim(t, `"'(),;`)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func scoreLine(line string, toks []string) int {
	lower := strings.ToLower(line)
	score := 0
	for _, t := range toks {
		if strings.Contains(lower, t) {
			score += 10 + len(t)
		} else {
			return 0
		}
	}
	return score
}
