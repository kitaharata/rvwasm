package analyze

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CIGatePolicy describes machine-checkable quality gates for rvsmoke.  It is
// intentionally conservative: empty fields mean "do not check", while negative
// numeric thresholds are treated as disabled.  This makes the JSON stable for
// CI systems that want to opt in one check at a time.
type CIGatePolicy struct {
	Name                    string   `json:"name,omitempty"`
	MaxIntegrityErrors      int      `json:"max_integrity_errors"`
	MaxIntegrityWarnings    int      `json:"max_integrity_warnings"`
	MaxVirtqueueAnomalies   int      `json:"max_virtqueue_anomalies"`
	MaxSmokeFailures        int      `json:"max_smoke_failures"`
	MinTraceLines           int      `json:"min_trace_lines,omitempty"`
	MinConsoleLines         int      `json:"min_console_lines,omitempty"`
	RequireArtifacts        []string `json:"require_artifacts,omitempty"`
	FailOnTopCauseContains  []string `json:"fail_on_top_cause_contains,omitempty"`
	WarnOnMissingBaseline   bool     `json:"warn_on_missing_baseline,omitempty"`
	TreatWarningsAsFailures bool     `json:"treat_warnings_as_failures,omitempty"`
}

type CIGateCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // pass, warn, fail
	Observed string `json:"observed,omitempty"`
	Expected string `json:"expected,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type CIGateReport struct {
	PolicyName string        `json:"policy_name,omitempty"`
	Status     string        `json:"status"`
	ExitCode   int           `json:"exit_code"`
	Checks     []CIGateCheck `json:"checks"`
	Summary    []string      `json:"summary,omitempty"`
}

func DefaultCIGatePolicy() CIGatePolicy {
	return CIGatePolicy{
		Name:                   "default",
		MaxIntegrityErrors:     0,
		MaxIntegrityWarnings:   -1,
		MaxVirtqueueAnomalies:  -1,
		MaxSmokeFailures:       0,
		FailOnTopCauseContains: []string{"panic", "oops", "illegal"},
	}
}

func ParseCIGatePolicyJSON(text string, def CIGatePolicy) (CIGatePolicy, error) {
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

func BuildCIGateReport(bundle DiagnosticBundle, integrity BundleIntegrityReport, sig LogSignatureSet, diff []DiagnosticBundleDiff, policy CIGatePolicy) CIGateReport {
	r := CIGateReport{PolicyName: firstNonEmpty(policy.Name, "custom"), Status: "pass"}
	add := func(status, name, observed, expected, detail string) {
		r.Checks = append(r.Checks, CIGateCheck{Name: name, Status: status, Observed: observed, Expected: expected, Detail: detail})
		if status == "fail" {
			r.Status = "fail"
		} else if status == "warn" && r.Status == "pass" {
			r.Status = "warn"
		}
	}
	threshold := func(name string, observed, max int) {
		if max < 0 {
			add("pass", name, strconv.Itoa(observed), "disabled", "threshold disabled")
			return
		}
		if observed > max {
			add("fail", name, strconv.Itoa(observed), "<= "+strconv.Itoa(max), "threshold exceeded")
		} else {
			add("pass", name, strconv.Itoa(observed), "<= "+strconv.Itoa(max), "")
		}
	}
	threshold("integrity_errors", integrity.Counts["error"], policy.MaxIntegrityErrors)
	warnStatusBefore := r.Status
	if policy.MaxIntegrityWarnings >= 0 && integrity.Counts["warn"] > policy.MaxIntegrityWarnings {
		status := "warn"
		if policy.TreatWarningsAsFailures {
			status = "fail"
		}
		add(status, "integrity_warnings", strconv.Itoa(integrity.Counts["warn"]), "<= "+strconv.Itoa(policy.MaxIntegrityWarnings), "warning threshold exceeded")
	} else {
		threshold("integrity_warnings", integrity.Counts["warn"], policy.MaxIntegrityWarnings)
	}
	_ = warnStatusBefore
	threshold("virtqueue_anomalies", len(bundle.Share.Anomalies), policy.MaxVirtqueueAnomalies)
	threshold("smoke_failures", countSmokeFailuresForGate(bundle.Smoke), policy.MaxSmokeFailures)
	if policy.MinTraceLines > 0 {
		if sig.TraceLines < policy.MinTraceLines {
			add("fail", "trace_lines", strconv.Itoa(sig.TraceLines), ">= "+strconv.Itoa(policy.MinTraceLines), "trace is shorter than expected")
		} else {
			add("pass", "trace_lines", strconv.Itoa(sig.TraceLines), ">= "+strconv.Itoa(policy.MinTraceLines), "")
		}
	}
	if policy.MinConsoleLines > 0 {
		if sig.ConsoleLines < policy.MinConsoleLines {
			add("fail", "console_lines", strconv.Itoa(sig.ConsoleLines), ">= "+strconv.Itoa(policy.MinConsoleLines), "console log is shorter than expected")
		} else {
			add("pass", "console_lines", strconv.Itoa(sig.ConsoleLines), ">= "+strconv.Itoa(policy.MinConsoleLines), "")
		}
	}
	if len(policy.RequireArtifacts) != 0 {
		present := map[string]bool{}
		for _, a := range bundle.Manifest.Artifacts {
			present[strings.TrimSpace(a.Role)] = true
		}
		for _, role := range policy.RequireArtifacts {
			role = strings.TrimSpace(role)
			if role == "" {
				continue
			}
			if !present[role] {
				add("fail", "artifact:"+role, "missing", "present", "required artifact pin is missing")
			} else {
				add("pass", "artifact:"+role, "present", "present", "")
			}
		}
	}
	top := strings.ToLower(bundleTopCause(bundle))
	for _, needle := range policy.FailOnTopCauseContains {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle == "" {
			continue
		}
		if top != "" && strings.Contains(top, needle) {
			add("fail", "top_stop_cause", bundleTopCause(bundle), "not containing "+needle, "top stop-cause matched fail pattern")
		}
	}
	if policy.WarnOnMissingBaseline && hasMissingBaselineDiff(diff) {
		add("warn", "baseline", "missing", "provided", "baseline compare was requested by policy")
	}
	if r.Status == "fail" {
		r.ExitCode = 1
	} else if policy.TreatWarningsAsFailures && r.Status == "warn" {
		r.Status = "fail"
		r.ExitCode = 1
	}
	for _, c := range r.Checks {
		if c.Status != "pass" {
			r.Summary = append(r.Summary, fmt.Sprintf("%s: %s (%s)", c.Status, c.Name, c.Detail))
		}
	}
	return r
}

func countSmokeFailuresForGate(rows []SmokeSummary) int {
	count := 0
	for _, sm := range rows {
		s := strings.ToLower(sm.TopCause + " " + sm.Status)
		if strings.Contains(s, "panic") || strings.Contains(s, "oops") || strings.Contains(s, "fault") || strings.Contains(s, "illegal") || strings.Contains(s, "virtqueue") || strings.Contains(s, "fail") {
			count++
		}
	}
	return count
}

func hasMissingBaselineDiff(rows []DiagnosticBundleDiff) bool {
	for _, r := range rows {
		if r.Kind == "missing-baseline" || r.Kind == "parse-error" {
			return true
		}
	}
	return false
}

func CIGateReportString(r CIGateReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ci gate status=%s exit=%d policy=%s\n", r.Status, r.ExitCode, firstNonEmpty(r.PolicyName, "-"))
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "  %-5s %-24s observed=%s expected=%s", c.Status, c.Name, firstNonEmpty(c.Observed, "-"), firstNonEmpty(c.Expected, "-"))
		if c.Detail != "" {
			fmt.Fprintf(&b, "  %s", c.Detail)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func CIGateReportJSON(r CIGateReport) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

// CIJUnitXML renders the gate, bundle integrity, smoke rows, and artifact checks
// as JUnit XML so ordinary CI systems can publish rvwasm diagnostics as tests.
func CIJUnitXML(gate CIGateReport, integrity BundleIntegrityReport, checks []ArtifactIntegrityRow, bundleDiff []DiagnosticBundleDiff) string {
	type failure struct {
		Message string `xml:"message,attr,omitempty"`
		Text    string `xml:",chardata"`
	}
	type testcase struct {
		Name    string   `xml:"name,attr"`
		Class   string   `xml:"classname,attr,omitempty"`
		Failure *failure `xml:"failure,omitempty"`
	}
	type suite struct {
		XMLName   xml.Name   `xml:"testsuite"`
		Name      string     `xml:"name,attr"`
		Tests     int        `xml:"tests,attr"`
		Failures  int        `xml:"failures,attr"`
		Timestamp string     `xml:"timestamp,attr,omitempty"`
		Cases     []testcase `xml:"testcase"`
	}
	cases := []testcase{}
	addCase := func(class, name, status, detail string) {
		tc := testcase{Name: safeJUnitName(name), Class: class}
		if status == "fail" || status == "error" {
			tc.Failure = &failure{Message: status, Text: detail}
		}
		cases = append(cases, tc)
	}
	for _, c := range gate.Checks {
		addCase("rvwasm.gate", c.Name, c.Status, c.Detail+" observed="+c.Observed+" expected="+c.Expected)
	}
	for _, iss := range integrity.Issues {
		addCase("rvwasm.integrity", iss.Area+"/"+iss.Field, issueJUnitStatus(iss.Severity), iss.Detail+" "+iss.Hint)
	}
	for _, c := range checks {
		status := "pass"
		if !c.LooksValid {
			status = "fail"
		}
		addCase("rvwasm.artifact", c.Role, status, fmt.Sprintf("bytes=%d sha=%s", c.Bytes, c.SHA256))
	}
	for _, d := range bundleDiff {
		status := "pass"
		if d.Kind == "parse-error" {
			status = "fail"
		}
		addCase("rvwasm.baseline", d.Area+"/"+d.Key, status, d.Kind+" "+d.Before+" -> "+d.After)
	}
	failures := 0
	for _, c := range cases {
		if c.Failure != nil {
			failures++
		}
	}
	out, _ := xml.MarshalIndent(suite{Name: "rvwasm", Tests: len(cases), Failures: failures, Timestamp: time.Now().UTC().Format(time.RFC3339), Cases: cases}, "", "  ")
	return xml.Header + string(out) + "\n"
}

func issueJUnitStatus(sev string) string {
	if sev == "error" {
		return "fail"
	}
	return "pass"
}

func safeJUnitName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	return strings.ReplaceAll(s, "\n", " ")
}

// CIHTMLReport produces a self-contained report suitable for CI artifacts.
func CIHTMLReport(ci CISummary, gate CIGateReport, integrity BundleIntegrityReport, sig LogSignatureSet, diff []DiagnosticBundleDiff, checks []ArtifactIntegrityRow) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm CI report</title>")
	b.WriteString("<style>body{font-family:system-ui,sans-serif;margin:2rem;line-height:1.4}table{border-collapse:collapse;width:100%;margin:1rem 0}td,th{border:1px solid #ddd;padding:.35rem .5rem;text-align:left}th{background:#f6f6f6}.pass{color:#077d23}.warn{color:#9a6700}.fail,.error{color:#b42318}code,pre{background:#f6f8fa;padding:.2rem .3rem;border-radius:4px}pre{overflow:auto;padding:1rem}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	fmt.Fprintf(&b, "<h1>rvwasm CI report</h1><p>Status: <strong class=%q>%s</strong> / exit %d</p>", html.EscapeString(ci.Status), html.EscapeString(ci.Status), ci.ExitCode)
	b.WriteString("<h2>Gate checks</h2><table><tr><th>Status</th><th>Name</th><th>Observed</th><th>Expected</th><th>Detail</th></tr>")
	for _, c := range gate.Checks {
		fmt.Fprintf(&b, "<tr><td class=%q>%s</td><td>%s</td><td><code>%s</code></td><td><code>%s</code></td><td>%s</td></tr>", html.EscapeString(c.Status), html.EscapeString(c.Status), html.EscapeString(c.Name), html.EscapeString(c.Observed), html.EscapeString(c.Expected), html.EscapeString(c.Detail))
	}
	b.WriteString("</table>")
	b.WriteString("<h2>Log signature</h2><pre>")
	b.WriteString(html.EscapeString(LogSignatureSetString(sig)))
	b.WriteString("</pre><h2>Bundle integrity</h2><pre>")
	b.WriteString(html.EscapeString(BundleIntegrityReportString(integrity)))
	b.WriteString("</pre>")
	if len(checks) != 0 {
		b.WriteString("<h2>Artifact file checks</h2><table><tr><th>Role</th><th>Result</th><th>Bytes</th><th>SHA-256</th></tr>")
		for _, c := range checks {
			status := "fail"
			if c.LooksValid {
				status = "pass"
			}
			fmt.Fprintf(&b, "<tr><td>%s</td><td class=%q>%s</td><td>%d</td><td><code>%s</code></td></tr>", html.EscapeString(c.Role), status, status, c.Bytes, html.EscapeString(c.SHA256))
		}
		b.WriteString("</table>")
	}
	b.WriteString("<h2>Baseline diff</h2><pre>")
	b.WriteString(html.EscapeString(DiagnosticBundleDiffString(diff)))
	b.WriteString("</pre>")
	return b.String()
}

// SARIF is intentionally minimal but valid enough for GitHub/code-scanning style
// viewers; locations point to synthetic rvwasm:// areas because diagnostics are
// emulator state rather than source files.
func CISARIFReport(integrity BundleIntegrityReport, gate CIGateReport) string {
	type sarifResult struct {
		RuleID  string `json:"ruleId"`
		Level   string `json:"level"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	type sarifRun struct {
		Tool struct {
			Driver struct {
				Name  string `json:"name"`
				Rules []any  `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	payload := struct {
		Version string     `json:"version"`
		Schema  string     `json:"$schema"`
		Runs    []sarifRun `json:"runs"`
	}{Version: "2.1.0", Schema: "https://json.schemastore.org/sarif-2.1.0.json"}
	run := sarifRun{}
	run.Tool.Driver.Name = "rvsmoke"
	add := func(rule, level, text string) {
		res := sarifResult{RuleID: rule, Level: sarifLevel(level)}
		res.Message.Text = text
		run.Results = append(run.Results, res)
	}
	for _, iss := range integrity.Issues {
		add("rvwasm.integrity."+iss.Area, iss.Severity, iss.Detail)
	}
	for _, c := range gate.Checks {
		if c.Status != "pass" {
			add("rvwasm.gate."+c.Name, c.Status, c.Detail)
		}
	}
	sort.SliceStable(run.Results, func(i, j int) bool { return run.Results[i].RuleID < run.Results[j].RuleID })
	payload.Runs = []sarifRun{run}
	b, _ := json.MarshalIndent(payload, "", "  ")
	return string(b)
}

func sarifLevel(status string) string {
	switch status {
	case "fail", "error":
		return "error"
	case "warn", "warning":
		return "warning"
	default:
		return "note"
	}
}
