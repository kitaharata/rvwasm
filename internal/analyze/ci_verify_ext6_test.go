package analyze

import (
	"strings"
	"testing"
)

func TestVerifyProvenanceAttestationPass(t *testing.T) {
	idx := BuildCIArtifactIndex([]CIArtifactInput{{Path: "report.html", Kind: "html", Data: []byte("ok")}})
	rel := ReleaseBundleManifest{Status: "pass", BundleSHA256: "b", ManifestSHA256: "m"}
	inv := BuildDependencyInventory("module x\ngo 1.23.2\n", idx)
	att := BuildProvenanceAttestation(rel, inv, idx, "2026-01-01T00:00:00Z")
	v := VerifyProvenanceAttestation(att, rel, inv, idx)
	if v.Status != "pass" {
		t.Fatalf("status=%s missing=%v mismatch=%v", v.Status, v.Missing, v.Mismatch)
	}
}

func TestVerifyProvenanceAttestationMismatch(t *testing.T) {
	idx := BuildCIArtifactIndex([]CIArtifactInput{{Path: "a.json", Kind: "json", Data: []byte("a")}})
	rel := ReleaseBundleManifest{Status: "pass", BundleSHA256: "b", ManifestSHA256: "m"}
	inv := BuildDependencyInventory("module x\ngo 1.23.2\n", idx)
	att := BuildProvenanceAttestation(rel, inv, idx, "2026-01-01T00:00:00Z")
	att.AttestationHash = "bad"
	v := VerifyProvenanceAttestation(att, rel, inv, idx)
	if v.Status != "fail" || len(v.Mismatch) == 0 {
		t.Fatalf("expected mismatch failure: %+v", v)
	}
}

func TestDependencyInventoryDiff(t *testing.T) {
	idx := BuildCIArtifactIndex(nil)
	base := BuildDependencyInventory("module x\ngo 1.23.1\nrequire example.com/a v1.0.0\n", idx)
	cur := BuildDependencyInventory("module x\ngo 1.23.2\nrequire example.com/a v1.1.0\nrequire example.com/b v0.1.0\n", idx)
	d := CompareDependencyInventory(cur, DependencyInventoryJSON(base))
	if d.Status != "warn" || len(d.ChangedModules) != 1 || len(d.AddedModules) != 1 || d.GoVersionChanged == "" {
		t.Fatalf("bad diff: %+v", d)
	}
}

func TestCompareReleaseHandoffPackageInspection(t *testing.T) {
	base := ReleaseHandoffPackageInspection{Status: "pass", Files: []ReleaseHandoffFile{{Path: "README.md", Bytes: 1, SHA256: "a", Required: true}}}
	cur := ReleaseHandoffPackageInspection{Status: "pass", Files: []ReleaseHandoffFile{{Path: "README.md", Bytes: 2, SHA256: "b", Required: true}, {Path: "extra.txt", Bytes: 1, SHA256: "c"}}}
	cmp := CompareReleaseHandoffPackageInspection(cur, ReleaseHandoffPackageInspectionJSON(base))
	if cmp.Status != "fail" || len(cmp.Changed) != 1 {
		t.Fatalf("bad compare: %+v", cmp)
	}
}

func TestRetentionManifestAndVerificationHTML(t *testing.T) {
	idx := BuildCIArtifactIndex([]CIArtifactInput{{Path: "rvwasm-junit.xml", Kind: "junit", Data: []byte("<xml/>")}, {Path: "rvwasm-release.zip", Kind: "zip", Data: []byte("zip")}})
	rel := ReleaseBundleManifest{Status: "fail", BundleSHA256: "b", ManifestSHA256: "m"}
	rm := BuildRetentionManifest(idx, rel, "2026-01-01T00:00:00Z")
	if rm.EntryCount != 2 || rm.Status != "ok" || rm.ManifestSHA256 == "" {
		t.Fatalf("bad retention: %+v", rm)
	}
	html := ReleaseVerificationHTML(rel, AttestationVerification{Status: "pass"}, DependencyInventoryDiff{Status: "warn"}, ReleaseHandoffPackageComparison{Status: "pass"}, rm)
	if !strings.Contains(html, "rvwasm release verification") || !strings.Contains(html, "Retention manifest") {
		t.Fatalf("bad html: %s", html)
	}
}
