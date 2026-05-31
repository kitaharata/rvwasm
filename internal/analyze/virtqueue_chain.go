package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

const (
	vqDescFNext     = uint16(1)
	vqDescFWrite    = uint16(2)
	vqDescFIndirect = uint16(4)
)

type VirtqueueDescriptor struct {
	Index    uint16 `json:"index"`
	Addr     uint64 `json:"addr"`
	Len      uint32 `json:"len"`
	Flags    uint16 `json:"flags"`
	Next     uint16 `json:"next"`
	Readable bool   `json:"readable"`
	Writable bool   `json:"writable"`
	Indirect bool   `json:"indirect"`
	Preview  string `json:"preview,omitempty"`
}

type VirtqueueChain struct {
	Device       string                `json:"device"`
	Queue        uint64                `json:"queue"`
	Num          uint64                `json:"num"`
	Ready        bool                  `json:"ready"`
	AvailIdx     uint16                `json:"avail_idx"`
	UsedIdx      uint16                `json:"used_idx"`
	Head         uint16                `json:"head"`
	RingSlot     uint16                `json:"ring_slot"`
	Indirect     bool                  `json:"indirect"`
	Descriptors  []VirtqueueDescriptor `json:"descriptors"`
	Error        string                `json:"error,omitempty"`
	LastNotifyAt uint64                `json:"last_notify_seq"`
}

func VirtqueueChains(b *mem.Bus, events []mem.AccessEvent, maxHeads int) []VirtqueueChain {
	if b == nil {
		return nil
	}
	if maxHeads <= 0 || maxHeads > 64 {
		maxHeads = 8
	}
	states := VirtqueueSummary(events)
	out := make([]VirtqueueChain, 0)
	for _, s := range states {
		if !s.Ready || s.Num == 0 || s.Desc == 0 || s.Driver == 0 || s.DeviceArea == 0 || s.Num > 32768 {
			continue
		}
		availIdx, ok := b.PeekU16(s.Driver + 2)
		if !ok {
			out = append(out, VirtqueueChain{Device: s.Device, Queue: s.Queue, Num: s.Num, Ready: s.Ready, Error: fmt.Sprintf("cannot read avail idx at %#x", s.Driver+2)})
			continue
		}
		usedIdx, _ := b.PeekU16(s.DeviceArea + 2)
		count := int(availIdx)
		if count > maxHeads {
			count = maxHeads
		}
		if count > int(s.Num) {
			count = int(s.Num)
		}
		start := int(availIdx) - count
		if start < 0 {
			start = 0
		}
		for i := start; i < int(availIdx); i++ {
			slot := uint16(i % int(s.Num))
			head, ok := b.PeekU16(s.Driver + 4 + uint64(slot)*2)
			chain := VirtqueueChain{Device: s.Device, Queue: s.Queue, Num: s.Num, Ready: s.Ready, AvailIdx: availIdx, UsedIdx: usedIdx, RingSlot: slot, LastNotifyAt: s.LastNotifyAt}
			if !ok {
				chain.Error = fmt.Sprintf("cannot read avail ring[%d] at %#x", slot, s.Driver+4+uint64(slot)*2)
				out = append(out, chain)
				continue
			}
			chain.Head = head
			chain.Descriptors, chain.Indirect, chain.Error = walkDescriptorChain(b, s.Desc, uint16(s.Num), head, 16)
			out = append(out, chain)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Device == out[j].Device {
			if out[i].Queue == out[j].Queue {
				return out[i].RingSlot < out[j].RingSlot
			}
			return out[i].Queue < out[j].Queue
		}
		return out[i].Device < out[j].Device
	})
	return out
}

