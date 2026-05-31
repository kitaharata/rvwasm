package cpu

import "fmt"

type accessKind uint8

const (
	accessFetch accessKind = iota
	accessLoad
	accessStore
)

const (
	pageSize            = uint64(4096)
	pageMask            = pageSize - 1
	satpModeBare uint64 = 0
	satpModeSv39 uint64 = 8
)

const (
	pteV uint64 = 1 << 0
	pteR uint64 = 1 << 1
	pteW uint64 = 1 << 2
	pteX uint64 = 1 << 3
	pteU uint64 = 1 << 4
	pteG uint64 = 1 << 5
	pteA uint64 = 1 << 6
	pteD uint64 = 1 << 7
)

type pageFault struct {
	cause uint64
	tval  uint64
}

func (p pageFault) Error() string {
	return fmt.Sprintf("page fault cause=%d tval=%#x", p.cause, p.tval)
}

func pageFaultCause(k accessKind) uint64 {
	switch k {
	case accessFetch:
		return ExcInstructionPageFault
	case accessStore:
		return ExcStorePageFault
	default:
		return ExcLoadPageFault
	}
}

func (h *Hart) effectivePriv(k accessKind) PrivMode {
	if k != accessFetch && h.Mode == PrivM && h.CSR[CSR_MSTATUS]&MSTATUS_MPRV != 0 {
		return PrivMode((h.CSR[CSR_MSTATUS] & MSTATUS_MPP_MASK) >> MSTATUS_MPP_SHIFT)
	}
	return h.Mode
}

func (h *Hart) translate(vaddr uint64, k accessKind) (uint64, error) {
	priv := h.effectivePriv(k)
	// Machine mode instruction fetches and ordinary M-mode data accesses are physical.
	// MPRV data accesses use the privilege encoded in mstatus.MPP above.
	if priv == PrivM {
		return vaddr, nil
	}

	satp := h.CSR[CSR_SATP]
	mode := satp >> 60
	if mode == satpModeBare {
		return vaddr, nil
	}
	if mode != satpModeSv39 {
		return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
	}

	// Sv39 virtual addresses must be sign-extended from bit 38.
	if ((vaddr >> 38) & 1) == 0 {
		if vaddr>>39 != 0 {
			return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
		}
	} else if vaddr>>39 != (1<<25)-1 {
		return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
	}

	vpn := [3]uint64{(vaddr >> 12) & 0x1ff, (vaddr >> 21) & 0x1ff, (vaddr >> 30) & 0x1ff}
	basePPN := satp & ((uint64(1) << 44) - 1)
	ptAddr := basePPN * pageSize

	for level := 2; level >= 0; level-- {
		pteAddr := ptAddr + vpn[level]*8
		pte, err := h.Bus.Read(pteAddr, 8)
		if err != nil {
			return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
		}
		if pte&pteV == 0 || (pte&pteR == 0 && pte&pteW != 0) {
			return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
		}

		if pte&(pteR|pteX) == 0 {
			ptAddr = ((pte >> 10) & ((uint64(1) << 44) - 1)) * pageSize
			continue
		}

		// Leaf PTE.
		if level > 0 {
			lowerMask := (uint64(1) << uint(level*9)) - 1
			if ((pte >> 10) & lowerMask) != 0 {
				return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
			}
		}

		if !h.pteAllows(pte, priv, k) {
			return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
		}

		need := pteA
		if k == accessStore {
			need |= pteD
		}
		if pte&need != need {
			// Behave like a simple hardware walker that sets A/D bits when the PTE is in RAM.
			newPTE := pte | need
			if err := h.Bus.Write(pteAddr, 8, newPTE); err == nil {
				pte = newPTE
			} else {
				return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
			}
		}

		ppn0 := (pte >> 10) & 0x1ff
		ppn1 := (pte >> 19) & 0x1ff
		ppn2 := (pte >> 28) & ((uint64(1) << 26) - 1)
		var physPPN uint64
		switch level {
		case 2: // 1 GiB page
			physPPN = (ppn2 << 18) | (vpn[1] << 9) | vpn[0]
		case 1: // 2 MiB page
			physPPN = (ppn2 << 18) | (ppn1 << 9) | vpn[0]
		default:
			physPPN = (ppn2 << 18) | (ppn1 << 9) | ppn0
		}
		return physPPN*pageSize + (vaddr & pageMask), nil
	}
	return 0, pageFault{cause: pageFaultCause(k), tval: vaddr}
}

