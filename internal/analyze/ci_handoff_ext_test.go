package analyze

import (
	"strings"
	"testing"
)

func TestGitHubActionsWorkflowYAML(t *testing.T) {
	y := GitHubActionsWorkflowYAML("linux-boot")
	for _, want := range []string{"actions/setup-go@v5", "go-version: '1.23.2'", "-policy-template linux-boot", "rvwasm-ci.html"} {
		if !strings.Contains(y, want) {
			t.Fatalf("workflow missing %q in\n%s", want, y)
		}
	}
}

func TestPolicyViolationTree(t *testing.T) {
	gate := CIGateReport{Status: "fail", Checks: []CIGateCheck{{Name: "top-stop-cause", Status: "fail", Observed: "panic", Expected: "no panic", Detail: "panic matched policy"}}}
	integrity := BundleIntegrityReport{Status: "warn", Issues: []BundleIntegrityIssue{{Severity: "warn", Area: "artifact", Field: "payload", Detail: "missing hash", Hint: "reload payload"}}}
	tree := BuildPolicyViolationTree(gate, integrity, nil, BundleTrendReport{})
	if tree.Status != "fail" || len(tree.Nodes) < 2 {
		t.Fatalf("unexpected tree: %#v", tree)
	}
	text := PolicyViolationTreeString(tree)
	if !strings.Contains(text, "top-stop-cause") || !strings.Contains(text, "missing hash") {
		t.Fatalf("tree text missing evidence:\n%s", text)
	}
}

func TestBundleHistoryAggregate(t *testing.T) {
	trend := BundleTrendReport{Status: "fail", Rows: []BundleSeriesRow{{Name: "a", Status: "pass", Phase: "opensbi", TopStopCause: "none"}, {Name: "b", Status: "fail", Phase: "linux", TopStopCause: "panic"}}, Diffs: []BundleSeriesDiff{{From: "a", To: "b", Area: "artifact", Key: "payload", Kind: "changed", Before: "aaa", After: "bbb"}}}
	agg := BuildBundleHistoryAggregate(trend)
	if agg.Runs != 2 || agg.Failures != 1 || agg.PhaseCounts["linux"] != 1 || len(agg.ArtifactDrift) != 1 {
		t.Fatalf("bad aggregate: %#v", agg)
	}
}

func TestMinimalReproPackageFiles(t *testing.T) {
	bundle := DiagnosticBundle{Manifest: ArtifactManifest{BootArgs: "console=ttyS0", HartCount: 1, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 4, SHA256: strings.Repeat("a", 64)}}}}
	runner := BuildHeadlessSmokeRunnerSpec(bundle, []string{"uart-blk"}, 123)
	files := MinimalReproPackageFiles(bundle, "", CISummary{Status: "pass"}, CIGateReport{Status: "pass"}, runner, DefaultCIGatePolicy(), PolicyViolationTree{Status: "pass"}, BundleHistoryAggregate{Status: "ok"})
	for _, path := range []string{"README.md", "diagnostic-bundle.json", "manifest.json", "runner-spec.json", "scripts/rvsmoke.sh"} {
		if strings.TrimSpace(files[path]) == "" {
			t.Fatalf("missing repro file %s", path)
		}
	}
	if !strings.Contains(files["README.md"], "rvwasm-minimal-repro") {
		t.Fatalf("unexpected README:\n%s", files["README.md"])
	}
}
