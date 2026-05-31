package analyze

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ReleaseVerificationGatePolicy describes final release-audit rules.  It is
// intentionally separate from the boot CI policy because it gates metadata and
// handoff quality rather than guest execution.
type ReleaseVerificationGatePolicy struct {
	Name                     string   `json:"name"`
	RequireReleasePass       bool     `json:"require_release_pass"`
	FailOnAttestationFail    bool     `json:"fail_on_attestation_fail"`
	FailOnSBOMFail           bool     `json:"fail_on_sbom_fail"`
	FailOnReleaseZipFail     bool     `json:"fail_on_release_zip_fail"`
	FailOnRetentionExpired   bool     `json:"fail_on_retention_expired"`
	MaxAttestationWarnings   int      `json:"max_attestation_warnings"`
	MaxSBOMWarnings          int      `json:"max_sbom_warnings"`
	MaxReleaseZipExtras      int      `json:"max_release_zip_extras"`
	MaxRetentionWarnings     int      `json:"max_retention_warnings"`
	MinArtifactRetentionDays int      `json:"min_artifact_retention_days"`
	ExpiringSoonDays         int      `json:"expiring_soon_days"`
	FailOnStatuses           []string `json:"fail_on_statuses,omitempty"`
}

type ReleaseVerificationGateCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Observed string `json:"observed,omitempty"`
	Expected string `json:"expected,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type ReleaseVerificationGateReport struct {
	PolicyName string                         `json:"policy_name"`
	Status     string                         `json:"status"`
	ExitCode   int                            `json:"exit_code"`
	Checks     []ReleaseVerificationGateCheck `json:"checks,omitempty"`
	Summary    []string                       `json:"summary,omitempty"`
}

type ReleaseVerificationPolicyTemplate struct {
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	Policy      ReleaseVerificationGatePolicy `json:"policy"`
}

func DefaultReleaseVerificationGatePolicy() ReleaseVerificationGatePolicy {
	return ReleaseVerificationGatePolicy{
		Name:                     "release-default",
		RequireReleasePass:       false,
		FailOnAttestationFail:    true,
		FailOnSBOMFail:           true,
		FailOnReleaseZipFail:     true,
		FailOnRetentionExpired:   true,
		MaxAttestationWarnings:   2,
		MaxSBOMWarnings:          -1,
		MaxReleaseZipExtras:      3,
		MaxRetentionWarnings:     -1,
		MinArtifactRetentionDays: 7,
		ExpiringSoonDays:         14,
		FailOnStatuses:           []string{"fail", "error"},
	}
}

func ReleaseVerificationGatePolicyTemplates() []ReleaseVerificationPolicyTemplate {
	def := DefaultReleaseVerificationGatePolicy()
	strict := def
	strict.Name = "release-strict"
	strict.RequireReleasePass = true
	strict.MaxAttestationWarnings = 0
	strict.MaxSBOMWarnings = 0
	strict.MaxReleaseZipExtras = 0
	strict.MaxRetentionWarnings = 0
	strict.MinArtifactRetentionDays = 30
	strict.ExpiringSoonDays = 30
	lenient := def
	lenient.Name = "release-lenient"
	lenient.RequireReleasePass = false
	lenient.FailOnSBOMFail = false
	lenient.FailOnRetentionExpired = false
	lenient.MaxAttestationWarnings = -1
	lenient.MaxReleaseZipExtras = -1
	lenient.MinArtifactRetentionDays = 0
	lenient.ExpiringSoonDays = 7
	archive := def
	archive.Name = "release-archive"
	archive.RequireReleasePass = false
	archive.MaxReleaseZipExtras = -1
	archive.MinArtifactRetentionDays = 90
	archive.ExpiringSoonDays = 45
	return []ReleaseVerificationPolicyTemplate{
		{Name: "default", Description: "balanced release metadata gate for routine rvwasm handoff", Policy: def},
		{Name: "strict", Description: "strict release gate for promoted artifacts", Policy: strict},
		{Name: "lenient", Description: "metadata collection mode that avoids failing on SBOM/retention drift", Policy: lenient},
		{Name: "archive", Description: "long-retention release archive gate", Policy: archive},
	}
}

func ReleaseVerificationGatePolicyTemplateByName(name string) (ReleaseVerificationPolicyTemplate, bool) {
	n := strings.TrimSpace(strings.ToLower(name))
	if n == "" {
		n = "default"
	}
	for _, t := range ReleaseVerificationGatePolicyTemplates() {
		if strings.EqualFold(t.Name, n) || strings.EqualFold(t.Policy.Name, n) {
			return t, true
		}
	}
	return ReleaseVerificationPolicyTemplate{}, false
}

func ReleaseVerificationGatePolicyTemplateListString() string {
	var b strings.Builder
	for _, t := range ReleaseVerificationGatePolicyTemplates() {
		fmt.Fprintf(&b, "%s\t%s\n", t.Name, t.Description)
	}
	return b.String()
}

func ReleaseVerificationGatePolicyTemplateJSON(name string) string {
	t, ok := ReleaseVerificationGatePolicyTemplateByName(name)
	if !ok {
		t, _ = ReleaseVerificationGatePolicyTemplateByName("default")
	}
	b, _ := json.MarshalIndent(t.Policy, "", "  ")
	return string(b)
}

func ParseReleaseVerificationGatePolicyJSON(text string, def ReleaseVerificationGatePolicy) (ReleaseVerificationGatePolicy, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return def, nil
	}
	p := def
	if err := json.Unmarshal([]byte(text), &p); err != nil {
		return def, err
	}
	return p, nil
}

func BuildReleaseVerificationGateReport(release ReleaseBundleManifest, attv AttestationVerification, sbomDiff DependencyInventoryDiff, zipCmp ReleaseHandoffPackageComparison, retentionAudit RetentionAudit, score ReleaseVerificationScore, policy ReleaseVerificationGatePolicy) ReleaseVerificationGateReport {
	r := ReleaseVerificationGateReport{PolicyName: firstNonEmpty(policy.Name, "release-custom"), Status: "pass"}
	add := func(status, name, observed, expected, detail string) {
		if status == "" {
			status = "pass"
		}
		r.Checks = append(r.Checks, ReleaseVerificationGateCheck{Name: name, Status: status, Observed: observed, Expected: expected, Detail: detail})
		if status == "fail" {
			r.Status = "fail"
		} else if status == "warn" && r.Status == "pass" {
			r.Status = "warn"
		}
	}
	statusSet := map[string]bool{}
	for _, s := range policy.FailOnStatuses {
		statusSet[strings.ToLower(strings.TrimSpace(s))] = true
	}
	if policy.RequireReleasePass && !strings.EqualFold(release.Status, "pass") {
		add("fail", "release-status", release.Status, "pass", "release manifest status must be pass")
	} else if statusSet[strings.ToLower(release.Status)] {
		add("fail", "release-status", release.Status, "not in fail_on_statuses", "release status is gated")
	} else {
		add("pass", "release-status", firstNonEmpty(release.Status, "empty"), "allowed", "")
	}
	if policy.FailOnAttestationFail && attv.Status == "fail" {
		add("fail", "attestation", attv.Status, "not fail", "provenance attestation verification failed")
	} else if attv.Status == "warn" {
		add("warn", "attestation", attv.Status, "pass", "attestation verification warnings present")
	} else {
		add("pass", "attestation", firstNonEmpty(attv.Status, "empty"), "pass", "")
	}
	if policy.MaxAttestationWarnings >= 0 {
		w := len(attv.Warnings)
		if w > policy.MaxAttestationWarnings {
			add("fail", "attestation-warnings", strconv.Itoa(w), "<= "+strconv.Itoa(policy.MaxAttestationWarnings), "too many attestation warnings")
		} else {
			add("pass", "attestation-warnings", strconv.Itoa(w), "<= "+strconv.Itoa(policy.MaxAttestationWarnings), "")
		}
	}
	if policy.FailOnSBOMFail && sbomDiff.Status == "fail" {
		add("fail", "sbom-diff", sbomDiff.Status, "not fail", "dependency inventory diff failed")
	} else if sbomDiff.Status == "warn" {
		add("warn", "sbom-diff", sbomDiff.Status, "pass", "dependency inventory drift detected")
	} else {
		add("pass", "sbom-diff", firstNonEmpty(sbomDiff.Status, "empty"), "pass", "")
	}
	if policy.MaxSBOMWarnings >= 0 {
		w := len(sbomDiff.Issues) + len(sbomDiff.AddedModules) + len(sbomDiff.ChangedModules) + len(sbomDiff.ArtifactKindChange)
		if w > policy.MaxSBOMWarnings {
			add("fail", "sbom-warning-count", strconv.Itoa(w), "<= "+strconv.Itoa(policy.MaxSBOMWarnings), "too many SBOM diff warnings")
		} else {
			add("pass", "sbom-warning-count", strconv.Itoa(w), "<= "+strconv.Itoa(policy.MaxSBOMWarnings), "")
		}
	}
	if policy.FailOnReleaseZipFail && zipCmp.Status == "fail" {
		add("fail", "release-zip-compare", zipCmp.Status, "not fail", "release handoff ZIP comparison failed")
	} else if zipCmp.Status == "warn" {
		add("warn", "release-zip-compare", zipCmp.Status, "pass", "release handoff ZIP drift detected")
	} else {
		add("pass", "release-zip-compare", firstNonEmpty(zipCmp.Status, "empty"), "pass", "")
	}
	if policy.MaxReleaseZipExtras >= 0 {
		if len(zipCmp.Extra) > policy.MaxReleaseZipExtras {
			add("fail", "release-zip-extra-files", strconv.Itoa(len(zipCmp.Extra)), "<= "+strconv.Itoa(policy.MaxReleaseZipExtras), "too many extra files in release handoff ZIP")
		} else {
			add("pass", "release-zip-extra-files", strconv.Itoa(len(zipCmp.Extra)), "<= "+strconv.Itoa(policy.MaxReleaseZipExtras), "")
		}
	}
	if policy.FailOnRetentionExpired && retentionAudit.Expired != 0 {
		add("fail", "retention-expired", strconv.Itoa(retentionAudit.Expired), "0", "retention manifest contains expired artifacts")
	} else if retentionAudit.ExpiringSoon != 0 {
		add("warn", "retention-expiring-soon", strconv.Itoa(retentionAudit.ExpiringSoon), "0", "artifacts expire soon")
	} else {
		add("pass", "retention-expiry", "0", "0", "")
	}
	if policy.MaxRetentionWarnings >= 0 {
		w := len(retentionAudit.Warnings)
		if w > policy.MaxRetentionWarnings {
			add("fail", "retention-warnings", strconv.Itoa(w), "<= "+strconv.Itoa(policy.MaxRetentionWarnings), "too many retention warnings")
		} else {
			add("pass", "retention-warnings", strconv.Itoa(w), "<= "+strconv.Itoa(policy.MaxRetentionWarnings), "")
		}
	}
	if policy.MinArtifactRetentionDays > 0 && retentionAudit.BelowMinimum != 0 {
		add("fail", "retention-min-days", strconv.Itoa(retentionAudit.BelowMinimum), "0 below "+strconv.Itoa(policy.MinArtifactRetentionDays)+"d", "artifacts do not meet minimum retention")
	} else {
		add("pass", "retention-min-days", strconv.Itoa(retentionAudit.BelowMinimum), "0", "")
	}
	if score.Status == "fail" {
		add("fail", "release-score", strconv.Itoa(score.Score), ">= 60", "overall verification score failed")
	} else if score.Status == "warn" {
		add("warn", "release-score", strconv.Itoa(score.Score), ">= 80", "overall verification score is degraded")
	} else {
		add("pass", "release-score", strconv.Itoa(score.Score), ">= 80", "")
	}
	if r.Status == "fail" {
		r.ExitCode = 1
	}
	for _, c := range r.Checks {
		if c.Status == "fail" || c.Status == "warn" {
			r.Summary = append(r.Summary, fmt.Sprintf("%s: %s (%s)", c.Status, c.Name, c.Detail))
		}
	}
	sort.Strings(r.Summary)
	return r
}

func ReleaseVerificationGateReportJSON(r ReleaseVerificationGateReport) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
func ReleaseVerificationGateReportString(r ReleaseVerificationGateReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release verification gate policy=%s status=%s checks=%d\n", r.PolicyName, r.Status, len(r.Checks))
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "  %-24s %-5s observed=%s expected=%s %s\n", c.Name, c.Status, firstNonEmpty(c.Observed, "-"), firstNonEmpty(c.Expected, "-"), c.Detail)
	}
	return b.String()
}

type RetentionAuditEntry struct {
	Path          string `json:"path"`
	Kind          string `json:"kind,omitempty"`
	Status        string `json:"status"`
	RetainDays    int    `json:"retain_days"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	DaysRemaining int    `json:"days_remaining,omitempty"`
	Detail        string `json:"detail,omitempty"`
}
type RetentionAudit struct {
	Status        string                `json:"status"`
	CheckedAt     string                `json:"checked_at"`
	EntryCount    int                   `json:"entry_count"`
	Expired       int                   `json:"expired"`
	ExpiringSoon  int                   `json:"expiring_soon"`
	BelowMinimum  int                   `json:"below_minimum"`
	MissingExpiry int                   `json:"missing_expiry"`
	Warnings      []string              `json:"warnings,omitempty"`
	Entries       []RetentionAuditEntry `json:"entries,omitempty"`
}

