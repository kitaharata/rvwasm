package mem

import (
	"encoding/binary"
	"fmt"
	"strings"
)

type Device interface {
	Read(addr uint64, size int) (uint64, error)
	Write(addr uint64, size int, val uint64) error
	Tick(cycles uint64)
}

type region struct {
	base uint64
	size uint64
	dev  Device
	name string
}

// AccessBucket records coarse read/write activity for DRAM and MMIO regions.
// It is intentionally tiny so the browser UI can sample it frequently during
// boot without keeping per-access logs.
type AccessBucket struct {
	Name       string `json:"name"`
	Base       uint64 `json:"base"`
	Size       uint64 `json:"size"`
	ReadOps    uint64 `json:"read_ops"`
	ReadBytes  uint64 `json:"read_bytes"`
	WriteOps   uint64 `json:"write_ops"`
	WriteBytes uint64 `json:"write_bytes"`
}

// AccessEvent is a small rolling timeline entry for recent DRAM/MMIO activity.
// It complements the coarse histogram and is meant for boot/probe debugging,
// not full instruction-level tracing.
type AccessEvent struct {
	Seq   uint64 `json:"seq"`
	Name  string `json:"name"`
	Reg   string `json:"reg,omitempty"`
	Addr  uint64 `json:"addr"`
	Size  int    `json:"size"`
	Write bool   `json:"write"`
	Value uint64 `json:"value"`
}

const accessTimelineRingSize = 1024
const watchpointHitRingSize = 256

