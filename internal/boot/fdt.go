package boot

import (
	"bytes"
	"encoding/binary"
)

const (
	fdtMagic     = 0xd00dfeed
	fdtBeginNode = 1
	fdtEndNode   = 2
	fdtProp      = 3
	fdtEnd       = 9
)

type fdtBuilder struct {
	structBlock bytes.Buffer
	strings     bytes.Buffer
	strOff      map[string]uint32
}

// VirtDTBConfig contains the rvwasm virt-machine details that are useful to
// vary between boot attempts.  The zero value deliberately preserves the old
// BuildVirtDTB behavior.
type VirtDTBConfig struct {
	MemBase     uint64
	MemSize     uint64
	TimebaseHz  uint32
	BootArgs    string
	InitrdStart uint64
	InitrdEnd   uint64
	HartCount   int
	Framebuffer FramebufferConfig
}

type FramebufferConfig struct {
	Base    uint64
	Size    uint64
	Width   uint32
	Height  uint32
	Stride  uint32
	Format  string
	Enabled bool
}

const (
	sysconBase          uint64 = 0x00100000
	sysconSize          uint64 = 0x1000
	sysconPhandle       uint32 = 0x100
	sysconPoweroffValue uint32 = 0x5555
	sysconRebootValue   uint32 = 0x7777
	sysconResetMask     uint32 = 0xffffffff
)

func BuildVirtDTB(memBase, memSize uint64, timebase uint32) []byte {
	return BuildVirtDTBConfig(VirtDTBConfig{MemBase: memBase, MemSize: memSize, TimebaseHz: timebase})
}

