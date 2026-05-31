package analyze

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// GitHubActionsWorkflowYAML returns a ready-to-commit CI workflow that runs
// rvsmoke against an exported diagnostic bundle. The workflow deliberately keeps
// artifact paths configurable because firmware/kernel images are usually stored
// outside the diagnostic bundle.
func GitHubActionsWorkflowYAML(policyTemplate string) string {
	if strings.TrimSpace(policyTemplate) == "" {
		policyTemplate = "linux-boot"
	}
	if _, ok := CIGatePolicyTemplateByName(policyTemplate); !ok {
		policyTemplate = "default"
	}
	return fmt.Sprintf(`name: rvwasm smoke diagnostics

on:
  pull_request:
  push:
    branches: [ main ]
  workflow_dispatch:

jobs:
  rvsmoke:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23.2'

      - name: Build tools
        run: |
          go test ./...
          CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o ./bin/rvsmoke ./cmd/rvsmoke

      - name: Run rvsmoke CI gate
        run: |
          ./bin/rvsmoke \
            -bundle artifacts/rvwasm-diagnostic-bundle.json \
            -trace artifacts/trace.txt \
            -console artifacts/console.txt \
            -policy-template %[1]s \
            -junit rvwasm-junit.xml \
            -sarif rvwasm.sarif \
            -html rvwasm-ci.html \
            -out md > rvwasm-ci-summary.md

      - name: Upload rvwasm reports
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: rvwasm-ci-reports
          path: |
            rvwasm-ci-summary.md
            rvwasm-ci.html
            rvwasm-junit.xml
            rvwasm.sarif
`, policyTemplate)
}

type PolicyViolationNode struct {
	ID       string                `json:"id"`
	Status   string                `json:"status"`
	Area     string                `json:"area"`
	Title    string                `json:"title"`
	Detail   string                `json:"detail,omitempty"`
	Evidence []string              `json:"evidence,omitempty"`
	Children []PolicyViolationNode `json:"children,omitempty"`
}

type PolicyViolationTree struct {
	Status  string                `json:"status"`
	Summary []string              `json:"summary,omitempty"`
	Nodes   []PolicyViolationNode `json:"nodes,omitempty"`
}

