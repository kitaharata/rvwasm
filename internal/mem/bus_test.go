package mem

import (
	"strings"
	"testing"
)

func TestWriteWatchpointRecordsOverlap(t *testing.T) {
	b := NewBus(0x80000000, 0x1000)
	b.AddWriteWatchpoint(0x80000010, 0x10, "stack")
	if err := b.Write(0x80000008, 8, 0xdeadbeef); err != nil {
		t.Fatal(err)
	}
	if got := b.LastWatchpoint(); got != "" {
		t.Fatalf("unexpected pre-overlap watchpoint: %s", got)
	}
	if err := b.Write(0x80000018, 4, 0x1234); err != nil {
		t.Fatal(err)
	}
	if got := b.LastWatchpoint(); got == "" {
		t.Fatalf("expected watchpoint hit")
	}
	b.ClearWriteWatchpoints()
	if got := b.LastWatchpoint(); got != "" {
		t.Fatalf("watchpoint state not cleared: %s", got)
	}
}

func TestReadWatchpointRecordsOverlap(t *testing.T) {
	b := NewBus(0x80000000, 0x1000)
	if err := b.Load(0x80000000, []byte{1, 2, 3, 4, 5, 6, 7, 8}); err != nil {
		t.Fatal(err)
	}
	b.AddReadWatchpoint(0x80000004, 0x4, "mmio-probe")
	if _, err := b.Read(0x80000000, 4); err != nil {
		t.Fatal(err)
	}
	if got := b.LastWatchpoint(); got != "" {
		t.Fatalf("unexpected pre-overlap watchpoint: %s", got)
	}
	if _, err := b.Read(0x80000006, 1); err != nil {
		t.Fatal(err)
	}
	if got := b.LastWatchpoint(); got == "" || !strings.Contains(got, "read watchpoint") {
		t.Fatalf("expected read watchpoint hit, got %q", got)
	}
	b.ClearReadWatchpoints()
	if got := len(b.ReadWatchpointSummary()); got != 0 {
		t.Fatalf("expected cleared read watchpoints, got %d", got)
	}
}

func TestAccessHistogramCountsDRAMAndMMIO(t *testing.T) {
	b := NewBus(0x80000000, 0x1000)
	uart := &dummyDev{}
	b.AddNamedDevice(0x10000000, 0x100, "dummy-mmio", uart)
	if err := b.Write(0x80000000, 4, 0x12345678); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Read(0x80000000, 4); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(0x10000000, 1, 0xaa); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Read(0x10000000, 1); err != nil {
		t.Fatal(err)
	}
	text := b.AccessHistogramString()
	if !strings.Contains(text, "dram") || !strings.Contains(text, "dummy-mmio") || !strings.Contains(text, "r=1") || !strings.Contains(text, "w=1") {
		t.Fatalf("unexpected histogram:\n%s", text)
	}
	b.ClearAccessHistogram()
	for _, a := range b.AccessHistogram() {
		if a.ReadOps != 0 || a.WriteOps != 0 {
			t.Fatalf("histogram not cleared: %+v", a)
		}
	}
}

type dummyDev struct{}

func (*dummyDev) Read(addr uint64, size int) (uint64, error)    { return 0x55, nil }
func (*dummyDev) Write(addr uint64, size int, val uint64) error { return nil }
func (*dummyDev) Tick(cycles uint64)                            {}

func TestAccessTimelineAndCompact(t *testing.T) {
	b := NewBus(0x80000000, 0x1000)
	b.SetTraceDRAMAccess(true)
	if err := b.Write(0x80000000, 4, 0x1111); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(0x80000004, 4, 0x2222); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Read(0x80000008, 4); err != nil {
		t.Fatal(err)
	}
	line := b.AccessTimelineString(10, "dram")
	if !strings.Contains(line, "W dram") || !strings.Contains(line, "R dram") {
		t.Fatalf("timeline missing accesses:\n%s", line)
	}
	compact := b.AccessTimelineCompactString(10, "")
	if !strings.Contains(compact, "count=2") {
		t.Fatalf("expected adjacent writes to compact, got:\n%s", compact)
	}
	b.ClearAccessTimeline()
	if got := b.AccessTimelineString(10, ""); got != "" {
		t.Fatalf("timeline not cleared: %q", got)
	}
}

func TestDecodeMMIORegisterNames(t *testing.T) {
	cases := []struct {
		name string
		base uint64
		addr uint64
		want string
	}{
		{"virtio-gpu", 0x10006000, 0x10006050, "QueueNotify"},
		{"uart16550", 0x10000000, 0x10000005, "LSR"},
		{"clint", 0x02000000, 0x02004000, "mtimecmp[0]"},
		{"plic", 0x0c000000, 0x0c200004, "ctx[0].claim"},
	}
	for _, tc := range cases {
		if got := DecodeMMIORegister(tc.name, tc.base, tc.addr); got != tc.want {
			t.Fatalf("DecodeMMIORegister(%s,%#x,%#x)=%q want %q", tc.name, tc.base, tc.addr, got, tc.want)
		}
	}
}

func TestAccessTimelineIncludesDecodedRegister(t *testing.T) {
	bus := NewBus(0x80000000, 0x1000)
	dev := &dummyDev{}
	bus.AddNamedDevice(0x10000000, 0x100, "uart16550", dev)
	if _, err := bus.Read(0x10000005, 1); err != nil {
		t.Fatal(err)
	}
	events := bus.AccessTimeline(1)
	if len(events) != 1 || events[0].Reg != "LSR" {
		t.Fatalf("events=%+v", events)
	}
	if s := bus.AccessTimelineString(1, "LSR"); !strings.Contains(s, "LSR") {
		t.Fatalf("timeline did not include decoded register: %s", s)
	}
}
