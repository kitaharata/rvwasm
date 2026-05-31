package dev

const UARTBase uint64 = 0x10000000

const (
	uartIERDataReady = 1 << 0
	uartIERTHRE      = 1 << 1
	uartLCRDLAB      = 1 << 7
	uartLSRDataReady = 1 << 0
	uartLSRTHRE      = 1 << 5
	uartLSRTEMT      = 1 << 6
)

type UART struct {
	Out func(byte)
	IRQ func(bool)

	ier byte
	lcr byte
	mcr byte
	dll byte
	dlm byte
	rx  []byte
}

func NewUART(out func(byte), irq func(bool)) *UART { return &UART{Out: out, IRQ: irq} }

func (u *UART) Inject(data []byte) {
	if len(data) == 0 {
		return
	}
	u.rx = append(u.rx, data...)
	u.updateIRQ()
}

func (u *UART) updateIRQ() {
	if u.IRQ == nil {
		return
	}
	pending := false
	if u.ier&uartIERDataReady != 0 && len(u.rx) > 0 {
		pending = true
	}
	if u.ier&uartIERTHRE != 0 {
		pending = true
	}
	u.IRQ(pending)
}

func (u *UART) Read(addr uint64, size int) (uint64, error) {
	off := addr - UARTBase
	var v byte
	switch off & 7 {
	case 0:
		if u.lcr&uartLCRDLAB != 0 {
			v = u.dll
		} else if len(u.rx) > 0 {
			v = u.rx[0]
			u.rx = u.rx[1:]
			u.updateIRQ()
		}
	case 1:
		if u.lcr&uartLCRDLAB != 0 {
			v = u.dlm
		} else {
			v = u.ier
		}
	case 2:
		// IIR: bit0=0 means an interrupt is pending. Use a minimal RDA/THRE signal.
		if u.ier&uartIERDataReady != 0 && len(u.rx) > 0 {
			v = 0x04
		} else if u.ier&uartIERTHRE != 0 {
			v = 0x02
		} else {
			v = 0x01
		}
	case 3:
		v = u.lcr
	case 4:
		v = u.mcr
	case 5:
		v = uartLSRTHRE | uartLSRTEMT
		if len(u.rx) > 0 {
			v |= uartLSRDataReady
		}
	default:
		v = 0
	}
	return uint64(v), nil
}

func (u *UART) Write(addr uint64, size int, val uint64) error {
	off := addr - UARTBase
	v := byte(val)
	switch off & 7 {
	case 0:
		if u.lcr&uartLCRDLAB != 0 {
			u.dll = v
		} else if u.Out != nil {
			u.Out(v)
		}
	case 1:
		if u.lcr&uartLCRDLAB != 0 {
			u.dlm = v
		} else {
			u.ier = v
			u.updateIRQ()
		}
	case 3:
		u.lcr = v
	case 4:
		u.mcr = v
	}
	return nil
}

func (u *UART) Tick(cycles uint64) {}