// BuildPolicyViolationTree turns flat gate checks, integrity issues, and bundle
// drift into a small cause tree. This is meant for CI output and handoff reports:
// it does not decide pass/fail; it explains why the gate already decided it.
func BuildPolicyViolationTree(gate CIGateReport, integrity BundleIntegrityReport, diffs []DiagnosticBundleDiff, trend BundleTrendReport) PolicyViolationTree {
	t := PolicyViolationTree{Status: firstNonEmpty(gate.Status, "unknown")}
	addRoot := func(n PolicyViolationNode) {
		t.Nodes = append(t.Nodes, n)
	}
	gateNode := PolicyViolationNode{ID: "gate", Status: gate.Status, Area: "ci-gate", Title: "CI gate checks"}
	for _, c := range gate.Checks {
		if c.Status == "pass" {
			continue
		}
		gateNode.Children = append(gateNode.Children, PolicyViolationNode{ID: "gate." + sanitizeID(c.Name), Status: c.Status, Area: "ci-gate", Title: c.Name, Detail: c.Detail, Evidence: []string{"observed=" + firstNonEmpty(c.Observed, "-"), "expected=" + firstNonEmpty(c.Expected, "-")}})
	}
	if len(gateNode.Children) != 0 {
		addRoot(gateNode)
		t.Summary = append(t.Summary, fmt.Sprintf("%d non-passing gate check(s)", len(gateNode.Children)))
	}

	integNode := PolicyViolationNode{ID: "integrity", Status: integrity.Status, Area: "bundle", Title: "Bundle integrity"}
	for _, is := range integrity.Issues {
		if is.Severity == "info" {
			continue
		}
		integNode.Children = append(integNode.Children, PolicyViolationNode{ID: "integrity." + sanitizeID(is.Area+"."+is.Field), Status: severityToStatus(is.Severity), Area: is.Area, Title: firstNonEmpty(is.Field, is.Area), Detail: is.Detail, Evidence: compactNonEmpty([]string{is.Hint})})
	}
	if len(integNode.Children) != 0 {
		addRoot(integNode)
		t.Summary = append(t.Summary, fmt.Sprintf("%d bundle integrity issue(s)", len(integNode.Children)))
	}

	driftNode := PolicyViolationNode{ID: "drift", Status: "warn", Area: "baseline", Title: "Baseline drift"}
	for _, d := range diffs {
		if d.Kind == "unchanged" {
			continue
		}
		driftNode.Children = append(driftNode.Children, PolicyViolationNode{ID: "drift." + sanitizeID(d.Area+"."+d.Key), Status: driftStatus(d.Kind), Area: d.Area, Title: d.Key, Detail: d.Kind, Evidence: compactNonEmpty([]string{"before=" + d.Before, "after=" + d.After})})
	}
	if len(driftNode.Children) != 0 {
		addRoot(driftNode)
		t.Summary = append(t.Summary, fmt.Sprintf("%d baseline drift item(s)", len(driftNode.Children)))
	}

	if len(trend.Diffs) != 0 {
		trendNode := PolicyViolationNode{ID: "trend", Status: trend.Status, Area: "history", Title: "Bundle trend changes"}
		limit := 24
		for i, d := range trend.Diffs {
			if i >= limit {
				trendNode.Children = append(trendNode.Children, PolicyViolationNode{ID: "trend.more", Status: "info", Area: "history", Title: "Additional trend diffs", Detail: fmt.Sprintf("%d more diff(s)", len(trend.Diffs)-limit)})
				break
			}
			trendNode.Children = append(trendNode.Children, PolicyViolationNode{ID: fmt.Sprintf("trend.%02d", i+1), Status: driftStatus(d.Kind), Area: d.Area, Title: d.Key, Detail: d.From + " -> " + d.To, Evidence: compactNonEmpty([]string{"before=" + d.Before, "after=" + d.After})})
		}
		addRoot(trendNode)
		t.Summary = append(t.Summary, fmt.Sprintf("%d trend diff(s)", len(trend.Diffs)))
	}
	if len(t.Nodes) == 0 {
		t.Summary = append(t.Summary, "no policy violations detected")
	}
	return t
}

func PolicyViolationTreeString(t PolicyViolationTree) string {
	var b strings.Builder
	fmt.Fprintf(&b, "policy violation tree status=%s\n", firstNonEmpty(t.Status, "unknown"))
	for _, s := range t.Summary {
		fmt.Fprintf(&b, "summary: %s\n", s)
	}
	var walk func(prefix string, n PolicyViolationNode)
	walk = func(prefix string, n PolicyViolationNode) {
		fmt.Fprintf(&b, "%s- [%s] %s/%s: %s", prefix, firstNonEmpty(n.Status, "info"), n.Area, n.ID, n.Title)
		if n.Detail != "" {
			fmt.Fprintf(&b, " — %s", n.Detail)
		}
		b.WriteByte('\n')
		for _, e := range n.Evidence {
			if strings.TrimSpace(e) != "" {
				fmt.Fprintf(&b, "%s    evidence: %s\n", prefix, e)
			}
		}
		for _, c := range n.Children {
			walk(prefix+"  ", c)
		}
	}
	for _, n := range t.Nodes {
		walk("", n)
	}
	return b.String()
}

func PolicyViolationTreeMarkdown(t PolicyViolationTree) string {
	var b strings.Builder
	b.WriteString("# rvwasm policy violation tree\n\n")
	fmt.Fprintf(&b, "- Status: `%s`\n", firstNonEmpty(t.Status, "unknown"))
	for _, s := range t.Summary {
		fmt.Fprintf(&b, "- %s\n", mdCell(s))
	}
	b.WriteByte('\n')
	var walk func(level int, n PolicyViolationNode)
	walk = func(level int, n PolicyViolationNode) {
		indent := strings.Repeat("  ", level)
		fmt.Fprintf(&b, "%s- **%s** `%s` %s", indent, mdCell(n.Title), firstNonEmpty(n.Status, "info"), mdCell(n.Area))
		if n.Detail != "" {
			fmt.Fprintf(&b, " — %s", mdCell(n.Detail))
		}
		b.WriteByte('\n')
		for _, e := range n.Evidence {
			if strings.TrimSpace(e) != "" {
				fmt.Fprintf(&b, "%s  - evidence: `%s`\n", indent, mdCell(e))
			}
		}
		for _, c := range n.Children {
			walk(level+1, c)
		}
	}
	for _, n := range t.Nodes {
		walk(0, n)
	}
	return b.String()
}

