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

// CIArtifactInput is an in-memory file entry that should appear in a CI artifact
// index. The caller owns reading the bytes so this package remains easy to test.
type CIArtifactInput struct {
	Path     string
	Kind     string
	Data     []byte
	Required bool
}

// CIArtifactIndexEntry records a deterministic checksum row for a generated CI
// artifact such as JUnit XML, SARIF, trend CSV, matrix HTML, or a repro ZIP.
type CIArtifactIndexEntry struct {
	Path     string `json:"path"`
	Kind     string `json:"kind,omitempty"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
	Required bool   `json:"required,omitempty"`
}

// CIArtifactIndex is a stable, content-addressed list of output artifacts. It is
// intended to be uploaded next to rvsmoke reports so later handoff can verify
// exactly which files were produced by a CI run.
type CIArtifactIndex struct {
	Status      string                 `json:"status"`
	FileCount   int                    `json:"file_count"`
	TotalBytes  int                    `json:"total_bytes"`
	Kinds       map[string]int         `json:"kinds,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
	Entries     []CIArtifactIndexEntry `json:"entries"`
	IndexSHA256 string                 `json:"index_sha256,omitempty"`
}

func BuildCIArtifactIndex(inputs []CIArtifactInput) CIArtifactIndex {
	idx := CIArtifactIndex{Status: "ok", Kinds: map[string]int{}}
	seen := map[string]bool{}
	for _, in := range inputs {
		path := cleanArtifactPath(in.Path)
		kind := strings.TrimSpace(in.Kind)
		if kind == "" {
			kind = inferArtifactKind(path)
		}
		if path == "" {
			idx.Warnings = append(idx.Warnings, "artifact with empty path ignored")
			idx.Status = worstArtifactIndexStatus(idx.Status, "warn")
			continue
		}
		if seen[path] {
			idx.Warnings = append(idx.Warnings, "duplicate artifact path: "+path)
			idx.Status = worstArtifactIndexStatus(idx.Status, "warn")
		}
		seen[path] = true
		sum := sha256.Sum256(in.Data)
		idx.Entries = append(idx.Entries, CIArtifactIndexEntry{Path: path, Kind: kind, Bytes: len(in.Data), SHA256: hex.EncodeToString(sum[:]), Required: in.Required})
		idx.FileCount++
		idx.TotalBytes += len(in.Data)
		idx.Kinds[kind]++
		if in.Required && len(in.Data) == 0 {
			idx.Warnings = append(idx.Warnings, "required artifact is empty: "+path)
			idx.Status = worstArtifactIndexStatus(idx.Status, "warn")
		}
	}
	sort.SliceStable(idx.Entries, func(i, j int) bool { return idx.Entries[i].Path < idx.Entries[j].Path })
	sort.Strings(idx.Warnings)
	clone := idx
	clone.IndexSHA256 = ""
	b, _ := json.Marshal(clone)
	sum := sha256.Sum256(b)
	idx.IndexSHA256 = hex.EncodeToString(sum[:])
	return idx
}

func cleanArtifactPath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.TrimPrefix(p, "./")
}

func inferArtifactKind(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.HasSuffix(p, ".xml"):
		return "junit"
	case strings.HasSuffix(p, ".sarif"):
		return "sarif"
	case strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".htm"):
		return "html"
	case strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".markdown"):
		return "markdown"
	case strings.HasSuffix(p, ".csv"):
		return "csv"
	case strings.HasSuffix(p, ".zip"):
		return "zip"
	case strings.HasSuffix(p, ".json"):
		return "json"
	case strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml"):
		return "workflow"
	default:
		return "artifact"
	}
}

func worstArtifactIndexStatus(a, b string) string {
	rank := map[string]int{"ok": 1, "pass": 1, "warn": 2, "fail": 3, "error": 3}
	if rank[strings.ToLower(b)] > rank[strings.ToLower(a)] {
		return b
	}
	if a == "" {
		return b
	}
	return a
}

func CIArtifactIndexJSON(idx CIArtifactIndex) string {
	b, _ := json.MarshalIndent(idx, "", "  ")
	return string(b)
}

