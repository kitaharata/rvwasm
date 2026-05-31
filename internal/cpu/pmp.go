package cpu

import "fmt"

const (
	pmpR      uint8 = 1 << 0
	pmpW      uint8 = 1 << 1
	pmpX      uint8 = 1 << 2
	pmpAOff   uint8 = 0
	pmpATOR   uint8 = 1
	pmpANA4   uint8 = 2
	pmpANAPOT uint8 = 3
	pmpL      uint8 = 1 << 7

	pmpEntryCount = 64
)

type pmpFault struct {
	kind accessKind
	addr uint64
}

func (p pmpFault) Error() string { return fmt.Sprintf("pmp fault kind=%d addr=%#x", p.kind, p.addr) }

func pmpCfgCSRForEntry(i int) uint16 {
	// RV64 packs eight 8-bit entries into each even-numbered pmpcfg CSR:
	// pmpcfg0 => entries 0..7, pmpcfg2 => 8..15, ..., pmpcfg14 => 56..63.
	return CSR_PMPCFG0 + uint16((i/8)*2)
}

func isPmpCfgCSR(addr uint16) bool {
	return addr >= CSR_PMPCFG0 && addr <= CSR_PMPCFG0+14 && (addr-CSR_PMPCFG0)%2 == 0
}

func (h *Hart) pmpCfg(i int) uint8 {
	if i < 0 || i >= pmpEntryCount {
		return 0
	}
	csr := pmpCfgCSRForEntry(i)
	shift := uint((i % 8) * 8)
	return uint8(h.CSR[csr] >> shift)
}

func (h *Hart) pmpSetCfgCSR(addr uint16, val uint64) {
	if !isPmpCfgCSR(addr) {
		h.CSR[addr] = val
		return
	}
	baseEntry := int((addr - CSR_PMPCFG0) / 2 * 8)
	old := h.CSR[addr]
	var out uint64
	for i := 0; i < 8; i++ {
		entry := baseEntry + i
		shift := uint(i * 8)
		newByte := uint8(val >> shift)
		oldByte := uint8(old >> shift)
		if entry < pmpEntryCount && oldByte&pmpL != 0 {
			newByte = oldByte
		}
		out |= uint64(newByte) << shift
	}
	h.CSR[addr] = out
	h.refreshPMPActive()
}

func (h *Hart) refreshPMPActive() {
	h.PMPChecked = true
	for i := 0; i < pmpEntryCount; i++ {
		if ((h.pmpCfg(i) >> 3) & 3) != pmpAOff {
			h.PMPActive = true
			return
		}
	}
	h.PMPActive = false
}

func (h *Hart) pmpSetAddrCSR(addr uint16, val uint64) {
	idx := int(addr - CSR_PMPADDR0)
	if idx < 0 || idx >= pmpEntryCount {
		h.CSR[addr] = val
		return
	}
	if h.pmpCfg(idx)&pmpL != 0 {
		return
	}
	// PMP address registers are 56-bit on RV64 for physical address bits [55:2].
	h.CSR[addr] = val & ((uint64(1) << 56) - 1)
}

func (h *Hart) anyPMPConfigured() bool {
	for i := 0; i < pmpEntryCount; i++ {
		if ((h.pmpCfg(i) >> 3) & 3) != pmpAOff {
			return true
		}
	}
	return false
}

func (h *Hart) pmpRange(i int) (lo, hi uint64, ok bool) {
	cfg := h.pmpCfg(i)
	a := (cfg >> 3) & 3
	addr := h.CSR[CSR_PMPADDR0+uint16(i)]
	switch a {
	case pmpAOff:
		return 0, 0, false
	case pmpATOR:
		lo = 0
		if i > 0 {
			lo = h.CSR[CSR_PMPADDR0+uint16(i-1)] << 2
		}
		hi = addr << 2
		return lo, hi, hi > lo
	case pmpANA4:
		lo = addr << 2
		return lo, lo + 4, true
	case pmpANAPOT:
		ones := 0
		for ones < 56 && ((addr>>uint(ones))&1) == 1 {
			ones++
		}
		size := uint64(1) << uint(ones+3)
		mask := (uint64(1) << uint(ones+1)) - 1
		lo = (addr &^ mask) << 2
		return lo, lo + size, true
	default:
		return 0, 0, false
	}
}

func (h *Hart) pmpPermits(pa uint64, k accessKind, priv PrivMode) bool {
	configured := false
	for i := 0; i < pmpEntryCount; i++ {
		cfg := h.pmpCfg(i)
		lo, hi, ok := h.pmpRange(i)
		if !ok {
			continue
		}
		configured = true
		if pa < lo || pa >= hi {
			continue
		}
		// Unlocked PMP entries do not restrict M-mode. Locked entries do.
		if priv == PrivM && cfg&pmpL == 0 {
			return true
		}
		switch k {
		case accessFetch:
			return cfg&pmpX != 0
		case accessStore:
			return cfg&pmpW != 0
		default:
			return cfg&pmpR != 0
		}
	}
	if !configured || priv == PrivM {
		return true
	}
	return false
}

func (h *Hart) checkPMP(pa uint64, size int, k accessKind, priv PrivMode) error {
	if size <= 0 {
		return nil
	}
	if !h.PMPActive {
		// Early OpenSBI executes in M-mode before PMP is configured. Scanning all
		// 64 PMP entries for every instruction fetch/load/store was pure overhead
		// in that phase and dominated browser runtime. Scan at most once to catch
		// tests/tools that seed PMP CSRs directly, then trust csrWrite() to refresh
		// the cached active state when guest software programs PMPCFG.
		if !h.PMPChecked {
			h.refreshPMPActive()
		}
		if !h.PMPActive {
			return nil
		}
	}
	for i := 0; i < size; i++ {
		addr := pa + uint64(i)
		if !h.pmpPermits(addr, k, priv) {
			return pmpFault{kind: k, addr: addr}
		}
	}
	return nil
}
