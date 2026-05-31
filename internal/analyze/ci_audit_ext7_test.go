package analyze

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRetentionAuditDetectsExpiredAndBelowMinimum(t *testing.T) {
	ret := RetentionManifest{SchemaVersion: "rvwasm.retention.v1", Entries: []RetentionEntry{
		{Path: "old.html", Kind: "html", RetainDays: 5, ExpiresAt: "2026-01-01T00:00:00Z"},
		{Path: "soon.json", Kind: "json", RetainDays: 30, ExpiresAt: "2026-01-05T00:00:00Z"},
	}}
	audit := BuildRetentionAudit(ret, "2026-01-03T00:00:00Z", 7, 3)
	if audit.Status != "fail" || audit.Expired != 1 || audit.ExpiringSoon != 1 || audit.BelowMinimum != 1 {
		t.Fatalf("unexpected audit: %+v", audit)
	}
}

func TestReleaseVerificationGatePolicyTemplatesAndReport(t *testing.T) {
	tmpl, ok := ReleaseVerificationGatePolicyTemplateByName("strict")
	if !ok || !tmpl.Policy.RequireReleasePass {
		t.Fatalf("strict template not found or not strict: %+v", tmpl)
	}
	release := ReleaseBundleManifest{Status: "warn", TopStopCause: "virtqueue anomaly"}
	attv := AttestationVerification{Status: "pass"}
	sbom := DependencyInventoryDiff{Status: "pass"}
	zipCmp := ReleaseHandoffPackageComparison{Status: "pass"}
	ret := RetentionAudit{Status: "pass"}
	score := BuildReleaseVerificationScore(release, attv, sbom, zipCmp, ret, MatrixFlakeReport{Status: "pass"})
	gate := BuildReleaseVerificationGateReport(release, attv, sbom, zipCmp, ret, score, tmpl.Policy)
	if gate.Status != "fail" {
		t.Fatalf("strict gate should fail non-pass release: %+v", gate)
	}
	js := ReleaseVerificationGatePolicyTemplateJSON("default")
	var p ReleaseVerificationGatePolicy
	if err := json.Unmarshal([]byte(js), &p); err != nil || p.Name == "" {
		t.Fatalf("bad policy json: %v %s", err, js)
	}
}

func TestReleaseVerificationScoreAndAuditHTML(t *testing.T) {
	release := ReleaseBundleManifest{Status: "pass", TopStopCause: "none"}
	attv := AttestationVerification{Status: "warn", Warnings: []string{"no artifacts"}}
	sbom := DependencyInventoryDiff{Status: "pass"}
	zipCmp := ReleaseHandoffPackageComparison{Status: "pass"}
	ret := RetentionManifest{SchemaVersion: "rvwasm.retention.v1", Entries: []RetentionEntry{{Path: "report.html", Kind: "html", RetainDays: 60, ExpiresAt: "2026-02-01T00:00:00Z"}}}
	policy := DefaultReleaseVerificationGatePolicy()
	audit := BuildReleaseAuditReport(release, attv, sbom, zipCmp, ret, MatrixFlakeReport{Status: "pass"}, policy, "2026-01-01T00:00:00Z")
	if audit.Score.Score <= 0 || audit.Score.Score > 100 {
		t.Fatalf("bad score: %+v", audit.Score)
	}
	if audit.Status == "fail" {
		t.Fatalf("audit unexpectedly failed: %+v", audit)
	}
	html := ReleaseAuditReportHTML(audit)
	if !strings.Contains(html, "rvwasm release audit") || !strings.Contains(html, "report.html") {
		t.Fatalf("html missing expected content: %s", html)
	}
}
