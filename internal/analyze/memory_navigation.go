package analyze

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type MemoryIndexSearchHit struct {
	Kind   string `json:"kind"`
	Addr   uint64 `json:"addr"`
	End    uint64 `json:"end,omitempty"`
	Type   string `json:"type,omitempty"`
	Label  string `json:"label,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func SearchMemoryIndex(idx MemoryIndex, query string, limit int) []MemoryIndexSearchHit {
	if limit <= 0 || limit > 256 {
		limit = 64
	}
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return nil
	}
	var addr *uint64
	if strings.HasPrefix(q, "0x") {
		if v, err := strconv.ParseUint(strings.TrimPrefix(q, "0x"), 16, 64); err == nil {
			addr = &v
		}
	}
	out := []MemoryIndexSearchHit{}
	for _, r := range idx.Ranges {
		if addr != nil && *addr >= r.Start && *addr <= r.End {
			out = append(out, MemoryIndexSearchHit{Kind: "range", Addr: r.Start, End: r.End, Label: r.Label, Detail: fmt.Sprintf("contains %#x", *addr)})
		} else if strings.Contains(strings.ToLower(r.Label), q) {
			out = append(out, MemoryIndexSearchHit{Kind: "range", Addr: r.Start, End: r.End, Label: r.Label})
		}
		for _, o := range r.Objects {
			end := objectEnd(o)
			match := false
			detail := o.Detail
			if addr != nil {
				match = *addr >= o.Addr && *addr <= end
				if match {
					detail = fmt.Sprintf("contains %#x; %s", *addr, o.Detail)
				}
			} else {
				lower := strings.ToLower(o.Type + " " + o.Detail)
				match = strings.Contains(lower, q)
			}
			if match {
				out = append(out, MemoryIndexSearchHit{Kind: "object", Addr: o.Addr, End: end, Type: o.Type, Detail: detail})
			}
		}
		if len(out) >= limit {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Addr < out[j].Addr })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func SearchMemoryIndexString(idx MemoryIndex, query string, limit int) string {
	hits := SearchMemoryIndex(idx, query, limit)
	if len(hits) == 0 {
		return fmt.Sprintf("<no memory index hits for %q>\n", strings.TrimSpace(query))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "memory index search %q hits=%d\n", strings.TrimSpace(query), len(hits))
	for _, h := range hits {
		if h.Kind == "range" {
			fmt.Fprintf(&b, "%-7s %#016x..%#016x %s %s\n", h.Kind, h.Addr, h.End, h.Label, h.Detail)
		} else {
			fmt.Fprintf(&b, "%-7s %#016x..%#016x %-22s %s\n", h.Kind, h.Addr, h.End, h.Type, h.Detail)
		}
	}
	return b.String()
}

type MemoryJumpHint struct {
	Address uint64 `json:"address"`
	Label   string `json:"label"`
	Reason  string `json:"reason"`
}

func MemoryJumpHints(idx MemoryIndex, max int) []MemoryJumpHint {
	if max <= 0 || max > 64 {
		max = 16
	}
	preferred := []string{"elf", "fdt", "linux-version-string", "opensbi-string", "kernel-cmdline-string", "gzip", "xz", "zstd", "cpio-newc", "squashfs", "ext4-super-magic", "busybox-string"}
	seen := map[string]bool{}
	out := []MemoryJumpHint{}
	for _, want := range preferred {
		for _, r := range idx.Ranges {
			for _, o := range r.Objects {
				if o.Type != want {
					continue
				}
				key := fmt.Sprintf("%s/%x", want, o.Addr)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, MemoryJumpHint{Address: o.Addr, Label: want, Reason: o.Detail})
				if len(out) >= max {
					return out
				}
			}
		}
	}
	return out
}

func MemoryJumpHintsString(idx MemoryIndex, max int) string {
	hints := MemoryJumpHints(idx, max)
	if len(hints) == 0 {
		return "<no memory jump hints>\n"
	}
	var b strings.Builder
	b.WriteString("memory jump hints\n")
	for _, h := range hints {
		fmt.Fprintf(&b, "%#016x %-22s %s\n", h.Address, h.Label, h.Reason)
	}
	return b.String()
}
