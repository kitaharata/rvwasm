package dev

import (
	"encoding/binary"
	"fmt"
	"github.com/kitaharata/rvwasm/internal/mem"
)

const VirtioGPUBase uint64 = 0x10006000

const (
	virtioDeviceGPU       uint32 = 16
	virtioGPUQueueSize           = 128
	virtioGPUQueueControl uint32 = 0
	virtioGPUQueueCursor  uint32 = 1
	virtioGPUQueueCount          = 2

	virtioGPUCmdGetDisplayInfo     uint32 = 0x0100
	virtioGPUCmdResourceCreate2D   uint32 = 0x0101
	virtioGPUCmdResourceUnref      uint32 = 0x0102
	virtioGPUCmdSetScanout         uint32 = 0x0103
	virtioGPUCmdResourceFlush      uint32 = 0x0104
	virtioGPUCmdTransferToHost2D   uint32 = 0x0105
	virtioGPUCmdResourceAttachBack uint32 = 0x0106
	virtioGPUCmdResourceDetachBack uint32 = 0x0107
	virtioGPUCmdUpdateCursor       uint32 = 0x0300
	virtioGPUCmdMoveCursor         uint32 = 0x0301

	virtioGPURespOKNoData      uint32 = 0x1100
	virtioGPURespOKDisplayInfo uint32 = 0x1101
	virtioGPURespErrUnspec     uint32 = 0x1200
)

type gpuResource struct {
	ID      uint32
	Format  uint32
	Width   uint32
	Height  uint32
	Backing uint64
	Length  uint32
}

type gpuFramebuffer struct {
	Base    uint64
	Width   uint32
	Height  uint32
	Stride  uint32
	Enabled bool
}

type gpuCursorState struct {
	Updates    uint64
	Moves      uint64
	ResourceID uint32
	ScanoutID  uint32
	X          uint32
	Y          uint32
	HotX       uint32
	HotY       uint32
	Visible    bool
}

// VirtioGPU is a deliberately small virtio-gpu 2D control device. It is meant
// for Linux probe/early modesetting diagnostics rather than acceleration: it
// responds to the common 2D setup commands and records scanout/resource state.
type VirtioGPU struct {
	bus *mem.Bus
	IRQ func(bool)

	deviceFeaturesSel uint32
	driverFeaturesSel uint32
	driverFeatures    [2]uint32
	queueSel          uint32
	queues            [virtioGPUQueueCount]virtQueue
	interruptStatus   uint32
	status            uint32
	configGeneration  uint32
	lastErr           string

	width      uint32
	height     uint32
	scanoutID  uint32
	scanoutRes uint32
	resources  map[uint32]gpuResource
	commands   uint64
	lastCmd    uint32
	fb         gpuFramebuffer
	transfers  uint64
	flushes    uint64
	lastCopy   uint32
	cursor     gpuCursorState
}

func NewVirtioGPU(bus *mem.Bus, irq func(bool)) *VirtioGPU {
	return &VirtioGPU{bus: bus, IRQ: irq, width: 1024, height: 768, resources: map[uint32]gpuResource{}}
}

func (v *VirtioGPU) LastError() string { return v.lastErr }

// SetFramebuffer connects RESOURCE_FLUSH / TRANSFER_TO_HOST_2D to a physical
// simple-framebuffer. The implementation copies packed 32-bit pixels from the
// resource backing memory into this framebuffer; this is sufficient for Linux
// virtio-gpu 2D probe/modeset diagnostics and browser-side screenshots.
func (v *VirtioGPU) SetFramebuffer(base uint64, width, height, stride uint32) {
	v.fb = gpuFramebuffer{Base: base, Width: width, Height: height, Stride: stride, Enabled: base != 0 && width != 0 && height != 0 && stride != 0}
}

