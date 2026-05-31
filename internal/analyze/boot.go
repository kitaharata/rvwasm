package analyze

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/debugmap"
	"github.com/kitaharata/rvwasm/internal/mem"
)

type BootEvent struct {
	Seq    uint64 `json:"seq"`
	Source string `json:"source"`
	Phase  string `json:"phase"`
	Detail string `json:"detail"`
}

var phaseMarkers = []struct {
	Needle string
	Phase  string
}{
	{"opensbi", "opensbi"},
	{"domain0", "opensbi-domain"},
	{"boot hart", "opensbi-hart"},
	{"linux version", "linux-entry"},
	{"kernel command line", "linux-cmdline"},
	{"sbi specification", "linux-sbi"},
	{"of: fdt", "linux-fdt"},
	{"serial:", "serial"},
	{"console [ttys0]", "console-uart"},
	{"console [hvc0]", "console-virtio"},
	{"virtio-mmio", "virtio-mmio"},
	{"virtio_blk", "virtio-blk"},
	{"virtio_console", "virtio-console"},
	{"virtio_net", "virtio-net"},
	{"virtio_rng", "virtio-rng"},
	{"virtio_gpu", "virtio-gpu"},
	{"simple-framebuffer", "simplefb"},
	{"vfs:", "vfs"},
	{"run /init", "init"},
	{"freeing unused kernel", "init"},
	{"kernel panic", "panic"},
	{"oops", "oops"},
	{"unable to handle", "fault"},
}

