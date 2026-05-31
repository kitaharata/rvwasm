package cpu

import (
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestPCProfileRecordsExecutedPCs(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x1000)
	// addi x1,x0,1 ; addi x1,x1,1 ; jal x0, -4
	prog := []byte{
		0x93, 0x00, 0x10, 0x00,
		0x93, 0x80, 0x10, 0x00,
		0x6f, 0xf0, 0xdf, 0xff,
	}
	if err := b.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h := NewHart(b)
	h.Reset(0x80000000, 0)
	h.SetProfile(true)
	for i := 0; i < 6; i++ {
		if !h.Step() {
			t.Fatalf("step %d failed: %v", i, h.LastError)
		}
	}
	text := h.PCProfileString(4, nil)
	if !strings.Contains(text, "80000004") || !strings.Contains(text, "count=") {
		t.Fatalf("unexpected profile:\n%s", text)
	}
	h.ClearProfile()
	if got := h.PCProfileString(4, nil); !strings.Contains(got, "empty") {
		t.Fatalf("profile did not clear: %s", got)
	}
}

func TestTraceCompactAndCSRAccessSummary(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x1000)
	// csrr x1, mstatus ; csrr x2, mstatus ; csrw mscratch, x1
	prog := []byte{
		0xf3, 0x20, 0x00, 0x30,
		0x73, 0x21, 0x00, 0x30,
		0x73, 0x90, 0x00, 0x34,
	}
	if err := b.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h := NewHart(b)
	h.Reset(0x80000000, 0)
	h.Trace = true
	h.SetCSRTrace(true)
	for i := 0; i < 3; i++ {
		if !h.Step() {
			t.Fatalf("step %d failed: %v", i, h.LastError)
		}
	}
	if compact := h.TraceCompactString(16, ""); compact == "" || !strings.Contains(compact, "asm=") {
		t.Fatalf("empty compact trace: %q", compact)
	}
	summary := h.CSRAccessSummaryString(8)
	if !strings.Contains(summary, "0x300") || !strings.Contains(summary, "reads=2") || !strings.Contains(summary, "0x340") {
		t.Fatalf("unexpected CSR summary:\n%s", summary)
	}
	h.ClearCSRTrace()
	if got := h.CSRAccessSummaryString(8); !strings.Contains(got, "empty") {
		t.Fatalf("CSR summary did not clear: %s", got)
	}
}
