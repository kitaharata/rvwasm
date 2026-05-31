package dev

import (
	"fmt"
	"github.com/kitaharata/rvwasm/internal/mem"
)

const VirtioConsoleBase uint64 = 0x10002000

const (
	virtioDeviceConsole     uint32 = 3
	virtioConsoleFSizeBit          = 0
	virtioConsoleQueueSize         = 128
	virtioConsoleQueueRx    uint32 = 0
	virtioConsoleQueueTx    uint32 = 1
	virtioConsoleQueueCount        = 2
)

type VirtioConsole struct {
	bus *mem.Bus
	IRQ func(bool)
	Out func(byte)

	rx []byte

	deviceFeaturesSel uint32
	driverFeaturesSel uint32
	driverFeatures    [2]uint32
	queueSel          uint32
	queues            [virtioConsoleQueueCount]virtQueue
	interruptStatus   uint32
	status            uint32
	configGeneration  uint32
	lastErr           string
	lastTxBytes       uint64
	lastRxBytes       uint64
}

func NewVirtioConsole(bus *mem.Bus, out func(byte), irq func(bool)) *VirtioConsole {
	return &VirtioConsole{bus: bus, Out: out, IRQ: irq}
}

func (v *VirtioConsole) Inject(data []byte) {
	if len(data) == 0 {
		return
	}
	v.rx = append(v.rx, data...)
	v.processRx()
}

func (v *VirtioConsole) LastError() string { return v.lastErr }

func (v *VirtioConsole) DebugString() string {
	q0 := v.queues[0]
	q1 := v.queues[1]
	return fmt.Sprintf("status=%#x isr=%#x qsel=%d rxReady=%v rxLast=%d txReady=%v txLast=%d pendingRx=%d txBytes=%d rxBytes=%d features[0]=%#x features[1]=%#x err=%q",
		v.status, v.interruptStatus, v.queueSel, q0.ready, q0.lastAvail, q1.ready, q1.lastAvail, len(v.rx), v.lastTxBytes, v.lastRxBytes, v.driverFeatures[0], v.driverFeatures[1], v.lastErr)
}

func (v *VirtioConsole) deviceFeatures(sel uint32) uint32 {
	switch sel {
	case 0:
		return (1 << virtioConsoleFSizeBit) | (1 << virtioRingFIndirectDescBit) | (1 << virtioRingFEventIdxBit)
	case virtioFVersion1Bit / 32:
		return 1 << (virtioFVersion1Bit % 32)
	default:
		return 0
	}
}

func (v *VirtioConsole) Read(addr uint64, size int) (uint64, error) {
	off := addr - VirtioConsoleBase
	read32 := func(x uint32) (uint64, error) { return partialRead32(x, off&3, size), nil }
	switch off &^ 3 {
	case 0x000:
		return read32(virtioMMIOMagic)
	case 0x004:
		return read32(virtioVersionModern)
	case 0x008:
		return read32(virtioDeviceConsole)
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
		if v.queueSel < virtioConsoleQueueCount {
			return read32(virtioConsoleQueueSize)
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
	case 0x100:
		// virtio_console_config: cols (u16), rows (u16)
		return read32(80 | (25 << 16))
	case 0x104:
		return read32(1) // max_nr_ports
	case 0x108:
		return read32(0) // emerg_wr
	default:
		return 0, nil
	}
}

func (v *VirtioConsole) Write(addr uint64, size int, val uint64) error {
	off := addr - VirtioConsoleBase
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
		}
	case 0x050:
		qidx := write32(0)
		switch qidx {
		case virtioConsoleQueueRx:
			v.processRx()
		case virtioConsoleQueueTx:
			v.processTx()
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
		if write32(0) != 0 && v.queueSel < virtioConsoleQueueCount {
			v.queues[v.queueSel] = virtQueue{}
			v.interruptStatus = 0
			v.updateIRQ()
		}
	}
	return nil
}

func (v *VirtioConsole) Tick(cycles uint64) {}

func (v *VirtioConsole) selectedQueue() *virtQueue {
	if v.queueSel >= virtioConsoleQueueCount {
		return nil
	}
	return &v.queues[v.queueSel]
}

func (v *VirtioConsole) reset() {
	v.deviceFeaturesSel = 0
	v.driverFeaturesSel = 0
	v.driverFeatures = [2]uint32{}
	v.queueSel = 0
	v.queues = [virtioConsoleQueueCount]virtQueue{}
	v.interruptStatus = 0
	v.status = 0
	v.lastErr = ""
	v.updateIRQ()
}

func (v *VirtioConsole) updateIRQ() {
	if v.IRQ != nil {
		v.IRQ(v.interruptStatus != 0)
	}
}

func (v *VirtioConsole) featuresAccepted() bool {
	for sel, got := range v.driverFeatures {
		allowed := v.deviceFeatures(uint32(sel))
		if got&^allowed != 0 {
			return false
		}
	}
	return v.driverFeatures[virtioFVersion1Bit/32]&(1<<(virtioFVersion1Bit%32)) != 0
}

func (v *VirtioConsole) featureNegotiated(sel uint32, bit uint32) bool {
	if sel >= uint32(len(v.driverFeatures)) {
		return false
	}
	return v.driverFeatures[sel]&(1<<bit) != 0
}

func (v *VirtioConsole) queueReady(q *virtQueue) bool {
	return v.status&virtioStatusDriverOK != 0 && q.ready && q.num != 0 && q.num <= virtioConsoleQueueSize && q.desc != 0 && q.driver != 0 && q.device != 0
}

