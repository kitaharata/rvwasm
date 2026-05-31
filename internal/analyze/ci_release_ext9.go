package analyze

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"sort"
	"strings"
	"time"
)

type WaiverExpiryCalendarEntry struct {
	WaiverID      string `json:"waiver_id"`
	Kind          string `json:"kind,omitempty"`
	Name          string `json:"name,omitempty"`
	Status        string `json:"status"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	DaysRemaining int    `json:"days_remaining,omitempty"`
	Owner         string `json:"owner,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Matches       int    `json:"matches,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

type WaiverExpiryCalendar struct {
	SchemaVersion string                      `json:"schema_version"`
	Status        string                      `json:"status"`
	CheckedAt     string                      `json:"checked_at"`
	Total         int                         `json:"total"`
	Active        int                         `json:"active"`
	ExpiringSoon  int                         `json:"expiring_soon"`
	Expired       int                         `json:"expired"`
	Invalid       int                         `json:"invalid"`
	Entries       []WaiverExpiryCalendarEntry `json:"entries,omitempty"`
	Issues        []string                    `json:"issues,omitempty"`
}

func BuildWaiverExpiryCalendar(waiverText string, waiverReport ReleaseWaiverReport, checkedAt string, expiringSoonDays int) WaiverExpiryCalendar {
	if checkedAt == "" {
		checkedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if expiringSoonDays <= 0 {
		expiringSoonDays = 14
	}
	now, _ := time.Parse(time.RFC3339, checkedAt)
	set, err := ParseReleaseWaiverSet(waiverText)
	cal := WaiverExpiryCalendar{SchemaVersion: "rvwasm.waiver-calendar.v1", Status: "pass", CheckedAt: checkedAt}
	if err != nil {
		cal.Status = "fail"
		cal.Issues = append(cal.Issues, "cannot parse waiver rules: "+err.Error())
		return cal
	}
	matchCount := map[string]int{}
	for _, m := range waiverReport.Matches {
		matchCount[m.WaiverID]++
	}
	for _, r := range set.Rules {
		entry := WaiverExpiryCalendarEntry{WaiverID: firstNonEmpty(r.ID, "<unnamed>"), Kind: r.Kind, Name: r.Name, Owner: r.Owner, Reason: r.Reason, ExpiresAt: r.ExpiresAt, Matches: matchCount[r.ID], Status: "active"}
		if r.ID == "" {
			entry.Status = "invalid"
			entry.Detail = "waiver id is required"
			cal.Invalid++
		} else if strings.TrimSpace(r.ExpiresAt) == "" {
			entry.Status = "invalid"
			entry.Detail = "expires_at is required for auditable waivers"
			cal.Invalid++
		} else if t, err := time.Parse(time.RFC3339, r.ExpiresAt); err != nil {
			entry.Status = "invalid"
			entry.Detail = "bad expires_at: " + err.Error()
			cal.Invalid++
		} else if !now.IsZero() {
			days := int(t.Sub(now).Hours() / 24)
			entry.DaysRemaining = days
			if t.Before(now) || t.Equal(now) {
				entry.Status = "expired"
				entry.Detail = "waiver has expired"
				cal.Expired++
			} else if days <= expiringSoonDays {
				entry.Status = "expiring-soon"
				entry.Detail = fmt.Sprintf("expires within %d days", expiringSoonDays)
				cal.ExpiringSoon++
			} else {
				cal.Active++
			}
		}
		cal.Entries = append(cal.Entries, entry)
	}
	sort.Slice(cal.Entries, func(i, j int) bool {
		a, b := cal.Entries[i], cal.Entries[j]
		if a.Status != b.Status {
			return waiverCalendarRank(a.Status) > waiverCalendarRank(b.Status)
		}
		return a.ExpiresAt < b.ExpiresAt
	})
	cal.Total = len(cal.Entries)
	if cal.Expired != 0 || cal.Invalid != 0 {
		cal.Status = "fail"
	} else if cal.ExpiringSoon != 0 {
		cal.Status = "warn"
	}
	return cal
}

func waiverCalendarRank(s string) int {
	switch s {
	case "expired", "invalid":
		return 4
	case "expiring-soon":
		return 3
	case "active":
		return 1
	default:
		return 0
	}
}

func WaiverExpiryCalendarJSON(c WaiverExpiryCalendar) string {
	b, _ := json.MarshalIndent(c, "", "  ")
	return string(b)
}
func WaiverExpiryCalendarString(c WaiverExpiryCalendar) string {
	var b strings.Builder
	fmt.Fprintf(&b, "waiver expiry calendar status=%s total=%d active=%d expiring=%d expired=%d invalid=%d checked_at=%s\n", c.Status, c.Total, c.Active, c.ExpiringSoon, c.Expired, c.Invalid, c.CheckedAt)
	for _, e := range c.Entries {
		fmt.Fprintf(&b, "  %-14s %-28s expires=%-20s days=%4d matches=%d owner=%s %s\n", e.Status, e.WaiverID, e.ExpiresAt, e.DaysRemaining, e.Matches, e.Owner, e.Detail)
	}
	for _, is := range c.Issues {
		fmt.Fprintf(&b, "issue: %s\n", is)
	}
	return b.String()
}
func WaiverExpiryCalendarHTML(c WaiverExpiryCalendar) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm waiver calendar</title><style>body{font-family:system-ui,sans-serif;margin:2rem}.expired,.invalid,.fail{background:#ffe6e6}.expiring-soon,.warn{background:#fff6cc}.active,.pass{background:#e8ffe8}table{border-collapse:collapse;width:100%}td,th{border:1px solid #ccc;padding:.35rem .5rem}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	fmt.Fprintf(&b, "<h1>Waiver expiry calendar</h1><p>Status <code class=%q>%s</code>; checked at <code>%s</code></p>", html.EscapeString(c.Status), html.EscapeString(c.Status), html.EscapeString(c.CheckedAt))
	b.WriteString("<table><tr><th>Status</th><th>Waiver</th><th>Kind</th><th>Name</th><th>Expires</th><th>Days</th><th>Matches</th><th>Owner</th><th>Detail</th></tr>")
	for _, e := range c.Entries {
		fmt.Fprintf(&b, "<tr class=%q><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%s</td><td>%s</td></tr>", html.EscapeString(e.Status), html.EscapeString(e.Status), html.EscapeString(e.WaiverID), html.EscapeString(e.Kind), html.EscapeString(e.Name), html.EscapeString(e.ExpiresAt), e.DaysRemaining, e.Matches, html.EscapeString(e.Owner), html.EscapeString(e.Detail))
	}
	b.WriteString("</table><h2>JSON</h2><pre>")
	b.WriteString(html.EscapeString(WaiverExpiryCalendarJSON(c)))
	b.WriteString("</pre>")
	return b.String()
}

type ReleaseAuditChangelogEntry struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
}

type ReleaseAuditChangelog struct {
	SchemaVersion string                       `json:"schema_version"`
	Status        string                       `json:"status"`
	Entries       []ReleaseAuditChangelogEntry `json:"entries,omitempty"`
}

func BuildReleaseAuditChangelog(diff ReleaseAuditDiffReport, waivers ReleaseWaiverReport, todos ReleaseAuditTodoReport, calendar WaiverExpiryCalendar) ReleaseAuditChangelog {
	c := ReleaseAuditChangelog{SchemaVersion: "rvwasm.release-changelog.v1", Status: "pass"}
	add := func(cat, sev, title, detail string) {
		c.Entries = append(c.Entries, ReleaseAuditChangelogEntry{Category: cat, Severity: firstNonEmpty(sev, "info"), Title: title, Detail: detail})
		if sev == "fail" {
			c.Status = "fail"
		} else if sev == "warn" && c.Status == "pass" {
			c.Status = "warn"
		}
	}
	if diff.Status != "" {
		add("audit-diff", severityFromStatus(diff.Status), fmt.Sprintf("audit status %s→%s, score %+d", diff.BeforeStatus, diff.AfterStatus, diff.ScoreDelta), fmt.Sprintf("%d changed items", len(diff.Items)))
	}
	for _, it := range diff.Items {
		add("audit-diff", firstNonEmpty(it.Severity, "info"), it.Kind+":"+it.Name, strings.TrimSpace(it.Before+" -> "+it.After+" "+it.Detail))
	}
	if waivers.IssueCount != 0 || waivers.Waived != 0 || waivers.Unwaived != 0 {
		add("waivers", severityFromStatus(waivers.Status), fmt.Sprintf("waivers active=%d unwaived=%d expired=%d", waivers.Waived, waivers.Unwaived, waivers.Expired), "")
	}
	if todos.Count != 0 {
		add("todo", severityFromStatus(todos.Status), fmt.Sprintf("%d release TODO item(s)", todos.Count), "")
	}
	if calendar.Total != 0 {
		add("waiver-calendar", severityFromStatus(calendar.Status), fmt.Sprintf("waiver calendar active=%d expiring=%d expired=%d invalid=%d", calendar.Active, calendar.ExpiringSoon, calendar.Expired, calendar.Invalid), "")
	}
	sort.SliceStable(c.Entries, func(i, j int) bool {
		if severityRank(c.Entries[i].Severity) != severityRank(c.Entries[j].Severity) {
			return severityRank(c.Entries[i].Severity) > severityRank(c.Entries[j].Severity)
		}
		return c.Entries[i].Category < c.Entries[j].Category
	})
	return c
}
func ReleaseAuditChangelogJSON(c ReleaseAuditChangelog) string {
	b, _ := json.MarshalIndent(c, "", "  ")
	return string(b)
}
func ReleaseAuditChangelogString(c ReleaseAuditChangelog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release audit changelog status=%s entries=%d\n", c.Status, len(c.Entries))
	for _, e := range c.Entries {
		fmt.Fprintf(&b, "  %-16s %-5s %s -- %s\n", e.Category, e.Severity, e.Title, e.Detail)
	}
	return b.String()
}
func ReleaseAuditChangelogMarkdown(c ReleaseAuditChangelog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Release audit changelog\n\nStatus: `%s`\n\n", c.Status)
	for _, e := range c.Entries {
		fmt.Fprintf(&b, "- **%s** `%s` %s\n  - %s\n", e.Category, e.Severity, e.Title, e.Detail)
	}
	return b.String()
}

type FinalReleaseDecision struct {
	SchemaVersion string   `json:"schema_version"`
	Decision      string   `json:"decision"`
	Status        string   `json:"status"`
	Score         int      `json:"score"`
	Blocking      []string `json:"blocking,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	NextActions   []string `json:"next_actions,omitempty"`
}

func BuildFinalReleaseDecision(a ReleaseAuditReport, waivers ReleaseWaiverReport, todos ReleaseAuditTodoReport, calendar WaiverExpiryCalendar, policy ReleaseVerificationGatePolicy) FinalReleaseDecision {
	d := FinalReleaseDecision{SchemaVersion: "rvwasm.final-decision.v1", Decision: "go", Status: "pass", Score: a.Score.Score}
	block := func(s string) { d.Blocking = append(d.Blocking, s); d.Decision = "no-go"; d.Status = "fail" }
	warn := func(s string) {
		d.Warnings = append(d.Warnings, s)
		if d.Status == "pass" {
			d.Status = "warn"
		}
		if d.Decision == "go" {
			d.Decision = "go-with-watch"
		}
	}
	if a.Gate.Status == "fail" {
		block("release verification gate failed")
	} else if a.Gate.Status == "warn" {
		warn("release verification gate has warnings")
	}
	if a.Score.Status == "fail" || a.Score.Score < 60 {
		block(fmt.Sprintf("release score too low: %d", a.Score.Score))
	} else if a.Score.Status == "warn" || a.Score.Score < 80 {
		warn(fmt.Sprintf("release score degraded: %d", a.Score.Score))
	}
	if waivers.Unwaived != 0 {
		block(fmt.Sprintf("%d unwaived audit issue(s)", waivers.Unwaived))
	}
	if waivers.Expired != 0 || calendar.Expired != 0 || calendar.Invalid != 0 {
		block("waivers are expired or invalid")
	}
	if calendar.ExpiringSoon != 0 {
		warn(fmt.Sprintf("%d waiver(s) expire soon", calendar.ExpiringSoon))
	}
	if policy.RequireReleasePass && a.Release.Status != "pass" {
		block("policy requires release manifest status pass")
	}
	if todos.Count != 0 {
		d.NextActions = append(d.NextActions, fmt.Sprintf("complete or waive %d release TODO item(s)", todos.Count))
	}
	if len(d.Blocking) == 0 && len(d.Warnings) == 0 {
		d.NextActions = append(d.NextActions, "tag and archive release handoff artifacts")
	}
	if len(d.Blocking) != 0 {
		d.NextActions = append(d.NextActions, "fix blocking items or create time-limited waivers")
	}
	return d
}
func FinalReleaseDecisionJSON(d FinalReleaseDecision) string {
	b, _ := json.MarshalIndent(d, "", "  ")
	return string(b)
}
func FinalReleaseDecisionString(d FinalReleaseDecision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "final release decision=%s status=%s score=%d\n", d.Decision, d.Status, d.Score)
	for _, x := range d.Blocking {
		fmt.Fprintf(&b, "  BLOCK: %s\n", x)
	}
	for _, x := range d.Warnings {
		fmt.Fprintf(&b, "  WARN:  %s\n", x)
	}
	for _, x := range d.NextActions {
		fmt.Fprintf(&b, "  NEXT:  %s\n", x)
	}
	return b.String()
}

type ReleaseEvidenceBundleInspection struct {
	SchemaVersion string   `json:"schema_version"`
	Status        string   `json:"status"`
	FileCount     int      `json:"file_count"`
	RequiredFound int      `json:"required_found"`
	RequiredTotal int      `json:"required_total"`
	Files         []string `json:"files,omitempty"`
	Issues        []string `json:"issues,omitempty"`
}

func ReleaseEvidenceBundleFiles(a ReleaseAuditReport, diff ReleaseAuditDiffReport, waivers ReleaseWaiverReport, todos ReleaseAuditTodoReport, calendar WaiverExpiryCalendar, changelog ReleaseAuditChangelog, decision FinalReleaseDecision) map[string]string {
	return map[string]string{
		"README.md":               "# rvwasm release evidence bundle\n\nThis package contains release audit evidence only. Raw firmware, kernels, disks and initrd images are intentionally excluded; use the release manifest SHA-256 pins for artifact retrieval.\n",
		"release-audit.json":      ReleaseAuditReportJSON(a),
		"release-audit.txt":       ReleaseAuditReportString(a),
		"release-audit-diff.json": ReleaseAuditDiffJSON(diff),
		"release-waivers.json":    ReleaseWaiverReportJSON(waivers),
		"release-todo.md":         ReleaseAuditTodoReportMarkdown(todos),
		"waiver-calendar.json":    WaiverExpiryCalendarJSON(calendar),
		"release-changelog.md":    ReleaseAuditChangelogMarkdown(changelog),
		"final-decision.json":     FinalReleaseDecisionJSON(decision),
		"final-decision.txt":      FinalReleaseDecisionString(decision),
	}
}

func InspectReleaseEvidenceBundleZipBytes(data []byte) (ReleaseEvidenceBundleInspection, error) {
	r := ReleaseEvidenceBundleInspection{SchemaVersion: "rvwasm.evidence-inspection.v1", Status: "pass"}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		r.Status = "fail"
		r.Issues = append(r.Issues, err.Error())
		return r, err
	}
	req := map[string]bool{"README.md": false, "release-audit.json": false, "release-waivers.json": false, "waiver-calendar.json": false, "final-decision.json": false}
	seen := map[string]bool{}
	for _, f := range zr.File {
		name := f.Name
		r.Files = append(r.Files, name)
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") || strings.Contains(name, "\\") {
			r.Issues = append(r.Issues, "unsafe path: "+name)
			r.Status = "fail"
		}
		if seen[name] {
			r.Issues = append(r.Issues, "duplicate path: "+name)
			r.Status = "fail"
		}
		seen[name] = true
		if _, ok := req[name]; ok {
			req[name] = true
		}
		if strings.HasSuffix(name, ".json") {
			rc, err := f.Open()
			if err != nil {
				r.Issues = append(r.Issues, "cannot open "+name+": "+err.Error())
				r.Status = "fail"
				continue
			}
			b, _ := io.ReadAll(io.LimitReader(rc, 8<<20))
			_ = rc.Close()
			var tmp any
			if err := json.Unmarshal(b, &tmp); err != nil {
				r.Issues = append(r.Issues, "bad json "+name+": "+err.Error())
				r.Status = "fail"
			}
		}
	}
	sort.Strings(r.Files)
	r.FileCount = len(r.Files)
	r.RequiredTotal = len(req)
	for p, ok := range req {
		if ok {
			r.RequiredFound++
		} else {
			r.Issues = append(r.Issues, "missing required file: "+p)
			r.Status = "fail"
		}
	}
	return r, nil
}
func ReleaseEvidenceBundleInspectionJSON(i ReleaseEvidenceBundleInspection) string {
	b, _ := json.MarshalIndent(i, "", "  ")
	return string(b)
}
func ReleaseEvidenceBundleInspectionString(i ReleaseEvidenceBundleInspection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release evidence bundle status=%s files=%d required=%d/%d\n", i.Status, i.FileCount, i.RequiredFound, i.RequiredTotal)
	for _, is := range i.Issues {
		fmt.Fprintf(&b, "issue: %s\n", is)
	}
	for _, f := range i.Files {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	return b.String()
}
