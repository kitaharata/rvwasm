package dev

import (
	"encoding/binary"
	"fmt"
	"github.com/kitaharata/rvwasm/internal/mem"
)

const VirtioBlockBase uint64 = 0x10001000

const (
	virtioMMIOMagic            uint32 = 0x74726976
	virtioVersionModern        uint32 = 2
	virtioDeviceBlock          uint32 = 2
	virtioVendorRVWASM         uint32 = 0x5256574d
	virtioStatusAck            uint32 = 1
	virtioStatusDriver         uint32 = 2
	virtioStatusDriverOK       uint32 = 4
	virtioStatusFeatures       uint32 = 8
	virtioStatusFailed         uint32 = 128
	virtioIRQUsedBuffer        uint32 = 1
	virtioIRQConfigChange      uint32 = 2
	virtioBlkFFlushBit                = 9
	virtioRingFIndirectDescBit        = 28
	virtioRingFEventIdxBit            = 29
	virtioFVersion1Bit                = 32
	virtioBlkSectorSize               = 512
	virtioQueueSize                   = 128

	virtqDescFNext     uint16 = 1
	virtqDescFWrite    uint16 = 2
	virtqDescFIndirect uint16 = 4

	virtioBlkTIn    uint32 = 0
	virtioBlkTOut   uint32 = 1
	virtioBlkTFlush uint32 = 4
	virtioBlkTGetID uint32 = 8

	virtioBlkSOK    byte = 0
	virtioBlkSIOErr byte = 1
)

type virtQueue struct {
	num       uint32
	ready     bool
	desc      uint64
	driver    uint64
	device    uint64
	lastAvail uint16
}

type virtqDesc struct {
	addr  uint64
	len   uint32
	flags uint16
	next  uint16
}

type VirtioBlock struct {
	bus *mem.Bus
	IRQ func(bool)

	disk []byte

	deviceFeaturesSel uint32
	driverFeaturesSel uint32
	driverFeatures    [2]uint32
	queueSel          uint32
	queue             virtQueue
	interruptStatus   uint32
	status            uint32
	configGeneration  uint32
	lastErr           string
}

func NewVirtioBlock(bus *mem.Bus, disk []byte, irq func(bool)) *VirtioBlock {
	v := &VirtioBlock{bus: bus, IRQ: irq}
	v.SetDisk(disk)
	return v
}

func (v *VirtioBlock) SetDisk(disk []byte) {
	if len(disk) == 0 {
		disk = make([]byte, virtioBlkSectorSize)
	}
	if rem := len(disk) % virtioBlkSectorSize; rem != 0 {
		pad := virtioBlkSectorSize - rem
		disk = append(append([]byte{}, disk...), make([]byte, pad)...)
	} else {
		disk = append([]byte{}, disk...)
	}
	v.disk = disk
	v.configGeneration++
}

func (v *VirtioBlock) Disk() []byte { return append([]byte{}, v.disk...) }

func (v *VirtioBlock) CapacitySectors() uint64 { return uint64(len(v.disk) / virtioBlkSectorSize) }

func (v *VirtioBlock) LastError() string { return v.lastErr }

func (v *VirtioBlock) DebugString() string {
	return fmt.Sprintf("status=%#x isr=%#x qsel=%d qnum=%d qready=%v lastAvail=%d desc=%#x avail=%#x used=%#x features[0]=%#x features[1]=%#x diskSectors=%d err=%q",
		v.status, v.interruptStatus, v.queueSel, v.queue.num, v.queue.ready, v.queue.lastAvail, v.queue.desc, v.queue.driver, v.queue.device, v.driverFeatures[0], v.driverFeatures[1], v.CapacitySectors(), v.lastErr)
}

func (v *VirtioBlock) deviceFeatures(sel uint32) uint32 {
	switch sel {
	case 0:
		return (1 << virtioBlkFFlushBit) | (1 << virtioRingFIndirectDescBit) | (1 << virtioRingFEventIdxBit)
	case virtioFVersion1Bit / 32:
		return 1 << (virtioFVersion1Bit % 32)
	default:
		return 0
	}
}