func BuildRetentionAudit(ret RetentionManifest, checkedAt string, minDays, soonDays int) RetentionAudit {
	if strings.TrimSpace(checkedAt) == "" {
		checkedAt = time.Now().UTC().Format(time.RFC3339)
	}
	now, err := time.Parse(time.RFC3339, checkedAt)
	if err != nil {
		now = time.Now().UTC()
		checkedAt = now.Format(time.RFC3339)
	}
	if soonDays < 0 {
		soonDays = 0
	}
	a := RetentionAudit{Status: "pass", CheckedAt: checkedAt, EntryCount: len(ret.Entries)}
	if ret.SchemaVersion == "" {
		a.Status = "warn"
		a.Warnings = append(a.Warnings, "retention manifest is empty")
	}
	for _, e := range ret.Entries {
		row := RetentionAuditEntry{Path: e.Path, Kind: e.Kind, Status: "pass", RetainDays: e.RetainDays, ExpiresAt: e.ExpiresAt}
		if minDays > 0 && e.RetainDays < minDays {
			row.Status = "fail"
			row.Detail = "retention below minimum"
			a.BelowMinimum++
		}
		if strings.TrimSpace(e.ExpiresAt) == "" {
			if row.Status == "pass" {
				row.Status = "warn"
				row.Detail = "missing expiry"
			}
			a.MissingExpiry++
		} else if t, err := time.Parse(time.RFC3339, e.ExpiresAt); err == nil {
			rem := int(t.Sub(now).Hours() / 24)
			row.DaysRemaining = rem
			if t.Before(now) || t.Equal(now) {
				row.Status = "fail"
				row.Detail = "expired"
				a.Expired++
			} else if soonDays > 0 && rem <= soonDays {
				if row.Status == "pass" {
					row.Status = "warn"
					row.Detail = "expiring soon"
				}
				a.ExpiringSoon++
			}
		} else {
			if row.Status == "pass" {
				row.Status = "warn"
				row.Detail = "bad expiry timestamp"
			}
			a.Warnings = append(a.Warnings, "bad expiry for "+e.Path+": "+err.Error())
		}
		if row.Status == "fail" {
			a.Status = "fail"
		} else if row.Status == "warn" && a.Status == "pass" {
			a.Status = "warn"
		}
		a.Entries = append(a.Entries, row)
	}
	sort.Slice(a.Entries, func(i, j int) bool { return a.Entries[i].Path < a.Entries[j].Path })
	sort.Strings(a.Warnings)
	return a
}
func RetentionAuditJSON(a RetentionAudit) string {
	b, _ := json.MarshalIndent(a, "", "  ")
	return string(b)
}
func RetentionAuditString(a RetentionAudit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "retention audit status=%s entries=%d expired=%d expiring_soon=%d below_minimum=%d missing_expiry=%d checked_at=%s\n", a.Status, a.EntryCount, a.Expired, a.ExpiringSoon, a.BelowMinimum, a.MissingExpiry, a.CheckedAt)
	for _, w := range a.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	for _, e := range a.Entries {
		if e.Status != "pass" {
			fmt.Fprintf(&b, "  %-34s %-5s retain=%dd days_remaining=%d %s\n", e.Path, e.Status, e.RetainDays, e.DaysRemaining, e.Detail)
		}
	}
	return b.String()
}

type ReleaseScoreComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Weight int    `json:"weight"`
	Points int    `json:"points"`
	Detail string `json:"detail,omitempty"`
}
type ReleaseVerificationScore struct {
	Status     string                  `json:"status"`
	Score      int                     `json:"score"`
	MaxScore   int                     `json:"max_score"`
	Components []ReleaseScoreComponent `json:"components,omitempty"`
	Summary    []string                `json:"summary,omitempty"`
}

func BuildReleaseVerificationScore(release ReleaseBundleManifest, attv AttestationVerification, sbomDiff DependencyInventoryDiff, zipCmp ReleaseHandoffPackageComparison, retentionAudit RetentionAudit, flakes MatrixFlakeReport) ReleaseVerificationScore {
	s := ReleaseVerificationScore{Status: "pass"}
	add := func(name, status string, weight int, detail string) {
		points := weight
		switch strings.ToLower(status) {
		case "fail", "error":
			points = 0
		case "warn":
			points = weight / 2
		case "empty":
			points = weight / 2
		}
		s.Components = append(s.Components, ReleaseScoreComponent{Name: name, Status: firstNonEmpty(status, "empty"), Weight: weight, Points: points, Detail: detail})
		s.Score += points
		s.MaxScore += weight
		if points < weight {
			s.Summary = append(s.Summary, fmt.Sprintf("%s=%s", name, firstNonEmpty(status, "empty")))
		}
	}
	add("release", release.Status, 25, release.TopStopCause)
	add("attestation", attv.Status, 20, strings.Join(append(append([]string{}, attv.Missing...), attv.Mismatch...), "; "))
	add("sbom", sbomDiff.Status, 15, strings.Join(append(append([]string{}, sbomDiff.AddedModules...), sbomDiff.ChangedModules...), "; "))
	add("release_zip", zipCmp.Status, 15, fmt.Sprintf("missing=%d changed=%d extra=%d", len(zipCmp.Missing), len(zipCmp.Changed), len(zipCmp.Extra)))
	add("retention", retentionAudit.Status, 15, fmt.Sprintf("expired=%d expiring=%d below_min=%d", retentionAudit.Expired, retentionAudit.ExpiringSoon, retentionAudit.BelowMinimum))
	add("matrix_flakes", flakes.Status, 10, fmt.Sprintf("flakes=%d", flakes.Flakes))
	if s.MaxScore > 0 {
		s.Score = int((float64(s.Score)/float64(s.MaxScore))*100 + 0.5)
		s.MaxScore = 100
	}
	switch {
	case s.Score < 60:
		s.Status = "fail"
	case s.Score < 85 || len(s.Summary) != 0:
		s.Status = "warn"
	default:
		s.Status = "pass"
	}
	sort.Strings(s.Summary)
	return s
}
func ReleaseVerificationScoreJSON(s ReleaseVerificationScore) string {
	b, _ := json.MarshalIndent(s, "", "  ")
	return string(b)
}
func ReleaseVerificationScoreString(s ReleaseVerificationScore) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release verification score status=%s score=%d/%d components=%d\n", s.Status, s.Score, s.MaxScore, len(s.Components))
	for _, c := range s.Components {
		fmt.Fprintf(&b, "  %-16s %-5s %d/%d %s\n", c.Name, c.Status, c.Points, c.Weight, c.Detail)
	}
	return b.String()
}