func (v *VirtioGPU) DebugString() string {
	q0 := v.queues[0]
	q1 := v.queues[1]
	return fmt.Sprintf("status=%#x isr=%#x qsel=%d ctrlReady=%v ctrlLast=%d cursorReady=%v cursorLast=%d features[0]=%#x features[1]=%#x commands=%d lastCmd=%#x scanout=%d resource=%d resources=%d size=%dx%d transfers=%d flushes=%d lastCopy=%d cursor=res%d scanout%d pos=%d,%d hot=%d,%d updates=%d moves=%d visible=%v fb=%v@%#x err=%q",
		v.status, v.interruptStatus, v.queueSel, q0.ready, q0.lastAvail, q1.ready, q1.lastAvail, v.driverFeatures[0], v.driverFeatures[1], v.commands, v.lastCmd, v.scanoutID, v.scanoutRes, len(v.resources), v.width, v.height, v.transfers, v.flushes, v.lastCopy, v.cursor.ResourceID, v.cursor.ScanoutID, v.cursor.X, v.cursor.Y, v.cursor.HotX, v.cursor.HotY, v.cursor.Updates, v.cursor.Moves, v.cursor.Visible, v.fb.Enabled, v.fb.Base, v.lastErr)
}

func (v *VirtioGPU) deviceFeatures(sel uint32) uint32 {
	switch sel {
	case 0:
		return (1 << virtioRingFIndirectDescBit) | (1 << virtioRingFEventIdxBit)
	case virtioFVersion1Bit / 32:
		return 1 << (virtioFVersion1Bit % 32)
	default:
		return 0
	}
}

func (v *VirtioGPU) Read(addr uint64, size int) (uint64, error) {
	off := addr - VirtioGPUBase
	read32 := func(x uint32) (uint64, error) { return partialRead32(x, off&3, size), nil }
	switch off &^ 3 {
	case 0x000:
		return read32(virtioMMIOMagic)
	case 0x004:
		return read32(virtioVersionModern)
	case 0x008:
		return read32(virtioDeviceGPU)
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
		if v.queueSel < virtioGPUQueueCount {
			return read32(virtioGPUQueueSize)
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
	case 0x100: // events_read
		return read32(0)
	case 0x104: // events_clear
		return read32(0)
	case 0x108: // num_scanouts
		return read32(1)
	case 0x10c: // num_capsets
		return read32(0)
	default:
		return 0, nil
	}
}

func (v *VirtioGPU) Write(addr uint64, size int, val uint64) error {
	off := addr - VirtioGPUBase
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
			if ready {
				v.processQueue(v.queueSel)
			}
		}
	case 0x050:
		v.processQueue(write32(0))
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
		if write32(0) != 0 && v.queueSel < virtioGPUQueueCount {
			v.queues[v.queueSel] = virtQueue{}
			v.interruptStatus = 0
			v.updateIRQ()
		}
	case 0x104:
		// events_clear; no async config events yet.
	}
	return nil
}

func (v *VirtioGPU) Tick(cycles uint64) {}

func (v *VirtioGPU) reset() {
	v.deviceFeaturesSel = 0
	v.driverFeaturesSel = 0
	v.driverFeatures = [2]uint32{}
	v.queueSel = 0
	v.queues = [virtioGPUQueueCount]virtQueue{}
	v.interruptStatus = 0
	v.status = 0
	v.lastErr = ""
	v.scanoutID = 0
	v.scanoutRes = 0
	v.resources = map[uint32]gpuResource{}
	v.transfers, v.flushes, v.lastCopy = 0, 0, 0
	v.cursor = gpuCursorState{}
	v.updateIRQ()
}

func (v *VirtioGPU) selectedQueue() *virtQueue {
	if v.queueSel >= virtioGPUQueueCount {
		return nil
	}
	return &v.queues[v.queueSel]
}

func (v *VirtioGPU) updateIRQ() {
	if v.IRQ != nil {
		v.IRQ(v.interruptStatus != 0)
	}
}

func (v *VirtioGPU) featuresAccepted() bool {
	for sel, got := range v.driverFeatures {
		if got&^v.deviceFeatures(uint32(sel)) != 0 {
			return false
		}
	}
	return v.driverFeatures[virtioFVersion1Bit/32]&(1<<(virtioFVersion1Bit%32)) != 0
}