func PolicyViolationTreeJSON(t PolicyViolationTree) string {
	b, _ := json.MarshalIndent(t, "", "  ")
	return string(b)
}

type BundleHistoryAggregate struct {
	Status        string             `json:"status"`
	Runs          int                `json:"runs"`
	Failures      int                `json:"failures"`
	Warnings      int                `json:"warnings"`
	PhaseCounts   map[string]int     `json:"phase_counts,omitempty"`
	CauseCounts   map[string]int     `json:"cause_counts,omitempty"`
	ArtifactDrift []BundleSeriesDiff `json:"artifact_drift,omitempty"`
	Summary       []string           `json:"summary,omitempty"`
}

func BuildBundleHistoryAggregate(trend BundleTrendReport) BundleHistoryAggregate {
	a := BundleHistoryAggregate{Status: firstNonEmpty(trend.Status, "empty"), PhaseCounts: map[string]int{}, CauseCounts: map[string]int{}}
	a.Runs = len(trend.Rows)
	for _, r := range trend.Rows {
		switch strings.ToLower(r.Status) {
		case "fail", "error":
			a.Failures++
		case "warn":
			a.Warnings++
		}
		if r.Phase != "" {
			a.PhaseCounts[r.Phase]++
		}
		if r.TopStopCause != "" {
			a.CauseCounts[r.TopStopCause]++
		}
	}
	for _, d := range trend.Diffs {
		if d.Area == "artifact" || strings.Contains(strings.ToLower(d.Key), "artifact") || strings.Contains(strings.ToLower(d.Area), "manifest") {
			a.ArtifactDrift = append(a.ArtifactDrift, d)
		}
	}
	a.Summary = append(a.Summary, fmt.Sprintf("runs=%d failures=%d warnings=%d", a.Runs, a.Failures, a.Warnings))
	if len(a.ArtifactDrift) != 0 {
		a.Summary = append(a.Summary, fmt.Sprintf("artifact/manifest drift=%d", len(a.ArtifactDrift)))
	}
	return a
}

