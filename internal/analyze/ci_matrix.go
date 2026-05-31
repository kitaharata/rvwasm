package analyze

import (
	"encoding/json"
	"fmt"
	"html"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CIGatePolicyTemplate is a named, documented CI policy preset.  It is useful
// both from the browser UI and from rvsmoke when a project wants a stable
// starting point without hand-writing policy JSON.
type CIGatePolicyTemplate struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Policy      CIGatePolicy `json:"policy"`
}

// CIGatePolicyTemplates returns conservative presets ordered from most common
// to most specialized.  The values are intentionally explicit so generated JSON
// can be committed to CI without depending on hidden defaults.
func CIGatePolicyTemplates() []CIGatePolicyTemplate {
	def := DefaultCIGatePolicy()
	lenient := def
	lenient.Name = "lenient"
	lenient.MaxIntegrityWarnings = -1
	lenient.MaxVirtqueueAnomalies = -1
	lenient.MaxSmokeFailures = -1
	lenient.FailOnTopCauseContains = nil

	strict := def
	strict.Name = "strict"
	strict.MaxIntegrityErrors = 0
	strict.MaxIntegrityWarnings = 0
	strict.MaxVirtqueueAnomalies = 0
	strict.MaxSmokeFailures = 0
	strict.MinTraceLines = 10
	strict.MinConsoleLines = 1
	strict.RequireArtifacts = []string{"firmware", "payload"}
	strict.FailOnTopCauseContains = []string{"panic", "oops", "illegal", "fault", "virtqueue"}
	strict.TreatWarningsAsFailures = true

	linux := def
	linux.Name = "linux-boot"
	linux.MaxIntegrityErrors = 0
	linux.MaxIntegrityWarnings = 1
	linux.MaxVirtqueueAnomalies = 0
	linux.MaxSmokeFailures = 0
	linux.MinTraceLines = 100
	linux.MinConsoleLines = 5
	linux.RequireArtifacts = []string{"firmware", "payload"}
	linux.FailOnTopCauseContains = []string{"panic", "oops", "illegal instruction", "page fault", "access fault"}

	artifact := def
	artifact.Name = "artifact-only"
	artifact.MaxIntegrityErrors = 0
	artifact.MaxIntegrityWarnings = -1
	artifact.MaxVirtqueueAnomalies = -1
	artifact.MaxSmokeFailures = -1
	artifact.FailOnTopCauseContains = nil
	artifact.RequireArtifacts = []string{"firmware"}

	return []CIGatePolicyTemplate{
		{Name: "default", Description: "Balanced gate: fail on integrity errors, smoke failures, and obvious panic/illegal stop causes.", Policy: def},
		{Name: "strict", Description: "Release/regression gate: warnings fail, artifacts are required, and trace/console must be non-empty.", Policy: strict},
		{Name: "linux-boot", Description: "Linux boot experiment gate: requires firmware/payload, minimal logs, and no virtqueue anomalies.", Policy: linux},
		{Name: "artifact-only", Description: "Only validate bundle and artifact pins; useful before a runnable trace exists.", Policy: artifact},
		{Name: "lenient", Description: "Collect diagnostics without failing on emulator/runtime observations.", Policy: lenient},
	}
}

func CIGatePolicyTemplateByName(name string) (CIGatePolicyTemplate, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "default"
	}
	for _, t := range CIGatePolicyTemplates() {
		if strings.ToLower(t.Name) == name || strings.ToLower(t.Policy.Name) == name {
			return t, true
		}
	}
	return CIGatePolicyTemplate{}, false
}

func CIGatePolicyTemplateJSON(name string) string {
	t, ok := CIGatePolicyTemplateByName(name)
	if !ok {
		return `{"error":"unknown policy template"}`
	}
	b, _ := json.MarshalIndent(t.Policy, "", "  ")
	return string(b)
}

func CIGatePolicyTemplatesJSON() string {
	b, _ := json.MarshalIndent(CIGatePolicyTemplates(), "", "  ")
	return string(b)
}

func CIGatePolicyTemplateListString() string {
	var b strings.Builder
	b.WriteString("available CI policy templates\n")
	for _, t := range CIGatePolicyTemplates() {
		fmt.Fprintf(&b, "  %-14s %s\n", t.Name, t.Description)
	}
	return b.String()
}

