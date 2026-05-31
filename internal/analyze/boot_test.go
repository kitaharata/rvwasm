package analyze

import (
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestBootTimelineCombinesConsoleAndMMIO(t *testing.T) {
	console := "OpenSBI v1.8\nLinux version 6.8.0\nKernel command line: console=ttyS0\n"
	events := []mem.AccessEvent{
		{Seq: 1, Name: "virtio-blk", Reg: "MagicValue", Addr: 0x10001000},
		{Seq: 2, Name: "virtio-blk", Reg: "Status", Write: true, Value: 0x3},
		{Seq: 3, Name: "virtio-blk", Reg: "QueueNotify", Write: true, Value: 0},
	}
	s := BootTimelineString(console, events, 20)
	for _, want := range []string{"opensbi", "linux-entry", "linux-cmdline", "probe-virtio-blk", "queue-notify-virtio-blk"} {
		if !strings.Contains(s, want) {
			t.Fatalf("timeline missing %q:\n%s", want, s)
		}
	}
}

func TestDeviceProbeString(t *testing.T) {
	events := []mem.AccessEvent{
		{Seq: 1, Name: "virtio-console", Reg: "MagicValue"},
		{Seq: 2, Name: "virtio-console", Reg: "DeviceID"},
		{Seq: 3, Name: "virtio-console", Reg: "Status", Write: true, Value: 0xb},
		{Seq: 4, Name: "virtio-console", Reg: "QueueReady", Write: true, Value: 1},
		{Seq: 5, Name: "virtio-console", Reg: "QueueNotify", Write: true, Value: 1},
	}
	s := DeviceProbeString(events)
	if !strings.Contains(s, "virtio-console") || !strings.Contains(s, "status=[0xb]") || !strings.Contains(s, "qNotify=1") {
		t.Fatalf("bad probe summary:\n%s", s)
	}
}

func TestVirtqueueString(t *testing.T) {
	events := []mem.AccessEvent{
		{Seq: 1, Name: "virtio-blk", Reg: "QueueSel", Write: true, Value: 0},
		{Seq: 2, Name: "virtio-blk", Reg: "QueueNum", Write: true, Value: 128},
		{Seq: 3, Name: "virtio-blk", Reg: "QueueDescLow", Write: true, Value: 0x1000},
		{Seq: 4, Name: "virtio-blk", Reg: "QueueDescHigh", Write: true, Value: 0x8},
		{Seq: 5, Name: "virtio-blk", Reg: "QueueReady", Write: true, Value: 1},
		{Seq: 6, Name: "virtio-blk", Reg: "QueueNotify", Write: true, Value: 0},
	}
	s := VirtqueueString(events)
	if !strings.Contains(s, "q=0") || !strings.Contains(s, "num=128") || !strings.Contains(s, "desc=0x800001000") || !strings.Contains(s, "notify=1") {
		t.Fatalf("bad virtqueue summary:\n%s", s)
	}
}

func TestPanicSummary(t *testing.T) {
	console := "normal\n[ 1.0] BUG: unable to handle kernel paging request\npc : ffffffff80001234\nKernel panic - not syncing\n"
	s := PanicSummary(console, nil, 20)
	if !strings.Contains(strings.ToLower(s), "panic/oops summary") || !strings.Contains(s, "ffffffff80001234") {
		t.Fatalf("bad panic summary:\n%s", s)
	}
}