func CIArtifactIndexString(idx CIArtifactIndex) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ci artifact index status=%s files=%d bytes=%d sha=%s\n", idx.Status, idx.FileCount, idx.TotalBytes, shortHash(idx.IndexSHA256))
	if len(idx.Kinds) != 0 {
		keys := make([]string, 0, len(idx.Kinds))
		for k := range idx.Kinds {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  kind %-12s %d\n", k, idx.Kinds[k])
		}
	}
	for _, w := range idx.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	for _, e := range idx.Entries {
		req := ""
		if e.Required {
			req = " required"
		}
		fmt.Fprintf(&b, "  %-36s kind=%-10s bytes=%-8d sha=%s%s\n", e.Path, e.Kind, e.Bytes, shortHash(e.SHA256), req)
	}
	return b.String()
}

// ReproZipChecksumVerification compares a current repro ZIP checksum manifest
// with a previously saved manifest. It catches package drift without requiring
// the full ZIP contents in a review comment.
type ReproZipChecksumVerification struct {
	Status  string   `json:"status"`
	Checked int      `json:"checked"`
	Missing []string `json:"missing,omitempty"`
	Changed []string `json:"changed,omitempty"`
	Extra   []string `json:"extra,omitempty"`
	Issues  []string `json:"issues,omitempty"`
}

func VerifyReproZipChecksumManifest(current ReproZipChecksumManifest, baselineText string) ReproZipChecksumVerification {
	v := ReproZipChecksumVerification{Status: "pass"}
	baselineText = strings.TrimSpace(baselineText)
	if baselineText == "" {
		v.Status = "warn"
		v.Issues = append(v.Issues, "no baseline checksum manifest provided")
		return v
	}
	var base ReproZipChecksumManifest
	if err := json.Unmarshal([]byte(baselineText), &base); err != nil {
		v.Status = "fail"
		v.Issues = append(v.Issues, "cannot parse baseline checksum manifest: "+err.Error())
		return v
	}
	baseMap := map[string]ReproZipChecksumEntry{}
	curMap := map[string]ReproZipChecksumEntry{}
	for _, e := range base.Entries {
		baseMap[e.Path] = e
	}
	for _, e := range current.Entries {
		curMap[e.Path] = e
	}
	for p, b := range baseMap {
		c, ok := curMap[p]
		if !ok {
			v.Missing = append(v.Missing, p)
			continue
		}
		v.Checked++
		if b.SHA256 != c.SHA256 || b.Bytes != c.Bytes || b.Required != c.Required {
			v.Changed = append(v.Changed, fmt.Sprintf("%s bytes %d→%d sha %s→%s", p, b.Bytes, c.Bytes, shortHash(b.SHA256), shortHash(c.SHA256)))
		}
	}
	for p := range curMap {
		if _, ok := baseMap[p]; !ok {
			v.Extra = append(v.Extra, p)
		}
	}
	sort.Strings(v.Missing)
	sort.Strings(v.Changed)
	sort.Strings(v.Extra)
	if len(v.Missing)+len(v.Changed) != 0 {
		v.Status = "fail"
	} else if len(v.Extra) != 0 || base.ManifestSHA256 != "" && current.ManifestSHA256 != "" && base.ManifestSHA256 != current.ManifestSHA256 {
		v.Status = "warn"
	}
	return v
}

func ReproZipChecksumVerificationJSON(v ReproZipChecksumVerification) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func ReproZipChecksumVerificationString(v ReproZipChecksumVerification) string {
	var b strings.Builder
	fmt.Fprintf(&b, "repro checksum verification status=%s checked=%d missing=%d changed=%d extra=%d\n", v.Status, v.Checked, len(v.Missing), len(v.Changed), len(v.Extra))
	for _, x := range v.Issues {
		fmt.Fprintf(&b, "issue: %s\n", x)
	}
	for _, x := range v.Missing {
		fmt.Fprintf(&b, "missing: %s\n", x)
	}
	for _, x := range v.Changed {
		fmt.Fprintf(&b, "changed: %s\n", x)
	}
	for _, x := range v.Extra {
		fmt.Fprintf(&b, "extra: %s\n", x)
	}
	return b.String()
}