// NamedDiagnosticBundle gives a stable display name to a bundle when comparing
// multiple exports from different runs.
type NamedDiagnosticBundle struct {
	Name   string           `json:"name"`
	Bundle DiagnosticBundle `json:"bundle"`
	Raw    string           `json:"-"`
}

type BundleSeriesRow struct {
	Index          int               `json:"index"`
	Name           string            `json:"name"`
	Status         string            `json:"status"`
	Phase          string            `json:"phase,omitempty"`
	TopStopCause   string            `json:"top_stop_cause,omitempty"`
	BundleSHA256   string            `json:"bundle_sha256,omitempty"`
	ManifestSHA256 string            `json:"manifest_sha256,omitempty"`
	BootArgs       string            `json:"boot_args,omitempty"`
	HartCount      int               `json:"hart_count,omitempty"`
	NextAddr       string            `json:"next_addr,omitempty"`
	Counts         map[string]int    `json:"counts,omitempty"`
	Artifacts      map[string]string `json:"artifact_hashes,omitempty"`
}

type BundleSeriesDiff struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Area   string `json:"area"`
	Key    string `json:"key"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Kind   string `json:"kind"`
}

type BundleTrendReport struct {
	Status  string             `json:"status"`
	Rows    []BundleSeriesRow  `json:"rows"`
	Diffs   []BundleSeriesDiff `json:"diffs,omitempty"`
	Summary []string           `json:"summary,omitempty"`
}

func BuildBundleTrendReport(items []NamedDiagnosticBundle) BundleTrendReport {
	r := BundleTrendReport{Status: "ok"}
	for i, item := range items {
		raw := item.Raw
		if strings.TrimSpace(raw) == "" {
			raw = DiagnosticBundleJSON(item.Bundle)
		}
		integrity := BuildBundleIntegrityReport(item.Bundle, raw)
		ci := BuildCISummary(item.Bundle, LogSignatureSet{}, integrity)
		row := BundleSeriesRow{
			Index:          i,
			Name:           firstNonEmpty(strings.TrimSpace(item.Name), fmt.Sprintf("bundle-%d", i+1)),
			Status:         ci.Status,
			Phase:          firstNonEmpty(item.Bundle.Triage.Phase, ci.Phase),
			TopStopCause:   bundleTopCause(item.Bundle),
			BundleSHA256:   integrity.BundleSHA256,
			ManifestSHA256: integrity.ManifestSHA256,
			BootArgs:       item.Bundle.Manifest.BootArgs,
			HartCount:      item.Bundle.Manifest.HartCount,
			NextAddr:       fmt.Sprintf("%#x", item.Bundle.Manifest.NextAddr),
			Counts:         map[string]int{},
			Artifacts:      map[string]string{},
		}
		for k, v := range ci.Counts {
			row.Counts[k] = v
		}
		for _, a := range item.Bundle.Manifest.Artifacts {
			row.Artifacts[a.Role] = shortHash(a.SHA256)
		}
		r.Rows = append(r.Rows, row)
		if ci.Status == "fail" {
			r.Status = "fail"
		} else if ci.Status == "warn" && r.Status == "ok" {
			r.Status = "warn"
		}
	}
	for i := 1; i < len(items); i++ {
		diffs := CompareDiagnosticBundles(items[i].Bundle, DiagnosticBundleJSON(items[i-1].Bundle))
		for _, d := range diffs {
			if d.Kind == "unchanged" {
				continue
			}
			r.Diffs = append(r.Diffs, BundleSeriesDiff{From: r.Rows[i-1].Name, To: r.Rows[i].Name, Area: d.Area, Key: d.Key, Before: d.Before, After: d.After, Kind: d.Kind})
		}
	}
	if len(r.Rows) == 0 {
		r.Status = "empty"
		r.Summary = append(r.Summary, "no bundles supplied")
		return r
	}
	r.Summary = append(r.Summary, fmt.Sprintf("%d bundle(s), %d diff(s)", len(r.Rows), len(r.Diffs)))
	if len(r.Rows) >= 2 {
		first := r.Rows[0]
		last := r.Rows[len(r.Rows)-1]
		if first.TopStopCause != last.TopStopCause {
			r.Summary = append(r.Summary, fmt.Sprintf("top stop-cause changed: %s -> %s", firstNonEmpty(first.TopStopCause, "-"), firstNonEmpty(last.TopStopCause, "-")))
		}
		if first.Phase != last.Phase {
			r.Summary = append(r.Summary, fmt.Sprintf("phase changed: %s -> %s", firstNonEmpty(first.Phase, "-"), firstNonEmpty(last.Phase, "-")))
		}
	}
	sort.SliceStable(r.Diffs, func(i, j int) bool {
		if r.Diffs[i].From == r.Diffs[j].From {
			if r.Diffs[i].Area == r.Diffs[j].Area {
				return r.Diffs[i].Key < r.Diffs[j].Key
			}
			return r.Diffs[i].Area < r.Diffs[j].Area
		}
		return r.Diffs[i].From < r.Diffs[j].From
	})
	return r
}

func BundleTrendReportJSON(r BundleTrendReport) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

func BundleTrendReportString(r BundleTrendReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bundle trend status=%s bundles=%d diffs=%d\n", r.Status, len(r.Rows), len(r.Diffs))
	for _, s := range r.Summary {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	if len(r.Rows) != 0 {
		b.WriteString("runs:\n")
		for _, row := range r.Rows {
			fmt.Fprintf(&b, "  [%d] %-18s status=%-5s phase=%-18s top=%s manifest=%s\n", row.Index, row.Name, row.Status, firstNonEmpty(row.Phase, "-"), firstNonEmpty(row.TopStopCause, "-"), shortHash(row.ManifestSHA256))
		}
	}
	if len(r.Diffs) != 0 {
		b.WriteString("diffs:\n")
		for _, d := range r.Diffs {
			fmt.Fprintf(&b, "  %s -> %s  %-12s %-18s %-8s %s => %s\n", d.From, d.To, d.Area, d.Key, d.Kind, firstNonEmpty(d.Before, "-"), firstNonEmpty(d.After, "-"))
		}
	}
	return b.String()
}

func BundleTrendReportMarkdown(r BundleTrendReport) string {
	var b strings.Builder
	b.WriteString("## Bundle trend\n\n")
	fmt.Fprintf(&b, "Status: `%s`  \nBundles: `%d`  \nDiffs: `%d`\n\n", r.Status, len(r.Rows), len(r.Diffs))
	for _, s := range r.Summary {
		fmt.Fprintf(&b, "- %s\n", mdCell(s))
	}
	if len(r.Rows) != 0 {
		b.WriteString("\n| # | Name | Status | Phase | Top stop-cause | Manifest SHA |\n|---:|---|---|---|---|---|\n")
		for _, row := range r.Rows {
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | `%s` |\n", row.Index, mdCell(row.Name), mdCell(row.Status), mdCell(firstNonEmpty(row.Phase, "-")), mdCell(firstNonEmpty(row.TopStopCause, "-")), shortHash(row.ManifestSHA256))
		}
	}
	if len(r.Diffs) != 0 {
		b.WriteString("\n| From | To | Area | Key | Kind | Before | After |\n|---|---|---|---|---|---|---|\n")
		for _, d := range r.Diffs {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", mdCell(d.From), mdCell(d.To), mdCell(d.Area), mdCell(d.Key), mdCell(d.Kind), mdCell(firstNonEmpty(d.Before, "-")), mdCell(firstNonEmpty(d.After, "-")))
		}
	}
	return b.String()
}

func BundleTrendReportHTML(r BundleTrendReport) string {
	md := BundleTrendReportMarkdown(r)
	js, _ := json.MarshalIndent(r, "", "  ")
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm bundle trend</title>")
	b.WriteString("<style>body{font-family:system-ui,sans-serif;margin:2rem;line-height:1.45}pre{background:#f6f8fa;padding:1rem;overflow:auto;border-radius:6px}.fail{color:#b42318}.warn{color:#9a6700}.ok,.pass{color:#077d23}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	fmt.Fprintf(&b, "<h1>rvwasm bundle trend</h1><p>Status: <strong class=%q>%s</strong></p>", html.EscapeString(r.Status), html.EscapeString(r.Status))
	b.WriteString("<h2>Markdown</h2><pre>")
	b.WriteString(html.EscapeString(md))
	b.WriteString("</pre><h2>JSON</h2><pre>")
	b.WriteString(html.EscapeString(string(js)))
	b.WriteString("</pre>")
	return b.String()
}

type CIActionItem struct {
	Priority string `json:"priority"`
	Area     string `json:"area"`
	Check    string `json:"check,omitempty"`
	Action   string `json:"action"`
	Detail   string `json:"detail,omitempty"`
}

type CIActionChecklist struct {
	Status string         `json:"status"`
	Items  []CIActionItem `json:"items"`
}

func BuildCIActionChecklist(gate CIGateReport, integrity BundleIntegrityReport, diff []DiagnosticBundleDiff) CIActionChecklist {
	r := CIActionChecklist{Status: "ok"}
	add := func(priority, area, check, action, detail string) {
		r.Items = append(r.Items, CIActionItem{Priority: priority, Area: area, Check: check, Action: action, Detail: detail})
		if priority == "critical" || priority == "high" {
			r.Status = "action-required"
		} else if r.Status == "ok" {
			r.Status = "review"
		}
	}
	for _, c := range gate.Checks {
		if c.Status == "pass" {
			continue
		}
		priority := "medium"
		if c.Status == "fail" {
			priority = "high"
		}
		add(priority, "gate", c.Name, actionForGateCheck(c.Name), c.Detail)
	}
	for _, iss := range integrity.Issues {
		if iss.Severity == "info" {
			continue
		}
		priority := "medium"
		if iss.Severity == "error" {
			priority = "critical"
		}
		add(priority, iss.Area, iss.Field, firstNonEmpty(iss.Hint, "inspect bundle integrity issue"), iss.Detail)
	}
	changedArtifacts := 0
	for _, d := range diff {
		if d.Area == "artifact" && d.Kind == "changed" {
			changedArtifacts++
		}
	}
	if changedArtifacts != 0 {
		add("medium", "baseline", "artifact-hash", "confirm the changed artifact is intentional and update the baseline bundle", strconv.Itoa(changedArtifacts)+" artifact pin(s) changed")
	}
	if len(r.Items) == 0 {
		r.Status = "ok"
		add("low", "summary", "none", "no immediate CI action required", "all active checks passed")
	}
	sort.SliceStable(r.Items, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		if rank[r.Items[i].Priority] == rank[r.Items[j].Priority] {
			if r.Items[i].Area == r.Items[j].Area {
				return r.Items[i].Check < r.Items[j].Check
			}
			return r.Items[i].Area < r.Items[j].Area
		}
		return rank[r.Items[i].Priority] < rank[r.Items[j].Priority]
	})
	return r
}

func actionForGateCheck(name string) string {
	switch name {
	case "integrity_errors", "integrity_warnings":
		return "open Bundle integrity and fix malformed artifact pins or manifest fields"
	case "virtqueue_anomalies":
		return "open Virtqueue anomalies and Descriptor chains; verify QueueReady and descriptor addresses"
	case "smoke_failures":
		return "open Smoke clusters and Stop checklist; rerun the failing preset with trace enabled"
	case "trace_lines", "console_lines":
		return "rerun smoke with a larger step budget or export the missing log"
	case "top_stop_cause":
		return "open Panic summary / Stop-cause evidence and place suggested breakpoints"
	default:
		if strings.HasPrefix(name, "artifact:") {
			return "load the required artifact and re-export the manifest/bundle"
		}
		return "inspect the CI gate detail and adjust policy or emulator state"
	}
}

func CIActionChecklistString(r CIActionChecklist) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ci action checklist status=%s items=%d\n", r.Status, len(r.Items))
	for _, it := range r.Items {
		fmt.Fprintf(&b, "  [%s] %-12s %-18s %s\n", it.Priority, it.Area, firstNonEmpty(it.Check, "-"), it.Action)
		if it.Detail != "" {
			fmt.Fprintf(&b, "        %s\n", it.Detail)
		}
	}
	return b.String()
}

func CIActionChecklistJSON(r CIActionChecklist) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

func CIActionChecklistMarkdown(r CIActionChecklist) string {
	var b strings.Builder
	b.WriteString("## CI action checklist\n\n")
	fmt.Fprintf(&b, "Status: `%s`\n\n", r.Status)
	b.WriteString("| Priority | Area | Check | Action | Detail |\n|---|---|---|---|---|\n")
	for _, it := range r.Items {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", mdCell(it.Priority), mdCell(it.Area), mdCell(firstNonEmpty(it.Check, "-")), mdCell(it.Action), mdCell(it.Detail))
	}
	return b.String()
}

func DefaultCompareName(path string, idx int) string {
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		return "bundle-" + strconv.Itoa(idx+1)
	}
	return base
}
