package analyze

import (
	"fmt"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

type VirtqueueAnomaly struct {
	Severity string `json:"severity"`
	Device   string `json:"device"`
	Queue    uint64 `json:"queue"`
	Head     uint16 `json:"head,omitempty"`
	Detail   string `json:"detail"`
}

// VirtqueueAnomalies inspects queue setup plus recent descriptor chains for
// issues that frequently explain Linux virtio probe stalls.
func VirtqueueAnomalies(b *mem.Bus, events []mem.AccessEvent, maxHeads int) []VirtqueueAnomaly {
	out := []VirtqueueAnomaly{}
	for _, s := range VirtqueueSummary(events) {
		if s.Num > 32768 {
			out = append(out, VirtqueueAnomaly{"error", s.Device, s.Queue, 0, fmt.Sprintf("queue size too large: %d", s.Num)})
		}
		if s.Ready {
			if s.Num == 0 {
				out = append(out, VirtqueueAnomaly{"error", s.Device, s.Queue, 0, "QueueReady=1 with QueueNum=0"})
			}
			if s.Desc == 0 || s.Driver == 0 || s.DeviceArea == 0 {
				out = append(out, VirtqueueAnomaly{"error", s.Device, s.Queue, 0, fmt.Sprintf("ready queue has incomplete addresses desc=%#x driver=%#x device=%#x", s.Desc, s.Driver, s.DeviceArea)})
			}
			if s.Desc%16 != 0 {
				out = append(out, VirtqueueAnomaly{"warn", s.Device, s.Queue, 0, fmt.Sprintf("descriptor table is not 16-byte aligned: %#x", s.Desc)})
			}
			if s.Driver%2 != 0 || s.DeviceArea%4 != 0 {
				out = append(out, VirtqueueAnomaly{"warn", s.Device, s.Queue, 0, fmt.Sprintf("ring alignment looks odd driver=%#x device=%#x", s.Driver, s.DeviceArea)})
			}
		} else if s.NotifyCount != 0 {
			out = append(out, VirtqueueAnomaly{"warn", s.Device, s.Queue, 0, fmt.Sprintf("queue was notified %d times but is not ready", s.NotifyCount)})
		}
	}
	for _, c := range VirtqueueChains(b, events, maxHeads) {
		if c.Error != "" {
			out = append(out, VirtqueueAnomaly{"error", c.Device, c.Queue, c.Head, c.Error})
		}
		for _, d := range c.Descriptors {
			if d.Len == 0 {
				out = append(out, VirtqueueAnomaly{"warn", c.Device, c.Queue, c.Head, fmt.Sprintf("descriptor %d has zero length", d.Index)})
			}
			if d.Flags&vqDescFIndirect != 0 {
				if d.Len%16 != 0 {
					out = append(out, VirtqueueAnomaly{"error", c.Device, c.Queue, c.Head, fmt.Sprintf("indirect descriptor %d length %d is not a multiple of 16", d.Index, d.Len)})
				}
				continue
			}
			if d.Len != 0 && !b.InDRAM(d.Addr, int(d.Len)) {
				out = append(out, VirtqueueAnomaly{"error", c.Device, c.Queue, c.Head, fmt.Sprintf("descriptor %d buffer outside DRAM addr=%#x len=%d", d.Index, d.Addr, d.Len)})
			}
		}
		if len(c.Descriptors) > 32 {
			out = append(out, VirtqueueAnomaly{"warn", c.Device, c.Queue, c.Head, fmt.Sprintf("long descriptor chain: %d descriptors", len(c.Descriptors))})
		}
	}
	return out
}

func VirtqueueAnomaliesString(b *mem.Bus, events []mem.AccessEvent, maxHeads int) string {
	rows := VirtqueueAnomalies(b, events, maxHeads)
	if len(rows) == 0 {
		return "<no virtqueue anomalies detected>\n"
	}
	var sb strings.Builder
	for _, a := range rows {
		fmt.Fprintf(&sb, "%-5s %-15s q=%d", a.Severity, a.Device, a.Queue)
		if a.Head != 0 {
			fmt.Fprintf(&sb, " head=%d", a.Head)
		}
		fmt.Fprintf(&sb, " %s\n", a.Detail)
	}
	return sb.String()
}
