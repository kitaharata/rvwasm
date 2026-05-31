package analyze

import (
	"strings"
	"testing"
)

func sampleCIBundle() DiagnosticBundle {
	data := []byte("firmware")
	m := ArtifactManifest{BootArgs: "console=ttyS0", HartCount: 1, NextAddr: 0x80200000, Artifacts: []ArtifactEntry{NewArtifactEntry("firmware", data, 0x80000000, 0x80000000, false, "")}}
	return DiagnosticBundle{
		Manifest:    m,
		Triage:      TriageDashboard{Phase: "opensbi"},
		StopCauses:  []StopCauseCandidate{{Rank: 1, Category: "virtqueue", Severity: "warn", Score: 50, Summary: "queue stalled"}},
		Suggestions: []BreakpointSuggestion{{Kind: "pc-breakpoint", Address: 0x80200000, Command: "break pc=0x80200000", Reason: "test"}},
		Smoke:       []SmokeSummary{{Preset: "uart-blk", Requested: 100, Ran: 100, Phase: "opensbi", TopCause: "virtqueue"}},
	}
}

func TestBundleIntegrityAndCISummary(t *testing.T) {
	b := sampleCIBundle()
	report := BuildBundleIntegrityReport(b, DiagnosticBundleJSON(b))
	if report.Status != "ok" {
		t.Fatalf("unexpected status: %#v", report)
	}
	sig := BuildLogSignatureSet("pc=0x80200000 ecall", "OpenSBI", b.Manifest)
	ci := BuildCISummary(b, sig, report)
	if ci.Status != "fail" || ci.ExitCode != 1 {
		t.Fatalf("virtqueue smoke should fail CI summary: %#v", ci)
	}
	if !strings.Contains(CISummaryString(ci), "virtqueue") {
		t.Fatalf("summary missing cause: %s", CISummaryString(ci))
	}
}

func TestReproductionValidationAndRunnerSpec(t *testing.T) {
	b := sampleCIBundle()
	p := BuildReproductionPlan(b, ProvenanceReport{Tool: "rvwasm-test"}, "uart-blk")
	sig := BuildLogSignatureSet("pc=0x80200000", "Linux", b.Manifest)
	v := ValidateReproductionPlan(b, p, sig)
	if v.Status != "pass" {
		t.Fatalf("expected pass: %s", ReproductionValidationReportString(v))
	}
	spec := BuildHeadlessSmokeRunnerSpec(b, []string{"uart-blk"}, 1234)
	text := HeadlessSmokeRunnerSpecString(spec)
	if !strings.Contains(text, "rvsmoke") || !strings.Contains(text, "uart-blk") {
		t.Fatalf("bad spec: %s", text)
	}
}

func TestBundleIntegrityDetectsBadArtifact(t *testing.T) {
	b := sampleCIBundle()
	b.Manifest.Artifacts[0].SHA256 = "bad"
	r := BuildBundleIntegrityReport(b, "")
	if r.Status != "error" || r.Counts["error"] == 0 {
		t.Fatalf("expected error: %#v", r)
	}
}
