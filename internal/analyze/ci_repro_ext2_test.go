package analyze

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestInspectMinimalReproZipBytes(t *testing.T) {
	b := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "console=ttyS0", HartCount: 1, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 4, LoadAddr: 0x80000000, EndAddr: 0x80000003, SHA256: strings.Repeat("a", 64)}}}}
	raw := DiagnosticBundleJSON(b)
	policy := CIGatePolicyTemplateJSON("default")
	files := map[string]string{
		"README.md":                "# repro\n",
		"diagnostic-bundle.json":   raw,
		"manifest.json":            ArtifactManifestJSON(b.Manifest),
		"runner-spec.json":         `{"presets":["uart-blk"]}`,
		"ci-policy.json":           policy,
		"ci-summary.json":          `{}`,
		"policy-violation-tree.md": "# tree\n",
		"history.txt":              "history\n",
		"scripts/rvsmoke.sh":       "go run ./cmd/rvsmoke -bundle diagnostic-bundle.json -policy ci-policy.json -out md\n",
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, text := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	insp, err := InspectMinimalReproZipBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if insp.Status != "pass" {
		t.Fatalf("status=%s checks=%v", insp.Status, insp.Checks)
	}
	if len(insp.Missing) != 0 {
		t.Fatalf("missing=%v", insp.Missing)
	}
	if len(insp.ArtifactPins) != 1 || insp.ArtifactPins[0].Role != "firmware" {
		t.Fatalf("artifact pins=%v", insp.ArtifactPins)
	}
	text := InspectMinimalReproZipString(insp)
	if !strings.Contains(text, "minimal repro zip status=pass") || !strings.Contains(text, "diagnostic-bundle.json") {
		t.Fatalf("unexpected text:\n%s", text)
	}
}

func TestInspectMinimalReproZipDetectsUnsafeAndMissing(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../bad")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("bad"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	insp, err := InspectMinimalReproZipBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if insp.Status != "fail" {
		t.Fatalf("expected fail, got %s", insp.Status)
	}
	if len(insp.Missing) == 0 {
		t.Fatalf("expected missing required files")
	}
}

func TestGitHubActionsMatrixWorkflowYAML(t *testing.T) {
	y := GitHubActionsMatrixWorkflowYAML("linux-boot", []string{"uart-blk", "simplefb"})
	for _, want := range []string{"strategy:", "matrix:", "'uart-blk'", "'simplefb'", "-policy-template linux-boot"} {
		if !strings.Contains(y, want) {
			t.Fatalf("workflow missing %q:\n%s", want, y)
		}
	}
}

func TestBundleTrendCSVAndChartData(t *testing.T) {
	a := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "a", HartCount: 1}, Triage: TriageDashboard{Status: "ok", Phase: "opensbi"}}
	b := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "b", HartCount: 1}, Triage: TriageDashboard{Status: "fail", Phase: "linux"}, StopCauses: []StopCauseCandidate{{Category: "panic"}}}
	trend := BuildBundleTrendReport([]NamedDiagnosticBundle{{Name: "base", Bundle: a}, {Name: "cur", Bundle: b}})
	csv := BundleTrendCSV(trend)
	if !strings.Contains(csv, "index,name,status") || !strings.Contains(csv, "cur") {
		t.Fatalf("bad csv:\n%s", csv)
	}
	chart := BuildBundleTrendChartData(trend)
	if len(chart.Points) != 2 || chart.Points[1].StatusScore != 2 || chart.Points[1].DiffCount == 0 {
		t.Fatalf("bad chart: %+v", chart)
	}
}
