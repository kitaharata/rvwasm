package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
)

// ReproZipChecksumEntry is a deterministic checksum row for one file in a
// minimal reproduction package. It is intentionally independent of ZIP metadata
// such as timestamps so the manifest remains stable across packaging tools.
type ReproZipChecksumEntry struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Required bool   `json:"required"`
}

// ReproZipChecksumManifest is a compact bill of materials for a reproduction
// ZIP. It can be committed next to a package, uploaded as a CI artifact, or used
// to compare two packages without extracting them.
type ReproZipChecksumManifest struct {
	Status         string                  `json:"status"`
	ZipSHA256      string                  `json:"zip_sha256,omitempty"`
	FileCount      int                     `json:"file_count"`
	RequiredCount  int                     `json:"required_count"`
	TotalBytes     int64                   `json:"total_bytes"`
	Missing        []string                `json:"missing,omitempty"`
	Entries        []ReproZipChecksumEntry `json:"entries"`
	ManifestSHA256 string                  `json:"manifest_sha256,omitempty"`
}

func BuildReproZipChecksumManifest(insp ReproZipInspection) ReproZipChecksumManifest {
	m := ReproZipChecksumManifest{Status: firstNonEmpty(insp.Status, "unknown"), ZipSHA256: insp.ZipSHA256, Missing: append([]string(nil), insp.Missing...)}
	for _, f := range insp.Files {
		e := ReproZipChecksumEntry{Path: f.Path, Bytes: f.Bytes, SHA256: f.SHA256, Required: f.Required}
		m.Entries = append(m.Entries, e)
		m.FileCount++
		m.TotalBytes += f.Bytes
		if f.Required {
			m.RequiredCount++
		}
	}
	sort.SliceStable(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	sort.Strings(m.Missing)
	// Hash only the deterministic content fields and not ManifestSHA256 itself.
	clone := m
	clone.ManifestSHA256 = ""
	b, _ := json.Marshal(clone)
	sum := sha256.Sum256(b)
	m.ManifestSHA256 = hex.EncodeToString(sum[:])
	return m
}

func ReproZipChecksumManifestJSON(m ReproZipChecksumManifest) string {
	b, _ := json.MarshalIndent(m, "", "  ")
	return string(b)
}

func ReproZipChecksumManifestString(m ReproZipChecksumManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "repro checksum manifest status=%s files=%d required=%d bytes=%d zip=%s manifest=%s\n", firstNonEmpty(m.Status, "unknown"), m.FileCount, m.RequiredCount, m.TotalBytes, shortHash(m.ZipSHA256), shortHash(m.ManifestSHA256))
	if len(m.Missing) != 0 {
		b.WriteString("missing:\n")
		for _, x := range m.Missing {
			fmt.Fprintf(&b, "  - %s\n", x)
		}
	}
	for _, e := range m.Entries {
		req := "optional"
		if e.Required {
			req = "required"
		}
		fmt.Fprintf(&b, "  %-30s %-8s bytes=%-8d sha=%s\n", e.Path, req, e.Bytes, shortHash(e.SHA256))
	}
	return b.String()
}

// MatrixResultInput names one rvsmoke JSON result to be aggregated.
type MatrixResultInput struct {
	Name string `json:"name"`
	JSON string `json:"-"`
}

type MatrixResultRow struct {
	Name               string            `json:"name"`
	Status             string            `json:"status"`
	ExitCode           int               `json:"exit_code,omitempty"`
	Policy             string            `json:"policy,omitempty"`
	Phase              string            `json:"phase,omitempty"`
	TopStopCause       string            `json:"top_stop_cause,omitempty"`
	IntegrityStatus    string            `json:"integrity_status,omitempty"`
	GateFailures       int               `json:"gate_failures,omitempty"`
	GateWarnings       int               `json:"gate_warnings,omitempty"`
	ArtifactMismatches int               `json:"artifact_mismatches,omitempty"`
	Counts             map[string]int    `json:"counts,omitempty"`
	ArtifactHashes     map[string]string `json:"artifact_hashes,omitempty"`
	Error              string            `json:"error,omitempty"`
}

type MatrixResultAggregate struct {
	Status   string            `json:"status"`
	Total    int               `json:"total"`
	Passed   int               `json:"passed"`
	Warnings int               `json:"warnings"`
	Failed   int               `json:"failed"`
	Rows     []MatrixResultRow `json:"rows"`
	Summary  []string          `json:"summary,omitempty"`
}

