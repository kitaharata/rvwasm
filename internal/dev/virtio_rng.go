package dev

import (
	"fmt"
	"github.com/kitaharata/rvwasm/internal/mem"
)

const VirtioRNGBase uint64 = 0x10004000

const (
	virtioDeviceRNG    uint32 = 4
	virtioRNGQueueSize        = 128
)

// VirtioRNG is a tiny virtio entropy source. It is intentionally deterministic
// by default so boot experiments and tests are reproducible; the browser UI can
// reseed it when desired.
type VirtioRNG struct {
	bus *mem.Bus
	IRQ func(bool)

	deviceFeaturesSel uint32
	driverFeaturesSel uint32
	driverFeatures    [2]uint32
	queueSel          uint32
	queue             virtQueue
	interruptStatus   uint32
	status            uint32
	configGeneration  uint32
	lastErr           string
	seed              uint64
	bytesServed       uint64
	requests          uint64
}

func NewVirtioRNG(bus *mem.Bus, irq func(bool)) *VirtioRNG {
	return &VirtioRNG{bus: bus, IRQ: irq, seed: 0x72767761736d726e}
}

func (v *VirtioRNG) SetSeed(seed uint64) {
	if seed == 0 {
		seed = 0x72767761736d726e
	}
	v.seed = seed
	v.configGeneration++
}

func (v *VirtioRNG) LastError() string { return v.lastErr }

func (v *VirtioRNG) DebugString() string {
	return fmt.Sprintf("status=%#x isr=%#x qsel=%d qnum=%d qready=%v lastAvail=%d desc=%#x avail=%#x used=%#x features[0]=%#x features[1]=%#x requests=%d bytes=%d seed=%#x err=%q",
		v.status, v.interruptStatus, v.queueSel, v.queue.num, v.queue.ready, v.queue.lastAvail, v.queue.desc, v.queue.driver, v.queue.device, v.driverFeatures[0], v.driverFeatures[1], v.requests, v.bytesServed, v.seed, v.lastErr)
}

func (v *VirtioRNG) deviceFeatures(sel uint32) uint32 {
	switch sel {
	case 0:
		return (1 << virtioRingFIndirectDescBit) | (1 << virtioRingFEventIdxBit)
	case virtioFVersion1Bit / 32:
		return 1 << (virtioFVersion1Bit % 32)
	default:
		return 0
	}
}

func (v *VirtioRNG) Read(addr uint64, size int) (uint64, error) {
	off := addr - VirtioRNGBase
	read32 := func(x uint32) (uint64, error) { return partialRead32(x, off&3, size), nil }
	switch off &^ 3 {
	case 0x000:
		return read32(virtioMMIOMagic)
	case 0x004:
		return read32(virtioVersionModern)
	case 0x008:
		return read32(virtioDeviceRNG)
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
			return read32(virtioRNGQueueSize)
		}
		return 0, nil
	case 0x038:
		if v.queueSel == 0 {
			return read32(v.queue.num)
		}
		return 0, nil
	case 0x044:
		if v.queueSel == 0 {
			return read32(bool32(v.queue.ready))
		}
		return 0, nil
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
		return read32(0)
	case 0x0fc:
		return read32(v.configGeneration)
	default:
		return 0, nil
	}
}

