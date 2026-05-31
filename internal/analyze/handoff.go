package analyze

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// ParseDiagnosticBundleText accepts either a plain DiagnosticBundle JSON object,
// a CompressedDiagnosticBundle JSON object, or a raw gzip+base64 payload. This
// makes handoff reports copy/paste friendly when users exchange compressed
// bundles in issues or chats.
func ParseDiagnosticBundleText(text string) (DiagnosticBundle, string, error) {
	raw, err := DecodeDiagnosticBundleText(text)
	if err != nil {
		return DiagnosticBundle{}, "", err
	}
	var b DiagnosticBundle
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return DiagnosticBundle{}, raw, err
	}
	return b, raw, nil
}

func DecodeDiagnosticBundleText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("empty bundle text")
	}
	if strings.HasPrefix(text, "{") {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &probe); err != nil {
			return "", err
		}
		if _, ok := probe["manifest"]; ok {
			return text, nil
		}
		if raw, ok := probe["base64"]; ok {
			var encoded string
			if err := json.Unmarshal(raw, &encoded); err != nil {
				return "", err
			}
			return gunzipBase64(encoded)
		}
	}
	return gunzipBase64(text)
}

func gunzipBase64(encoded string) (string, error) {
	encoded = compactBase64(encoded)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	out, err := io.ReadAll(gz)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func compactBase64(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func DecodeDiagnosticBundleJSONString(text string) string {
	raw, err := DecodeDiagnosticBundleText(text)
	if err != nil {
		return `{"error":` + strconv.Quote(err.Error()) + `}`
	}
	var pretty any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		return raw
	}
	b, _ := json.MarshalIndent(pretty, "", "  ")
	return string(b)
}

// DiagnosticBundleDiff is a concise current-vs-baseline comparison intended for
// regression handoff. It deliberately summarizes by stable names rather than raw
// object order.
type DiagnosticBundleDiff struct {
	Area   string `json:"area"`
	Key    string `json:"key"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Kind   string `json:"kind"`
}

func CompareDiagnosticBundles(current DiagnosticBundle, baselineText string) []DiagnosticBundleDiff {
	baselineText = strings.TrimSpace(baselineText)
	if baselineText == "" {
		return []DiagnosticBundleDiff{{Area: "$", Key: "bundle", Kind: "missing-baseline", After: "current bundle only"}}
	}
	base, baseJSON, err := ParseDiagnosticBundleText(baselineText)
	if err != nil {
		return []DiagnosticBundleDiff{{Area: "$", Key: "bundle", Kind: "parse-error", Before: err.Error()}}
	}
	_ = baseJSON
	rows := []DiagnosticBundleDiff{}
	add := func(area, key, before, after string) {
		if before != after {
			rows = append(rows, DiagnosticBundleDiff{Area: area, Key: key, Before: before, After: after, Kind: "changed"})
		}
	}
	add("triage", "status", base.Triage.Status, current.Triage.Status)
	add("triage", "phase", base.Triage.Phase, current.Triage.Phase)
	add("triage", "top_cause", bundleTopCause(base), bundleTopCause(current))
	add("manifest", "boot_args", base.Manifest.BootArgs, current.Manifest.BootArgs)
	add("manifest", "hart_count", strconv.Itoa(base.Manifest.HartCount), strconv.Itoa(current.Manifest.HartCount))
	add("manifest", "next_addr", fmt.Sprintf("%#x", base.Manifest.NextAddr), fmt.Sprintf("%#x", current.Manifest.NextAddr))
	diffArtifactHashes(base.Manifest.Artifacts, current.Manifest.Artifacts, &rows)
	diffStringCounts("stop_causes", countStopCauses(base.StopCauses), countStopCauses(current.StopCauses), &rows)
	diffStringCounts("smoke_clusters", countSmokeClusters(base.Clusters), countSmokeClusters(current.Clusters), &rows)
	add("watchpoints", "hit_count", strconv.Itoa(len(base.Watches)), strconv.Itoa(len(current.Watches)))
	add("suggestions", "count", strconv.Itoa(len(base.Suggestions)), strconv.Itoa(len(current.Suggestions)))
	if len(rows) == 0 {
		rows = append(rows, DiagnosticBundleDiff{Area: "$", Key: "bundle", Kind: "unchanged"})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		order := map[string]int{"parse-error": 0, "missing-baseline": 1, "changed": 2, "added": 3, "removed": 4, "unchanged": 5}
		if order[rows[i].Kind] == order[rows[j].Kind] {
			if rows[i].Area == rows[j].Area {
				return rows[i].Key < rows[j].Key
			}
			return rows[i].Area < rows[j].Area
		}
		return order[rows[i].Kind] < order[rows[j].Kind]
	})
	return rows
}

func diffArtifactHashes(a, b []ArtifactEntry, rows *[]DiagnosticBundleDiff) {
	ma, mb := map[string]ArtifactEntry{}, map[string]ArtifactEntry{}
	seen := map[string]bool{}
	for _, x := range a {
		ma[x.Role] = x
		seen[x.Role] = true
	}
	for _, x := range b {
		mb[x.Role] = x
		seen[x.Role] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		x, okA := ma[k]
		y, okB := mb[k]
		switch {
		case !okA:
			*rows = append(*rows, DiagnosticBundleDiff{Area: "artifact", Key: k, Kind: "added", After: artifactBrief(y)})
		case !okB:
			*rows = append(*rows, DiagnosticBundleDiff{Area: "artifact", Key: k, Kind: "removed", Before: artifactBrief(x)})
		default:
			if x.SHA256 != y.SHA256 || x.Bytes != y.Bytes || x.LoadAddr != y.LoadAddr || x.Entry != y.Entry {
				*rows = append(*rows, DiagnosticBundleDiff{Area: "artifact", Key: k, Kind: "changed", Before: artifactBrief(x), After: artifactBrief(y)})
			}
		}
	}
}

func countStopCauses(rows []StopCauseCandidate) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		out[r.Category]++
	}
	return out
}

func countSmokeClusters(rows []SmokeFailureCluster) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		out[r.Cause] += r.Count
	}
	return out
}

func diffStringCounts(area string, a, b map[string]int, rows *[]DiagnosticBundleDiff) {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if a[k] != b[k] {
			*rows = append(*rows, DiagnosticBundleDiff{Area: area, Key: k, Before: strconv.Itoa(a[k]), After: strconv.Itoa(b[k]), Kind: "changed"})
		}
	}
}

func bundleTopCause(b DiagnosticBundle) string {
	if len(b.StopCauses) != 0 {
		return b.StopCauses[0].Category
	}
	if len(b.Triage.TopCandidates) != 0 {
		return b.Triage.TopCandidates[0].Category
	}
	return ""
}

func DiagnosticBundleDiffString(rows []DiagnosticBundleDiff) string {
	if len(rows) == 0 {
		return "<no diagnostic bundle diff>\n"
	}
	var b strings.Builder
	b.WriteString("diagnostic bundle baseline diff\n")
	for _, r := range rows {
		switch r.Kind {
		case "unchanged", "missing-baseline", "parse-error":
			fmt.Fprintf(&b, "%s %s.%s %s\n", r.Kind, r.Area, r.Key, firstNonEmpty(r.After, r.Before))
		case "added":
			fmt.Fprintf(&b, "+ %-14s %-20s %s\n", r.Area, r.Key, r.After)
		case "removed":
			fmt.Fprintf(&b, "- %-14s %-20s %s\n", r.Area, r.Key, r.Before)
		default:
			fmt.Fprintf(&b, "* %-14s %-20s %s -> %s\n", r.Area, r.Key, r.Before, r.After)
		}
	}
	return b.String()
}

type ProvenanceReport struct {
	Tool              string `json:"tool"`
	GeneratedAt       string `json:"generated_at"`
	ManifestSHA256    string `json:"manifest_sha256"`
	BundleSHA256      string `json:"bundle_sha256,omitempty"`
	TraceSHA256       string `json:"trace_sha256,omitempty"`
	ConsoleSHA256     string `json:"console_sha256,omitempty"`
	TraceLines        int    `json:"trace_lines"`
	ConsoleBytes      int    `json:"console_bytes"`
	BootArgs          string `json:"boot_args"`
	HartCount         int    `json:"hart_count"`
	TopStopCause      string `json:"top_stop_cause,omitempty"`
	FirstArtifactHash string `json:"first_artifact_hash,omitempty"`
}

func BuildProvenanceReport(tool, generatedAt string, manifest ArtifactManifest, trace, console, bundleJSON string, stopCauses []StopCauseCandidate) ProvenanceReport {
	manifestJSON := ArtifactManifestJSON(manifest)
	p := ProvenanceReport{Tool: tool, GeneratedAt: generatedAt, ManifestSHA256: shaText(manifestJSON), BundleSHA256: shaText(bundleJSON), TraceSHA256: shaText(trace), ConsoleSHA256: shaText(console), TraceLines: countNonEmptyLines(trace), ConsoleBytes: len(console), BootArgs: manifest.BootArgs, HartCount: manifest.HartCount}
	if len(stopCauses) != 0 {
		p.TopStopCause = stopCauses[0].Category
	}
	if len(manifest.Artifacts) != 0 {
		p.FirstArtifactHash = shortHash(manifest.Artifacts[0].SHA256)
	}
	return p
}

func shaText(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func countNonEmptyLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func ProvenanceReportString(p ProvenanceReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "provenance tool=%s generated=%s\n", p.Tool, p.GeneratedAt)
	fmt.Fprintf(&b, "manifest sha256=%s\n", shortHash(p.ManifestSHA256))
	if p.BundleSHA256 != "" {
		fmt.Fprintf(&b, "bundle   sha256=%s\n", shortHash(p.BundleSHA256))
	}
	if p.TraceSHA256 != "" {
		fmt.Fprintf(&b, "trace    sha256=%s lines=%d\n", shortHash(p.TraceSHA256), p.TraceLines)
	}
	if p.ConsoleSHA256 != "" {
		fmt.Fprintf(&b, "console  sha256=%s bytes=%d\n", shortHash(p.ConsoleSHA256), p.ConsoleBytes)
	}
	fmt.Fprintf(&b, "harts=%d bootargs=%s\n", p.HartCount, p.BootArgs)
	if p.TopStopCause != "" {
		fmt.Fprintf(&b, "top stop cause=%s\n", p.TopStopCause)
	}
	return b.String()
}

func ProvenanceReportJSON(p ProvenanceReport) string {
	b, _ := json.MarshalIndent(p, "", "  ")
	return string(b)
}

func RegressionHandoffMarkdown(bundle DiagnosticBundle, provenance ProvenanceReport, baselineText string) string {
	var b strings.Builder
	b.WriteString("# rvwasm regression handoff\n\n")
	b.WriteString("## Provenance\n\n```text\n")
	b.WriteString(ProvenanceReportString(provenance))
	b.WriteString("```\n\n")
	b.WriteString("## Top stop causes\n\n")
	if len(bundle.StopCauses) == 0 {
		b.WriteString("_none_\n\n")
	} else {
		b.WriteString("| Rank | Severity | Score | Category | Summary |\n|---:|---|---:|---|---|\n")
		for _, c := range bundle.StopCauses {
			fmt.Fprintf(&b, "| %d | %s | %d | %s | %s |\n", c.Rank, mdCell(c.Severity), c.Score, mdCell(c.Category), mdCell(truncate(c.Summary, 160)))
		}
		b.WriteByte('\n')
	}
	b.WriteString("## Suggested breakpoints/watchpoints\n\n```text\n")
	b.WriteString(BreakpointSuggestionsString(bundle.Suggestions))
	b.WriteString("```\n\n")
	b.WriteString("## Stop checklist\n\n```text\n")
	b.WriteString(StopCauseChecklistString(bundle.StopCauses))
	b.WriteString("```\n\n")
	if strings.TrimSpace(baselineText) != "" {
		b.WriteString("## Baseline diff\n\n```text\n")
		b.WriteString(DiagnosticBundleDiffString(CompareDiagnosticBundles(bundle, baselineText)))
		b.WriteString("```\n\n")
	}
	b.WriteString("## Manifest\n\n```text\n")
	b.WriteString(ArtifactManifestString(bundle.Manifest))
	b.WriteString("```\n")
	return b.String()
}