func BootTimeline(console string, events []mem.AccessEvent, limit int) []BootEvent {
	if limit <= 0 || limit > 512 {
		limit = 128
	}
	out := make([]BootEvent, 0, limit)
	seenPhaseLine := map[string]bool{}
	for i, raw := range strings.Split(console, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		for _, m := range phaseMarkers {
			if strings.Contains(lower, m.Needle) {
				key := m.Phase + ":" + line
				if seenPhaseLine[key] {
					continue
				}
				seenPhaseLine[key] = true
				out = append(out, BootEvent{Seq: uint64(i + 1), Source: "console", Phase: m.Phase, Detail: truncate(line, 180)})
				break
			}
		}
	}
	deviceSeen := map[string]bool{}
	for _, ev := range events {
		if ev.Name == "dram" || ev.Name == "" {
			continue
		}
		if strings.HasPrefix(ev.Name, "virtio-") {
			if !deviceSeen[ev.Name] {
				deviceSeen[ev.Name] = true
				out = append(out, BootEvent{Seq: 1_000_000 + ev.Seq, Source: "mmio", Phase: "probe-" + ev.Name, Detail: fmt.Sprintf("first %s access %s addr=%#x reg=%s", rw(ev.Write), ev.Name, ev.Addr, ev.Reg)})
			}
			if ev.Reg == "Status" && ev.Write {
				out = append(out, BootEvent{Seq: 1_000_000 + ev.Seq, Source: "mmio", Phase: "status-" + ev.Name, Detail: fmt.Sprintf("status=%#x", ev.Value)})
			}
			if ev.Reg == "QueueNotify" && ev.Write {
				out = append(out, BootEvent{Seq: 1_000_000 + ev.Seq, Source: "mmio", Phase: "queue-notify-" + ev.Name, Detail: fmt.Sprintf("queue=%d", ev.Value)})
			}
		}
		if ev.Name == "plic" && ev.Reg != "" && strings.Contains(ev.Reg, ".claim") {
			out = append(out, BootEvent{Seq: 1_000_000 + ev.Seq, Source: "mmio", Phase: "plic-claim", Detail: fmt.Sprintf("%s val=%#x", ev.Reg, ev.Value)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func BootTimelineString(console string, events []mem.AccessEvent, limit int) string {
	items := BootTimeline(console, events, limit)
	if len(items) == 0 {
		return "<no boot timeline events yet>\n"
	}
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "%09d %-8s %-22s %s\n", it.Seq, it.Source, it.Phase, it.Detail)
	}
	return b.String()
}

type DeviceProbe struct {
	Name          string   `json:"name"`
	Reads         uint64   `json:"reads"`
	Writes        uint64   `json:"writes"`
	IdentityReads uint64   `json:"identity_reads"`
	StatusWrites  []uint64 `json:"status_writes"`
	QueueReady    uint64   `json:"queue_ready_writes"`
	QueueNotify   uint64   `json:"queue_notify_writes"`
	Interrupts    uint64   `json:"interrupt_status_reads"`
	LastReg       string   `json:"last_reg"`
	LastValue     uint64   `json:"last_value"`
}

func DeviceProbeSummary(events []mem.AccessEvent) []DeviceProbe {
	by := map[string]*DeviceProbe{}
	order := []string{}
	for _, ev := range events {
		if !strings.HasPrefix(ev.Name, "virtio-") && ev.Name != "uart16550" && ev.Name != "plic" && ev.Name != "clint" {
			continue
		}
		p := by[ev.Name]
		if p == nil {
			p = &DeviceProbe{Name: ev.Name}
			by[ev.Name] = p
			order = append(order, ev.Name)
		}
		if ev.Write {
			p.Writes++
		} else {
			p.Reads++
		}
		switch ev.Reg {
		case "MagicValue", "Version", "DeviceID", "VendorID":
			if !ev.Write {
				p.IdentityReads++
			}
		case "Status":
			if ev.Write {
				p.StatusWrites = append(p.StatusWrites, ev.Value)
			}
		case "QueueReady":
			if ev.Write {
				p.QueueReady++
			}
		case "QueueNotify":
			if ev.Write {
				p.QueueNotify++
			}
		case "InterruptStatus":
			if !ev.Write {
				p.Interrupts++
			}
		}
		p.LastReg = ev.Reg
		p.LastValue = ev.Value
	}
	sort.Strings(order)
	out := make([]DeviceProbe, 0, len(order))
	for _, name := range order {
		out = append(out, *by[name])
	}
	return out
}

func DeviceProbeString(events []mem.AccessEvent) string {
	probes := DeviceProbeSummary(events)
	if len(probes) == 0 {
		return "<no device probe MMIO activity recorded>\n"
	}
	var b strings.Builder
	for _, p := range probes {
		fmt.Fprintf(&b, "%-15s reads=%d writes=%d idReads=%d qReady=%d qNotify=%d irqReads=%d last=%s:%#x status=%s\n",
			p.Name, p.Reads, p.Writes, p.IdentityReads, p.QueueReady, p.QueueNotify, p.Interrupts, p.LastReg, p.LastValue, compactHexList(p.StatusWrites, 10))
	}
	return b.String()
}

type VirtqueueState struct {
	Device       string `json:"device"`
	Queue        uint64 `json:"queue"`
	Num          uint64 `json:"num"`
	Ready        bool   `json:"ready"`
	Desc         uint64 `json:"desc"`
	Driver       uint64 `json:"driver"`
	DeviceArea   uint64 `json:"device_area"`
	NotifyCount  uint64 `json:"notify_count"`
	LastNotifyAt uint64 `json:"last_notify_seq"`
}

func VirtqueueSummary(events []mem.AccessEvent) []VirtqueueState {
	type key struct {
		dev string
		q   uint64
	}
	currentQ := map[string]uint64{}
	states := map[key]*VirtqueueState{}
	get := func(dev string, q uint64) *VirtqueueState {
		k := key{dev, q}
		s := states[k]
		if s == nil {
			s = &VirtqueueState{Device: dev, Queue: q}
			states[k] = s
		}
		return s
	}
	for _, ev := range events {
		if !strings.HasPrefix(ev.Name, "virtio-") || !ev.Write {
			continue
		}
		q := currentQ[ev.Name]
		s := get(ev.Name, q)
		switch ev.Reg {
		case "QueueSel":
			currentQ[ev.Name] = ev.Value
			get(ev.Name, ev.Value)
		case "QueueNum":
			s.Num = ev.Value
		case "QueueReady":
			s.Ready = ev.Value != 0
		case "QueueDescLow":
			s.Desc = (s.Desc &^ 0xffffffff) | (ev.Value & 0xffffffff)
		case "QueueDescHigh":
			s.Desc = (s.Desc & 0xffffffff) | (ev.Value << 32)
		case "QueueDriverLow":
			s.Driver = (s.Driver &^ 0xffffffff) | (ev.Value & 0xffffffff)
		case "QueueDriverHigh":
			s.Driver = (s.Driver & 0xffffffff) | (ev.Value << 32)
		case "QueueDeviceLow":
			s.DeviceArea = (s.DeviceArea &^ 0xffffffff) | (ev.Value & 0xffffffff)
		case "QueueDeviceHigh":
			s.DeviceArea = (s.DeviceArea & 0xffffffff) | (ev.Value << 32)
		case "QueueNotify":
			qs := get(ev.Name, ev.Value)
			qs.NotifyCount++
			qs.LastNotifyAt = ev.Seq
		}
	}
	out := make([]VirtqueueState, 0, len(states))
	for _, s := range states {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Device == out[j].Device {
			return out[i].Queue < out[j].Queue
		}
		return out[i].Device < out[j].Device
	})
	return out
}

func VirtqueueString(events []mem.AccessEvent) string {
	states := VirtqueueSummary(events)
	if len(states) == 0 {
		return "<no virtqueue setup/notify activity recorded>\n"
	}
	var b strings.Builder
	for _, s := range states {
		fmt.Fprintf(&b, "%-15s q=%d num=%d ready=%v desc=%#x avail=%#x used=%#x notify=%d lastNotifySeq=%d\n",
			s.Device, s.Queue, s.Num, s.Ready, s.Desc, s.Driver, s.DeviceArea, s.NotifyCount, s.LastNotifyAt)
	}
	return b.String()
}

var panicLineRe = regexp.MustCompile(`(?i)(kernel panic|oops|bug:|unable to handle|call trace|cpu:\s|pc\s*:|epc\s*:|ra\s*:|badaddr|fault|panic)`)

func PanicSummary(console string, symbols *debugmap.Table, maxLines int) string {
	if maxLines <= 0 || maxLines > 512 {
		maxLines = 120
	}
	lines := strings.Split(console, "\n")
	keep := make([]string, 0, maxLines)
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if line == "" {
			continue
		}
		if panicLineRe.MatchString(line) {
			start := i - 2
			if start < 0 {
				start = 0
			}
			end := i + 3
			if end > len(lines) {
				end = len(lines)
			}
			for j := start; j < end; j++ {
				l := strings.TrimRight(lines[j], "\r")
				if l != "" {
					keep = appendUniqueLine(keep, l, maxLines)
				}
			}
		}
	}
	var b strings.Builder
	if len(keep) == 0 {
		b.WriteString("<no panic/oops/fault signature found in console log>\n")
		return b.String()
	}
	b.WriteString("panic/oops summary:\n")
	for _, l := range keep {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if symbols != nil {
		b.WriteString("\n[symbolized addresses]\n")
		b.WriteString(symbols.AnalyzeLog(strings.Join(keep, "\n"), 64))
	}
	return b.String()
}

func appendUniqueLine(lines []string, line string, max int) []string {
	for _, x := range lines {
		if x == line {
			return lines
		}
	}
	lines = append(lines, line)
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}

func rw(write bool) string {
	if write {
		return "write"
	}
	return "read"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func compactHexList(vals []uint64, max int) string {
	if len(vals) == 0 {
		return "[]"
	}
	if max <= 0 || max > len(vals) {
		max = len(vals)
	}
	parts := make([]string, 0, max+1)
	for _, v := range vals[:max] {
		parts = append(parts, fmt.Sprintf("%#x", v))
	}
	if len(vals) > max {
		parts = append(parts, fmt.Sprintf("...(+%d)", len(vals)-max))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
