package analyze

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestCIGateReportPolicy(t *testing.T) {
	bundle := DiagnosticBundle{
		Manifest:   ArtifactManifest{HartCount: 1, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 1, SHA256: strings.Repeat("a", 64)}}},
		StopCauses: []StopCauseCandidate{{Category: "panic/oops"}},
		Smoke:      []SmokeSummary{{Preset: "uart", TopCause: "panic"}},
	}
	integrity := BuildBundleIntegrityReport(bundle, DiagnosticBundleJSON(bundle))
	policy := DefaultCIGatePolicy()
	policy.RequireArtifacts = []string{"firmware", "payload"}
	report := BuildCIGateReport(bundle, integrity, LogSignatureSet{TraceLines: 2}, CompareDiagnosticBundles(bundle, ""), policy)
	if report.Status != "fail" || report.ExitCode != 1 {
		t.Fatalf("expected fail report: %#v", report)
	}
	s := CIGateReportString(report)
	if !strings.Contains(s, "payload") || !strings.Contains(s, "top_stop_cause") {
		t.Fatalf("unexpected gate string:\n%s", s)
	}
}

func TestCIJUnitHTMLAndSARIF(t *testing.T) {
	bundle := DiagnosticBundle{Manifest: ArtifactManifest{HartCount: 1}, StopCauses: []StopCauseCandidate{{Category: "illegal-instruction"}}}
	integrity := BuildBundleIntegrityReport(bundle, DiagnosticBundleJSON(bundle))
	gate := BuildCIGateReport(bundle, integrity, LogSignatureSet{}, nil, DefaultCIGatePolicy())
	j := CIJUnitXML(gate, integrity, nil, nil)
	var decoded struct {
		XMLName xml.Name `xml:"testsuite"`
	}
	if err := xml.Unmarshal([]byte(j), &decoded); err != nil {
		t.Fatalf("bad junit xml: %v\n%s", err, j)
	}
	if !strings.Contains(j, "rvwasm.gate") {
		t.Fatalf("junit missing gate: %s", j)
	}
	h := CIHTMLReport(BuildCISummary(bundle, LogSignatureSet{}, integrity), gate, integrity, LogSignatureSet{}, nil, nil)
	if !strings.Contains(h, "rvwasm CI report") || !strings.Contains(h, "Bundle integrity") {
		t.Fatalf("bad html: %s", h)
	}
	sarif := CISARIFReport(integrity, gate)
	if !strings.Contains(sarif, "2.1.0") || !strings.Contains(sarif, "rvsmoke") {
		t.Fatalf("bad sarif: %s", sarif)
	}
}
