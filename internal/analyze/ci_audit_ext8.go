package analyze

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

type ReleaseAuditDiffItem struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Severity string `json:"severity,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type ReleaseAuditDiffReport struct {
	Status       string                 `json:"status"`
	Checked      int                    `json:"checked"`
	BeforeStatus string                 `json:"before_status,omitempty"`
	AfterStatus  string                 `json:"after_status,omitempty"`
	BeforeScore  int                    `json:"before_score,omitempty"`
	AfterScore   int                    `json:"after_score,omitempty"`
	ScoreDelta   int                    `json:"score_delta,omitempty"`
	Items        []ReleaseAuditDiffItem `json:"items,omitempty"`
	Issues       []string               `json:"issues,omitempty"`
}

func BuildReleaseAuditDiff(current ReleaseAuditReport, baselineText string) ReleaseAuditDiffReport {
	d := ReleaseAuditDiffReport{Status: "pass"}
	baselineText = strings.TrimSpace(baselineText)
	if baselineText == "" {
		d.Status = "warn"
		d.Issues = append(d.Issues, "no baseline release audit provided")
		return d
	}
	var base ReleaseAuditReport
	if err := json.Unmarshal([]byte(baselineText), &base); err != nil {
		d.Status = "fail"
		d.Issues = append(d.Issues, "cannot parse baseline release audit: "+err.Error())
		return d
	}
	d.BeforeStatus, d.AfterStatus = base.Status, current.Status
	d.BeforeScore, d.AfterScore = base.Score.Score, current.Score.Score
	d.ScoreDelta = current.Score.Score - base.Score.Score
	add := func(kind, name, before, after, severity, detail string) {
		if before == after && detail == "" {
			return
		}
		d.Items = append(d.Items, ReleaseAuditDiffItem{Kind: kind, Name: name, Before: before, After: after, Severity: severity, Detail: detail})
	}
	add("summary", "status", base.Status, current.Status, severityFromTransition(base.Status, current.Status), "release audit status changed")
	if base.Score.Score != current.Score.Score {
		sev := "info"
		if current.Score.Score < base.Score.Score {
			sev = "warn"
		}
		if current.Score.Status == "fail" {
			sev = "fail"
		}
		add("summary", "score", fmt.Sprintf("%d", base.Score.Score), fmt.Sprintf("%d", current.Score.Score), sev, "release verification score changed")
	}
	bm := map[string]ReleaseVerificationGateCheck{}
	for _, c := range base.Gate.Checks {
		bm[c.Name] = c
	}
	cm := map[string]ReleaseVerificationGateCheck{}
	for _, c := range current.Gate.Checks {
		cm[c.Name] = c
	}
	for name, c := range cm {
		b, ok := bm[name]
		if !ok {
			add("gate", name, "missing", c.Status, severityFromStatus(c.Status), c.Detail)
			continue
		}
		if b.Status != c.Status || b.Observed != c.Observed || b.Detail != c.Detail {
			add("gate", name, b.Status+"/"+b.Observed, c.Status+"/"+c.Observed, severityFromTransition(b.Status, c.Status), c.Detail)
		}
	}
	for name, b := range bm {
		if _, ok := cm[name]; !ok {
			add("gate", name, b.Status, "missing", "warn", "gate check disappeared")
		}
	}
	sm := map[string]ReleaseScoreComponent{}
	for _, c := range base.Score.Components {
		sm[c.Name] = c
	}
	for _, c := range current.Score.Components {
		b, ok := sm[c.Name]
		if !ok || b.Status != c.Status || b.Points != c.Points || b.Detail != c.Detail {
			before := "missing"
			if ok {
				before = fmt.Sprintf("%s %d/%d", b.Status, b.Points, b.Weight)
			}
			add("score", c.Name, before, fmt.Sprintf("%s %d/%d", c.Status, c.Points, c.Weight), severityFromStatus(c.Status), c.Detail)
		}
	}
	d.Checked = len(d.Items)
	if len(d.Items) != 0 && d.Status == "pass" {
		d.Status = "warn"
	}
	for _, it := range d.Items {
		if it.Severity == "fail" {
			d.Status = "fail"
			break
		}
	}
	sort.Slice(d.Items, func(i, j int) bool { return d.Items[i].Kind+":"+d.Items[i].Name < d.Items[j].Kind+":"+d.Items[j].Name })
	return d
}

func ReleaseAuditDiffJSON(d ReleaseAuditDiffReport) string {
	b, _ := json.MarshalIndent(d, "", "  ")
	return string(b)
}
func ReleaseAuditDiffString(d ReleaseAuditDiffReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release audit diff status=%s score=%d→%d delta=%+d changes=%d\n", d.Status, d.BeforeScore, d.AfterScore, d.ScoreDelta, len(d.Items))
	for _, is := range d.Issues {
		fmt.Fprintf(&b, "issue: %s\n", is)
	}
	for _, it := range d.Items {
		fmt.Fprintf(&b, "  %-8s %-24s %-5s %s → %s %s\n", it.Kind, it.Name, it.Severity, it.Before, it.After, it.Detail)
	}
	return b.String()
}

type ReleaseAuditIssue struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitempty"`
}

