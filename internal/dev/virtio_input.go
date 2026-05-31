package dev

import (
	"encoding/binary"
	"fmt"
	"github.com/kitaharata/rvwasm/internal/mem"
)

const VirtioInputBase uint64 = 0x10005000

const (
	virtioDeviceInput      uint32 = 18
	virtioInputQueueSize          = 128
	virtioInputQueueEvent  uint32 = 0
	virtioInputQueueStatus uint32 = 1
	virtioInputQueueCount         = 2

	virtioInputCfgUnset    byte = 0x00
	virtioInputCfgIDName   byte = 0x01
	virtioInputCfgIDSerial byte = 0x02
	virtioInputCfgIDDevids byte = 0x03
	virtioInputCfgPropBits byte = 0x10
	virtioInputCfgEVBits   byte = 0x11

	virtioInputEvSyn uint16 = 0x00
	virtioInputEvKey uint16 = 0x01
	virtioInputEvRel uint16 = 0x02
)

type VirtioInputEvent struct {
	Type  uint16
	Code  uint16
	Value int32
}

type VirtioInput struct {
	bus *mem.Bus
	IRQ func(bool)

	events       []VirtioInputEvent
	statusEvents []VirtioInputEvent

	deviceFeaturesSel uint32
	driverFeaturesSel uint32
	driverFeatures    [2]uint32
	queueSel          uint32
	queues            [virtioInputQueueCount]virtQueue
	interruptStatus   uint32
	status            uint32
	configGeneration  uint32
	lastErr           string

	cfgSelect        byte
	cfgSubsel        byte
	eventsSent       uint64
	statusesReceived uint64
}

func NewVirtioInput(bus *mem.Bus, irq func(bool)) *VirtioInput {
	return &VirtioInput{bus: bus, IRQ: irq}
}

func (v *VirtioInput) LastError() string { return v.lastErr }

func (v *VirtioInput) DebugString() string {
	q0 := v.queues[0]
	q1 := v.queues[1]
	return fmt.Sprintf("status=%#x isr=%#x qsel=%d eventReady=%v eventLast=%d statusReady=%v statusLast=%d pendingEvents=%d sent=%d statusEvents=%d features[0]=%#x features[1]=%#x cfg=%#x/%#x err=%q",
		v.status, v.interruptStatus, v.queueSel, q0.ready, q0.lastAvail, q1.ready, q1.lastAvail, len(v.events), v.eventsSent, v.statusesReceived, v.driverFeatures[0], v.driverFeatures[1], v.cfgSelect, v.cfgSubsel, v.lastErr)
}

func (v *VirtioInput) InjectEvent(typ, code uint16, value int32) {
	v.events = append(v.events, VirtioInputEvent{Type: typ, Code: code, Value: value})
	v.processEventQueue()
}

func (v *VirtioInput) InjectKey(code uint16, down bool) {
	val := int32(0)
	if down {
		val = 1
	}
	v.events = append(v.events,
		VirtioInputEvent{Type: virtioInputEvKey, Code: code, Value: val},
		VirtioInputEvent{Type: virtioInputEvSyn, Code: 0, Value: 0},
	)
	v.processEventQueue()
}

func (v *VirtioInput) deviceFeatures(sel uint32) uint32 {
	switch sel {
	case 0:
		return (1 << virtioRingFIndirectDescBit) | (1 << virtioRingFEventIdxBit)
	case virtioFVersion1Bit / 32:
		return 1 << (virtioFVersion1Bit % 32)
	default:
		return 0
	}
}