func walkDescriptorChain(b *mem.Bus, table uint64, num uint16, head uint16, maxDepth int) ([]VirtqueueDescriptor, bool, string) {
	if num == 0 || head >= num {
		return nil, false, fmt.Sprintf("head %d outside queue size %d", head, num)
	}
	seen := map[uint16]bool{}
	idx := head
	out := make([]VirtqueueDescriptor, 0, maxDepth)
	for depth := 0; depth < maxDepth; depth++ {
		if seen[idx] {
			return out, false, fmt.Sprintf("descriptor loop at %d", idx)
		}
		seen[idx] = true
		d, ok := readDesc(b, table, idx)
		if !ok {
			return out, false, fmt.Sprintf("cannot read descriptor %d at %#x", idx, table+uint64(idx)*16)
		}
		if d.Flags&vqDescFIndirect != 0 {
			d.Indirect = true
			out = append(out, d)
			if d.Len%16 != 0 || d.Len == 0 {
				return out, true, fmt.Sprintf("bad indirect table length %d", d.Len)
			}
			innerNum := uint16(d.Len / 16)
			if innerNum > 1024 {
				return out, true, fmt.Sprintf("indirect table too large: %d descriptors", innerNum)
			}
			inner, _, err := walkDescriptorChain(b, d.Addr, innerNum, 0, int(innerNum)+1)
			out = append(out, inner...)
			return out, true, err
		}
		out = append(out, d)
		if d.Flags&vqDescFNext == 0 {
			return out, false, ""
		}
		if d.Next >= num {
			return out, false, fmt.Sprintf("next %d outside queue size %d", d.Next, num)
		}
		idx = d.Next
	}
	return out, false, fmt.Sprintf("descriptor chain exceeded depth %d", maxDepth)
}

func readDesc(b *mem.Bus, table uint64, idx uint16) (VirtqueueDescriptor, bool) {
	base := table + uint64(idx)*16
	addr, ok := b.PeekU64(base)
	if !ok {
		return VirtqueueDescriptor{}, false
	}
	ln, ok := b.PeekU32(base + 8)
	if !ok {
		return VirtqueueDescriptor{}, false
	}
	flags, ok := b.PeekU16(base + 12)
	if !ok {
		return VirtqueueDescriptor{}, false
	}
	next, ok := b.PeekU16(base + 14)
	if !ok {
		return VirtqueueDescriptor{}, false
	}
	d := VirtqueueDescriptor{Index: idx, Addr: addr, Len: ln, Flags: flags, Next: next, Writable: flags&vqDescFWrite != 0, Readable: flags&vqDescFWrite == 0, Indirect: flags&vqDescFIndirect != 0}
	if ln > 0 && ln <= 64 && flags&vqDescFIndirect == 0 {
		if data, ok := b.PeekBytes(addr, int(ln)); ok {
			d.Preview = previewBytes(data)
		}
	}
	return d, true
}

func previewBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	limit := len(data)
	if limit > 32 {
		limit = 32
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		if i != 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02x", data[i])
	}
	if len(data) > limit {
		fmt.Fprintf(&b, " ...(+%d)", len(data)-limit)
	}
	return b.String()
}

func VirtqueueChainsString(b *mem.Bus, events []mem.AccessEvent, maxHeads int) string {
	chains := VirtqueueChains(b, events, maxHeads)
	if len(chains) == 0 {
		return "<no ready virtqueue descriptor chains found>\n"
	}
	var sb strings.Builder
	for _, c := range chains {
		fmt.Fprintf(&sb, "%s q=%d avail=%d used=%d slot=%d head=%d indirect=%v notifySeq=%d\n", c.Device, c.Queue, c.AvailIdx, c.UsedIdx, c.RingSlot, c.Head, c.Indirect, c.LastNotifyAt)
		if c.Error != "" {
			fmt.Fprintf(&sb, "  error: %s\n", c.Error)
		}
		for _, d := range c.Descriptors {
			dirs := "R"
			if d.Writable {
				dirs = "W"
			}
			fmt.Fprintf(&sb, "  [%03d] %s addr=%#x len=%d flags=%#x next=%d", d.Index, dirs, d.Addr, d.Len, d.Flags, d.Next)
			if d.Indirect {
				sb.WriteString(" indirect")
			}
			if d.Preview != "" {
				fmt.Fprintf(&sb, " data=%s", d.Preview)
			}
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