func BuildMatrixResultAggregate(inputs []MatrixResultInput) MatrixResultAggregate {
	a := MatrixResultAggregate{Status: "pass"}
	for i, in := range inputs {
		name := firstNonEmpty(strings.TrimSpace(in.Name), fmt.Sprintf("result-%d", i+1))
		row := parseMatrixResultRow(name, in.JSON)
		a.Rows = append(a.Rows, row)
		a.Total++
		switch strings.ToLower(row.Status) {
		case "pass", "ok":
			a.Passed++
		case "warn", "warning":
			a.Warnings++
			if a.Status == "pass" {
				a.Status = "warn"
			}
		default:
			a.Failed++
			a.Status = "fail"
		}
	}
	sort.SliceStable(a.Rows, func(i, j int) bool {
		rank := map[string]int{"fail": 0, "error": 0, "warn": 1, "warning": 1, "pass": 2, "ok": 2}
		ri, rj := rank[strings.ToLower(a.Rows[i].Status)], rank[strings.ToLower(a.Rows[j].Status)]
		if ri == rj {
			return a.Rows[i].Name < a.Rows[j].Name
		}
		return ri < rj
	})
	a.Summary = append(a.Summary, fmt.Sprintf("total=%d pass=%d warn=%d fail=%d", a.Total, a.Passed, a.Warnings, a.Failed))
	for _, r := range a.Rows {
		if strings.ToLower(r.Status) == "fail" || strings.ToLower(r.Status) == "error" {
			a.Summary = append(a.Summary, fmt.Sprintf("%s failed: %s", r.Name, firstNonEmpty(firstNonEmpty(r.TopStopCause, r.Error), "ci gate")))
		}
	}
	if a.Total == 0 {
		a.Status = "empty"
		a.Summary = []string{"no matrix result files supplied"}
	}
	return a
}

func parseMatrixResultRow(name, text string) MatrixResultRow {
	row := MatrixResultRow{Name: name, Status: "fail", Counts: map[string]int{}, ArtifactHashes: map[string]string{}}
	if strings.TrimSpace(text) == "" {
		row.Error = "empty JSON"
		return row
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		row.Error = err.Error()
		return row
	}
	if raw, ok := root["ci"]; ok {
		var ci struct {
			Status   string         `json:"status"`
			ExitCode int            `json:"exit_code"`
			Phase    string         `json:"phase"`
			Counts   map[string]int `json:"counts"`
		}
		_ = json.Unmarshal(raw, &ci)
		row.Status = firstNonEmpty(ci.Status, row.Status)
		row.ExitCode = ci.ExitCode
		row.Phase = ci.Phase
		for k, v := range ci.Counts {
			row.Counts[k] = v
		}
	}
	if raw, ok := root["gate"]; ok {
		var gate struct {
			Status     string        `json:"status"`
			ExitCode   int           `json:"exit_code"`
			PolicyName string        `json:"policy_name"`
			Checks     []CIGateCheck `json:"checks"`
		}
		_ = json.Unmarshal(raw, &gate)
		row.Status = worstStatus(row.Status, gate.Status)
		if gate.ExitCode != 0 {
			row.ExitCode = gate.ExitCode
		}
		row.Policy = gate.PolicyName
		for _, c := range gate.Checks {
			switch c.Status {
			case "fail":
				row.GateFailures++
			case "warn":
				row.GateWarnings++
			}
			if c.Name == "top_stop_cause" && c.Observed != "" {
				row.TopStopCause = c.Observed
			}
		}
	}
	if raw, ok := root["integrity"]; ok {
		var integ struct {
			Status   string           `json:"status"`
			Manifest ArtifactManifest `json:"manifest"`
			Counts   map[string]int   `json:"counts"`
		}
		_ = json.Unmarshal(raw, &integ)
		row.IntegrityStatus = integ.Status
		row.Status = worstStatus(row.Status, integ.Status)
		for _, a := range integ.Manifest.Artifacts {
			row.ArtifactHashes[a.Role] = shortHash(a.SHA256)
		}
		for k, v := range integ.Counts {
			row.Counts["integrity_"+k] = v
		}
	}
	if raw, ok := root["artifact_checks"]; ok {
		var checks []ArtifactIntegrityRow
		_ = json.Unmarshal(raw, &checks)
		for _, c := range checks {
			if !c.LooksValid {
				row.ArtifactMismatches++
			}
		}
		if row.ArtifactMismatches != 0 {
			row.Status = "fail"
		}
	}
	if row.ExitCode != 0 {
		row.Status = "fail"
	}
	return row
}

