package analyze

import (
	"strings"
	"testing"
	"time"
)

func sampleReleaseAuditForExt8() ReleaseAuditReport {
	policy := DefaultReleaseVerificationGatePolicy()
	ret := RetentionManifest{SchemaVersion: "rvwasm.retention.v1", Status: "ok", Entries: []RetentionEntry{{Path: "rvwasm-ci.html", Kind: "html", RetainDays: 1, ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)}}}
	release := ReleaseBundleManifest{SchemaVersion: "rvwasm.release.v1", Status: "warn", TopStopCause: "virtqueue anomaly"}
	attv := AttestationVerification{Status: "fail", Missing: []string{"material:artifact-manifest"}}
	sbom := DependencyInventoryDiff{Status: "warn", AddedModules: []string{"example.com/new v1.2.3"}}
	zipCmp := ReleaseHandoffPackageComparison{Status: "fail", Missing: []string{"release.html"}}
	return BuildReleaseAuditReport(release, attv, sbom, zipCmp, ret, MatrixFlakeReport{Status: "pass"}, policy, time.Now().UTC().Format(time.RFC3339))
}

func TestReleaseAuditDiffWaiversTodos(t *testing.T) {
	a := sampleReleaseAuditForExt8()
	base := a
	base.Status = "pass"
	base.Score.Score = 100
	d := BuildReleaseAuditDiff(a, ReleaseAuditReportJSON(base))
	if d.Status == "pass" || d.ScoreDelta >= 0 || len(d.Items) == 0 {
		t.Fatalf("expected degraded diff, got %#v", d)
	}
	waivers := `{"schema_version":"rvwasm.release-waivers.v1","rules":[{"id":"waive-release-zip","kind":"release_zip","contains":"release.html","reason":"known fixture","expires_at":"2099-01-01T00:00:00Z"}]}`
	wr := BuildReleaseWaiverReport(a, waivers, "2026-01-01T00:00:00Z")
	if wr.Waived == 0 || wr.Unwaived == 0 {
		t.Fatalf("expected both waived and unwaived issues, got %#v", wr)
	}
	todo := BuildReleaseAuditTodoReport(a, wr)
	if todo.Count == 0 || !strings.Contains(ReleaseAuditTodoReportMarkdown(todo), "Release audit TODO") {
		t.Fatalf("missing TODO output: %#v", todo)
	}
	if !strings.Contains(ReleaseAuditExtendedHTML(a, d, wr, todo), "Waivers") {
		t.Fatalf("extended HTML missing section")
	}
}

func TestReleaseWaiverTemplate(t *testing.T) {
	text := ReleaseWaiverTemplateJSON()
	if !strings.Contains(text, "rvwasm.release-waivers.v1") {
		t.Fatalf("bad template: %s", text)
	}
	if _, err := ParseReleaseWaiverSet(text); err != nil {
		t.Fatal(err)
	}
}