func BundleHistoryAggregateString(a BundleHistoryAggregate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bundle history status=%s runs=%d failures=%d warnings=%d\n", a.Status, a.Runs, a.Failures, a.Warnings)
	writeCounts := func(title string, m map[string]int) {
		if len(m) == 0 {
			return
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(title + "\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "  %-24s %d\n", k, m[k])
		}
	}
	writeCounts("phases", a.PhaseCounts)
	writeCounts("top causes", a.CauseCounts)
	if len(a.ArtifactDrift) != 0 {
		b.WriteString("artifact/manifest drift\n")
		for _, d := range a.ArtifactDrift {
			fmt.Fprintf(&b, "  %s %s.%s %s -> %s\n", d.Kind, d.Area, d.Key, d.Before, d.After)
		}
	}
	return b.String()
}

func BundleHistoryAggregateJSON(a BundleHistoryAggregate) string {
	b, _ := json.MarshalIndent(a, "", "  ")
	return string(b)
}

type ReproPackageFile struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

type MinimalReproPackageSpec struct {
	Name     string             `json:"name"`
	Files    []ReproPackageFile `json:"files"`
	Commands []string           `json:"commands"`
	Notes    []string           `json:"notes,omitempty"`
}

func BuildMinimalReproPackageSpec(bundle DiagnosticBundle, runner HeadlessSmokeRunnerSpec) MinimalReproPackageSpec {
	s := MinimalReproPackageSpec{Name: "rvwasm-minimal-repro"}
	s.Files = []ReproPackageFile{
		{"README.md", "human reproduction checklist"},
		{"diagnostic-bundle.json", "machine-readable diagnostic bundle"},
		{"manifest.json", "artifact pins and boot configuration"},
		{"runner-spec.json", "rvsmoke presets and command hints"},
		{"ci-policy.json", "starting CI gate policy"},
		{"ci-summary.json", "rvsmoke CI summary payload"},
		{"scripts/rvsmoke.sh", "copy/paste validation command"},
	}
	s.Commands = append([]string(nil), runner.Commands...)
	if len(s.Commands) == 0 {
		s.Commands = []string{"go run ./cmd/rvsmoke -bundle diagnostic-bundle.json -policy ci-policy.json -out md"}
	}
	s.Notes = []string{"Raw firmware/kernel/disk images are not embedded; use manifest SHA-256 pins to fetch or verify them.", "Attach this package with matching artifacts or artifact download instructions."}
	if len(bundle.Manifest.Artifacts) == 0 {
		s.Notes = append(s.Notes, "No artifact pins were present in the source bundle.")
	}
	return s
}

func MinimalReproPackageMarkdown(spec MinimalReproPackageSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", spec.Name)
	b.WriteString("## Files\n\n| Path | Description |\n|---|---|\n")
	for _, f := range spec.Files {
		fmt.Fprintf(&b, "| `%s` | %s |\n", f.Path, mdCell(f.Description))
	}
	if len(spec.Commands) != 0 {
		b.WriteString("\n## Commands\n\n")
		for _, c := range spec.Commands {
			fmt.Fprintf(&b, "```bash\n%s\n```\n", c)
		}
	}
	if len(spec.Notes) != 0 {
		b.WriteString("\n## Notes\n\n")
		for _, n := range spec.Notes {
			fmt.Fprintf(&b, "- %s\n", mdCell(n))
		}
	}
	return b.String()
}

func MinimalReproPackageFiles(bundle DiagnosticBundle, rawBundle string, ci CISummary, gate CIGateReport, runner HeadlessSmokeRunnerSpec, policy CIGatePolicy, tree PolicyViolationTree, history BundleHistoryAggregate) map[string]string {
	if strings.TrimSpace(rawBundle) == "" {
		rawBundle = DiagnosticBundleJSON(bundle)
	} else if _, _, err := ParseDiagnosticBundleText(rawBundle); err != nil {
		// When rvsmoke is invoked with -manifest only, rawBundle is the raw
		// manifest JSON rather than a full DiagnosticBundle. The repro package
		// must still contain diagnostic-bundle.json, so fall back to the
		// synthesized bundle.
		rawBundle = DiagnosticBundleJSON(bundle)
	}
	summaryPayload := map[string]any{"ci": ci, "gate": gate, "policy_violation_tree": tree, "history": history}
	summaryJSON, _ := json.MarshalIndent(summaryPayload, "", "  ")
	manifestJSON, _ := json.MarshalIndent(bundle.Manifest, "", "  ")
	runnerJSON, _ := json.MarshalIndent(runner, "", "  ")
	policyJSON, _ := json.MarshalIndent(policy, "", "  ")
	spec := BuildMinimalReproPackageSpec(bundle, runner)
	script := "#!/usr/bin/env bash\nset -euo pipefail\ngo run ./cmd/rvsmoke -bundle diagnostic-bundle.json -policy ci-policy.json -out md\n"
	return map[string]string{
		"README.md":                MinimalReproPackageMarkdown(spec),
		"diagnostic-bundle.json":   rawBundle,
		"manifest.json":            string(manifestJSON),
		"runner-spec.json":         string(runnerJSON),
		"ci-policy.json":           string(policyJSON),
		"ci-summary.json":          string(summaryJSON),
		"policy-violation-tree.md": PolicyViolationTreeMarkdown(tree),
		"history.txt":              BundleHistoryAggregateString(history),
		"scripts/rvsmoke.sh":       script,
	}
}

func compactNonEmpty(in []string) []string {
	out := []string{}
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
func severityToStatus(s string) string {
	switch strings.ToLower(s) {
	case "error", "critical":
		return "fail"
	case "warn", "warning":
		return "warn"
	default:
		return "info"
	}
}
func driftStatus(s string) string {
	switch strings.ToLower(s) {
	case "removed", "parse-error":
		return "fail"
	case "changed", "added", "missing-baseline":
		return "warn"
	default:
		return "info"
	}
}
func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() == 0 || strings.HasSuffix(b.String(), "-") == false {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}
