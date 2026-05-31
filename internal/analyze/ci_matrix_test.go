package analyze

import (
	"strings"
	"testing"
)

func TestCIGatePolicyTemplates(t *testing.T) {
	if _, ok := CIGatePolicyTemplateByName("linux-boot"); !ok {
		t.Fatalf("missing linux-boot template")
	}
	js := CIGatePolicyTemplateJSON("strict")
	if !strings.Contains(js, "treat_warnings_as_failures") || !strings.Contains(js, "firmware") {
		t.Fatalf("strict template JSON missing expected fields: %s", js)
	}
	if !strings.Contains(CIGatePolicyTemplateListString(), "artifact-only") {
		t.Fatalf("template list missing artifact-only")
	}
}

func TestBundleTrendReport(t *testing.T) {
	a := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "console=ttyS0", HartCount: 1, NextAddr: 0x80200000, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 4, LoadAddr: 0x80000000, EndAddr: 0x80000003, SHA256: strings.Repeat("a", 64)}}}, Triage: TriageDashboard{Phase: "opensbi"}, StopCauses: []StopCauseCandidate{{Category: "virtqueue", Score: 10}}}
	b := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "console=hvc0", HartCount: 2, NextAddr: 0x80200000, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 4, LoadAddr: 0x80000000, EndAddr: 0x80000003, SHA256: strings.Repeat("b", 64)}}}, Triage: TriageDashboard{Phase: "linux"}, StopCauses: []StopCauseCandidate{{Category: "panic", Score: 20}}}
	r := BuildBundleTrendReport([]NamedDiagnosticBundle{{Name: "base", Bundle: a}, {Name: "current", Bundle: b}})
	if len(r.Rows) != 2 || len(r.Diffs) == 0 {
		t.Fatalf("unexpected trend: %+v", r)
	}
	text := BundleTrendReportString(r)
	if !strings.Contains(text, "top stop-cause changed") || !strings.Contains(text, "base -> current") {
		t.Fatalf("trend text missing expected info: %s", text)
	}
	if !strings.Contains(BundleTrendReportMarkdown(r), "| base | current |") {
		t.Fatalf("trend markdown missing diff table")
	}
}

func TestCIActionChecklist(t *testing.T) {
	gate := CIGateReport{Status: "fail", Checks: []CIGateCheck{{Name: "virtqueue_anomalies", Status: "fail", Detail: "too many"}}}
	integrity := BundleIntegrityReport{Issues: []BundleIntegrityIssue{{Severity: "error", Area: "artifact", Field: "firmware", Detail: "bad hash", Hint: "reload firmware"}}}
	cl := BuildCIActionChecklist(gate, integrity, []DiagnosticBundleDiff{{Area: "artifact", Key: "payload", Kind: "changed"}})
	if cl.Status != "action-required" || len(cl.Items) < 2 {
		t.Fatalf("unexpected checklist: %+v", cl)
	}
	s := CIActionChecklistString(cl)
	if !strings.Contains(s, "reload firmware") || !strings.Contains(s, "Descriptor chains") {
		t.Fatalf("checklist missing hints: %s", s)
	}
}
