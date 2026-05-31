package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

type MemoryIndexRange struct {
	Start   uint64         `json:"start"`
	End     uint64         `json:"end"`
	Objects []MemoryObject `json:"objects"`
	Label   string         `json:"label"`
}

type MemoryIndex struct {
	DRAMBase uint64             `json:"dram_base"`
	DRAMSize uint64             `json:"dram_size"`
	Counts   map[string]int     `json:"counts"`
	Ranges   []MemoryIndexRange `json:"ranges"`
}

// BuildMemoryIndex groups nearby signature hits into ranges. It is useful when
// an initrd or kernel image contains many related strings/signatures and a flat
// scan is too noisy.
func BuildMemoryIndex(b *mem.Bus, maxPerType int, gap uint64) MemoryIndex {
	if gap == 0 {
		gap = 0x20000
	}
	objs := ScanMemoryObjects(b, maxPerType)
	idx := MemoryIndex{Counts: MemoryObjectTypeCounts(objs)}
	if b != nil {
		idx.DRAMBase = b.DRAMBase
		idx.DRAMSize = uint64(len(b.DRAM))
	}
	if len(objs) == 0 {
		return idx
	}
	sort.Slice(objs, func(i, j int) bool { return objs[i].Addr < objs[j].Addr })
	cur := MemoryIndexRange{Start: objs[0].Addr, End: objectEnd(objs[0]), Objects: []MemoryObject{objs[0]}}
	for _, o := range objs[1:] {
		oe := objectEnd(o)
		if o.Addr <= cur.End+gap {
			cur.Objects = append(cur.Objects, o)
			if oe > cur.End {
				cur.End = oe
			}
			continue
		}
		cur.Label = rangeLabel(cur.Objects)
		idx.Ranges = append(idx.Ranges, cur)
		cur = MemoryIndexRange{Start: o.Addr, End: oe, Objects: []MemoryObject{o}}
	}
	cur.Label = rangeLabel(cur.Objects)
	idx.Ranges = append(idx.Ranges, cur)
	return idx
}

func MemoryIndexString(b *mem.Bus, maxPerType int) string {
	idx := BuildMemoryIndex(b, maxPerType, 0x20000)
	if len(idx.Ranges) == 0 {
		return "<no indexed guest memory structures found>\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "DRAM %#x..%#x indexed ranges=%d\n", idx.DRAMBase, idx.DRAMBase+idx.DRAMSize, len(idx.Ranges))
	keys := make([]string, 0, len(idx.Counts))
	for k := range idx.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sb.WriteString("counts:")
	for _, k := range keys {
		fmt.Fprintf(&sb, " %s=%d", k, idx.Counts[k])
	}
	sb.WriteByte('\n')
	for i, r := range idx.Ranges {
		fmt.Fprintf(&sb, "\n[%02d] %#016x..%#016x %s objects=%d\n", i, r.Start, r.End, r.Label, len(r.Objects))
		limit := len(r.Objects)
		if limit > 16 {
			limit = 16
		}
		for _, o := range r.Objects[:limit] {
			if o.Size != 0 {
				fmt.Fprintf(&sb, "  %#016x %-22s size=%#x %s\n", o.Addr, o.Type, o.Size, o.Detail)
			} else {
				fmt.Fprintf(&sb, "  %#016x %-22s %s\n", o.Addr, o.Type, o.Detail)
			}
		}
		if len(r.Objects) > limit {
			fmt.Fprintf(&sb, "  ... %d more objects\n", len(r.Objects)-limit)
		}
	}
	return sb.String()
}

func objectEnd(o MemoryObject) uint64 {
	size := o.Size
	if size == 0 {
		size = 1
	}
	return o.Addr + size - 1
}

func rangeLabel(objs []MemoryObject) string {
	counts := map[string]int{}
	for _, o := range objs {
		counts[o.Type]++
	}
	preferred := []string{"elf", "fdt", "linux-version-string", "opensbi-string", "kernel-cmdline-string", "gzip", "xz", "zstd", "cpio-newc", "squashfs", "ext4-super-magic", "busybox-string"}
	parts := []string{}
	for _, p := range preferred {
		if counts[p] != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", p, counts[p]))
		}
	}
	if len(parts) == 0 {
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
			if len(parts) == 3 {
				break
			}
		}
	}
	return strings.Join(parts, " ")
}