type ReleaseAuditReport struct {
	SchemaVersion     string                          `json:"schema_version"`
	Status            string                          `json:"status"`
	Score             ReleaseVerificationScore        `json:"score"`
	Gate              ReleaseVerificationGateReport   `json:"gate"`
	Retention         RetentionAudit                  `json:"retention_audit"`
	Attestation       AttestationVerification         `json:"attestation_verification"`
	SBOMDiff          DependencyInventoryDiff         `json:"sbom_diff"`
	ReleaseZipCompare ReleaseHandoffPackageComparison `json:"release_zip_compare"`
	Release           ReleaseBundleManifest           `json:"release_manifest"`
}

func BuildReleaseAuditReport(release ReleaseBundleManifest, attv AttestationVerification, sbomDiff DependencyInventoryDiff, zipCmp ReleaseHandoffPackageComparison, retention RetentionManifest, flakes MatrixFlakeReport, policy ReleaseVerificationGatePolicy, checkedAt string) ReleaseAuditReport {
	audit := BuildRetentionAudit(retention, checkedAt, policy.MinArtifactRetentionDays, policy.ExpiringSoonDays)
	score := BuildReleaseVerificationScore(release, attv, sbomDiff, zipCmp, audit, flakes)
	gate := BuildReleaseVerificationGateReport(release, attv, sbomDiff, zipCmp, audit, score, policy)
	status := gate.Status
	if status == "pass" {
		status = score.Status
	}
	return ReleaseAuditReport{SchemaVersion: "rvwasm.release-audit.v1", Status: status, Score: score, Gate: gate, Retention: audit, Attestation: attv, SBOMDiff: sbomDiff, ReleaseZipCompare: zipCmp, Release: release}
}
func ReleaseAuditReportJSON(r ReleaseAuditReport) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
func ReleaseAuditReportString(r ReleaseAuditReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release audit status=%s score=%d gate=%s\n", r.Status, r.Score.Score, r.Gate.Status)
	b.WriteString(ReleaseVerificationGateReportString(r.Gate))
	b.WriteString("\n")
	b.WriteString(ReleaseVerificationScoreString(r.Score))
	b.WriteString("\n")
	b.WriteString(RetentionAuditString(r.Retention))
	return b.String()
}
func ReleaseAuditReportHTML(r ReleaseAuditReport) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm release audit</title><style>body{font-family:system-ui,sans-serif;margin:2rem;line-height:1.45}nav{position:sticky;top:0;background:white;border-bottom:1px solid #ddd;padding:.5rem 0}nav a{margin-right:1rem}.fail{background:#ffe6e6}.warn{background:#fff6cc}.pass,.ok{background:#e8ffe8}pre{background:#f7f7f7;padding:1rem;overflow:auto}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:.35rem .5rem}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	b.WriteString("<nav><a href=\"#summary\">Summary</a><a href=\"#gate\">Gate</a><a href=\"#score\">Score</a><a href=\"#retention\">Retention</a><a href=\"#json\">JSON</a></nav>")
	fmt.Fprintf(&b, "<h1 id=\"summary\">rvwasm release audit</h1><p>Status <code class=%q>%s</code>; score <strong>%d/100</strong>; release <code>%s</code>; top stop-cause <code>%s</code></p>", html.EscapeString(r.Status), html.EscapeString(r.Status), r.Score.Score, html.EscapeString(r.Release.Status), html.EscapeString(r.Release.TopStopCause))
	b.WriteString("<h2 id=\"gate\">Verification gate</h2><table><tr><th>Check</th><th>Status</th><th>Observed</th><th>Expected</th><th>Detail</th></tr>")
	for _, c := range r.Gate.Checks {
		fmt.Fprintf(&b, "<tr class=%q><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>", html.EscapeString(c.Status), html.EscapeString(c.Name), html.EscapeString(c.Status), html.EscapeString(c.Observed), html.EscapeString(c.Expected), html.EscapeString(c.Detail))
	}
	b.WriteString("</table><h2 id=\"score\">Score components</h2><table><tr><th>Component</th><th>Status</th><th>Points</th><th>Weight</th><th>Detail</th></tr>")
	for _, c := range r.Score.Components {
		fmt.Fprintf(&b, "<tr class=%q><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%s</td></tr>", html.EscapeString(c.Status), html.EscapeString(c.Name), html.EscapeString(c.Status), c.Points, c.Weight, html.EscapeString(c.Detail))
	}
	b.WriteString("</table><h2 id=\"retention\">Retention audit</h2><pre>")
	b.WriteString(html.EscapeString(RetentionAuditString(r.Retention)))
	b.WriteString("</pre><h2 id=\"json\">JSON</h2><pre>")
	b.WriteString(html.EscapeString(ReleaseAuditReportJSON(r)))
	b.WriteString("</pre>")
	return b.String()
}
