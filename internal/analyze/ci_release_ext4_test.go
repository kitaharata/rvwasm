package analyze

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCIArtifactIndexDeterministic(t *testing.T) {
	idx := BuildCIArtifactIndex([]CIArtifactInput{
		{Path: "./reports/ci.html", Data: []byte("html")},
		{Path: "reports/ci.json", Kind: "json", Data: []byte("{}"), Required: true},
	})
	if idx.FileCount != 2 || idx.TotalBytes != 6 || idx.IndexSHA256 == "" {
		t.Fatalf("bad index: %+v", idx)
	}
	if idx.Entries[0].Path != "reports/ci.html" || idx.Entries[0].Kind != "html" {
		t.Fatalf("path/kind normalization failed: %+v", idx.Entries)
	}
	if !strings.Contains(CIArtifactIndexString(idx), "ci artifact index") {
		t.Fatalf("missing text output")
	}
}

func TestVerifyReproZipChecksumManifest(t *testing.T) {
	cur := ReproZipChecksumManifest{Entries: []ReproZipChecksumEntry{{Path: "README.md", Bytes: 1, SHA256: "a", Required: true}, {Path: "extra.txt", Bytes: 1, SHA256: "b"}}}
	base := ReproZipChecksumManifest{Entries: []ReproZipChecksumEntry{{Path: "README.md", Bytes: 2, SHA256: "old", Required: true}, {Path: "missing.txt", Bytes: 1, SHA256: "z"}}}
	bb, _ := json.Marshal(base)
	v := VerifyReproZipChecksumManifest(cur, string(bb))
	if v.Status != "fail" || len(v.Changed) == 0 || len(v.Missing) == 0 || len(v.Extra) == 0 {
		t.Fatalf("expected changed/missing/extra failure: %+v", v)
	}
	if !strings.Contains(ReproZipChecksumVerificationString(v), "changed") {
		t.Fatalf("missing string details")
	}
}

func TestMatrixFlakeReport(t *testing.T) {
	agg := BuildMatrixResultAggregate([]MatrixResultInput{
		{Name: "uart#1", JSON: `{"ci":{"status":"pass"}}`},
		{Name: "uart#2", JSON: `{"ci":{"status":"fail","exit_code":1}}`},
		{Name: "simplefb#1", JSON: `{"ci":{"status":"pass"}}`},
		{Name: "simplefb#2", JSON: `{"ci":{"status":"pass"}}`},
	})
	r := BuildMatrixFlakeReport(agg)
	if r.Status != "warn" || r.Flakes != 1 {
		t.Fatalf("expected one flake: %+v", r)
	}
	if !strings.Contains(MatrixFlakeReportHTML(r), "rvsmoke matrix flakes") {
		t.Fatalf("missing html")
	}
}

func TestReleaseBundleManifest(t *testing.T) {
	bundle := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "console=ttyS0", HartCount: 1}, StopCauses: []StopCauseCandidate{{Category: "panic"}}}
	idx := BuildCIArtifactIndex([]CIArtifactInput{{Path: "ci.html", Data: []byte("x")}})
	ci := CISummary{Status: "pass"}
	gate := CIGateReport{Status: "pass"}
	m := BuildReleaseBundleManifest(bundle, DiagnosticBundleJSON(bundle), BuildLogSignatureSet("pc=0x1", "hello", bundle.Manifest), ci, gate, idx, MatrixResultAggregate{}, MatrixFlakeReport{}, ReproZipChecksumManifest{}, ReproZipChecksumVerification{})
	if m.Status != "pass" || m.BundleSHA256 == "" || m.ArtifactIndex.FileCount != 1 {
		t.Fatalf("bad release manifest: %+v", m)
	}
	if !strings.Contains(ReleaseBundleManifestHTML(m, MatrixResultAggregate{}, MatrixFlakeReport{}), "href=\"#artifacts\"") {
		t.Fatalf("missing nav html")
	}
}
