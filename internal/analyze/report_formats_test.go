package analyze

import (
	"strings"
	"testing"
)

func TestBootRegressionMarkdownHTML(t *testing.T) {
	r := BootRegressionReport{
		Status:         "pc=0x80000000",
		BootEvents:     []BootEvent{{Seq: 1, Source: "console", Phase: "linux-entry", Detail: "Linux version test"}},
		DeviceProbe:    []DeviceProbe{{Name: "virtio-blk", Reads: 4, Writes: 2, IdentityReads: 4, QueueReady: 1, QueueNotify: 1, LastReg: "Status", LastValue: 0xf}},
		Virtqueues:     []VirtqueueState{{Device: "virtio-blk", Queue: 0, Ready: true, Num: 8, Desc: 0x81000000, Driver: 0x81001000, DeviceArea: 0x81002000, NotifyCount: 1}},
		MemoryCounts:   map[string]int{"elf": 1},
		InitcallCounts: map[string]int{"virtio": 2},
		TraceStats:     TraceStats{Lines: 1, Steps: 1, FirstPC: 0x80000000, LastPC: 0x80000004, TopASM: map[string]int{"addi": 1}},
	}
	md := BootRegressionReportMarkdown(r)
	if !strings.Contains(md, "# rvwasm boot regression report") || !strings.Contains(md, "virtio-blk") {
		t.Fatalf("markdown missing expected content:\n%s", md)
	}
	html := BootRegressionReportHTML(r)
	if !strings.Contains(html, "<!doctype html>") || !strings.Contains(html, "rvwasm boot regression report") {
		t.Fatalf("html missing expected content:\n%s", html)
	}
}
