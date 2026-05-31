package analyze

import (
	"fmt"
	"sort"
	"strings"
)

type VirtqueueTriageItem struct {
	Severity string           `json:"severity"`
	Priority int              `json:"priority"`
	Device   string           `json:"device"`
	Queue    uint64           `json:"queue"`
	Head     uint16           `json:"head,omitempty"`
	Detail   string           `json:"detail"`
	Hint     string           `json:"hint"`
	Anomaly  VirtqueueAnomaly `json:"anomaly"`
}

type VirtqueueTriageReport struct {
	Counts map[string]int        `json:"counts"`
	Items  []VirtqueueTriageItem `json:"items"`
}

func VirtqueueAnomalyTriage(rows []VirtqueueAnomaly) VirtqueueTriageReport {
	items := make([]VirtqueueTriageItem, 0, len(rows))
	counts := map[string]int{}
	for _, a := range rows {
		sev, pri := normalizeAnomalySeverity(a)
		counts[sev]++
		items = append(items, VirtqueueTriageItem{Severity: sev, Priority: pri, Device: a.Device, Queue: a.Queue, Head: a.Head, Detail: a.Detail, Hint: hintForVirtqueueAnomaly(a), Anomaly: a})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			if items[i].Device == items[j].Device {
				if items[i].Queue == items[j].Queue {
					return items[i].Detail < items[j].Detail
				}
				return items[i].Queue < items[j].Queue
			}
			return items[i].Device < items[j].Device
		}
		return items[i].Priority < items[j].Priority
	})
	return VirtqueueTriageReport{Counts: counts, Items: items}
}

func VirtqueueAnomalyTriageString(rows []VirtqueueAnomaly) string {
	rep := VirtqueueAnomalyTriage(rows)
	if len(rep.Items) == 0 {
		return "<no virtqueue anomalies detected>\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "virtqueue anomaly triage critical=%d error=%d warn=%d info=%d\n", rep.Counts["critical"], rep.Counts["error"], rep.Counts["warn"], rep.Counts["info"])
	for _, it := range rep.Items {
		fmt.Fprintf(&b, "%8s p=%d %-15s q=%d", it.Severity, it.Priority, it.Device, it.Queue)
		if it.Head != 0 {
			fmt.Fprintf(&b, " head=%d", it.Head)
		}
		fmt.Fprintf(&b, " %s\n  hint: %s\n", it.Detail, it.Hint)
	}
	return b.String()
}

func normalizeAnomalySeverity(a VirtqueueAnomaly) (string, int) {
	d := strings.ToLower(a.Detail)
	sev := strings.ToLower(a.Severity)
	switch {
	case strings.Contains(d, "outside dram") || strings.Contains(d, "loop") || strings.Contains(d, "incomplete addresses") || strings.Contains(d, "queueready=1 with queuenum=0"):
		return "critical", 0
	case sev == "error" || strings.Contains(d, "indirect") || strings.Contains(d, "too large"):
		return "error", 1
	case sev == "warn" || strings.Contains(d, "alignment") || strings.Contains(d, "not ready"):
		return "warn", 2
	default:
		return "info", 3
	}
}