func (v *VirtioInput) Read(addr uint64, size int) (uint64, error) {
	off := addr - VirtioInputBase
	read32 := func(x uint32) (uint64, error) { return partialRead32(x, off&3, size), nil }
	switch off &^ 3 {
	case 0x000:
		return read32(virtioMMIOMagic)
	case 0x004:
		return read32(virtioVersionModern)
	case 0x008:
		return read32(virtioDeviceInput)
	case 0x00c:
		return read32(virtioVendorRVWASM)
	case 0x010:
		return read32(v.deviceFeatures(v.deviceFeaturesSel))
	case 0x014:
		return read32(v.deviceFeaturesSel)
	case 0x020:
		if v.driverFeaturesSel < uint32(len(v.driverFeatures)) {
			return read32(v.driverFeatures[v.driverFeaturesSel])
		}
		return 0, nil
	case 0x024:
		return read32(v.driverFeaturesSel)
	case 0x030:
		return read32(v.queueSel)
	case 0x034:
		if v.queueSel < virtioInputQueueCount {
			return read32(virtioInputQueueSize)
		}
		return 0, nil
	case 0x038:
		if q := v.selectedQueue(); q != nil {
			return read32(q.num)
		}
		return 0, nil
	case 0x044:
		if q := v.selectedQueue(); q != nil {
			return read32(bool32(q.ready))
		}
		return 0, nil
	case 0x060:
		return read32(v.interruptStatus)
	case 0x070:
		return read32(v.status)
	case 0x080:
		if q := v.selectedQueue(); q != nil {
			return read32(uint32(q.desc))
		}
		return 0, nil
	case 0x084:
		if q := v.selectedQueue(); q != nil {
			return read32(uint32(q.desc >> 32))
		}
		return 0, nil
	case 0x090:
		if q := v.selectedQueue(); q != nil {
			return read32(uint32(q.driver))
		}
		return 0, nil
	case 0x094:
		if q := v.selectedQueue(); q != nil {
			return read32(uint32(q.driver >> 32))
		}
		return 0, nil
	case 0x0a0:
		if q := v.selectedQueue(); q != nil {
			return read32(uint32(q.device))
		}
		return 0, nil
	case 0x0a4:
		if q := v.selectedQueue(); q != nil {
			return read32(uint32(q.device >> 32))
		}
		return 0, nil
	case 0x0c0:
		return read32(0)
	case 0x0fc:
		return read32(v.configGeneration)
	}
	if off >= 0x100 && off < 0x180 {
		return v.readConfig(off-0x100, size), nil
	}
	return 0, nil
}

func (v *VirtioInput) Write(addr uint64, size int, val uint64) error {
	off := addr - VirtioInputBase
	write32 := func(cur uint32) uint32 { return partialWrite32(cur, off&3, size, val) }
	switch off &^ 3 {
	case 0x014:
		v.deviceFeaturesSel = write32(v.deviceFeaturesSel)
	case 0x020:
		if v.driverFeaturesSel < uint32(len(v.driverFeatures)) {
			v.driverFeatures[v.driverFeaturesSel] = write32(v.driverFeatures[v.driverFeaturesSel])
		}
	case 0x024:
		v.driverFeaturesSel = write32(v.driverFeaturesSel)
	case 0x030:
		v.queueSel = write32(v.queueSel)
	case 0x038:
		if q := v.selectedQueue(); q != nil {
			q.num = write32(q.num)
		}
	case 0x044:
		if q := v.selectedQueue(); q != nil {
			ready := write32(bool32(q.ready)) != 0
			if !ready {
				q.lastAvail = 0
			}
			q.ready = ready
			if ready && v.queueSel == virtioInputQueueEvent {
				v.processEventQueue()
			}
		}
	case 0x050:
		qidx := write32(0)
		switch qidx {
		case virtioInputQueueEvent:
			v.processEventQueue()
		case virtioInputQueueStatus:
			v.processStatusQueue()
		}
	case 0x064:
		v.interruptStatus &^= write32(0)
		v.updateIRQ()
	case 0x070:
		newStatus := write32(v.status)
		if newStatus == 0 {
			v.reset()
		} else {
			if newStatus&virtioStatusFeatures != 0 && !v.featuresAccepted() {
				newStatus &^= virtioStatusFeatures
			}
			if newStatus&virtioStatusDriverOK != 0 && newStatus&virtioStatusFeatures == 0 {
				v.fail("DRIVER_OK before FEATURES_OK")
				newStatus |= virtioStatusFailed
			}
			v.status = newStatus
			v.processEventQueue()
			v.processStatusQueue()
		}
	case 0x080:
		if q := v.selectedQueue(); q != nil {
			q.desc = (q.desc &^ uint64(0xffffffff)) | uint64(write32(uint32(q.desc)))
		}
	case 0x084:
		if q := v.selectedQueue(); q != nil {
			q.desc = (q.desc & 0xffffffff) | (uint64(write32(uint32(q.desc>>32))) << 32)
		}
	case 0x090:
		if q := v.selectedQueue(); q != nil {
			q.driver = (q.driver &^ uint64(0xffffffff)) | uint64(write32(uint32(q.driver)))
		}
	case 0x094:
		if q := v.selectedQueue(); q != nil {
			q.driver = (q.driver & 0xffffffff) | (uint64(write32(uint32(q.driver>>32))) << 32)
		}
	case 0x0a0:
		if q := v.selectedQueue(); q != nil {
			q.device = (q.device &^ uint64(0xffffffff)) | uint64(write32(uint32(q.device)))
		}
	case 0x0a4:
		if q := v.selectedQueue(); q != nil {
			q.device = (q.device & 0xffffffff) | (uint64(write32(uint32(q.device>>32))) << 32)
		}
	case 0x0c0:
		if write32(0) != 0 && v.queueSel < virtioInputQueueCount {
			v.queues[v.queueSel] = virtQueue{}
			v.interruptStatus = 0
			v.updateIRQ()
		}
	}
	if off == 0x100 && size == 1 {
		v.cfgSelect = byte(val)
		v.configGeneration++
	}
	if off == 0x101 && size == 1 {
		v.cfgSubsel = byte(val)
		v.configGeneration++
	}
	return nil
}

