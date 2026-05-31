package dev

import "encoding/binary"

const PLICBase uint64 = 0x0c000000

const (
	plicPriorityBase  uint64 = 0x000000
	plicPendingBase   uint64 = 0x001000
	plicEnableBase    uint64 = 0x002000
	plicEnableStride  uint64 = 0x80
	plicContextBase   uint64 = 0x200000
	plicContextStride uint64 = 0x1000
	plicSourceCount          = 64
)

type PLIC struct {
	priority  [plicSourceCount]uint32
	pending   [plicSourceCount]bool
	enable    [][(plicSourceCount + 31) / 32]uint32
	threshold []uint32
	Notify    func(context int, set bool)
}

func NewPLIC(notify func(context int, set bool)) *PLIC { return NewPLICWithContexts(2, notify) }

func NewPLICWithContexts(contexts int, notify func(context int, set bool)) *PLIC {
	if contexts <= 0 {
		contexts = 2
	}
	p := &PLIC{enable: make([][(plicSourceCount + 31) / 32]uint32, contexts), threshold: make([]uint32, contexts), Notify: notify}
	for i := 1; i < plicSourceCount; i++ {
		p.priority[i] = 1
	}
	return p
}

func (p *PLIC) ContextCount() int { return len(p.enable) }

func (p *PLIC) SetIRQ(source uint32, set bool) {
	if source == 0 || source >= plicSourceCount {
		return
	}
	p.pending[source] = set
	p.update()
}

func (p *PLIC) update() {
	if p.Notify == nil {
		return
	}
	for ctx := 0; ctx < len(p.enable); ctx++ {
		p.Notify(ctx, p.best(ctx) != 0)
	}
}

func (p *PLIC) best(ctx int) uint32 {
	if ctx < 0 || ctx >= len(p.enable) {
		return 0
	}
	var best uint32
	var bestPrio uint32
	for source := uint32(1); source < plicSourceCount; source++ {
		if !p.pending[source] || !p.enabled(ctx, source) {
			continue
		}
		prio := p.priority[source]
		if prio == 0 || prio <= p.threshold[ctx] {
			continue
		}
		if prio > bestPrio || (prio == bestPrio && (best == 0 || source < best)) {
			best, bestPrio = source, prio
		}
	}
	return best
}

func (p *PLIC) enabled(ctx int, source uint32) bool {
	word := source / 32
	bit := source % 32
	return p.enable[ctx][word]&(1<<bit) != 0
}

func (p *PLIC) claim(ctx int) uint32 {
	id := p.best(ctx)
	if id != 0 {
		p.pending[id] = false
	}
	p.update()
	return id
}

func (p *PLIC) complete(ctx int, id uint32) {
	_ = id
	p.update()
}

func (p *PLIC) Read(addr uint64, size int) (uint64, error) {
	off := addr - PLICBase
	if size != 1 && size != 2 && size != 4 && size != 8 {
		return 0, nil
	}

	if off >= plicPriorityBase && off < plicPriorityBase+plicSourceCount*4 {
		idx := off / 4
		return partialRead32(p.priority[idx], off%4, size), nil
	}
	if off >= plicPendingBase && off < plicPendingBase+uint64(len(p.enable[0]))*4 {
		word := (off - plicPendingBase) / 4
		var bits uint32
		for i := uint32(0); i < 32; i++ {
			id := uint32(word*32) + i
			if id < plicSourceCount && p.pending[id] {
				bits |= 1 << i
			}
		}
		return partialRead32(bits, off%4, size), nil
	}
	if off >= plicEnableBase && off < plicEnableBase+uint64(len(p.enable))*plicEnableStride {
		ctx := int((off - plicEnableBase) / plicEnableStride)
		ctxOff := (off - plicEnableBase) % plicEnableStride
		word := ctxOff / 4
		if ctx >= 0 && ctx < len(p.enable) && word < uint64(len(p.enable[ctx])) {
			return partialRead32(p.enable[ctx][word], ctxOff%4, size), nil
		}
	}
	if off >= plicContextBase && off < plicContextBase+uint64(len(p.enable))*plicContextStride {
		ctx := int((off - plicContextBase) / plicContextStride)
		ctxOff := (off - plicContextBase) % plicContextStride
		if ctx >= 0 && ctx < len(p.enable) {
			switch ctxOff {
			case 0:
				return partialRead32(p.threshold[ctx], 0, size), nil
			case 4:
				return partialRead32(p.claim(ctx), 0, size), nil
			}
		}
	}
	return 0, nil
}

func (p *PLIC) Write(addr uint64, size int, val uint64) error {
	off := addr - PLICBase
	if size != 1 && size != 2 && size != 4 && size != 8 {
		return nil
	}
	if off >= plicPriorityBase && off < plicPriorityBase+plicSourceCount*4 {
		idx := off / 4
		p.priority[idx] = partialWrite32(p.priority[idx], off%4, size, val)
		p.update()
		return nil
	}
	if off >= plicEnableBase && off < plicEnableBase+uint64(len(p.enable))*plicEnableStride {
		ctx := int((off - plicEnableBase) / plicEnableStride)
		ctxOff := (off - plicEnableBase) % plicEnableStride
		word := ctxOff / 4
		if ctx >= 0 && ctx < len(p.enable) && word < uint64(len(p.enable[ctx])) {
			p.enable[ctx][word] = partialWrite32(p.enable[ctx][word], ctxOff%4, size, val)
			p.enable[ctx][0] &^= 1
			p.update()
		}
		return nil
	}
	if off >= plicContextBase && off < plicContextBase+uint64(len(p.enable))*plicContextStride {
		ctx := int((off - plicContextBase) / plicContextStride)
		ctxOff := (off - plicContextBase) % plicContextStride
		if ctx >= 0 && ctx < len(p.enable) {
			switch ctxOff {
			case 0:
				p.threshold[ctx] = partialWrite32(p.threshold[ctx], 0, size, val)
				p.update()
			case 4:
				p.complete(ctx, uint32(val))
			}
		}
	}
	return nil
}

func (p *PLIC) Tick(cycles uint64) {}

func partialRead32(v uint32, off uint64, size int) uint64 {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	if off >= 4 {
		return 0
	}
	if off+uint64(size) > 4 {
		size = int(4 - off)
	}
	var out uint64
	for i := 0; i < size; i++ {
		out |= uint64(b[off+uint64(i)]) << (8 * i)
	}
	return out
}

func partialWrite32(old uint32, off uint64, size int, val uint64) uint32 {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], old)
	if off >= 4 {
		return old
	}
	if off+uint64(size) > 4 {
		size = int(4 - off)
	}
	for i := 0; i < size; i++ {
		b[off+uint64(i)] = byte(val >> (8 * i))
	}
	return binary.LittleEndian.Uint32(b[:])
}