// MatrixFlakeGroup reports instability for repeated matrix runs that share a
// normalized key. Names such as uart#1, uart#2, uart@nightly, and uart-run2 are
// treated as the same key.
type MatrixFlakeGroup struct {
	Key      string            `json:"key"`
	Total    int               `json:"total"`
	Pass     int               `json:"pass"`
	Warn     int               `json:"warn"`
	Fail     int               `json:"fail"`
	Flaky    bool              `json:"flaky"`
	Statuses map[string]int    `json:"statuses,omitempty"`
	Rows     []MatrixResultRow `json:"rows,omitempty"`
}

type MatrixFlakeReport struct {
	Status string             `json:"status"`
	Total  int                `json:"total"`
	Flakes int                `json:"flakes"`
	Groups []MatrixFlakeGroup `json:"groups,omitempty"`
}

func BuildMatrixFlakeReport(a MatrixResultAggregate) MatrixFlakeReport {
	r := MatrixFlakeReport{Status: "pass"}
	groups := map[string]*MatrixFlakeGroup{}
	for _, row := range a.Rows {
		key := matrixFlakeKey(row.Name)
		g := groups[key]
		if g == nil {
			g = &MatrixFlakeGroup{Key: key, Statuses: map[string]int{}}
			groups[key] = g
		}
		g.Total++
		g.Rows = append(g.Rows, row)
		status := canonicalMatrixStatus(row.Status)
		g.Statuses[status]++
		switch status {
		case "pass":
			g.Pass++
		case "warn":
			g.Warn++
		default:
			g.Fail++
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := *groups[k]
		if g.Total > 1 && countNonZero(g.Pass, g.Warn, g.Fail) > 1 {
			g.Flaky = true
			r.Flakes++
			r.Status = "warn"
		}
		r.Total += g.Total
		r.Groups = append(r.Groups, g)
	}
	if r.Total == 0 {
		r.Status = "empty"
	}
	return r
}

func matrixFlakeKey(name string) string {
	n := strings.TrimSpace(strings.ToLower(name))
	for _, sep := range []string{"#", "@", ":"} {
		if i := strings.Index(n, sep); i >= 0 {
			n = n[:i]
		}
	}
	for _, suffix := range []string{"-run", "_run", "-attempt", "_attempt"} {
		if i := strings.LastIndex(n, suffix); i >= 0 {
			trail := n[i+len(suffix):]
			if trail == "" || allDigits(trail) {
				n = n[:i]
			}
		}
	}
	if n == "" {
		return "unnamed"
	}
	return n
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func canonicalMatrixStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "pass", "ok":
		return "pass"
	case "warn", "warning":
		return "warn"
	default:
		return "fail"
	}
}

func countNonZero(xs ...int) int {
	c := 0
	for _, x := range xs {
		if x != 0 {
			c++
		}
	}
	return c
}

func MatrixFlakeReportJSON(r MatrixFlakeReport) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

func MatrixFlakeReportString(r MatrixFlakeReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "matrix flake report status=%s total=%d flaky_groups=%d\n", r.Status, r.Total, r.Flakes)
	for _, g := range r.Groups {
		flag := "stable"
		if g.Flaky {
			flag = "flaky"
		}
		fmt.Fprintf(&b, "  %-16s %-6s total=%d pass=%d warn=%d fail=%d\n", g.Key, flag, g.Total, g.Pass, g.Warn, g.Fail)
	}
	return b.String()
}

func MatrixFlakeReportHTML(r MatrixFlakeReport) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvsmoke matrix flakes</title><style>body{font-family:system-ui,sans-serif;margin:2rem}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:.35rem .5rem}.flaky{background:#fff3cd}.stable{background:#e8ffe8}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	fmt.Fprintf(&b, "<h1>rvsmoke matrix flakes</h1><p>Status: <code>%s</code>; total=%d; flaky groups=%d</p>", html.EscapeString(r.Status), r.Total, r.Flakes)
	b.WriteString("<table><thead><tr><th>Key</th><th>State</th><th>Total</th><th>Pass</th><th>Warn</th><th>Fail</th></tr></thead><tbody>")
	for _, g := range r.Groups {
		cls := "stable"
		state := "stable"
		if g.Flaky {
			cls, state = "flaky", "flaky"
		}
		fmt.Fprintf(&b, "<tr class=%q><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td></tr>", cls, html.EscapeString(g.Key), state, g.Total, g.Pass, g.Warn, g.Fail)
	}
	b.WriteString("</tbody></table><h2>JSON</h2><pre>")
	b.WriteString(html.EscapeString(MatrixFlakeReportJSON(r)))
	b.WriteString("</pre>")
	return b.String()
}