type ReleaseWaiverRule struct {
	ID        string `json:"id"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	Status    string `json:"status,omitempty"`
	Contains  string `json:"contains,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Owner     string `json:"owner,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type ReleaseWaiverSet struct {
	SchemaVersion string              `json:"schema_version"`
	Rules         []ReleaseWaiverRule `json:"rules"`
}

type ReleaseWaiverMatch struct {
	IssueID   string `json:"issue_id"`
	WaiverID  string `json:"waiver_id"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type ReleaseWaiverReport struct {
	Status       string               `json:"status"`
	IssueCount   int                  `json:"issue_count"`
	Waived       int                  `json:"waived"`
	Unwaived     int                  `json:"unwaived"`
	Expired      int                  `json:"expired"`
	Unmatched    []string             `json:"unmatched_waivers,omitempty"`
	Issues       []ReleaseAuditIssue  `json:"issues,omitempty"`
	Matches      []ReleaseWaiverMatch `json:"matches,omitempty"`
	WaiverSchema string               `json:"waiver_schema,omitempty"`
}

func ExtractReleaseAuditIssues(a ReleaseAuditReport) []ReleaseAuditIssue {
	issues := []ReleaseAuditIssue{}
	add := func(kind, name, status, detail string) {
		status = firstNonEmpty(status, "warn")
		sev := severityFromStatus(status)
		id := strings.ToLower(strings.ReplaceAll(kind+":"+name+":"+status, " ", "-"))
		issues = append(issues, ReleaseAuditIssue{ID: id, Kind: kind, Name: name, Status: status, Severity: sev, Detail: detail})
	}
	for _, c := range a.Gate.Checks {
		if c.Status != "" && !strings.EqualFold(c.Status, "pass") {
			add("gate", c.Name, c.Status, firstNonEmpty(c.Detail, c.Observed))
		}
	}
	for _, c := range a.Score.Components {
		if c.Status != "" && !strings.EqualFold(c.Status, "pass") {
			add("score", c.Name, c.Status, c.Detail)
		}
	}
	for _, e := range a.Retention.Entries {
		if e.Status != "" && !strings.EqualFold(e.Status, "pass") {
			add("retention", firstNonEmpty(e.Path, e.Kind), e.Status, e.Detail)
		}
	}
	for _, x := range a.Attestation.Missing {
		add("attestation", x, "fail", "missing attestation field/material")
	}
	for _, x := range a.Attestation.Mismatch {
		add("attestation", x, "fail", "attestation mismatch")
	}
	for _, x := range a.Attestation.Warnings {
		add("attestation", x, "warn", "attestation warning")
	}
	for _, x := range a.SBOMDiff.Issues {
		add("sbom", x, a.SBOMDiff.Status, "dependency inventory issue")
	}
	for _, x := range a.SBOMDiff.AddedModules {
		add("sbom", x, "warn", "module added")
	}
	for _, x := range a.SBOMDiff.ChangedModules {
		add("sbom", x, "warn", "module changed")
	}
	for _, x := range a.ReleaseZipCompare.Issues {
		add("release_zip", x, a.ReleaseZipCompare.Status, "release ZIP issue")
	}
	for _, x := range a.ReleaseZipCompare.Missing {
		add("release_zip", x, "fail", "file missing from release ZIP")
	}
	for _, x := range a.ReleaseZipCompare.Changed {
		add("release_zip", x, "fail", "file changed in release ZIP")
	}
	for _, x := range a.ReleaseZipCompare.Extra {
		add("release_zip", x, "warn", "extra file in release ZIP")
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return issues
}

func ReleaseWaiverTemplateJSON() string {
	w := ReleaseWaiverSet{SchemaVersion: "rvwasm.release-waivers.v1", Rules: []ReleaseWaiverRule{{ID: "example-retention-expiring", Kind: "retention", Status: "warn", Contains: "expiring", Reason: "known short-lived CI artifact", Owner: "release-owner", ExpiresAt: time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.RFC3339)}}}
	b, _ := json.MarshalIndent(w, "", "  ")
	return string(b)
}

func ParseReleaseWaiverSet(text string) (ReleaseWaiverSet, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ReleaseWaiverSet{SchemaVersion: "rvwasm.release-waivers.v1"}, nil
	}
	var w ReleaseWaiverSet
	if err := json.Unmarshal([]byte(text), &w); err != nil {
		return w, err
	}
	return w, nil
}

func BuildReleaseWaiverReport(a ReleaseAuditReport, waiverText string, checkedAt string) ReleaseWaiverReport {
	if checkedAt == "" {
		checkedAt = time.Now().UTC().Format(time.RFC3339)
	}
	now, _ := time.Parse(time.RFC3339, checkedAt)
	set, err := ParseReleaseWaiverSet(waiverText)
	r := ReleaseWaiverReport{Status: "pass", WaiverSchema: set.SchemaVersion}
	issues := ExtractReleaseAuditIssues(a)
	r.IssueCount = len(issues)
	if err != nil {
		r.Status = "fail"
		r.Unwaived = len(issues)
		r.Issues = issues
		r.Unmatched = append(r.Unmatched, "cannot parse waivers: "+err.Error())
		return r
	}
	matchedRules := map[string]bool{}
	waivedIssue := map[string]bool{}
	for _, is := range issues {
		for _, rule := range set.Rules {
			if !waiverRuleMatches(rule, is) {
				continue
			}
			matchedRules[rule.ID] = true
			status := "active"
			if rule.ExpiresAt != "" {
				if t, err := time.Parse(time.RFC3339, rule.ExpiresAt); err == nil && !now.IsZero() && (t.Before(now) || t.Equal(now)) {
					status = "expired"
					r.Expired++
				}
			}
			r.Matches = append(r.Matches, ReleaseWaiverMatch{IssueID: is.ID, WaiverID: rule.ID, Status: status, Reason: rule.Reason, ExpiresAt: rule.ExpiresAt})
			if status == "active" {
				waivedIssue[is.ID] = true
			}
		}
	}
	for _, rule := range set.Rules {
		if !matchedRules[rule.ID] {
			r.Unmatched = append(r.Unmatched, firstNonEmpty(rule.ID, "<unnamed>"))
		}
	}
	for _, is := range issues {
		if waivedIssue[is.ID] {
			r.Waived++
			continue
		}
		r.Unwaived++
		r.Issues = append(r.Issues, is)
		if is.Severity == "fail" {
			r.Status = "fail"
		} else if r.Status == "pass" {
			r.Status = "warn"
		}
	}
	if r.Expired != 0 && r.Status == "pass" {
		r.Status = "warn"
	}
	return r
}

func ReleaseWaiverReportJSON(r ReleaseWaiverReport) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
func ReleaseWaiverReportString(r ReleaseWaiverReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release waivers status=%s issues=%d waived=%d unwaived=%d expired=%d\n", r.Status, r.IssueCount, r.Waived, r.Unwaived, r.Expired)
	for _, m := range r.Matches {
		fmt.Fprintf(&b, "  waived %-44s by=%s status=%s %s\n", m.IssueID, m.WaiverID, m.Status, m.Reason)
	}
	for _, is := range r.Issues {
		fmt.Fprintf(&b, "  unwaived %-12s %-28s %-5s %s\n", is.Kind, is.Name, is.Status, is.Detail)
	}
	for _, u := range r.Unmatched {
		fmt.Fprintf(&b, "  unmatched waiver: %s\n", u)
	}
	return b.String()
}

func waiverRuleMatches(r ReleaseWaiverRule, is ReleaseAuditIssue) bool {
	if r.ID == "" {
		return false
	}
	if r.Kind != "" && !strings.EqualFold(r.Kind, is.Kind) {
		return false
	}
	if r.Name != "" && !strings.EqualFold(r.Name, is.Name) {
		return false
	}
	if r.Status != "" && !strings.EqualFold(r.Status, is.Status) {
		return false
	}
	if r.Contains != "" {
		hay := strings.ToLower(is.ID + " " + is.Kind + " " + is.Name + " " + is.Status + " " + is.Detail)
		if !strings.Contains(hay, strings.ToLower(r.Contains)) {
			return false
		}
	}
	return true
}

type ReleaseAuditTodo struct {
	Priority string `json:"priority"`
	Owner    string `json:"owner,omitempty"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Query    string `json:"query,omitempty"`
}

type ReleaseAuditTodoReport struct {
	Status string             `json:"status"`
	Count  int                `json:"count"`
	Todos  []ReleaseAuditTodo `json:"todos,omitempty"`
}

func BuildReleaseAuditTodoReport(a ReleaseAuditReport, waivers ReleaseWaiverReport) ReleaseAuditTodoReport {
	waived := map[string]bool{}
	for _, m := range waivers.Matches {
		if m.Status == "active" {
			waived[m.IssueID] = true
		}
	}
	issues := ExtractReleaseAuditIssues(a)
	r := ReleaseAuditTodoReport{Status: "pass"}
	for _, is := range issues {
		if waived[is.ID] {
			continue
		}
		prio := "P2"
		if is.Severity == "fail" {
			prio = "P0"
		} else if is.Kind == "retention" || is.Kind == "sbom" {
			prio = "P1"
		}
		title := todoTitleForIssue(is)
		r.Todos = append(r.Todos, ReleaseAuditTodo{Priority: prio, Title: title, Detail: is.Detail, Query: is.Kind + " " + is.Name})
		if is.Severity == "fail" {
			r.Status = "fail"
		} else if r.Status == "pass" {
			r.Status = "warn"
		}
	}
	sort.Slice(r.Todos, func(i, j int) bool {
		if r.Todos[i].Priority != r.Todos[j].Priority {
			return r.Todos[i].Priority < r.Todos[j].Priority
		}
		return r.Todos[i].Title < r.Todos[j].Title
	})
	r.Count = len(r.Todos)
	return r
}

func ReleaseAuditTodoReportJSON(r ReleaseAuditTodoReport) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
func ReleaseAuditTodoReportMarkdown(r ReleaseAuditTodoReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Release audit TODO\n\nStatus: `%s`; items: %d\n\n", r.Status, r.Count)
	for _, t := range r.Todos {
		fmt.Fprintf(&b, "- [ ] **%s** %s\n  - %s\n  - query: `%s`\n", t.Priority, t.Title, t.Detail, t.Query)
	}
	return b.String()
}
func ReleaseAuditTodoReportString(r ReleaseAuditTodoReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release audit TODO status=%s count=%d\n", r.Status, r.Count)
	for _, t := range r.Todos {
		fmt.Fprintf(&b, "  [%s] %s -- %s\n", t.Priority, t.Title, t.Detail)
	}
	return b.String()
}

func ReleaseAuditExtendedHTML(a ReleaseAuditReport, diff ReleaseAuditDiffReport, waivers ReleaseWaiverReport, todos ReleaseAuditTodoReport) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm release audit extended</title><style>body{font-family:system-ui,sans-serif;margin:2rem;line-height:1.45}nav{position:sticky;top:0;background:white;border-bottom:1px solid #ddd;padding:.5rem 0;z-index:1}nav a{margin-right:1rem}.fail{background:#ffe6e6}.warn{background:#fff6cc}.pass,.ok{background:#e8ffe8}.info{background:#eef4ff}pre{background:#f7f7f7;padding:1rem;overflow:auto}table{border-collapse:collapse;width:100%}td,th{border:1px solid #ccc;padding:.35rem .5rem;vertical-align:top}code{background:#f4f4f4;padding:.1rem .25rem}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	b.WriteString("<nav><a href=\"#summary\">Summary</a><a href=\"#gate\">Gate</a><a href=\"#score\">Score</a><a href=\"#diff\">Diff</a><a href=\"#waivers\">Waivers</a><a href=\"#todo\">TODO</a><a href=\"#retention\">Retention</a><a href=\"#json\">JSON</a></nav>")
	fmt.Fprintf(&b, "<h1 id=\"summary\">rvwasm release audit</h1><p>Status <code class=%q>%s</code>; score <strong>%d/100</strong>; diff <code class=%q>%s</code>; waivers <code class=%q>%s</code>; TODO <strong>%d</strong></p>", html.EscapeString(a.Status), html.EscapeString(a.Status), a.Score.Score, html.EscapeString(diff.Status), html.EscapeString(diff.Status), html.EscapeString(waivers.Status), html.EscapeString(waivers.Status), todos.Count)
	b.WriteString("<h2 id=\"gate\">Verification gate</h2><table><tr><th>Check</th><th>Status</th><th>Observed</th><th>Expected</th><th>Detail</th></tr>")
	for _, c := range a.Gate.Checks {
		fmt.Fprintf(&b, "<tr class=%q><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>", html.EscapeString(c.Status), html.EscapeString(c.Name), html.EscapeString(c.Status), html.EscapeString(c.Observed), html.EscapeString(c.Expected), html.EscapeString(c.Detail))
	}
	b.WriteString("</table><h2 id=\"score\">Score components</h2><table><tr><th>Component</th><th>Status</th><th>Points</th><th>Detail</th></tr>")
	for _, c := range a.Score.Components {
		fmt.Fprintf(&b, "<tr class=%q><td>%s</td><td>%s</td><td>%d/%d</td><td>%s</td></tr>", html.EscapeString(c.Status), html.EscapeString(c.Name), html.EscapeString(c.Status), c.Points, c.Weight, html.EscapeString(c.Detail))
	}
	b.WriteString("</table><h2 id=\"diff\">Audit diff</h2><pre>")
	b.WriteString(html.EscapeString(ReleaseAuditDiffString(diff)))
	b.WriteString("</pre><h2 id=\"waivers\">Waivers</h2><pre>")
	b.WriteString(html.EscapeString(ReleaseWaiverReportString(waivers)))
	b.WriteString("</pre><h2 id=\"todo\">TODO</h2><pre>")
	b.WriteString(html.EscapeString(ReleaseAuditTodoReportMarkdown(todos)))
	b.WriteString("</pre><h2 id=\"retention\">Retention</h2><pre>")
	b.WriteString(html.EscapeString(RetentionAuditString(a.Retention)))
	payload := map[string]any{"audit": a, "diff": diff, "waivers": waivers, "todo": todos}
	j, _ := json.MarshalIndent(payload, "", "  ")
	b.WriteString("</pre><h2 id=\"json\">JSON</h2><pre>")
	b.WriteString(html.EscapeString(string(j)))
	b.WriteString("</pre>")
	return b.String()
}

func todoTitleForIssue(is ReleaseAuditIssue) string {
	switch is.Kind {
	case "gate":
		return "Fix release gate check: " + is.Name
	case "score":
		return "Improve release score component: " + is.Name
	case "retention":
		return "Update artifact retention for: " + is.Name
	case "attestation":
		return "Regenerate or verify provenance attestation: " + is.Name
	case "sbom":
		return "Review dependency inventory drift: " + is.Name
	case "release_zip":
		return "Refresh release handoff ZIP: " + is.Name
	default:
		return "Review release audit issue: " + is.Name
	}
}

func severityFromStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fail", "error":
		return "fail"
	case "warn", "warning":
		return "warn"
	case "pass", "ok":
		return "pass"
	default:
		return "info"
	}
}

func severityFromTransition(before, after string) string {
	bs, as := severityRank(before), severityRank(after)
	if as > bs {
		return severityFromStatus(after)
	}
	if as < bs {
		return "info"
	}
	return severityFromStatus(after)
}
func severityRank(s string) int {
	switch severityFromStatus(s) {
	case "fail":
		return 3
	case "warn":
		return 2
	case "pass":
		return 0
	default:
		return 1
	}
}