func (v *VirtioInput) Tick(cycles uint64) {}

func (v *VirtioInput) selectedQueue() *virtQueue {
	if v.queueSel >= virtioInputQueueCount {
		return nil
	}
	return &v.queues[v.queueSel]
}

func (v *VirtioInput) reset() {
	v.deviceFeaturesSel = 0
	v.driverFeaturesSel = 0
	v.driverFeatures = [2]uint32{}
	v.queueSel = 0
	v.queues = [virtioInputQueueCount]virtQueue{}
	v.interruptStatus = 0
	v.status = 0
	v.lastErr = ""
	v.events = nil
	v.statusEvents = nil
	v.cfgSelect = 0
	v.cfgSubsel = 0
	v.updateIRQ()
}

func (v *VirtioInput) updateIRQ() {
	if v.IRQ != nil {
		v.IRQ(v.interruptStatus != 0)
	}
}

func (v *VirtioInput) featuresAccepted() bool {
	for sel, got := range v.driverFeatures {
		allowed := v.deviceFeatures(uint32(sel))
		if got&^allowed != 0 {
			return false
		}
	}
	return v.driverFeatures[virtioFVersion1Bit/32]&(1<<(virtioFVersion1Bit%32)) != 0
}

func (v *VirtioInput) featureNegotiated(sel uint32, bit uint32) bool {
	if sel >= uint32(len(v.driverFeatures)) {
		return false
	}
	return v.driverFeatures[sel]&(1<<bit) != 0
}

func (v *VirtioInput) queueReady(q *virtQueue) bool {
	return v.status&virtioStatusDriverOK != 0 && q.ready && q.num != 0 && q.num <= virtioInputQueueSize && q.desc != 0 && q.driver != 0 && q.device != 0
}

func (v *VirtioInput) processEventQueue() {
	q := &v.queues[virtioInputQueueEvent]
	if len(v.events) == 0 || !v.queueReady(q) {
		return
	}
	for len(v.events) > 0 {
		availIdx, err := v.read16(q.driver + 2)
		if err != nil || q.lastAvail == availIdx {
			return
		}
		head, err := v.read16(q.driver + 4 + uint64(q.lastAvail%uint16(q.num))*2)
		if err != nil {
			v.fail("read event avail ring: %v", err)
			return
		}
		usedLen := v.fillEvent(head, q)
		v.publishUsed(q, head, usedLen)
		if usedLen == 0 {
			return
		}
	}
}

func (v *VirtioInput) processStatusQueue() {
	q := &v.queues[virtioInputQueueStatus]
	if !v.queueReady(q) {
		return
	}
	for {
		availIdx, err := v.read16(q.driver + 2)
		if err != nil || q.lastAvail == availIdx {
			return
		}
		head, err := v.read16(q.driver + 4 + uint64(q.lastAvail%uint16(q.num))*2)
		if err != nil {
			v.fail("read status avail ring: %v", err)
			return
		}
		usedLen := v.consumeStatus(head, q)
		v.publishUsed(q, head, usedLen)
	}
}

func (v *VirtioInput) fillEvent(head uint16, q *virtQueue) uint32 {
	descs, err := v.readChain(head, q)
	if err != nil {
		v.fail("event descriptor chain: %v", err)
		return 0
	}
	var used uint32
	for _, d := range descs {
		if d.flags&virtqDescFWrite == 0 || d.len < 8 {
			continue
		}
		for d.len-used >= 8 && len(v.events) > 0 {
			ev := v.events[0]
			v.events = v.events[1:]
			var buf [8]byte
			binary.LittleEndian.PutUint16(buf[0:2], ev.Type)
			binary.LittleEndian.PutUint16(buf[2:4], ev.Code)
			binary.LittleEndian.PutUint32(buf[4:8], uint32(ev.Value))
			if err := v.copyToGuest(d.addr+uint64(used), buf[:]); err != nil {
				v.fail("write input event: %v", err)
				return used
			}
			used += 8
			v.eventsSent++
		}
		if len(v.events) == 0 {
			break
		}
	}
	return used
}

