package dev

import "testing"

func TestUARTInputRaisesPLICIRQ(t *testing.T) {
	var mIRQ, sIRQ bool
	p := NewPLIC(func(context int, set bool) {
		switch context {
		case 0:
			mIRQ = set
		case 1:
			sIRQ = set
		}
	})
	u := NewUART(nil, func(set bool) { p.SetIRQ(10, set) })

	// Enable source 10 for S-mode context and enable UART receive interrupts.
	if err := p.Write(PLICBase+0x2080, 4, 1<<10); err != nil { // context 1 enable word 0
		t.Fatal(err)
	}
	if err := u.Write(UARTBase+1, 1, 1); err != nil {
		t.Fatal(err)
	}
	u.Inject([]byte("A"))
	if !sIRQ || mIRQ {
		t.Fatalf("irq state m=%v s=%v, want m=false s=true", mIRQ, sIRQ)
	}
	v, err := u.Read(UARTBase, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v != 'A' {
		t.Fatalf("read %#x, want 'A'", v)
	}
	if sIRQ {
		t.Fatalf("S IRQ still pending after draining input")
	}
}

func TestPLICMultiContextNotify(t *testing.T) {
	seen := map[int]bool{}
	p := NewPLICWithContexts(4, func(ctx int, set bool) { seen[ctx] = set })
	_ = p.Write(PLICBase+plicEnableBase+plicEnableStride*3, 4, 1<<10)
	p.SetIRQ(10, true)
	if !seen[3] {
		t.Fatalf("context 3 was not notified: %#v", seen)
	}
	if seen[0] || seen[1] || seen[2] {
		t.Fatalf("unexpected contexts notified: %#v", seen)
	}
	id, _ := p.Read(PLICBase+plicContextBase+plicContextStride*3+4, 4)
	if id != 10 {
		t.Fatalf("claim context 3 got %d", id)
	}
}
