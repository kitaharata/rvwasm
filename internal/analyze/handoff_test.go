package analyze

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompressedDiagnosticBundleRoundTrip(t *testing.T) {
	bundle := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "console=ttyS0", HartCount: 1}, StopCauses: []StopCauseCandidate{{Rank: 1, Category: "illegal-instruction", Severity: "error", Score: 80}}}
	text := DiagnosticBundleJSON(bundle)
	c, err := CompressDiagnosticBundleJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, _ := json.Marshal(c)
	decoded, err := DecodeDiagnosticBundleText(string(wrapped))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decoded, "illegal-instruction") {
		t.Fatalf("decoded bundle missing stop cause: %s", decoded)
	}
	parsed, _, err := ParseDiagnosticBundleText(c.Base64)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Manifest.BootArgs != "console=ttyS0" {
		t.Fatalf("bad parsed manifest: %#v", parsed.Manifest)
	}
}

func TestDiagnosticBundleDiff(t *testing.T) {
	base := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "a", HartCount: 1, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 1, SHA256: "1111"}}}, StopCauses: []StopCauseCandidate{{Category: "page-fault"}}}
	cur := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "b", HartCount: 2, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 2, SHA256: "2222"}}}, StopCauses: []StopCauseCandidate{{Category: "virtqueue"}}}
	rows := CompareDiagnosticBundles(cur, DiagnosticBundleJSON(base))
	s := DiagnosticBundleDiffString(rows)
	if !strings.Contains(s, "boot_args") || !strings.Contains(s, "firmware") || !strings.Contains(s, "virtqueue") {
		t.Fatalf("unexpected diff:\n%s", s)
	}
}

func TestProvenanceAndHandoffMarkdown(t *testing.T) {
	bundle := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "console=hvc0", HartCount: 2}, StopCauses: []StopCauseCandidate{{Rank: 1, Category: "virtqueue", Severity: "warn", Score: 50, Summary: "queue stalled"}}, Suggestions: []BreakpointSuggestion{{Kind: "pc-breakpoint", Address: 0x80200000, Confidence: "medium", Reason: "test"}}}
	p := BuildProvenanceReport("rvwasm-test", "now", bundle.Manifest, "pc=0x80200000", "Linux", DiagnosticBundleJSON(bundle), bundle.StopCauses)
	if p.TraceLines != 1 || p.TopStopCause != "virtqueue" || p.ManifestSHA256 == "" {
		t.Fatalf("bad provenance: %#v", p)
	}
	md := RegressionHandoffMarkdown(bundle, p, "")
	if !strings.Contains(md, "rvwasm regression handoff") || !strings.Contains(md, "queue stalled") || !strings.Contains(md, "0x80200000") {
		t.Fatalf("bad markdown:\n%s", md)
	}
}