func (v *VirtioBlock) Read(addr uint64, size int) (uint64, error) {
	off := addr - VirtioBlockBase
	read32 := func(x uint32) (uint64, error) { return partialRead32(x, 0, size), nil }
	switch off {
	case 0x000:
		return read32(virtioMMIOMagic)
	case 0x004:
		return read32(virtioVersionModern)
	case 0x008:
		return read32(virtioDeviceBlock)
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
		if v.queueSel == 0 {
			return read32(virtioQueueSize)
		}
		return 0, nil
	case 0x038:
		return read32(v.queue.num)
	case 0x044:
		return read32(bool32(v.queue.ready))
	case 0x060:
		return read32(v.interruptStatus)
	case 0x070:
		return read32(v.status)
	case 0x080:
		return read32(uint32(v.queue.desc))
	case 0x084:
		return read32(uint32(v.queue.desc >> 32))
	case 0x090:
		return read32(uint32(v.queue.driver))
	case 0x094:
		return read32(uint32(v.queue.driver >> 32))
	case 0x0a0:
		return read32(uint32(v.queue.device))
	case 0x0a4:
		return read32(uint32(v.queue.device >> 32))
	case 0x0c0:
		return read32(0) // QueueReset complete/not in reset
	case 0x0fc:
		return read32(v.configGeneration)
	}
	if off >= 0x100 && off < 0x108 {
		return partialRead64(v.CapacitySectors(), off-0x100, size), nil
	}
	// The rest of the block-device configuration space is optional for this
	// minimal device.  Return zeros for fields such as size_max, seg_max,
	// geometry, blk_size and writeback when their feature bits are not offered.
	if off >= 0x108 && off < 0x140 {
		return 0, nil
	}
	return 0, nil
}