func (v *VirtioConsole) processTx() {
	q := &v.queues[virtioConsoleQueueTx]
	if !v.queueReady(q) {
		return
	}
	for {
		availIdx, err := v.read16(q.driver + 2)
		if err != nil || q.lastAvail == availIdx {
			return
		}
		ringOff := uint64(q.lastAvail%uint16(q.num)) * 2
		head, err := v.read16(q.driver + 4 + ringOff)
		if err != nil {
			v.fail("read tx avail ring: %v", err)
			return
		}
		usedLen := v.consumeTx(head, q)
		v.publishUsed(q, head, usedLen)
	}
}

func (v *VirtioConsole) processRx() {
	q := &v.queues[virtioConsoleQueueRx]
	if len(v.rx) == 0 || !v.queueReady(q) {
		return
	}
	for len(v.rx) > 0 {
		availIdx, err := v.read16(q.driver + 2)
		if err != nil || q.lastAvail == availIdx {
			return
		}
		ringOff := uint64(q.lastAvail%uint16(q.num)) * 2
		head, err := v.read16(q.driver + 4 + ringOff)
		if err != nil {
			v.fail("read rx avail ring: %v", err)
			return
		}
		usedLen := v.fillRx(head, q)
		v.publishUsed(q, head, usedLen)
		if usedLen == 0 {
			return
		}
	}
}

func (v *VirtioConsole) publishUsed(q *virtQueue, head uint16, usedLen uint32) {
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

func (v *VirtioConsole) consumeTx(head uint16, q *virtQueue) uint32 {
	descs, err := v.readChain(head, q)
	if err != nil {
		v.fail("tx descriptor chain: %v", err)
		return 0
	}
	var used uint32
	for _, d := range descs {
		if d.flags&virtqDescFWrite != 0 || d.len == 0 {
			continue
		}
		buf := make([]byte, d.len)
		if err := v.copyFromGuest(buf, d.addr); err != nil {
			v.fail("read tx data: %v", err)
			return used
		}
		for _, b := range buf {
			if v.Out != nil {
				v.Out(b)
			}
		}
		used += d.len
		v.lastTxBytes += uint64(d.len)
	}
	return used
}

func (v *VirtioConsole) fillRx(head uint16, q *virtQueue) uint32 {
	descs, err := v.readChain(head, q)
	if err != nil {
		v.fail("rx descriptor chain: %v", err)
		return 0
	}
	var used uint32
	for _, d := range descs {
		if d.flags&virtqDescFWrite == 0 || d.len == 0 {
			continue
		}
		n := int(d.len)
		if n > len(v.rx) {
			n = len(v.rx)
		}
		if n == 0 {
			break
		}
		if err := v.copyToGuest(d.addr, v.rx[:n]); err != nil {
			v.fail("write rx data: %v", err)
			return used
		}
		v.rx = v.rx[n:]
		used += uint32(n)
		v.lastRxBytes += uint64(n)
		if len(v.rx) == 0 {
			break
		}
	}
	return used
}

func (v *VirtioConsole) readChain(head uint16, q *virtQueue) ([]virtqDesc, error) {
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

func (v *VirtioConsole) readDirectChain(head uint16, table uint64, limit uint16, allowIndirect bool) ([]virtqDesc, error) {
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

func (v *VirtioConsole) readIndirectChain(d virtqDesc) ([]virtqDesc, error) {
	if d.len == 0 || d.len%16 != 0 {
		return nil, fmt.Errorf("bad indirect table length %d", d.len)
	}
	limit := d.len / 16
	if limit > 1024 {
		return nil, fmt.Errorf("indirect table too large: %d descriptors", limit)
	}
	return v.readDirectChain(0, d.addr, uint16(limit), false)
}

func (v *VirtioConsole) readDescAt(table uint64, id uint16) (virtqDesc, error) {
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

func (v *VirtioConsole) shouldInterrupt(q *virtQueue, oldUsedIdx, newUsedIdx uint16) bool {
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

func (v *VirtioConsole) fail(format string, args ...any) {
	v.lastErr = fmt.Sprintf(format, args...)
	v.status |= virtioStatusFailed
}

func (v *VirtioConsole) copyToGuest(addr uint64, data []byte) error {
	for i, b := range data {
		if err := v.bus.Write(addr+uint64(i), 1, uint64(b)); err != nil {
			return err
		}
	}
	return nil
}

func (v *VirtioConsole) copyFromGuest(dst []byte, addr uint64) error {
	for i := range dst {
		x, err := v.bus.Read(addr+uint64(i), 1)
		if err != nil {
			return err
		}
		dst[i] = byte(x)
	}
	return nil
}

func (v *VirtioConsole) read16(addr uint64) (uint16, error) {
	x, err := v.bus.Read(addr, 2)
	return uint16(x), err
}
func (v *VirtioConsole) read32(addr uint64) (uint32, error) {
	x, err := v.bus.Read(addr, 4)
	return uint32(x), err
}
func (v *VirtioConsole) read64(addr uint64) (uint64, error)  { return v.bus.Read(addr, 8) }
func (v *VirtioConsole) write16(addr uint64, x uint16) error { return v.bus.Write(addr, 2, uint64(x)) }
func (v *VirtioConsole) write32(addr uint64, x uint32) error { return v.bus.Write(addr, 4, uint64(x)) }