func (v *VirtioRNG) Write(addr uint64, size int, val uint64) error {
	off := addr - VirtioRNGBase
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
			if ready {
				v.processQueue()
			}
		}
	case 0x050:
		if write32(0) == 0 {
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
			v.processQueue()
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

func (v *VirtioRNG) Tick(cycles uint64) {}

func (v *VirtioRNG) reset() {
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

func (v *VirtioRNG) updateIRQ() {
	if v.IRQ != nil {
		v.IRQ(v.interruptStatus != 0)
	}
}

func (v *VirtioRNG) featuresAccepted() bool {
	for sel, got := range v.driverFeatures {
		if got&^v.deviceFeatures(uint32(sel)) != 0 {
			return false
		}
	}
	return v.driverFeatures[virtioFVersion1Bit/32]&(1<<(virtioFVersion1Bit%32)) != 0
}

func (v *VirtioRNG) featureNegotiated(sel uint32, bit uint32) bool {
	if sel >= uint32(len(v.driverFeatures)) {
		return false
	}
	return v.driverFeatures[sel]&(1<<bit) != 0
}

func (v *VirtioRNG) queueReady() bool {
	q := &v.queue
	return v.status&virtioStatusDriverOK != 0 && q.ready && q.num != 0 && q.num <= virtioRNGQueueSize && q.desc != 0 && q.driver != 0 && q.device != 0
}

func (v *VirtioRNG) processQueue() {
	if !v.queueReady() {
		return
	}
	for {
		availIdx, err := v.read16(v.queue.driver + 2)
		if err != nil || v.queue.lastAvail == availIdx {
			return
		}
		head, err := v.read16(v.queue.driver + 4 + uint64(v.queue.lastAvail%uint16(v.queue.num))*2)
		if err != nil {
			v.fail("read avail ring: %v", err)
			return
		}
		usedLen := v.fillRequest(head)
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

func (v *VirtioRNG) fillRequest(head uint16) uint32 {
	descs, err := v.readChain(head)
	if err != nil {
		v.fail("descriptor chain: %v", err)
		return 0
	}
	var used uint32
	for _, d := range descs {
		if d.flags&virtqDescFWrite == 0 || d.len == 0 {
			continue
		}
		for i := uint32(0); i < d.len; i++ {
			if err := v.bus.Write(d.addr+uint64(i), 1, uint64(v.nextByte())); err != nil {
				v.fail("write entropy: %v", err)
				return used
			}
		}
		used += d.len
		v.bytesServed += uint64(d.len)
	}
	v.requests++
	return used
}

func (v *VirtioRNG) nextByte() byte {
	// xorshift64*: small deterministic PRNG sufficient for an emulated entropy
	// device test source; not used for host security.
	x := v.seed
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	v.seed = x
	return byte((x * 0x2545F4914F6CDD1D) >> 56)
}

func (v *VirtioRNG) readChain(head uint16) ([]virtqDesc, error) {
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

func (v *VirtioRNG) readDirectChain(head uint16, table uint64, limit uint16, allowIndirect bool) ([]virtqDesc, error) {
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

func (v *VirtioRNG) readIndirectChain(d virtqDesc) ([]virtqDesc, error) {
	if d.len == 0 || d.len%16 != 0 {
		return nil, fmt.Errorf("bad indirect table length %d", d.len)
	}
	limit := d.len / 16
	if limit > 1024 {
		return nil, fmt.Errorf("indirect table too large: %d descriptors", limit)
	}
	return v.readDirectChain(0, d.addr, uint16(limit), false)
}

func (v *VirtioRNG) readDesc(id uint16) (virtqDesc, error) { return v.readDescAt(v.queue.desc, id) }

func (v *VirtioRNG) readDescAt(table uint64, id uint16) (virtqDesc, error) {
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

func (v *VirtioRNG) shouldInterrupt(oldUsedIdx, newUsedIdx uint16) bool {
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
	return availFlags&1 == 0
}

func (v *VirtioRNG) fail(format string, args ...any) {
	v.lastErr = fmt.Sprintf(format, args...)
	v.status |= virtioStatusFailed
}

func (v *VirtioRNG) read16(addr uint64) (uint16, error) {
	x, err := v.bus.Read(addr, 2)
	return uint16(x), err
}
func (v *VirtioRNG) read32(addr uint64) (uint32, error) {
	x, err := v.bus.Read(addr, 4)
	return uint32(x), err
}
func (v *VirtioRNG) read64(addr uint64) (uint64, error)  { return v.bus.Read(addr, 8) }
func (v *VirtioRNG) write16(addr uint64, x uint16) error { return v.bus.Write(addr, 2, uint64(x)) }
func (v *VirtioRNG) write32(addr uint64, x uint32) error { return v.bus.Write(addr, 4, uint64(x)) }
