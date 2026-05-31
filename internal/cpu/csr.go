package cpu

const (
	CSR_USTATUS  uint16 = 0x000
	CSR_FFLAGS   uint16 = 0x001
	CSR_FRM      uint16 = 0x002
	CSR_FCSR     uint16 = 0x003
	CSR_UIE      uint16 = 0x004
	CSR_UTVEC    uint16 = 0x005
	CSR_USCRATCH uint16 = 0x040
	CSR_UEPC     uint16 = 0x041
	CSR_UCAUSE   uint16 = 0x042
	CSR_UTVAL    uint16 = 0x043
	CSR_UIP      uint16 = 0x044

	CSR_SSTATUS    uint16 = 0x100
	CSR_SEDELEG    uint16 = 0x102
	CSR_SIDELEG    uint16 = 0x103
	CSR_SIE        uint16 = 0x104
	CSR_STVEC      uint16 = 0x105
	CSR_SCOUNTEREN uint16 = 0x106
	CSR_SENVCFG    uint16 = 0x10a
	CSR_SSCRATCH   uint16 = 0x140
	CSR_SEPC       uint16 = 0x141
	CSR_SCAUSE     uint16 = 0x142
	CSR_STVAL      uint16 = 0x143
	CSR_SIP        uint16 = 0x144
	CSR_SATP       uint16 = 0x180

	CSR_MVENDORID  uint16 = 0xf11
	CSR_MARCHID    uint16 = 0xf12
	CSR_MIMPID     uint16 = 0xf13
	CSR_MHARTID    uint16 = 0xf14
	CSR_MCONFIGPTR uint16 = 0xf15

	CSR_PMPCFG0  uint16 = 0x3a0
	CSR_PMPCFG1  uint16 = 0x3a1
	CSR_PMPCFG2  uint16 = 0x3a2
	CSR_PMPCFG3  uint16 = 0x3a3
	CSR_PMPCFG4  uint16 = 0x3a4
	CSR_PMPCFG5  uint16 = 0x3a5
	CSR_PMPCFG6  uint16 = 0x3a6
	CSR_PMPCFG7  uint16 = 0x3a7
	CSR_PMPADDR0 uint16 = 0x3b0

	CSR_MSTATUS       uint16 = 0x300
	CSR_MISA          uint16 = 0x301
	CSR_MEDELEG       uint16 = 0x302
	CSR_MIDELEG       uint16 = 0x303
	CSR_MIE           uint16 = 0x304
	CSR_MTVEC         uint16 = 0x305
	CSR_MCOUNTEREN    uint16 = 0x306
	CSR_MCOUNTINHIBIT uint16 = 0x320
	CSR_MSTATUSH      uint16 = 0x310
	CSR_MENVCFG       uint16 = 0x30a
	CSR_MENVCFGH      uint16 = 0x31a
	CSR_MSTATEEN0     uint16 = 0x30c
	CSR_MSTATEEN1     uint16 = 0x30d
	CSR_MSTATEEN2     uint16 = 0x30e
	CSR_MSTATEEN3     uint16 = 0x30f
	CSR_MSECCFG       uint16 = 0x747
	CSR_MSCRATCH      uint16 = 0x340
	CSR_MEPC          uint16 = 0x341
	CSR_MCAUSE        uint16 = 0x342
	CSR_MTVAL         uint16 = 0x343
	CSR_MIP           uint16 = 0x344
	CSR_MTINST        uint16 = 0x34a
	CSR_MTVAL2        uint16 = 0x34b

	CSR_MCYCLE       uint16 = 0xb00
	CSR_MINSTRET     uint16 = 0xb02
	CSR_CYCLE        uint16 = 0xc00
	CSR_TIME         uint16 = 0xc01
	CSR_INSTRET      uint16 = 0xc02
	CSR_HPMCOUNTER3  uint16 = 0xc03
	CSR_MHPMCOUNTER3 uint16 = 0xb03
)

const (
	MSTATUS_UIE       uint64 = 1 << 0
	MSTATUS_SIE       uint64 = 1 << 1
	MSTATUS_MIE       uint64 = 1 << 3
	MSTATUS_UPIE      uint64 = 1 << 4
	MSTATUS_SPIE      uint64 = 1 << 5
	MSTATUS_MPIE      uint64 = 1 << 7
	MSTATUS_SPP       uint64 = 1 << 8
	MSTATUS_MPP_SHIFT        = 11
	MSTATUS_MPP_MASK  uint64 = 3 << MSTATUS_MPP_SHIFT
	MSTATUS_MPRV      uint64 = 1 << 17
	MSTATUS_FS_MASK   uint64 = 3 << 13
	MSTATUS_XS_MASK   uint64 = 3 << 15
	MSTATUS_SUM       uint64 = 1 << 18
	MSTATUS_MXR       uint64 = 1 << 19
	MSTATUS_TVM       uint64 = 1 << 20
	MSTATUS_TW        uint64 = 1 << 21
	MSTATUS_TSR       uint64 = 1 << 22
	MSTATUS_UXL_MASK  uint64 = 3 << 32
	MSTATUS_SXL_MASK  uint64 = 3 << 34
	MSTATUS_SD        uint64 = 1 << 63
)