func BuildVirtDTBConfig(cfg VirtDTBConfig) []byte {
	if cfg.MemBase == 0 {
		cfg.MemBase = 0x80000000
	}
	if cfg.MemSize == 0 {
		cfg.MemSize = 128 * 1024 * 1024
	}
	if cfg.TimebaseHz == 0 {
		cfg.TimebaseHz = 10000000
	}
	if cfg.BootArgs == "" {
		cfg.BootArgs = "console=ttyS0 earlycon=sbi root=/dev/vda rw"
	}
	if cfg.HartCount <= 0 {
		cfg.HartCount = 1
	}

	b := &fdtBuilder{strOff: map[string]uint32{}}
	b.begin("")
	b.propStr("model", "rvwasm")
	b.propStrList("compatible", []string{"riscv-virtio", "riscv-virtio,qemu"})
	b.propU32("#address-cells", 2)
	b.propU32("#size-cells", 2)

	b.begin("aliases")
	b.propStr("serial0", "/soc/serial@10000000")
	b.end()

	b.begin("chosen")
	b.propStr("stdout-path", "/soc/serial@10000000")
	b.propStr("bootargs", cfg.BootArgs)
	if cfg.InitrdStart != 0 && cfg.InitrdEnd > cfg.InitrdStart {
		b.propU64("linux,initrd-start", cfg.InitrdStart)
		b.propU64("linux,initrd-end", cfg.InitrdEnd)
	}
	if cfg.Framebuffer.Enabled && cfg.Framebuffer.Base != 0 && cfg.Framebuffer.Size != 0 {
		fb := cfg.Framebuffer
		if fb.Format == "" {
			fb.Format = "a8r8g8b8"
		}
		b.begin("framebuffer@" + hex32(uint32(fb.Base)))
		b.propStr("compatible", "simple-framebuffer")
		b.propStr("status", "okay")
		b.propReg64("reg", fb.Base, fb.Size)
		b.propU32("width", fb.Width)
		b.propU32("height", fb.Height)
		b.propU32("stride", fb.Stride)
		b.propStr("format", fb.Format)
		b.end()
	}
	b.end()

	b.begin("cpus")
	b.propU32("#address-cells", 1)
	b.propU32("#size-cells", 0)
	b.propU32("timebase-frequency", cfg.TimebaseHz)
	for hart := 0; hart < cfg.HartCount; hart++ {
		b.begin("cpu@" + dec(hart))
		b.propStr("device_type", "cpu")
		b.propU32("reg", uint32(hart))
		b.propStr("status", "okay")
		b.propStr("compatible", "riscv")
		b.propStr("riscv,isa", "rv64imac_zicsr_zifencei")
		b.propStr("mmu-type", "riscv,sv39")
		b.begin("interrupt-controller")
		b.propEmpty("interrupt-controller")
		b.propU32("#interrupt-cells", 1)
		b.propStr("compatible", "riscv,cpu-intc")
		b.propU32("phandle", cpuIntcPhandle(hart))
		b.end()
		b.end()
	}
	b.end()

	b.begin("memory@80000000")
	b.propStr("device_type", "memory")
	b.propReg64("reg", cfg.MemBase, cfg.MemSize)
	b.end()

	b.begin("reboot")
	b.propStr("compatible", "syscon-reboot")
	b.propU32("regmap", sysconPhandle)
	b.propU32("offset", 0)
	b.propU32("mask", sysconResetMask)
	b.propU32("value", sysconRebootValue)
	b.end()

	b.begin("poweroff")
	b.propStr("compatible", "syscon-poweroff")
	b.propU32("regmap", sysconPhandle)
	b.propU32("offset", 0)
	b.propU32("mask", sysconResetMask)
	b.propU32("value", sysconPoweroffValue)
	b.end()

	b.begin("soc")
	b.propU32("#address-cells", 2)
	b.propU32("#size-cells", 2)
	b.propEmpty("ranges")
	b.propStr("compatible", "simple-bus")

	b.begin("test@100000")
	b.propStrList("compatible", []string{"sifive,test1", "sifive,test0", "syscon"})
	b.propReg64("reg", sysconBase, sysconSize)
	b.propU32("phandle", sysconPhandle)
	b.end()

	b.begin("serial@10000000")
	b.propStr("compatible", "ns16550a")
	b.propReg64("reg", 0x10000000, 0x100)
	b.propU32("clock-frequency", 3686400)
	b.propU32("current-speed", 115200)
	b.propU32("interrupt-parent", 2)
	b.propU32("interrupts", 10)
	b.end()

	b.begin("virtio_mmio@10001000")
	b.propStr("compatible", "virtio,mmio")
	b.propReg64("reg", 0x10001000, 0x200)
	b.propU32("interrupt-parent", 2)
	b.propU32("interrupts", 1)
	b.propEmpty("dma-coherent")
	b.end()

	b.begin("virtio_mmio@10002000")
	b.propStr("compatible", "virtio,mmio")
	b.propReg64("reg", 0x10002000, 0x200)
	b.propU32("interrupt-parent", 2)
	b.propU32("interrupts", 2)
	b.propEmpty("dma-coherent")
	b.end()

	b.begin("virtio_mmio@10003000")
	b.propStr("compatible", "virtio,mmio")
	b.propReg64("reg", 0x10003000, 0x200)
	b.propU32("interrupt-parent", 2)
	b.propU32("interrupts", 3)
	b.propEmpty("dma-coherent")
	b.propBytes("local-mac-address", []byte{0x02, 0x72, 0x76, 0x77, 0x00, 0x01})
	b.end()

	b.begin("virtio_mmio@10004000")
	b.propStr("compatible", "virtio,mmio")
	b.propReg64("reg", 0x10004000, 0x200)
	b.propU32("interrupt-parent", 2)
	b.propU32("interrupts", 4)
	b.propEmpty("dma-coherent")
	b.end()

	b.begin("virtio_mmio@10005000")
	b.propStr("compatible", "virtio,mmio")
	b.propReg64("reg", 0x10005000, 0x200)
	b.propU32("interrupt-parent", 2)
	b.propU32("interrupts", 5)
	b.propEmpty("dma-coherent")
	b.end()

	b.begin("virtio_mmio@10006000")
	b.propStr("compatible", "virtio,mmio")
	b.propReg64("reg", 0x10006000, 0x200)
	b.propU32("interrupt-parent", 2)
	b.propU32("interrupts", 6)
	b.propEmpty("dma-coherent")
	b.end()

	b.begin("clint@2000000")
	b.propStrList("compatible", []string{"sifive,clint0", "riscv,clint0"})
	b.propReg64("reg", 0x02000000, 0x10000)
	b.propU32Slice("interrupts-extended", clintInterruptsExtended(cfg.HartCount))
	b.end()

	b.begin("plic@c000000")
	b.propStrList("compatible", []string{"sifive,plic-1.0.0", "riscv,plic0"})
	b.propU32("#interrupt-cells", 1)
	b.propEmpty("interrupt-controller")
	b.propReg64("reg", 0x0c000000, 0x4000000)
	b.propU32Slice("interrupts-extended", plicInterruptsExtended(cfg.HartCount))
	b.propU32("riscv,ndev", 63)
	b.propU32("phandle", 2)
	b.end()

	b.end() // soc
	b.end() // root
	b.token(fdtEnd)

	memrsv := make([]byte, 16) // one zero reservation entry
	offMem := uint32(40)
	offStruct := offMem + uint32(len(memrsv))
	offStrings := offStruct + uint32(b.structBlock.Len())
	total := offStrings + uint32(b.strings.Len())

	out := bytes.NewBuffer(make([]byte, 0, total))
	be32(out, fdtMagic)
	be32(out, total)
	be32(out, offStruct)
	be32(out, offStrings)
	be32(out, offMem)
	be32(out, 17)
	be32(out, 16)
	be32(out, 0)
	be32(out, uint32(b.strings.Len()))
	be32(out, uint32(b.structBlock.Len()))
	out.Write(memrsv)
	out.Write(b.structBlock.Bytes())
	out.Write(b.strings.Bytes())
	return out.Bytes()
}

