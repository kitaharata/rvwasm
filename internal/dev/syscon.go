package dev

const (
	SysconBase          uint64 = 0x00100000
	SysconSize          uint64 = 0x1000
	SysconPoweroffValue uint32 = 0x5555
	SysconRebootValue   uint32 = 0x7777
)

// Syscon implements the tiny QEMU-virt style syscon/test register used by
// OpenSBI's generic syscon-poweroff and syscon-reboot reset drivers.
type Syscon struct {
	OnPoweroff func()
	OnReboot   func()

	value uint32
}

func NewSyscon(onPoweroff, onReboot func()) *Syscon {
	return &Syscon{OnPoweroff: onPoweroff, OnReboot: onReboot}
}

func (s *Syscon) Read(addr uint64, size int) (uint64, error) {
	switch size {
	case 1:
		shift := uint((addr-SysconBase)&3) * 8
		return uint64(byte(s.value >> shift)), nil
	case 2:
		shift := uint((addr-SysconBase)&2) * 8
		return uint64(uint16(s.value >> shift)), nil
	case 4, 8:
		return uint64(s.value), nil
	default:
		return 0, nil
	}
}

func (s *Syscon) Write(addr uint64, size int, val uint64) error {
	off := addr - SysconBase
	if off >= 4 {
		return nil
	}
	mask := uint32(0xffffffff)
	v := uint32(val)
	switch size {
	case 1:
		shift := uint((off & 3) * 8)
		mask = 0xff << shift
		v = uint32(byte(val)) << shift
	case 2:
		shift := uint((off & 2) * 8)
		mask = 0xffff << shift
		v = uint32(uint16(val)) << shift
	case 4, 8:
		mask = 0xffffffff
		v = uint32(val)
	}
	s.value = (s.value &^ mask) | (v & mask)
	switch s.value {
	case SysconPoweroffValue:
		if s.OnPoweroff != nil {
			s.OnPoweroff()
		}
	case SysconRebootValue:
		if s.OnReboot != nil {
			s.OnReboot()
		}
	}
	return nil
}

func (s *Syscon) Tick(cycles uint64) {}