func (v *VirtioInput) consumeStatus(head uint16, q *virtQueue) uint32 {
	descs, err := v.readChain(head, q)
	if err != nil {
		v.fail("status descriptor chain: %v", err)
		return 0
	}
	var used uint32
	for _, d := range descs {
		if d.flags&virtqDescFWrite != 0 || d.len < 8 {
			continue
		}
		buf := make([]byte, d.len)
		if err := v.copyFromGuest(buf, d.addr); err != nil {
			v.fail("read input status event: %v", err)
			return used
		}
		for off := 0; off+8 <= len(buf); off += 8 {
			v.statusEvents = append(v.statusEvents, VirtioInputEvent{Type: binary.LittleEndian.Uint16(buf[off:]), Code: binary.LittleEndian.Uint16(buf[off+2:]), Value: int32(binary.LittleEndian.Uint32(buf[off+4:]))})
			v.statusesReceived++
		}
		used += d.len
	}
	return used
}

func (v *VirtioInput) publishUsed(q *virtQueue, head uint16, usedLen uint32) {
	usedIdx, err := v.read16(q.device + 2)
	if err != nil {
		v.fail("read used idx: %v", err)
		return
	}
	newUsedIdx := usedIdx + 1
	usedOff := q.device + 4 + uint64(usedIdx%uint16(q.num))*8
	_ = v.write32(usedOff, uint32(head))
	_ = v.write32(usedOff+4, usedLen)
	_ = v.write16(q.device+2, newUsedIdx)
	q.lastAvail++
	if v.shouldInterrupt(q, usedIdx, newUsedIdx) {
		v.interruptStatus |= virtioIRQUsedBuffer
		v.updateIRQ()
	}
}

func (v *VirtioInput) readChain(head uint16, q *virtQueue) ([]virtqDesc, error) {
	if uint32(head) >= q.num {
		return nil, fmt.Errorf("head %d outside queue num %d", head, q.num)
	}
	d, err := v.readDescAt(q.desc, head)
	if err != nil {
		return nil, err
	}
	if d.flags&virtqDescFIndirect != 0 {
		return v.readIndirectChain(d)
	}
	return v.readDirectChain(head, q.desc, uint16(q.num), true)
}

func (v *VirtioInput) readDirectChain(head uint16, table uint64, limit uint16, allowIndirect bool) ([]virtqDesc, error) {
	var out []virtqDesc
	seen := map[uint16]bool{}
	id := head
	for step := 0; step < int(limit); step++ {
		if id >= limit {
			return nil, fmt.Errorf("descriptor %d outside table size %d", id, limit)
		}
		if seen[id] {
			return nil, fmt.Errorf("descriptor loop at %d", id)
		}
		seen[id] = true
		d, err := v.readDescAt(table, id)
		if err != nil {
			return nil, err
		}
		if d.flags&virtqDescFIndirect != 0 {
			if !allowIndirect || len(out) != 0 {
				return nil, fmt.Errorf("invalid nested/late indirect descriptor")
			}
			return v.readIndirectChain(d)
		}
		out = append(out, d)
		if d.flags&virtqDescFNext == 0 {
			return out, nil
		}
		id = d.next
	}
	return nil, fmt.Errorf("descriptor chain too long")
}

func (v *VirtioInput) readIndirectChain(d virtqDesc) ([]virtqDesc, error) {
	if d.len == 0 || d.len%16 != 0 {
		return nil, fmt.Errorf("bad indirect table length %d", d.len)
	}
	limit := d.len / 16
	if limit > 1024 {
		return nil, fmt.Errorf("indirect table too large: %d descriptors", limit)
	}
	return v.readDirectChain(0, d.addr, uint16(limit), false)
}

func (v *VirtioInput) readDescAt(table uint64, id uint16) (virtqDesc, error) {
	base := table + uint64(id)*16
	addr, err := v.read64(base)
	if err != nil {
		return virtqDesc{}, err
	}
	ln, err := v.read32(base + 8)
	if err != nil {
		return virtqDesc{}, err
	}
	flags, err := v.read16(base + 12)
	if err != nil {
		return virtqDesc{}, err
	}
	next, err := v.read16(base + 14)
	if err != nil {
		return virtqDesc{}, err
	}
	return virtqDesc{addr: addr, len: ln, flags: flags, next: next}, nil
}