const (
	MIP_SSIP uint64 = 1 << 1
	MIP_MSIP uint64 = 1 << 3
	MIP_STIP uint64 = 1 << 5
	MIP_MTIP uint64 = 1 << 7
	MIP_SEIP uint64 = 1 << 9
	MIP_MEIP uint64 = 1 << 11
)

var sstatusMask uint64 = MSTATUS_UIE | MSTATUS_SIE | MSTATUS_UPIE | MSTATUS_SPIE | MSTATUS_SPP | MSTATUS_FS_MASK | MSTATUS_XS_MASK | MSTATUS_SUM | MSTATUS_MXR | MSTATUS_UXL_MASK | MSTATUS_SD
var sipMask uint64 = MIP_SSIP | MIP_STIP | MIP_SEIP

func misaRV64IMAC() uint64 {
	var x uint64 = 2 << 62 // MXL=64
	x |= 1 << 0            // A
	x |= 1 << 2            // C
	x |= 1 << 8            // I
	x |= 1 << 12           // M
	x |= 1 << 18           // S
	x |= 1 << 20           // U
	return x
}

func (h *Hart) initCSR() {
	h.CSR = make(map[uint16]uint64, 256)
	h.CSR[CSR_MISA] = misaRV64IMAC()
	h.CSR[CSR_MVENDORID] = 0
	h.CSR[CSR_MARCHID] = 0
	h.CSR[CSR_MIMPID] = 0
	h.CSR[CSR_MHARTID] = h.HartID
	h.CSR[CSR_MSTATUS] = (2 << 32) | (2 << 34) // UXL/SXL=64
	h.CSR[CSR_SATP] = 0
	h.CSR[CSR_MENVCFG] = 0
	h.CSR[CSR_SENVCFG] = 0
	h.CSR[CSR_MSECCFG] = 0
	h.CSR[CSR_MCOUNTEREN] = ^uint64(0)
	h.CSR[CSR_SCOUNTEREN] = ^uint64(0)
	h.CSR[CSR_MCOUNTINHIBIT] = 0
}

func (h *Hart) csrExists(addr uint16) bool {
	switch addr {
	case CSR_USTATUS, CSR_UIE, CSR_UTVEC, CSR_USCRATCH, CSR_UEPC, CSR_UCAUSE, CSR_UTVAL, CSR_UIP:
		return true
	case CSR_SSTATUS, CSR_SEDELEG, CSR_SIDELEG, CSR_SIE, CSR_STVEC, CSR_SCOUNTEREN, CSR_SENVCFG, CSR_SSCRATCH, CSR_SEPC, CSR_SCAUSE, CSR_STVAL, CSR_SIP, CSR_SATP:
		return true
	case CSR_MVENDORID, CSR_MARCHID, CSR_MIMPID, CSR_MHARTID, CSR_MCONFIGPTR:
		return true
	case CSR_MSTATUS, CSR_MISA, CSR_MEDELEG, CSR_MIDELEG, CSR_MIE, CSR_MTVEC, CSR_MCOUNTEREN, CSR_MCOUNTINHIBIT, CSR_MSTATUSH, CSR_MENVCFG, CSR_MENVCFGH, CSR_MSECCFG, CSR_MSTATEEN0, CSR_MSTATEEN1, CSR_MSTATEEN2, CSR_MSTATEEN3, CSR_MSCRATCH, CSR_MEPC, CSR_MCAUSE, CSR_MTVAL, CSR_MIP, CSR_MTINST, CSR_MTVAL2:
		return true
	case CSR_MCYCLE, CSR_MINSTRET, CSR_CYCLE, CSR_TIME, CSR_INSTRET:
		return true
	}
	if isPmpCfgCSR(addr) || (addr >= CSR_PMPADDR0 && addr < CSR_PMPADDR0+pmpEntryCount) {
		return true
	}
	// HPM counters are implemented as hard-wired zero counters so kernels can
	// enumerate them without falling into an infinite illegal-instruction probe.
	if (addr >= CSR_HPMCOUNTER3 && addr <= 0xc1f) || (addr >= CSR_MHPMCOUNTER3 && addr <= 0xb1f) {
		return true
	}
	return false
}