// ReleaseBundleManifest ties together the input diagnostic bundle and the CI
// artifacts emitted around it. It is the top-level handoff object for a CI run.
type ReleaseBundleManifest struct {
	SchemaVersion        string                       `json:"schema_version"`
	Status               string                       `json:"status"`
	BundleSHA256         string                       `json:"bundle_sha256,omitempty"`
	ManifestSHA256       string                       `json:"manifest_sha256,omitempty"`
	BootArgs             string                       `json:"boot_args,omitempty"`
	HartCount            int                          `json:"hart_count,omitempty"`
	TopStopCause         string                       `json:"top_stop_cause,omitempty"`
	TraceSHA256          string                       `json:"trace_sha256,omitempty"`
	ConsoleSHA256        string                       `json:"console_sha256,omitempty"`
	CIStatus             string                       `json:"ci_status,omitempty"`
	GateStatus           string                       `json:"gate_status,omitempty"`
	MatrixStatus         string                       `json:"matrix_status,omitempty"`
	FlakeStatus          string                       `json:"flake_status,omitempty"`
	ArtifactIndex        CIArtifactIndex              `json:"artifact_index,omitempty"`
	ReproChecksums       ReproZipChecksumManifest     `json:"repro_zip_checksums,omitempty"`
	ChecksumVerification ReproZipChecksumVerification `json:"checksum_verification,omitempty"`
	Notes                []string                     `json:"notes,omitempty"`
}

func BuildReleaseBundleManifest(bundle DiagnosticBundle, raw string, sig LogSignatureSet, ci CISummary, gate CIGateReport, index CIArtifactIndex, matrix MatrixResultAggregate, flakes MatrixFlakeReport, checksums ReproZipChecksumManifest, verify ReproZipChecksumVerification) ReleaseBundleManifest {
	status := "pass"
	for _, s := range []string{ci.Status, gate.Status, index.Status, matrix.Status, flakes.Status, verify.Status} {
		status = releaseWorstStatus(status, s)
	}
	m := ReleaseBundleManifest{
		SchemaVersion:        "rvwasm.release.v1",
		Status:               status,
		BundleSHA256:         shaText(firstNonEmpty(raw, DiagnosticBundleJSON(bundle))),
		ManifestSHA256:       shaText(ArtifactManifestJSON(bundle.Manifest)),
		BootArgs:             bundle.Manifest.BootArgs,
		HartCount:            bundle.Manifest.HartCount,
		TopStopCause:         bundleTopCause(bundle),
		TraceSHA256:          sig.TraceSHA256,
		ConsoleSHA256:        sig.ConsoleSHA256,
		CIStatus:             ci.Status,
		GateStatus:           gate.Status,
		MatrixStatus:         matrix.Status,
		FlakeStatus:          flakes.Status,
		ArtifactIndex:        index,
		ReproChecksums:       checksums,
		ChecksumVerification: verify,
	}
	if index.FileCount == 0 {
		m.Notes = append(m.Notes, "no generated CI artifacts were indexed")
	}
	if flakes.Flakes != 0 {
		m.Notes = append(m.Notes, fmt.Sprintf("matrix has %d flaky group(s)", flakes.Flakes))
	}
	return m
}

func releaseWorstStatus(a, b string) string {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if b == "" || b == "empty" {
		return firstNonEmpty(a, "pass")
	}
	if b == "error" {
		b = "fail"
	}
	if a == "error" {
		a = "fail"
	}
	rank := map[string]int{"pass": 1, "ok": 1, "warn": 2, "warning": 2, "fail": 3}
	if rank[b] > rank[a] {
		return b
	}
	if a == "" {
		return b
	}
	return a
}

func ReleaseBundleManifestJSON(m ReleaseBundleManifest) string {
	b, _ := json.MarshalIndent(m, "", "  ")
	return string(b)
}