func cpuIntcPhandle(hart int) uint32 {
	if hart == 0 {
		return 1
	}
	return uint32(hart + 2) // reserve phandle 2 for the PLIC.
}

func clintInterruptsExtended(harts int) []uint32 {
	out := make([]uint32, 0, harts*4)
	for hart := 0; hart < harts; hart++ {
		ph := cpuIntcPhandle(hart)
		out = append(out, ph, 3, ph, 7)
	}
	return out
}

func plicInterruptsExtended(harts int) []uint32 {
	out := make([]uint32, 0, harts*4)
	for hart := 0; hart < harts; hart++ {
		ph := cpuIntcPhandle(hart)
		out = append(out, ph, 11, ph, 9)
	}
	return out
}

func hex32(v uint32) string {
	const digits = "0123456789abcdef"
	var buf [8]byte
	started := false
	j := 0
	for i := 7; i >= 0; i-- {
		n := (v >> uint(i*4)) & 0xf
		if n != 0 || started || i == 0 {
			started = true
			buf[j] = digits[n]
			j++
		}
	}
	return string(buf[:j])
}

func dec(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func (b *fdtBuilder) token(t uint32) { be32(&b.structBlock, t) }
func (b *fdtBuilder) align() {
	for b.structBlock.Len()%4 != 0 {
		b.structBlock.WriteByte(0)
	}
}

func (b *fdtBuilder) begin(name string) {
	b.token(fdtBeginNode)
	b.structBlock.WriteString(name)
	b.structBlock.WriteByte(0)
	b.align()
}
func (b *fdtBuilder) end() { b.token(fdtEndNode) }

func (b *fdtBuilder) nameOff(name string) uint32 {
	if off, ok := b.strOff[name]; ok {
		return off
	}
	off := uint32(b.strings.Len())
	b.strings.WriteString(name)
	b.strings.WriteByte(0)
	b.strOff[name] = off
	return off
}

func (b *fdtBuilder) prop(name string, data []byte) {
	b.token(fdtProp)
	be32(&b.structBlock, uint32(len(data)))
	be32(&b.structBlock, b.nameOff(name))
	b.structBlock.Write(data)
	b.align()
}
func (b *fdtBuilder) propEmpty(name string)              { b.prop(name, nil) }
func (b *fdtBuilder) propBytes(name string, data []byte) { b.prop(name, data) }
func (b *fdtBuilder) propStr(name, s string)             { b.prop(name, append([]byte(s), 0)) }
func (b *fdtBuilder) propStrList(name string, ss []string) {
	var d []byte
	for _, s := range ss {
		d = append(d, []byte(s)...)
		d = append(d, 0)
	}
	b.prop(name, d)
}
func (b *fdtBuilder) propU32(name string, v uint32) {
	var d [4]byte
	binary.BigEndian.PutUint32(d[:], v)
	b.prop(name, d[:])
}
func (b *fdtBuilder) propU64(name string, v uint64) {
	var d [8]byte
	binary.BigEndian.PutUint64(d[:], v)
	b.prop(name, d[:])
}
func (b *fdtBuilder) propU32Slice(name string, vs []uint32) {
	d := make([]byte, len(vs)*4)
	for i, v := range vs {
		binary.BigEndian.PutUint32(d[i*4:], v)
	}
	b.prop(name, d)
}
func (b *fdtBuilder) propReg64(name string, base, size uint64) {
	d := make([]byte, 16)
	binary.BigEndian.PutUint64(d[0:], base)
	binary.BigEndian.PutUint64(d[8:], size)
	b.prop(name, d)
}
func be32(w *bytes.Buffer, v uint32) {
	var d [4]byte
	binary.BigEndian.PutUint32(d[:], v)
	w.Write(d[:])
}
