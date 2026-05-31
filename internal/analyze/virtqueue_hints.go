package analyze

import (
	"fmt"
	"strings"
)

// VirtqueueHint adds a pragmatic next-action suggestion to an anomaly.
type VirtqueueHint struct {
	Anomaly VirtqueueAnomaly `json:"anomaly"`
	Hint    string           `json:"hint"`
}

func VirtqueueAnomalyHints(rows []VirtqueueAnomaly) []VirtqueueHint {
	out := make([]VirtqueueHint, 0, len(rows))
	for _, a := range rows {
		out = append(out, VirtqueueHint{Anomaly: a, Hint: hintForVirtqueueAnomaly(a)})
	}
	return out
}

func VirtqueueAnomalyHintsString(rows []VirtqueueAnomaly) string {
	hints := VirtqueueAnomalyHints(rows)
	if len(hints) == 0 {
		return "<no virtqueue anomalies detected; no hints>\n"
	}
	var b strings.Builder
	for _, h := range hints {
		a := h.Anomaly
		fmt.Fprintf(&b, "%-5s %-15s q=%d", a.Severity, a.Device, a.Queue)
		if a.Head != 0 {
			fmt.Fprintf(&b, " head=%d", a.Head)
		}
		fmt.Fprintf(&b, " %s\n  hint: %s\n", a.Detail, h.Hint)
	}
	return b.String()
}

func hintForVirtqueueAnomaly(a VirtqueueAnomaly) string {
	d := strings.ToLower(a.Detail)
	switch {
	case strings.Contains(d, "queueready=1 with queuenum=0") || strings.Contains(d, "queue size too large"):
		return "Check the guest's QueueNum write path and whether the device reported QueueNumMax before DRIVER_OK. Linux often aborts probe when QueueNum is zero or larger than QueueNumMax."
	case strings.Contains(d, "incomplete addresses"):
		return "Verify QueueDescLow/High, QueueDriverLow/High and QueueDeviceLow/High writes. The driver should program all three physical addresses before QueueReady=1."
	case strings.Contains(d, "descriptor table is not 16-byte aligned"):
		return "Descriptor tables must be 16-byte aligned. Inspect DMA allocator alignment or a truncated high/low address register write."
	case strings.Contains(d, "ring alignment looks odd"):
		return "Avail ring should be at least 2-byte aligned and used ring 4-byte aligned. Recheck queue layout math for this queue size."
	case strings.Contains(d, "not ready"):
		return "QueueNotify arrived before QueueReady/DRIVER_OK. Look for failed feature negotiation or status reset between queue setup and notify."
	case strings.Contains(d, "outside dram"):
		return "The descriptor points outside guest DRAM. Confirm the guest physical address, high address registers, and whether an IOMMU/DMA offset assumption leaked in."
	case strings.Contains(d, "indirect") && strings.Contains(d, "multiple of 16"):
		return "Indirect descriptor tables are arrays of 16-byte descriptors. A non-multiple length usually means the descriptor length or endian decode is wrong."
	case strings.Contains(d, "zero length"):
		return "Zero-length descriptors are unusual for block/console/net data buffers. Check descriptor construction and whether the chain head is stale."
	case strings.Contains(d, "long descriptor chain"):
		return "Long chains may indicate a descriptor loop or a stale NEXT flag. Export the descriptor DOT graph and inspect NEXT edges."
	case strings.Contains(d, "loop"):
		return "A descriptor loop prevents queue completion. Inspect NEXT indices in Descriptor DOT and validate loop detection against the queue size."
	default:
		return "Compare this queue's register setup, descriptor chain, and device-specific last error in Diagnostics."
	}
}