func (h *Hart) csrCounterAllowed(addr uint16) bool {
	var bit uint
	switch addr {
	case CSR_CYCLE:
		bit = 0
	case CSR_TIME:
		bit = 1
	case CSR_INSTRET:
		bit = 2
	default:
		if addr >= CSR_HPMCOUNTER3 && addr <= 0xc1f {
			bit = uint(addr - CSR_HPMCOUNTER3 + 3)
		} else {
			return true
		}
	}
	if h.Mode < PrivM && h.CSR[CSR_MCOUNTEREN]&(uint64(1)<<bit) == 0 {
		return false
	}
	if h.Mode < PrivS && h.CSR[CSR_SCOUNTEREN]&(uint64(1)<<bit) == 0 {
		return false
	}
	return true
}

func (h *Hart) csrRead(addr uint16) uint64 {
	switch addr {
	case CSR_SSTATUS:
		return h.CSR[CSR_MSTATUS] & sstatusMask
	case CSR_SIE:
		return h.CSR[CSR_MIE] & h.CSR[CSR_MIDELEG]
	case CSR_SIP:
		return h.CSR[CSR_MIP] & sipMask
	case CSR_CYCLE, CSR_MCYCLE:
		return h.Cycle
	case CSR_INSTRET, CSR_MINSTRET:
		return h.Instret
	case CSR_TIME:
		return h.Time
	case CSR_MISA:
		return misaRV64IMAC()
	case CSR_MHARTID:
		return h.HartID
	default:
		if (addr >= CSR_HPMCOUNTER3 && addr <= 0xc1f) || (addr >= CSR_MHPMCOUNTER3 && addr <= 0xb1f) {
			return 0
		}
		return h.CSR[addr]
	}
}

// ReadCSR returns a CSR value using the same aliases and counters as guest
// CSR reads. It is intended for UI/debug views, not for permission checks.
func (h *Hart) ReadCSR(addr uint16) uint64 { return h.csrRead(addr) }

func (h *Hart) csrWrite(addr uint16, val uint64) {
	switch addr {
	case CSR_MISA, CSR_MVENDORID, CSR_MARCHID, CSR_MIMPID, CSR_MHARTID, CSR_MCONFIGPTR:
		return
	case CSR_SSTATUS:
		old := h.CSR[CSR_MSTATUS]
		h.CSR[CSR_MSTATUS] = (old &^ sstatusMask) | (val & sstatusMask)
	case CSR_MSTATUS:
		// Keep rvwasm in RV64 S/U mode and normalize unsupported MPP=2 writes.
		if ((val & MSTATUS_MPP_MASK) >> MSTATUS_MPP_SHIFT) == 2 {
			val &^= MSTATUS_MPP_MASK
		}
		val &^= MSTATUS_UXL_MASK | MSTATUS_SXL_MASK | MSTATUS_SD
		val |= (2 << 32) | (2 << 34)
		h.CSR[CSR_MSTATUS] = val
	case CSR_SENVCFG:
		// Keep envcfg WARL-minimal.  The guest may write cache-management
		// enable bits while probing; unsupported bits read back as zero.
		h.CSR[CSR_SENVCFG] = val & h.CSR[CSR_MENVCFG]
	case CSR_MENVCFG:
		h.CSR[CSR_MENVCFG] = val
		h.CSR[CSR_SENVCFG] &= val
	case CSR_MSTATEEN0, CSR_MSTATEEN1, CSR_MSTATEEN2, CSR_MSTATEEN3:
		// State-enable CSRs are present as a conservative all-zero WARL stub.
		h.CSR[addr] = 0
	case CSR_SIE:
		mask := h.CSR[CSR_MIDELEG]
		h.CSR[CSR_MIE] = (h.CSR[CSR_MIE] &^ mask) | (val & mask)
	case CSR_SIP:
		h.CSR[CSR_MIP] = (h.CSR[CSR_MIP] &^ sipMask) | (val & sipMask)
	case CSR_MIP:
		// software may write some pending bits; CLINT overwrites MSIP/MTIP as time passes.
		h.CSR[CSR_MIP] = val
	case CSR_CYCLE, CSR_TIME, CSR_INSTRET:
		return
	case CSR_MCYCLE:
		h.Cycle = val
	case CSR_MINSTRET:
		h.Instret = val
	default:
		if isPmpCfgCSR(addr) {
			h.pmpSetCfgCSR(addr, val)
			return
		}
		if addr >= CSR_PMPADDR0 && addr < CSR_PMPADDR0+pmpEntryCount {
			h.pmpSetAddrCSR(addr, val)
			return
		}
		h.CSR[addr] = val
	}
}

func (h *Hart) SetPending(mask uint64, set bool) {
	if set {
		h.CSR[CSR_MIP] |= mask
	} else {
		h.CSR[CSR_MIP] &^= mask
	}
}