func (v *VirtioBlock) Write(addr uint64, size int, val uint64) error {
	off := addr - VirtioBlockBase
	write32 := func(cur uint32) uint32 { return partialWrite32(cur, off&3, size, val) }
	switch off {
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
		if v.queueSel == 0 {
			v.queue.num = write32(v.queue.num)
		}
	case 0x044:
		if v.queueSel == 0 {
			ready := write32(bool32(v.queue.ready)) != 0
			if !ready {
				v.queue.lastAvail = 0
			}
			v.queue.ready = ready
		}
	case 0x050:
		q := write32(0)
		if q == 0 {
			v.processQueue()
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
		v.queue.desc = (v.queue.desc &^ uint64(0xffffffff)) | uint64(write32(uint32(v.queue.desc)))
	case 0x084:
		v.queue.desc = (v.queue.desc & 0xffffffff) | (uint64(write32(uint32(v.queue.desc>>32))) << 32)
	case 0x090:
		v.queue.driver = (v.queue.driver &^ uint64(0xffffffff)) | uint64(write32(uint32(v.queue.driver)))
	case 0x094:
		v.queue.driver = (v.queue.driver & 0xffffffff) | (uint64(write32(uint32(v.queue.driver>>32))) << 32)
	case 0x0a0:
		v.queue.device = (v.queue.device &^ uint64(0xffffffff)) | uint64(write32(uint32(v.queue.device)))
	case 0x0a4:
		v.queue.device = (v.queue.device & 0xffffffff) | (uint64(write32(uint32(v.queue.device>>32))) << 32)
	case 0x0c0:
		if write32(0) != 0 && v.queueSel == 0 {
			v.queue = virtQueue{}
			v.interruptStatus = 0
			v.updateIRQ()
		}
	}
	return nil
}

func (v *VirtioBlock) Tick(cycles uint64) {}

func (v *VirtioBlock) reset() {
	v.deviceFeaturesSel = 0
	v.driverFeaturesSel = 0
	v.driverFeatures = [2]uint32{}
	v.queueSel = 0
	v.queue = virtQueue{}
	v.interruptStatus = 0
	v.status = 0
	v.lastErr = ""
	v.updateIRQ()
}

func (v *VirtioBlock) updateIRQ() {
	if v.IRQ != nil {
		v.IRQ(v.interruptStatus != 0)
	}
}

func (v *VirtioBlock) featuresAccepted() bool {
	// This device only offers VIRTIO_F_VERSION_1.  Reject FEATURES_OK if the
	// driver failed to acknowledge it or selected any feature bits we did not
	// advertise.  Linux relies on this handshake to choose modern MMIO layout.
	for sel, got := range v.driverFeatures {
		allowed := v.deviceFeatures(uint32(sel))
		if got&^allowed != 0 {
			return false
		}
	}
	return v.driverFeatures[virtioFVersion1Bit/32]&(1<<(virtioFVersion1Bit%32)) != 0
}

func (v *VirtioBlock) processQueue() {
	if v.status&virtioStatusDriverOK == 0 {
		return
	}
	if !v.queue.ready || v.queue.num == 0 || v.queue.num > virtioQueueSize || v.queue.desc == 0 || v.queue.driver == 0 || v.queue.device == 0 {
		return
	}
	for {
		availIdx, err := v.read16(v.queue.driver + 2)
		if err != nil || v.queue.lastAvail == availIdx {
			return
		}
		ringOff := uint64(v.queue.lastAvail%uint16(v.queue.num)) * 2
		head, err := v.read16(v.queue.driver + 4 + ringOff)
		if err != nil {
			v.fail("read avail ring: %v", err)
			return
		}
		usedLen := v.processRequest(head)
		usedIdx, err := v.read16(v.queue.device + 2)
		if err != nil {
			v.fail("read used idx: %v", err)
			return
		}
		newUsedIdx := usedIdx + 1
		usedOff := v.queue.device + 4 + uint64(usedIdx%uint16(v.queue.num))*8
		_ = v.write32(usedOff, uint32(head))
		_ = v.write32(usedOff+4, usedLen)
		_ = v.write16(v.queue.device+2, newUsedIdx)
		v.queue.lastAvail++
		if v.shouldInterrupt(usedIdx, newUsedIdx) {
			v.interruptStatus |= virtioIRQUsedBuffer
			v.updateIRQ()
		}
	}
}

func (v *VirtioBlock) featureNegotiated(sel uint32, bit uint32) bool {
	if sel >= uint32(len(v.driverFeatures)) {
		return false
	}
	return v.driverFeatures[sel]&(1<<bit) != 0
}

func virtqNeedEvent(eventIdx, newIdx, oldIdx uint16) bool {
	return uint16(newIdx-eventIdx-1) < uint16(newIdx-oldIdx)
}

func (v *VirtioBlock) shouldInterrupt(oldUsedIdx, newUsedIdx uint16) bool {
	if v.featureNegotiated(virtioRingFEventIdxBit/32, virtioRingFEventIdxBit%32) {
		event, err := v.read16(v.queue.device + 4 + uint64(v.queue.num)*8)
		if err != nil {
			v.fail("read used event: %v", err)
			return true
		}
		return virtqNeedEvent(event, newUsedIdx, oldUsedIdx)
	}
	availFlags, err := v.read16(v.queue.driver)
	if err != nil {
		v.fail("read avail flags: %v", err)
		return true
	}
	return availFlags&1 == 0 // VIRTQ_AVAIL_F_NO_INTERRUPT is not set.
}

func (v *VirtioBlock) processRequest(head uint16) uint32 {
	descs, err := v.readChain(head)
	if err != nil {
		v.fail("descriptor chain: %v", err)
		return 0
	}
	if len(descs) < 2 || descs[0].len < 16 {
		v.setStatus(descs, virtioBlkSIOErr)
		return 1
	}
	typ, _ := v.read32(descs[0].addr)
	sector, _ := v.read64(descs[0].addr + 8)
	status := virtioBlkSOK
	var used uint32 = 1

	statusDesc := descs[len(descs)-1]
	if statusDesc.len == 0 || statusDesc.flags&virtqDescFWrite == 0 {
		v.setStatus(descs, virtioBlkSIOErr)
		return 0
	}
	dataDescs := descs[1 : len(descs)-1]

	switch typ {
	case virtioBlkTIn:
		off := sector * virtioBlkSectorSize
		for _, d := range dataDescs {
			if d.flags&virtqDescFWrite == 0 || off+uint64(d.len) > uint64(len(v.disk)) {
				status = virtioBlkSIOErr
				break
			}
			if err := v.copyToGuest(d.addr, v.disk[off:off+uint64(d.len)]); err != nil {
				status = virtioBlkSIOErr
				v.fail("copy read data: %v", err)
				break
			}
			off += uint64(d.len)
			used += d.len
		}
	case virtioBlkTOut:
		off := sector * virtioBlkSectorSize
		for _, d := range dataDescs {
			if d.flags&virtqDescFWrite != 0 || off+uint64(d.len) > uint64(len(v.disk)) {
				status = virtioBlkSIOErr
				break
			}
			buf := make([]byte, d.len)
			if err := v.copyFromGuest(buf, d.addr); err != nil {
				status = virtioBlkSIOErr
				v.fail("copy write data: %v", err)
				break
			}
			copy(v.disk[off:off+uint64(d.len)], buf)
			off += uint64(d.len)
		}
	case virtioBlkTFlush:
		// Nothing to flush; disk is in memory.
	case virtioBlkTGetID:
		id := []byte("rvwasm-virtio-blk")
		for _, d := range dataDescs {
			if d.flags&virtqDescFWrite == 0 {
				status = virtioBlkSIOErr
				break
			}
			n := int(d.len)
			if n > len(id) {
				n = len(id)
			}
			if err := v.copyToGuest(d.addr, id[:n]); err != nil {
				status = virtioBlkSIOErr
				break
			}
			used += uint32(n)
			break
		}
	default:
		status = virtioBlkSIOErr
	}
	v.setStatus(descs, status)
	return used
}

func (v *VirtioBlock) readChain(head uint16) ([]virtqDesc, error) {
	if uint32(head) >= v.queue.num {
		return nil, fmt.Errorf("head %d outside queue num %d", head, v.queue.num)
	}
	d, err := v.readDesc(head)
	if err != nil {
		return nil, err
	}
	if d.flags&virtqDescFIndirect != 0 {
		return v.readIndirectChain(d)
	}
	return v.readDirectChain(head, v.queue.desc, uint16(v.queue.num), true)
}

func (v *VirtioBlock) readDirectChain(head uint16, table uint64, limit uint16, allowIndirect bool) ([]virtqDesc, error) {
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

func (v *VirtioBlock) readIndirectChain(d virtqDesc) ([]virtqDesc, error) {
	if d.len == 0 || d.len%16 != 0 {
		return nil, fmt.Errorf("bad indirect table length %d", d.len)
	}
	limit := d.len / 16
	if limit > 1024 {
		return nil, fmt.Errorf("indirect table too large: %d descriptors", limit)
	}
	return v.readDirectChain(0, d.addr, uint16(limit), false)
}

func (v *VirtioBlock) readDesc(id uint16) (virtqDesc, error) {
	return v.readDescAt(v.queue.desc, id)
}

func (v *VirtioBlock) readDescAt(table uint64, id uint16) (virtqDesc, error) {
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

func (v *VirtioBlock) setStatus(descs []virtqDesc, status byte) {
	for i := len(descs) - 1; i >= 0; i-- {
		d := descs[i]
		if d.flags&virtqDescFWrite != 0 && d.len > 0 {
			_ = v.bus.Write(d.addr, 1, uint64(status))
			return
		}
	}
}

func (v *VirtioBlock) fail(format string, args ...any) {
	v.lastErr = fmt.Sprintf(format, args...)
	v.status |= virtioStatusFailed
}

func (v *VirtioBlock) copyToGuest(addr uint64, data []byte) error {
	for i, b := range data {
		if err := v.bus.Write(addr+uint64(i), 1, uint64(b)); err != nil {
			return err
		}
	}
	return nil
}

func (v *VirtioBlock) copyFromGuest(dst []byte, addr uint64) error {
	for i := range dst {
		v, err := v.bus.Read(addr+uint64(i), 1)
		if err != nil {
			return err
		}
		dst[i] = byte(v)
	}
	return nil
}

func (v *VirtioBlock) read16(addr uint64) (uint16, error) {
	x, err := v.bus.Read(addr, 2)
	return uint16(x), err
}
func (v *VirtioBlock) read32(addr uint64) (uint32, error) {
	x, err := v.bus.Read(addr, 4)
	return uint32(x), err
}
func (v *VirtioBlock) read64(addr uint64) (uint64, error)  { return v.bus.Read(addr, 8) }
func (v *VirtioBlock) write16(addr uint64, x uint16) error { return v.bus.Write(addr, 2, uint64(x)) }
func (v *VirtioBlock) write32(addr uint64, x uint32) error { return v.bus.Write(addr, 4, uint64(x)) }

func bool32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func partialRead64(v uint64, off uint64, size int) uint64 {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	if off >= 8 {
		return 0
	}
	if off+uint64(size) > 8 {
		size = int(8 - off)
	}
	var out uint64
	for i := 0; i < size; i++ {
		out |= uint64(b[off+uint64(i)]) << (8 * i)
	}
	return out
}