func ReleaseBundleManifestString(m ReleaseBundleManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release bundle manifest status=%s bundle=%s manifest=%s\n", m.Status, shortHash(m.BundleSHA256), shortHash(m.ManifestSHA256))
	fmt.Fprintf(&b, "ci=%s gate=%s matrix=%s flakes=%s top=%s\n", firstNonEmpty(m.CIStatus, "-"), firstNonEmpty(m.GateStatus, "-"), firstNonEmpty(m.MatrixStatus, "-"), firstNonEmpty(m.FlakeStatus, "-"), firstNonEmpty(m.TopStopCause, "-"))
	fmt.Fprintf(&b, "boot harts=%d args=%s\n", m.HartCount, m.BootArgs)
	if len(m.Notes) != 0 {
		for _, n := range m.Notes {
			fmt.Fprintf(&b, "note: %s\n", n)
		}
	}
	b.WriteString(CIArtifactIndexString(m.ArtifactIndex))
	if m.ChecksumVerification.Status != "" {
		b.WriteString(ReproZipChecksumVerificationString(m.ChecksumVerification))
	}
	return b.String()
}

func ReleaseBundleManifestHTML(m ReleaseBundleManifest, matrix MatrixResultAggregate, flakes MatrixFlakeReport) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm release manifest</title><style>body{font-family:system-ui,sans-serif;margin:2rem;line-height:1.4}nav{position:sticky;top:0;background:white;border-bottom:1px solid #ddd;padding:.5rem 0}nav a{margin-right:1rem}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:.35rem .5rem}code{background:#f4f4f4;padding:.1rem .25rem}.fail{background:#ffe6e6}.warn{background:#fff6cc}.pass,.ok{background:#e8ffe8}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	b.WriteString("<nav><a href=\"#summary\">Summary</a><a href=\"#artifacts\">Artifacts</a><a href=\"#matrix\">Matrix</a><a href=\"#checksums\">Checksums</a><a href=\"#json\">JSON</a></nav>")
	fmt.Fprintf(&b, "<h1 id=\"summary\">rvwasm release manifest</h1><p>Status: <code class=%q>%s</code></p>", html.EscapeString(m.Status), html.EscapeString(m.Status))
	fmt.Fprintf(&b, "<p>Bundle <code>%s</code>; manifest <code>%s</code>; top stop-cause <code>%s</code></p>", html.EscapeString(shortHash(m.BundleSHA256)), html.EscapeString(shortHash(m.ManifestSHA256)), html.EscapeString(firstNonEmpty(m.TopStopCause, "-")))
	fmt.Fprintf(&b, "<p>CI <code>%s</code>; gate <code>%s</code>; matrix <code>%s</code>; flakes <code>%s</code></p>", html.EscapeString(firstNonEmpty(m.CIStatus, "-")), html.EscapeString(firstNonEmpty(m.GateStatus, "-")), html.EscapeString(firstNonEmpty(m.MatrixStatus, "-")), html.EscapeString(firstNonEmpty(m.FlakeStatus, "-")))
	b.WriteString("<h2 id=\"artifacts\">CI artifact index</h2><table><tr><th>Path</th><th>Kind</th><th>Bytes</th><th>SHA-256</th></tr>")
	for _, e := range m.ArtifactIndex.Entries {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%d</td><td><code>%s</code></td></tr>", html.EscapeString(e.Path), html.EscapeString(e.Kind), e.Bytes, html.EscapeString(shortHash(e.SHA256)))
	}
	b.WriteString("</table>")
	b.WriteString("<h2 id=\"matrix\">Matrix</h2><pre>")
	b.WriteString(html.EscapeString(MatrixResultAggregateString(matrix)))
	b.WriteString("</pre><h3>Flakes</h3><pre>")
	b.WriteString(html.EscapeString(MatrixFlakeReportString(flakes)))
	b.WriteString("</pre><h2 id=\"checksums\">Repro checksum verification</h2><pre>")
	b.WriteString(html.EscapeString(ReproZipChecksumVerificationString(m.ChecksumVerification)))
	b.WriteString("</pre><h2 id=\"json\">JSON</h2><pre>")
	b.WriteString(html.EscapeString(ReleaseBundleManifestJSON(m)))
	b.WriteString("</pre>")
	return b.String()
}
