package analyze

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWaiverExpiryCalendar(t *testing.T) {
	waivers := `{"schema_version":"rvwasm.release-waivers.v1","rules":[{"id":"soon","kind":"gate","status":"warn","expires_at":"2026-06-05T00:00:00Z"},{"id":"bad","expires_at":"not-a-date"}]}`
	report := ReleaseWaiverReport{Matches: []ReleaseWaiverMatch{{IssueID: "gate:x:warn", WaiverID: "soon", Status: "active"}}}
	cal := BuildWaiverExpiryCalendar(waivers, report, "2026-06-01T00:00:00Z", 7)
	if cal.Status != "fail" || cal.ExpiringSoon != 1 || cal.Invalid != 1 || cal.Total != 2 {
		t.Fatalf("unexpected calendar: %+v", cal)
	}
	if !strings.Contains(WaiverExpiryCalendarString(cal), "soon") {
		t.Fatalf("calendar text missing waiver id")
	}
}

func TestFinalReleaseDecisionNoGo(t *testing.T) {
	a := ReleaseAuditReport{Status: "warn", Score: ReleaseVerificationScore{Score: 75, Status: "warn"}, Gate: ReleaseVerificationGateReport{Status: "pass"}, Release: ReleaseBundleManifest{Status: "pass"}}
	w := ReleaseWaiverReport{Unwaived: 1}
	d := BuildFinalReleaseDecision(a, w, ReleaseAuditTodoReport{Count: 1}, WaiverExpiryCalendar{}, DefaultReleaseVerificationGatePolicy())
	if d.Decision != "no-go" || len(d.Blocking) == 0 {
		t.Fatalf("expected no-go decision: %+v", d)
	}
}

func TestReleaseEvidenceBundleInspection(t *testing.T) {
	a := ReleaseAuditReport{SchemaVersion: "rvwasm.release-audit.v1", Status: "pass", Score: ReleaseVerificationScore{Score: 100, Status: "pass"}, Gate: ReleaseVerificationGateReport{Status: "pass"}}
	cal := WaiverExpiryCalendar{SchemaVersion: "rvwasm.waiver-calendar.v1", Status: "pass", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	decision := FinalReleaseDecision{SchemaVersion: "rvwasm.final-decision.v1", Decision: "go", Status: "pass", Score: 100}
	files := ReleaseEvidenceBundleFiles(a, ReleaseAuditDiffReport{Status: "pass"}, ReleaseWaiverReport{Status: "pass"}, ReleaseAuditTodoReport{Status: "pass"}, cal, ReleaseAuditChangelog{Status: "pass"}, decision)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, text := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(text))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	insp, err := InspectReleaseEvidenceBundleZipBytes(buf.Bytes())
	if err != nil || insp.Status != "pass" || insp.RequiredFound != insp.RequiredTotal {
		t.Fatalf("bad inspection err=%v insp=%+v", err, insp)
	}
}