func (h *Hart) pteAllows(pte uint64, priv PrivMode, k accessKind) bool {
	u := pte&pteU != 0
	if priv == PrivU && !u {
		return false
	}
	if priv == PrivS && u {
		// S-mode can never execute U pages. SUM only permits data accesses.
		if k == accessFetch || h.CSR[CSR_MSTATUS]&MSTATUS_SUM == 0 {
			return false
		}
	}

	switch k {
	case accessFetch:
		return pte&pteX != 0
	case accessStore:
		return pte&pteW != 0
	default:
		if pte&pteR != 0 {
			return true
		}
		return pte&pteX != 0 && h.CSR[CSR_MSTATUS]&MSTATUS_MXR != 0
	}
}

func (h *Hart) readVirtualFetch(addr uint64, size int) (uint64, error) {
	if size <= 0 || size > 8 {
		return 0, fmt.Errorf("bad read size %d", size)
	}
	priv := h.effectivePriv(accessFetch)
	if addr&pageMask+uint64(size) <= pageSize {
		pa, err := h.translate(addr, accessFetch)
		if err != nil {
			return 0, err
		}
		if err := h.checkPMP(pa, size, accessFetch, priv); err != nil {
			return 0, err
		}
		return h.Bus.ReadNoTrace(pa, size)
	}
	var out uint64
	for i := 0; i < size; i++ {
		pa, err := h.translate(addr+uint64(i), accessFetch)
		if err != nil {
			return 0, err
		}
		if err := h.checkPMP(pa, 1, accessFetch, priv); err != nil {
			return 0, err
		}
		b, err := h.Bus.ReadNoTrace(pa, 1)
		if err != nil {
			return 0, err
		}
		out |= b << (8 * i)
	}
	return out, nil
}

func (h *Hart) readVirtual(addr uint64, size int, k accessKind) (uint64, error) {
	if size <= 0 || size > 8 {
		return 0, fmt.Errorf("bad read size %d", size)
	}
	priv := h.effectivePriv(k)
	if addr&pageMask+uint64(size) <= pageSize {
		pa, err := h.translate(addr, k)
		if err != nil {
			return 0, err
		}
		if err := h.checkPMP(pa, size, k, priv); err != nil {
			return 0, err
		}
		return h.Bus.Read(pa, size)
	}
	var out uint64
	for i := 0; i < size; i++ {
		pa, err := h.translate(addr+uint64(i), k)
		if err != nil {
			return 0, err
		}
		if err := h.checkPMP(pa, 1, k, priv); err != nil {
			return 0, err
		}
		b, err := h.Bus.Read(pa, 1)
		if err != nil {
			return 0, err
		}
		out |= b << (8 * i)
	}
	return out, nil
}

func (h *Hart) writeVirtual(addr uint64, size int, val uint64) error {
	if size <= 0 || size > 8 {
		return fmt.Errorf("bad write size %d", size)
	}
	priv := h.effectivePriv(accessStore)
	if addr&pageMask+uint64(size) <= pageSize {
		pa, err := h.translate(addr, accessStore)
		if err != nil {
			return err
		}
		if err := h.checkPMP(pa, size, accessStore, priv); err != nil {
			return err
		}
		return h.Bus.Write(pa, size, val)
	}
	for i := 0; i < size; i++ {
		pa, err := h.translate(addr+uint64(i), accessStore)
		if err != nil {
			return err
		}
		if err := h.checkPMP(pa, 1, accessStore, priv); err != nil {
			return err
		}
		if err := h.Bus.Write(pa, 1, val>>(8*i)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hart) raiseAccessError(k accessKind, addr uint64, err error) {
	if pf, ok := err.(pageFault); ok {
		h.raiseException(pf.cause, pf.tval)
		return
	}
	if pf, ok := err.(pmpFault); ok {
		switch pf.kind {
		case accessFetch:
			h.raiseException(ExcInstructionAccessFault, pf.addr)
		case accessStore:
			h.raiseException(ExcStoreAccessFault, pf.addr)
		default:
			h.raiseException(ExcLoadAccessFault, pf.addr)
		}
		return
	}
	switch k {
	case accessFetch:
		h.raiseException(ExcInstructionAccessFault, addr)
	case accessStore:
		h.raiseException(ExcStoreAccessFault, addr)
	default:
		h.raiseException(ExcLoadAccessFault, addr)
	}
}