type WatchpointHit struct {
	Seq  uint64 `json:"seq"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	Op   string `json:"op"`
	Addr uint64 `json:"addr"`
	End  uint64 `json:"end"`
}

type Watchpoint struct {
	Base uint64
	Size uint64
	Name string
}

type Bus struct {
	DRAMBase uint64
	DRAM     []byte
	// TraceDRAMAccess controls whether ordinary DRAM data loads/stores are
	// recorded in the rolling access timeline. The histogram still counts DRAM
	// accesses, and MMIO accesses are always recorded. Keeping this off by
	// default avoids turning OpenSBI/libfdt string scans into a browser-side trace
	// bottleneck during boot.
	TraceDRAMAccess   bool
	regions           []region
	tickDevices       []Device
	writeWatchpoints  []Watchpoint
	readWatchpoints   []Watchpoint
	lastWatchpoint    string
	access            map[string]*AccessBucket
	accessTimeline    []AccessEvent
	accessTimelinePos int
	accessTimelineCnt int
	accessSeq         uint64
	watchpointHits    []WatchpointHit
	watchpointHitPos  int
	watchpointHitCnt  int
}

func NewBus(dramBase uint64, dramSize uint64) *Bus {
	b := &Bus{DRAMBase: dramBase, DRAM: make([]byte, dramSize), access: map[string]*AccessBucket{}}
	b.access["dram"] = &AccessBucket{Name: "dram", Base: dramBase, Size: dramSize}
	return b
}

func (b *Bus) ResetDRAM() { clear(b.DRAM) }

func (b *Bus) SetTraceDRAMAccess(enable bool) { b.TraceDRAMAccess = enable }

func (b *Bus) IsTraceDRAMAccessEnabled() bool { return b.TraceDRAMAccess }

func (b *Bus) AddDevice(base, size uint64, d Device) {
	b.AddNamedDevice(base, size, fmt.Sprintf("%T@%#x", d, base), d)
}

func (b *Bus) AddNamedDevice(base, size uint64, name string, d Device) {
	if name == "" {
		name = fmt.Sprintf("%T@%#x", d, base)
	}
	b.regions = append(b.regions, region{base: base, size: size, dev: d, name: name})
	if b.access == nil {
		b.access = map[string]*AccessBucket{}
	}
	if _, ok := b.access[name]; !ok {
		b.access[name] = &AccessBucket{Name: name, Base: base, Size: size}
	}
	// Only CLINT currently has time-dependent Tick side effects. The other MMIO
	// devices implement no-op Tick methods, so calling all of them every guest
	// instruction is wasted work in OpenSBI/libfdt hot loops.
	if name == "clint" {
		b.tickDevices = append(b.tickDevices, d)
	}
}

func (b *Bus) AddWriteWatchpoint(base, size uint64, name string) {
	if size == 0 {
		return
	}
	b.writeWatchpoints = append(b.writeWatchpoints, Watchpoint{Base: base, Size: size, Name: name})
}

func (b *Bus) AddReadWatchpoint(base, size uint64, name string) {
	if size == 0 {
		return
	}
	b.readWatchpoints = append(b.readWatchpoints, Watchpoint{Base: base, Size: size, Name: name})
}

func (b *Bus) ClearWriteWatchpoints() {
	b.writeWatchpoints = nil
	b.lastWatchpoint = ""
}

func (b *Bus) ClearReadWatchpoints() {
	b.readWatchpoints = nil
	b.lastWatchpoint = ""
}

func (b *Bus) LastWatchpoint() string { return b.lastWatchpoint }

func (b *Bus) ClearLastWatchpoint() { b.lastWatchpoint = "" }

func (b *Bus) WatchpointSummary() []Watchpoint {
	out := make([]Watchpoint, len(b.writeWatchpoints))
	copy(out, b.writeWatchpoints)
	return out
}

func (b *Bus) ReadWatchpointSummary() []Watchpoint {
	out := make([]Watchpoint, len(b.readWatchpoints))
	copy(out, b.readWatchpoints)
	return out
}

func (b *Bus) checkWriteWatchpoint(addr uint64, size int) {
	b.checkWatchpoint(addr, size, b.writeWatchpoints, "write", "write")
}

func (b *Bus) checkReadWatchpoint(addr uint64, size int) {
	b.checkWatchpoint(addr, size, b.readWatchpoints, "read", "read")
}

func (b *Bus) checkWatchpoint(addr uint64, size int, points []Watchpoint, kind, op string) {
	if size <= 0 || len(points) == 0 {
		return
	}
	end := addr + uint64(size)
	for _, w := range points {
		wend := w.Base + w.Size
		if addr < wend && end > w.Base {
			name := w.Name
			if name == "" {
				name = fmt.Sprintf("%#x+%#x", w.Base, w.Size)
			}
			b.lastWatchpoint = fmt.Sprintf("%s watchpoint %s hit by %s %#x..%#x", kind, name, op, addr, end-1)
			b.recordWatchpointHit(kind, name, op, addr, end-1)
			return
		}
	}
}

func (b *Bus) recordWatchpointHit(kind, name, op string, addr, end uint64) {
	if len(b.watchpointHits) != watchpointHitRingSize {
		b.watchpointHits = make([]WatchpointHit, watchpointHitRingSize)
	}
	hit := WatchpointHit{Seq: b.accessSeq + 1, Kind: kind, Name: name, Op: op, Addr: addr, End: end}
	b.watchpointHits[b.watchpointHitPos%watchpointHitRingSize] = hit
	b.watchpointHitPos++
	if b.watchpointHitCnt < watchpointHitRingSize {
		b.watchpointHitCnt++
	}
}

func (b *Bus) WatchpointHits(limit int) []WatchpointHit {
	if b.watchpointHitCnt == 0 {
		return nil
	}
	if limit <= 0 || limit > b.watchpointHitCnt {
		limit = b.watchpointHitCnt
	}
	start := b.watchpointHitPos - limit
	out := make([]WatchpointHit, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (start + i) % watchpointHitRingSize
		if idx < 0 {
			idx += watchpointHitRingSize
		}
		out = append(out, b.watchpointHits[idx])
	}
	return out
}

func (b *Bus) WatchpointHitsString(limit int, filter string) string {
	filter = strings.ToLower(strings.TrimSpace(filter))
	var out string
	for _, h := range b.WatchpointHits(limit) {
		line := fmt.Sprintf("%06d %-5s %-20s %-5s %#x..%#x", h.Seq, h.Kind, h.Name, h.Op, h.Addr, h.End)
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			out += line + "\n"
		}
	}
	if out == "" {
		return "<no watchpoint hits>\n"
	}
	return out
}

func (b *Bus) ClearWatchpointHits() {
	b.watchpointHits = nil
	b.watchpointHitPos = 0
	b.watchpointHitCnt = 0
}

func (b *Bus) InDRAM(addr uint64, size int) bool {
	if addr < b.DRAMBase {
		return false
	}
	off := addr - b.DRAMBase
	return off+uint64(size) <= uint64(len(b.DRAM))
}

func (b *Bus) dramOff(addr uint64, size int) (uint64, bool) {
	if !b.InDRAM(addr, size) {
		return 0, false
	}
	return addr - b.DRAMBase, true
}

func (b *Bus) findRegion(addr uint64, size int) *region {
	for i := range b.regions {
		r := &b.regions[i]
		if addr >= r.base && addr+uint64(size) <= r.base+r.size {
			return r
		}
	}
	return nil
}

func (b *Bus) find(addr uint64, size int) Device {
	if r := b.findRegion(addr, size); r != nil {
		return r.dev
	}
	return nil
}

func (b *Bus) noteAccess(name string, base, size uint64, isWrite bool, n int) {
	if b.access == nil {
		b.access = map[string]*AccessBucket{}
	}
	a := b.access[name]
	if a == nil {
		a = &AccessBucket{Name: name, Base: base, Size: size}
		b.access[name] = a
	}
	if isWrite {
		a.WriteOps++
		a.WriteBytes += uint64(n)
	} else {
		a.ReadOps++
		a.ReadBytes += uint64(n)
	}
}

func (b *Bus) recordAccessEvent(name string, base uint64, addr uint64, size int, isWrite bool, val uint64) {
	if size <= 0 {
		return
	}
	if len(b.accessTimeline) != accessTimelineRingSize {
		b.accessTimeline = make([]AccessEvent, accessTimelineRingSize)
	}
	b.accessSeq++
	b.accessTimeline[b.accessTimelinePos%accessTimelineRingSize] = AccessEvent{
		Seq:   b.accessSeq,
		Name:  name,
		Reg:   DecodeMMIORegister(name, base, addr),
		Addr:  addr,
		Size:  size,
		Write: isWrite,
		Value: val,
	}
	b.accessTimelinePos++
	if b.accessTimelineCnt < accessTimelineRingSize {
		b.accessTimelineCnt++
	}
}

func (b *Bus) AccessTimeline(limit int) []AccessEvent {
	if b.accessTimelineCnt == 0 {
		return nil
	}
	if limit <= 0 || limit > b.accessTimelineCnt {
		limit = b.accessTimelineCnt
	}
	start := b.accessTimelinePos - limit
	out := make([]AccessEvent, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (start + i) % accessTimelineRingSize
		if idx < 0 {
			idx += accessTimelineRingSize
		}
		out = append(out, b.accessTimeline[idx])
	}
	return out
}

func (b *Bus) AccessTimelineString(limit int, filter string) string {
	filter = strings.ToLower(strings.TrimSpace(filter))
	var out string
	for _, ev := range b.AccessTimeline(limit) {
		op := "R"
		if ev.Write {
			op = "W"
		}
		line := fmt.Sprintf("%06d %s %-15s %-22s addr=%#x size=%d val=%#x", ev.Seq, op, ev.Name, ev.Reg, ev.Addr, ev.Size, ev.Value)
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			out += line + "\n"
		}
	}
	return out
}

func (b *Bus) AccessTimelineCompactString(limit int, filter string) string {
	events := b.AccessTimeline(limit)
	filter = strings.ToLower(strings.TrimSpace(filter))
	type group struct {
		first, last AccessEvent
		count       int
	}
	groups := []group{}
	flush := func(g group) {
		if g.count == 0 {
			return
		}
		op := "R"
		if g.first.Write {
			op = "W"
		}
		line := fmt.Sprintf("%06d..%06d %s %-15s %-22s count=%d addr=%#x..%#x last=%#x", g.first.Seq, g.last.Seq, op, g.first.Name, g.first.Reg, g.count, g.first.Addr, g.last.Addr, g.last.Value)
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			groups = append(groups, g)
		}
	}
	cur := group{}
	for _, ev := range events {
		if cur.count == 0 {
			cur = group{first: ev, last: ev, count: 1}
			continue
		}
		prevEnd := cur.last.Addr + uint64(cur.last.Size)
		if ev.Name == cur.first.Name && ev.Reg == cur.first.Reg && ev.Write == cur.first.Write && ev.Size == cur.first.Size && (ev.Addr == prevEnd || ev.Addr == cur.last.Addr) {
			cur.last = ev
			cur.count++
		} else {
			flush(cur)
			cur = group{first: ev, last: ev, count: 1}
		}
	}
	flush(cur)
	var out string
	for _, g := range groups {
		op := "R"
		if g.first.Write {
			op = "W"
		}
		out += fmt.Sprintf("%06d..%06d %s %-15s %-22s count=%d addr=%#x..%#x last=%#x\n", g.first.Seq, g.last.Seq, op, g.first.Name, g.first.Reg, g.count, g.first.Addr, g.last.Addr, g.last.Value)
	}
	return out
}

func (b *Bus) ClearAccessTimeline() {
	b.accessTimeline = nil
	b.accessTimelinePos = 0
	b.accessTimelineCnt = 0
}

func (b *Bus) AccessHistogram() []AccessBucket {
	out := make([]AccessBucket, 0, len(b.access))
	for _, a := range b.access {
		out = append(out, *a)
	}
	return out
}

func (b *Bus) AccessHistogramString() string {
	var out string
	for _, a := range b.AccessHistogram() {
		out += fmt.Sprintf("%s base=%#x size=%#x r=%d/%dB w=%d/%dB\n", a.Name, a.Base, a.Size, a.ReadOps, a.ReadBytes, a.WriteOps, a.WriteBytes)
	}
	return out
}

func (b *Bus) ClearAccessHistogram() {
	for _, a := range b.access {
		a.ReadOps, a.ReadBytes, a.WriteOps, a.WriteBytes = 0, 0, 0, 0
	}
}

// DecodeMMIORegister returns a compact register name for known rvwasm MMIO regions.
// It is intentionally device-name based so UI traces remain useful even when the
// concrete device type is hidden behind the generic Bus.Device interface.
func DecodeMMIORegister(name string, base, addr uint64) string {
	if name == "dram" {
		return ""
	}
	off := addr - base
	if strings.HasPrefix(name, "virtio-") {
		if reg := decodeVirtioMMIO(off); reg != "" {
			return reg
		}
	}
	switch name {
	case "uart16550":
		return decodeUART16550(off)
	case "syscon":
		if off < 4 {
			return "reset"
		}
		return fmt.Sprintf("+%#x", off)
	case "clint":
		return decodeCLINT(off)
	case "plic":
		return decodePLIC(off)
	}
	return fmt.Sprintf("+%#x", off)
}

func decodeVirtioMMIO(off uint64) string {
	switch off &^ 3 {
	case 0x000:
		return "MagicValue"
	case 0x004:
		return "Version"
	case 0x008:
		return "DeviceID"
	case 0x00c:
		return "VendorID"
	case 0x010:
		return "DeviceFeatures"
	case 0x014:
		return "DeviceFeaturesSel"
	case 0x020:
		return "DriverFeatures"
	case 0x024:
		return "DriverFeaturesSel"
	case 0x030:
		return "QueueSel"
	case 0x034:
		return "QueueNumMax"
	case 0x038:
		return "QueueNum"
	case 0x044:
		return "QueueReady"
	case 0x050:
		return "QueueNotify"
	case 0x060:
		return "InterruptStatus"
	case 0x064:
		return "InterruptACK"
	case 0x070:
		return "Status"
	case 0x080:
		return "QueueDescLow"
	case 0x084:
		return "QueueDescHigh"
	case 0x090:
		return "QueueDriverLow"
	case 0x094:
		return "QueueDriverHigh"
	case 0x0a0:
		return "QueueDeviceLow"
	case 0x0a4:
		return "QueueDeviceHigh"
	case 0x0c0:
		return "QueueReset"
	case 0x0fc:
		return "ConfigGeneration"
	default:
		if off >= 0x100 {
			return fmt.Sprintf("Config+%#x", off-0x100)
		}
	}
	return fmt.Sprintf("+%#x", off)
}

func decodeUART16550(off uint64) string {
	switch off & 7 {
	case 0:
		return "RBR/THR/DLL"
	case 1:
		return "IER/DLM"
	case 2:
		return "IIR/FCR"
	case 3:
		return "LCR"
	case 4:
		return "MCR"
	case 5:
		return "LSR"
	case 6:
		return "MSR"
	case 7:
		return "SCR"
	}
	return fmt.Sprintf("+%#x", off)
}

func decodeCLINT(off uint64) string {
	if off < 0x4000 {
		return fmt.Sprintf("msip[%d]", off/4)
	}
	if off >= 0x4000 && off < 0xbff8 {
		return fmt.Sprintf("mtimecmp[%d]", (off-0x4000)/8)
	}
	if off >= 0xbff8 && off < 0xc000 {
		return "mtime"
	}
	return fmt.Sprintf("+%#x", off)
}

func decodePLIC(off uint64) string {
	switch {
	case off < 0x1000:
		return fmt.Sprintf("priority[%d]", off/4)
	case off >= 0x1000 && off < 0x1080:
		return fmt.Sprintf("pending[%d]", (off-0x1000)/4)
	case off >= 0x2000 && off < 0x200000:
		return fmt.Sprintf("enable+%#x", off-0x2000)
	case off >= 0x200000:
		ctx := (off - 0x200000) / 0x1000
		reg := (off - 0x200000) % 0x1000
		if reg == 0 {
			return fmt.Sprintf("ctx[%d].threshold", ctx)
		}
		if reg == 4 {
			return fmt.Sprintf("ctx[%d].claim", ctx)
		}
		return fmt.Sprintf("ctx[%d]+%#x", ctx, reg)
	}
	return fmt.Sprintf("+%#x", off)
}

// ReadNoTrace reads from the bus without updating access histograms/timelines or
// triggering read watchpoints. It is intended for high-frequency instruction
// fetches and UI peeks; architecturally visible data/MMIO loads should use Read.
func (b *Bus) ReadNoTrace(addr uint64, size int) (uint64, error) {
	if off, ok := b.dramOff(addr, size); ok {
		p := b.DRAM[off : off+uint64(size)]
		switch size {
		case 1:
			return uint64(p[0]), nil
		case 2:
			return uint64(binary.LittleEndian.Uint16(p)), nil
		case 4:
			return uint64(binary.LittleEndian.Uint32(p)), nil
		case 8:
			return binary.LittleEndian.Uint64(p), nil
		default:
			return 0, fmt.Errorf("bad read size %d", size)
		}
	}
	if r := b.findRegion(addr, size); r != nil {
		return r.dev.Read(addr, size)
	}
	return 0, fmt.Errorf("bus read fault at %#x size %d", addr, size)
}

func (b *Bus) Read(addr uint64, size int) (uint64, error) {
	b.checkReadWatchpoint(addr, size)
	if off, ok := b.dramOff(addr, size); ok {
		p := b.DRAM[off : off+uint64(size)]
		var v uint64
		switch size {
		case 1:
			v = uint64(p[0])
		case 2:
			v = uint64(binary.LittleEndian.Uint16(p))
		case 4:
			v = uint64(binary.LittleEndian.Uint32(p))
		case 8:
			v = binary.LittleEndian.Uint64(p)
		default:
			return 0, fmt.Errorf("bad read size %d", size)
		}
		if b.TraceDRAMAccess {
			b.noteAccess("dram", b.DRAMBase, uint64(len(b.DRAM)), false, size)
			b.recordAccessEvent("dram", b.DRAMBase, addr, size, false, v)
		}
		return v, nil
	}
	if r := b.findRegion(addr, size); r != nil {
		b.noteAccess(r.name, r.base, r.size, false, size)
		v, err := r.dev.Read(addr, size)
		if err == nil {
			b.recordAccessEvent(r.name, r.base, addr, size, false, v)
		}
		return v, err
	}
	return 0, fmt.Errorf("bus read fault at %#x size %d", addr, size)
}

func (b *Bus) Write(addr uint64, size int, val uint64) error {
	b.checkWriteWatchpoint(addr, size)
	if off, ok := b.dramOff(addr, size); ok {
		p := b.DRAM[off : off+uint64(size)]
		switch size {
		case 1:
			p[0] = byte(val)
		case 2:
			binary.LittleEndian.PutUint16(p, uint16(val))
		case 4:
			binary.LittleEndian.PutUint32(p, uint32(val))
		case 8:
			binary.LittleEndian.PutUint64(p, val)
		default:
			return fmt.Errorf("bad write size %d", size)
		}
		if b.TraceDRAMAccess {
			b.noteAccess("dram", b.DRAMBase, uint64(len(b.DRAM)), true, size)
			b.recordAccessEvent("dram", b.DRAMBase, addr, size, true, val)
		}
		return nil
	}
	if r := b.findRegion(addr, size); r != nil {
		b.noteAccess(r.name, r.base, r.size, true, size)
		err := r.dev.Write(addr, size, val)
		if err == nil {
			b.recordAccessEvent(r.name, r.base, addr, size, true, val)
		}
		return err
	}
	return fmt.Errorf("bus write fault at %#x size %d", addr, size)
}

func (b *Bus) Load(addr uint64, data []byte) error {
	if off, ok := b.dramOff(addr, len(data)); ok {
		copy(b.DRAM[off:], data)
		return nil
	}
	return fmt.Errorf("load outside DRAM at %#x len %d", addr, len(data))
}

func (b *Bus) Tick(cycles uint64) {
	for _, d := range b.tickDevices {
		d.Tick(cycles)
	}
}

// Peek reads guest physical DRAM without updating access histograms,
// timelines, watchpoints, or device side effects. It is intended for debug
// analyzers such as virtqueue/memory scanners.
func (b *Bus) Peek(addr uint64, size int) (uint64, bool) {
	if off, ok := b.dramOff(addr, size); ok {
		p := b.DRAM[off : off+uint64(size)]
		switch size {
		case 1:
			return uint64(p[0]), true
		case 2:
			return uint64(binary.LittleEndian.Uint16(p)), true
		case 4:
			return uint64(binary.LittleEndian.Uint32(p)), true
		case 8:
			return binary.LittleEndian.Uint64(p), true
		}
	}
	return 0, false
}

// PeekBytes returns a copy of guest physical DRAM without side effects.
func (b *Bus) PeekBytes(addr uint64, size int) ([]byte, bool) {
	if size < 0 {
		return nil, false
	}
	if off, ok := b.dramOff(addr, size); ok {
		out := make([]byte, size)
		copy(out, b.DRAM[off:off+uint64(size)])
		return out, true
	}
	return nil, false
}

func (b *Bus) PeekU16(addr uint64) (uint16, bool) {
	v, ok := b.Peek(addr, 2)
	return uint16(v), ok
}

func (b *Bus) PeekU32(addr uint64) (uint32, bool) {
	v, ok := b.Peek(addr, 4)
	return uint32(v), ok
}

func (b *Bus) PeekU64(addr uint64) (uint64, bool) { return b.Peek(addr, 8) }