func (v *VirtioGPU) featureNegotiated(sel uint32, bit uint32) bool {
	if sel >= uint32(len(v.driverFeatures)) {
		return false
	}
	return v.driverFeatures[sel]&(1<<bit) != 0
}

func (v *VirtioGPU) queueReady(qidx uint32) bool {
	if qidx >= virtioGPUQueueCount {
		return false
	}
	q := &v.queues[qidx]
	return v.status&virtioStatusDriverOK != 0 && q.ready && q.num != 0 && q.num <= virtioGPUQueueSize && q.desc != 0 && q.driver != 0 && q.device != 0
}

func (v *VirtioGPU) processQueue(qidx uint32) {
	if !v.queueReady(qidx) {
		return
	}
	q := &v.queues[qidx]
	for {
		availIdx, err := v.read16(q.driver + 2)
		if err != nil || q.lastAvail == availIdx {
			return
		}
		head, err := v.read16(q.driver + 4 + uint64(q.lastAvail%uint16(q.num))*2)
		if err != nil {
			v.fail("read avail ring: %v", err)
			return
		}
		usedLen := v.handleCommand(head, q, qidx)
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
}

func (v *VirtioGPU) handleCommand(head uint16, q *virtQueue, qidx uint32) uint32 {
	descs, err := v.readChain(head, q)
	if err != nil {
		v.fail("descriptor chain: %v", err)
		return 0
	}
	req := v.readRequestBytes(descs, 4096)
	resp := v.gpuResponseForQueue(req, qidx)
	return v.writeResponseBytes(descs, resp)
}

func (v *VirtioGPU) readRequestBytes(descs []virtqDesc, max int) []byte {
	var out []byte
	for _, d := range descs {
		if d.flags&virtqDescFWrite != 0 {
			continue
		}
		for i := uint32(0); i < d.len && len(out) < max; i++ {
			x, err := v.bus.Read(d.addr+uint64(i), 1)
			if err != nil {
				v.fail("read request: %v", err)
				return out
			}
			out = append(out, byte(x))
		}
	}
	return out
}

func (v *VirtioGPU) gpuResponse(req []byte) []byte {
	return v.gpuResponseForQueue(req, virtioGPUQueueControl)
}

func (v *VirtioGPU) gpuResponseForQueue(req []byte, qidx uint32) []byte {
	if len(req) < 24 {
		return v.responseHeader(virtioGPURespErrUnspec)
	}
	cmd := binary.LittleEndian.Uint32(req[0:4])
	v.lastCmd = cmd
	v.commands++
	switch cmd {
	case virtioGPUCmdGetDisplayInfo:
		return v.displayInfoResponse()
	case virtioGPUCmdResourceCreate2D:
		if len(req) >= 40 {
			id := binary.LittleEndian.Uint32(req[24:28])
			v.resources[id] = gpuResource{ID: id, Format: binary.LittleEndian.Uint32(req[28:32]), Width: binary.LittleEndian.Uint32(req[32:36]), Height: binary.LittleEndian.Uint32(req[36:40])}
		}
		return v.responseHeader(virtioGPURespOKNoData)
	case virtioGPUCmdResourceAttachBack:
		if len(req) >= 48 {
			id := binary.LittleEndian.Uint32(req[24:28])
			entries := binary.LittleEndian.Uint32(req[28:32])
			r := v.resources[id]
			r.ID = id
			if entries > 0 && len(req) >= 48 {
				r.Backing = binary.LittleEndian.Uint64(req[32:40])
				r.Length = binary.LittleEndian.Uint32(req[40:44])
			}
			v.resources[id] = r
		}
		return v.responseHeader(virtioGPURespOKNoData)
	case virtioGPUCmdSetScanout:
		if len(req) >= 48 {
			v.width = binary.LittleEndian.Uint32(req[32:36])
			v.height = binary.LittleEndian.Uint32(req[36:40])
			v.scanoutID = binary.LittleEndian.Uint32(req[40:44])
			v.scanoutRes = binary.LittleEndian.Uint32(req[44:48])
		}
		return v.responseHeader(virtioGPURespOKNoData)
	case virtioGPUCmdTransferToHost2D:
		if len(req) >= 56 {
			id := binary.LittleEndian.Uint32(req[48:52])
			v.copyResourceToFramebuffer(id)
		}
		v.transfers++
		return v.responseHeader(virtioGPURespOKNoData)
	case virtioGPUCmdResourceFlush:
		if len(req) >= 48 {
			id := binary.LittleEndian.Uint32(req[40:44])
			if id == 0 {
				id = v.scanoutRes
			}
			v.copyResourceToFramebuffer(id)
		}
		v.flushes++
		return v.responseHeader(virtioGPURespOKNoData)
	case virtioGPUCmdResourceUnref:
		if len(req) >= 28 {
			delete(v.resources, binary.LittleEndian.Uint32(req[24:28]))
		}
		return v.responseHeader(virtioGPURespOKNoData)
	case virtioGPUCmdResourceDetachBack:
		if len(req) >= 28 {
			id := binary.LittleEndian.Uint32(req[24:28])
			r := v.resources[id]
			r.Backing, r.Length = 0, 0
			v.resources[id] = r
		}
		return v.responseHeader(virtioGPURespOKNoData)
	case virtioGPUCmdUpdateCursor, virtioGPUCmdMoveCursor:
		v.handleCursorCommand(cmd, req, qidx)
		return v.responseHeader(virtioGPURespOKNoData)
	default:
		v.fail("unsupported gpu command %#x", cmd)
		return v.responseHeader(virtioGPURespErrUnspec)
	}
}

func (v *VirtioGPU) handleCursorCommand(cmd uint32, req []byte, qidx uint32) {
	if len(req) < 48 {
		return
	}
	// struct virtio_gpu_update_cursor embeds hdr + cursor position followed by
	// resource_id/hot_x/hot_y. Older kernels may submit cursor commands through
	// either queue during early probe; accept both but count actual cursor queue use.
	v.cursor.ScanoutID = binary.LittleEndian.Uint32(req[24:28])
	v.cursor.X = binary.LittleEndian.Uint32(req[28:32])
	v.cursor.Y = binary.LittleEndian.Uint32(req[32:36])
	v.cursor.ResourceID = binary.LittleEndian.Uint32(req[36:40])
	if len(req) >= 48 {
		v.cursor.HotX = binary.LittleEndian.Uint32(req[40:44])
		v.cursor.HotY = binary.LittleEndian.Uint32(req[44:48])
	}
	v.cursor.Visible = v.cursor.ResourceID != 0
	if cmd == virtioGPUCmdMoveCursor {
		v.cursor.Moves++
	} else {
		v.cursor.Updates++
	}
	if qidx != virtioGPUQueueCursor {
		v.lastErr = fmt.Sprintf("cursor command %#x arrived on queue %d", cmd, qidx)
	}
}

func (v *VirtioGPU) CursorState() map[string]uint64 {
	return map[string]uint64{
		"updates":     v.cursor.Updates,
		"moves":       v.cursor.Moves,
		"resource_id": uint64(v.cursor.ResourceID),
		"scanout_id":  uint64(v.cursor.ScanoutID),
		"x":           uint64(v.cursor.X),
		"y":           uint64(v.cursor.Y),
		"hot_x":       uint64(v.cursor.HotX),
		"hot_y":       uint64(v.cursor.HotY),
	}
}

func (v *VirtioGPU) copyResourceToFramebuffer(id uint32) {
	if !v.fb.Enabled || id == 0 {
		return
	}
	r, ok := v.resources[id]
	if !ok || r.Backing == 0 || r.Length == 0 || r.Width == 0 || r.Height == 0 {
		return
	}
	w, h := r.Width, r.Height
	if w > v.fb.Width {
		w = v.fb.Width
	}
	if h > v.fb.Height {
		h = v.fb.Height
	}
	bytesPerRow := w * 4
	if bytesPerRow > v.fb.Stride {
		bytesPerRow = v.fb.Stride
	}
	available := r.Length
	copied := uint32(0)
	for y := uint32(0); y < h && available > 0; y++ {
		rowBytes := bytesPerRow
		if rowBytes > available {
			rowBytes = available
		}
		for x := uint32(0); x < rowBytes; x++ {
			val, err := v.bus.Read(r.Backing+uint64(y*r.Width*4+x), 1)
			if err != nil {
				v.fail("gpu fb copy read: %v", err)
				v.lastCopy = copied
				return
			}
			if err := v.bus.Write(v.fb.Base+uint64(y*v.fb.Stride+x), 1, val); err != nil {
				v.fail("gpu fb copy write: %v", err)
				v.lastCopy = copied
				return
			}
			copied++
		}
		if available <= r.Width*4 {
			break
		}
		available -= r.Width * 4
	}
	v.lastCopy = copied
}

func (v *VirtioGPU) responseHeader(typ uint32) []byte {
	resp := make([]byte, 24)
	binary.LittleEndian.PutUint32(resp[0:4], typ)
	return resp
}

func (v *VirtioGPU) displayInfoResponse() []byte {
	resp := make([]byte, 24+16*24)
	binary.LittleEndian.PutUint32(resp[0:4], virtioGPURespOKDisplayInfo)
	// struct virtio_gpu_resp_display_info uses 16 pmodes. pModes[0] starts at
	// byte 24: rect x/y/width/height, enabled, flags.
	pmode := resp[24:48]
	binary.LittleEndian.PutUint32(pmode[8:12], v.width)
	binary.LittleEndian.PutUint32(pmode[12:16], v.height)
	binary.LittleEndian.PutUint32(pmode[16:20], 1)
	return resp
}

func (v *VirtioGPU) writeResponseBytes(descs []virtqDesc, resp []byte) uint32 {
	written := uint32(0)
	pos := 0
	for _, d := range descs {
		if d.flags&virtqDescFWrite == 0 || d.len == 0 || pos >= len(resp) {
			continue
		}
		for i := uint32(0); i < d.len && pos < len(resp); i++ {
			if err := v.bus.Write(d.addr+uint64(i), 1, uint64(resp[pos])); err != nil {
				v.fail("write response: %v", err)
				return written
			}
			pos++
			written++
		}
	}
	return written
}

func (v *VirtioGPU) readChain(head uint16, q *virtQueue) ([]virtqDesc, error) {
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

func (v *VirtioGPU) readDirectChain(head uint16, table uint64, limit uint16, allowIndirect bool) ([]virtqDesc, error) {
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

func (v *VirtioGPU) readIndirectChain(d virtqDesc) ([]virtqDesc, error) {
	if d.len == 0 || d.len%16 != 0 {
		return nil, fmt.Errorf("bad indirect table length %d", d.len)
	}
	limit := d.len / 16
	if limit > 1024 {
		return nil, fmt.Errorf("indirect table too large: %d descriptors", limit)
	}
	return v.readDirectChain(0, d.addr, uint16(limit), false)
}

func (v *VirtioGPU) readDescAt(table uint64, id uint16) (virtqDesc, error) {
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

func (v *VirtioGPU) shouldInterrupt(q *virtQueue, oldUsedIdx, newUsedIdx uint16) bool {
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

func (v *VirtioGPU) fail(format string, args ...any) {
	v.lastErr = fmt.Sprintf(format, args...)
	v.status |= virtioStatusFailed
}

func (v *VirtioGPU) read16(addr uint64) (uint16, error) {
	x, err := v.bus.Read(addr, 2)
	return uint16(x), err
}
func (v *VirtioGPU) read32(addr uint64) (uint32, error) {
	x, err := v.bus.Read(addr, 4)
	return uint32(x), err
}
func (v *VirtioGPU) read64(addr uint64) (uint64, error)  { return v.bus.Read(addr, 8) }
func (v *VirtioGPU) write16(addr uint64, x uint16) error { return v.bus.Write(addr, 2, uint64(x)) }
func (v *VirtioGPU) write32(addr uint64, x uint32) error { return v.bus.Write(addr, 4, uint64(x)) }
