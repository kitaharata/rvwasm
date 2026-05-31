package cpu

import (
	"strings"
	"testing"
)

func TestDisasmBasicTraceMnemonics(t *testing.T) {
	for raw, want := range map[uint32]string{
		0x00100093: "addi x1,x0,1",
		0x00000073: "ecall",
		0x30200073: "mret",
	} {
		got := DisasmForTest(raw, 32)
		if !strings.Contains(got, want) {
			t.Fatalf("disasm(%#x)=%q want contains %q", raw, got, want)
		}
	}
}

func TestTraceLinesFiltered(t *testing.T) {
	h := &Hart{Trace: true}
	h.traceStep(0x80000000, 0x00100093, 32)
	h.traceTrap(2, 0, false, PrivM)
	lines := h.TraceLinesFiltered(8, "trap")
	if len(lines) != 1 || !strings.Contains(lines[0], "trap") {
		t.Fatalf("filtered trace lines = %#v", lines)
	}
}