func (v *VirtioInput) shouldInterrupt(q *virtQueue, oldUsedIdx, newUsedIdx uint16) bool {
	if v.featureNegotiated(virtioRingFEventIdxBit/32, virtioRingFEventIdxBit%32) {
		event, err := v.read16(q.device + 4 + uint64(q.num)*8)
		if err != nil {
			v.fail("read used event: %v", err)
			return true
		}
		return virtqNeedEvent(event, newUsedIdx, oldUsedIdx)
	}
	availFlags, err := v.read16(q.driver)
	if err != nil {
		v.fail("read avail flags: %v", err)
		return true
	}
	return availFlags&1 == 0
}

func (v *VirtioInput) readConfig(off uint64, size int) uint64 {
	var cfg [128]byte
	cfg[0] = v.cfgSelect
	cfg[1] = v.cfgSubsel
	data := v.configData()
	if len(data) > 0xff {
		data = data[:0xff]
	}
	cfg[2] = byte(len(data))
	copy(cfg[8:], data)
	if off >= uint64(len(cfg)) {
		return 0
	}
	if off+uint64(size) > uint64(len(cfg)) {
		size = int(uint64(len(cfg)) - off)
	}
	var out uint64
	for i := 0; i < size; i++ {
		out |= uint64(cfg[off+uint64(i)]) << (8 * i)
	}
	return out
}

func (v *VirtioInput) configData() []byte {
	switch v.cfgSelect {
	case virtioInputCfgIDName:
		return []byte("rvwasm virtio keyboard")
	case virtioInputCfgIDSerial:
		return []byte("rvwasm-input0")
	case virtioInputCfgIDDevids:
		// bustype/vendor/product/version, little-endian u16 values.
		data := make([]byte, 8)
		binary.LittleEndian.PutUint16(data[0:2], 0x06)   // BUS_VIRTUAL
		binary.LittleEndian.PutUint16(data[2:4], 0x5257) // RV
		binary.LittleEndian.PutUint16(data[4:6], 0x0001)
		binary.LittleEndian.PutUint16(data[6:8], 0x0001)
		return data
	case virtioInputCfgPropBits:
		return []byte{0}
	case virtioInputCfgEVBits:
		switch v.cfgSubsel {
		case byte(virtioInputEvKey):
			bits := make([]byte, 96)
			// A small useful keyboard subset: Enter, Space, arrows, letters and digits.
			for _, code := range []uint16{1, 14, 28, 57, 103, 105, 106, 108} {
				setBit(bits, code)
			}
			for code := uint16(2); code <= 11; code++ {
				setBit(bits, code)
			}
			for code := uint16(16); code <= 50; code++ {
				setBit(bits, code)
			}
			return bits
		case byte(virtioInputEvRel):
			bits := make([]byte, 1)
			setBit(bits, 0)
			setBit(bits, 1)
			return bits
		default:
			ev := make([]byte, 1)
			setBit(ev, virtioInputEvKey)
			setBit(ev, virtioInputEvRel)
			return ev
		}
	default:
		return nil
	}
}

func setBit(buf []byte, bit uint16) {
	i := int(bit / 8)
	if i < len(buf) {
		buf[i] |= 1 << (bit % 8)
	}
}

func (v *VirtioInput) fail(format string, args ...any) {
	v.lastErr = fmt.Sprintf(format, args...)
	v.status |= virtioStatusFailed
}

func (v *VirtioInput) copyToGuest(addr uint64, data []byte) error {
	for i, b := range data {
		if err := v.bus.Write(addr+uint64(i), 1, uint64(b)); err != nil {
			return err
		}
	}
	return nil
}
func (v *VirtioInput) copyFromGuest(dst []byte, addr uint64) error {
	for i := range dst {
		x, err := v.bus.Read(addr+uint64(i), 1)
		if err != nil {
			return err
		}
		dst[i] = byte(x)
	}
	return nil
}
func (v *VirtioInput) read16(addr uint64) (uint16, error) {
	x, err := v.bus.Read(addr, 2)
	return uint16(x), err
}
func (v *VirtioInput) read32(addr uint64) (uint32, error) {
	x, err := v.bus.Read(addr, 4)
	return uint32(x), err
}
func (v *VirtioInput) read64(addr uint64) (uint64, error)  { return v.bus.Read(addr, 8) }
func (v *VirtioInput) write16(addr uint64, x uint16) error { return v.bus.Write(addr, 2, uint64(x)) }
func (v *VirtioInput) write32(addr uint64, x uint32) error { return v.bus.Write(addr, 4, uint64(x)) }