func worstStatus(a, b string) string {
	rank := func(s string) int {
		switch strings.ToLower(s) {
		case "fail", "error":
			return 3
		case "warn", "warning":
			return 2
		case "pass", "ok":
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

func MatrixResultAggregateJSON(a MatrixResultAggregate) string {
	b, _ := json.MarshalIndent(a, "", "  ")
	return string(b)
}

func MatrixResultAggregateString(a MatrixResultAggregate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "matrix aggregate status=%s total=%d pass=%d warn=%d fail=%d\n", a.Status, a.Total, a.Passed, a.Warnings, a.Failed)
	for _, s := range a.Summary {
		fmt.Fprintf(&b, "summary: %s\n", s)
	}
	for _, r := range a.Rows {
		fmt.Fprintf(&b, "  %-16s %-5s policy=%s phase=%s cause=%s gate_fail=%d gate_warn=%d artifact_mismatch=%d\n", r.Name, r.Status, firstNonEmpty(r.Policy, "-"), firstNonEmpty(r.Phase, "-"), firstNonEmpty(r.TopStopCause, "-"), r.GateFailures, r.GateWarnings, r.ArtifactMismatches)
		if r.Error != "" {
			fmt.Fprintf(&b, "      error: %s\n", r.Error)
		}
	}
	return b.String()
}

func MatrixResultAggregateMarkdown(a MatrixResultAggregate) string {
	var b strings.Builder
	b.WriteString("# rvsmoke matrix aggregate\n\n")
	fmt.Fprintf(&b, "- Status: `%s`\n- Total: `%d`\n- Pass: `%d`\n- Warn: `%d`\n- Fail: `%d`\n\n", a.Status, a.Total, a.Passed, a.Warnings, a.Failed)
	b.WriteString("| Name | Status | Policy | Phase | Top cause | Gate fail | Gate warn | Artifact mismatches |\n|---|---|---|---|---|---:|---:|---:|\n")
	for _, r := range a.Rows {
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s | %d | %d | %d |\n", mdCell(r.Name), r.Status, mdCell(r.Policy), mdCell(r.Phase), mdCell(r.TopStopCause), r.GateFailures, r.GateWarnings, r.ArtifactMismatches)
	}
	return b.String()
}

func MatrixResultAggregateHTML(a MatrixResultAggregate) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvsmoke matrix aggregate</title><style>body{font-family:system-ui,sans-serif;margin:2rem}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:.35rem .5rem}.fail{background:#ffe6e6}.warn{background:#fff6cc}.pass{background:#e8ffe8}code{background:#f4f4f4;padding:.1rem .25rem}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	fmt.Fprintf(&b, "<h1>rvsmoke matrix aggregate</h1><p>Status: <code>%s</code>; total=%d pass=%d warn=%d fail=%d</p>", html.EscapeString(a.Status), a.Total, a.Passed, a.Warnings, a.Failed)
	b.WriteString("<table><thead><tr><th>Name</th><th>Status</th><th>Policy</th><th>Phase</th><th>Top cause</th><th>Gate fail</th><th>Gate warn</th><th>Artifact mismatches</th></tr></thead><tbody>")
	for _, r := range a.Rows {
		cls := strings.ToLower(r.Status)
		if cls != "fail" && cls != "warn" && cls != "pass" {
			cls = ""
		}
		fmt.Fprintf(&b, "<tr class=%q><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td></tr>", cls, html.EscapeString(r.Name), html.EscapeString(r.Status), html.EscapeString(r.Policy), html.EscapeString(r.Phase), html.EscapeString(r.TopStopCause), r.GateFailures, r.GateWarnings, r.ArtifactMismatches)
	}
	b.WriteString("</tbody></table><h2>JSON payload</h2><pre>")
	b.WriteString(html.EscapeString(MatrixResultAggregateJSON(a)))
	b.WriteString("</pre>")
	return b.String()
}

func BundleTrendReportStandaloneHTML(t BundleTrendReport) string {
	return BundleTrendReportHTML(t)
}
