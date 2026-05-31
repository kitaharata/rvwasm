package analyze

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

// ArtifactManifestDiff reports reproducibility-affecting changes between two
// boot artifact manifests. It is intentionally textual and JSON-friendly so it
// can be copied into issue reports.
type ArtifactManifestDiff struct {
	Role   string `json:"role"`
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Kind   string `json:"kind"`
}

func CompareArtifactManifests(current ArtifactManifest, baselineJSON string) []ArtifactManifestDiff {
	baselineJSON = strings.TrimSpace(baselineJSON)
	if baselineJSON == "" {
		return []ArtifactManifestDiff{{Role: "$", Field: "manifest", Kind: "missing-baseline", After: "current manifest only"}}
	}
	var base ArtifactManifest
	if err := json.Unmarshal([]byte(baselineJSON), &base); err != nil {
		return []ArtifactManifestDiff{{Role: "$", Field: "manifest", Kind: "parse-error", Before: err.Error()}}
	}
	rows := []ArtifactManifestDiff{}
	add := func(role, field, before, after string) {
		if before != after {
			rows = append(rows, ArtifactManifestDiff{Role: role, Field: field, Before: before, After: after, Kind: "changed"})
		}
	}
	add("$", "boot_args", base.BootArgs, current.BootArgs)
	add("$", "hart_count", strconv.Itoa(base.HartCount), strconv.Itoa(current.HartCount))
	add("$", "next_addr", fmt.Sprintf("%#x", base.NextAddr), fmt.Sprintf("%#x", current.NextAddr))
	add("$", "dtb_addr", fmt.Sprintf("%#x", base.DTBAddr), fmt.Sprintf("%#x", current.DTBAddr))
	add("$", "dynamic_info_addr", fmt.Sprintf("%#x", base.DynamicInfoAddr), fmt.Sprintf("%#x", current.DynamicInfoAddr))

	byRole := map[string]ArtifactEntry{}
	seen := map[string]bool{}
	for _, a := range base.Artifacts {
		byRole[a.Role] = a
	}
	for _, a := range current.Artifacts {
		seen[a.Role] = true
		b, ok := byRole[a.Role]
		if !ok {
			rows = append(rows, ArtifactManifestDiff{Role: a.Role, Field: "artifact", Kind: "added", After: artifactBrief(a)})
			continue
		}
		add(a.Role, "bytes", strconv.Itoa(b.Bytes), strconv.Itoa(a.Bytes))
		add(a.Role, "range", fmt.Sprintf("%#x..%#x", b.LoadAddr, b.EndAddr), fmt.Sprintf("%#x..%#x", a.LoadAddr, a.EndAddr))
		add(a.Role, "entry", fmt.Sprintf("%#x", b.Entry), fmt.Sprintf("%#x", a.Entry))
		add(a.Role, "elf", strconv.FormatBool(b.ELF), strconv.FormatBool(a.ELF))
		add(a.Role, "sha256", shortHash(b.SHA256), shortHash(a.SHA256))
		add(a.Role, "note", b.Note, a.Note)
	}
	for _, b := range base.Artifacts {
		if !seen[b.Role] {
			rows = append(rows, ArtifactManifestDiff{Role: b.Role, Field: "artifact", Kind: "removed", Before: artifactBrief(b)})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, ArtifactManifestDiff{Role: "$", Field: "manifest", Kind: "unchanged"})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		order := map[string]int{"parse-error": 0, "missing-baseline": 1, "changed": 2, "added": 3, "removed": 4, "unchanged": 5}
		if order[rows[i].Kind] == order[rows[j].Kind] {
			if rows[i].Role == rows[j].Role {
				return rows[i].Field < rows[j].Field
			}
			return rows[i].Role < rows[j].Role
		}
		return order[rows[i].Kind] < order[rows[j].Kind]
	})
	return rows
}

func artifactBrief(a ArtifactEntry) string {
	return fmt.Sprintf("bytes=%d range=%#x..%#x entry=%#x elf=%v sha=%s", a.Bytes, a.LoadAddr, a.EndAddr, a.Entry, a.ELF, shortHash(a.SHA256))
}

func ArtifactManifestDiffString(rows []ArtifactManifestDiff) string {
	if len(rows) == 0 {
		return "<no artifact manifest diff>\n"
	}
	var b strings.Builder
	b.WriteString("artifact manifest diff\n")
	for _, r := range rows {
		switch r.Kind {
		case "unchanged", "missing-baseline", "parse-error":
			fmt.Fprintf(&b, "%s %s.%s %s\n", r.Kind, r.Role, r.Field, firstNonEmpty(r.After, r.Before))
		case "added":
			fmt.Fprintf(&b, "+ %-10s %-12s %s\n", r.Role, r.Field, r.After)
		case "removed":
			fmt.Fprintf(&b, "- %-10s %-12s %s\n", r.Role, r.Field, r.Before)
		default:
			fmt.Fprintf(&b, "* %-10s %-12s %s -> %s\n", r.Role, r.Field, r.Before, r.After)
		}
	}
	return b.String()
}

// BreakpointSuggestion is a copy/paste-friendly candidate produced from stop
// cause evidence, recent trace PCs, and watchpoint hit addresses.
type BreakpointSuggestion struct {
	Kind       string `json:"kind"`
	Address    uint64 `json:"address"`
	Length     uint64 `json:"length,omitempty"`
	Hart       int    `json:"hart,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
	Command    string `json:"command"`
}

var pcRE = regexp.MustCompile(`(?i)(?:pc=|epc=|mepc=|sepc=|addr=)?\b0x[0-9a-f]{8,16}\b`)

func SuggestBreakpoints(rows []StopCauseCandidate, trace string, hits []mem.WatchpointHit, limit int) []BreakpointSuggestion {
	if limit <= 0 || limit > 64 {
		limit = 16
	}
	out := []BreakpointSuggestion{}
	seen := map[string]bool{}
	add := func(s BreakpointSuggestion) {
		key := fmt.Sprintf("%s:%x:%x:%s:%d", s.Kind, s.Address, s.Length, s.Mode, s.Hart)
		if s.Address == 0 || seen[key] || len(out) >= limit {
			return
		}
		seen[key] = true
		if s.Command == "" {
			switch s.Kind {
			case "pc-breakpoint":
				s.Command = fmt.Sprintf("break pc=%#x", s.Address)
			case "read-watchpoint":
				s.Command = fmt.Sprintf("watch-read addr=%#x len=%#x", s.Address, firstNonZero64(s.Length, 8))
			case "write-watchpoint":
				s.Command = fmt.Sprintf("watch-write addr=%#x len=%#x", s.Address, firstNonZero64(s.Length, 8))
			}
		}
		out = append(out, s)
	}
	for _, c := range rows {
		confidence := "medium"
		if c.Score >= 80 {
			confidence = "high"
		}
		for _, e := range c.Evidence {
			for _, addr := range extractHexAddresses(e, 3) {
				kind := "pc-breakpoint"
				length := uint64(0)
				if strings.Contains(strings.ToLower(c.Category), "access") || strings.Contains(strings.ToLower(c.Category), "page") {
					kind = "read-watchpoint"
					length = 8
				}
				add(BreakpointSuggestion{Kind: kind, Address: addr, Length: length, Confidence: confidence, Reason: c.Category + ": " + truncate(c.Summary, 120)})
			}
		}
	}
	// Prefer the last few trace PCs because they are usually closer to the stop.
	lines := strings.Split(trace, "\n")
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		line := lines[i]
		if !strings.Contains(strings.ToLower(line), "pc=") {
			continue
		}
		for _, addr := range extractHexAddresses(line, 2) {
			add(BreakpointSuggestion{Kind: "pc-breakpoint", Address: addr, Confidence: "medium", Reason: "recent trace PC: " + truncate(line, 120)})
		}
	}
	for i := len(hits) - 1; i >= 0 && len(out) < limit; i-- {
		h := hits[i]
		kind := "write-watchpoint"
		if h.Kind == "read" || h.Op == "read" {
			kind = "read-watchpoint"
		}
		add(BreakpointSuggestion{Kind: kind, Address: h.Addr, Length: h.End - h.Addr + 1, Confidence: "high", Reason: fmt.Sprintf("repeat %s watchpoint hit %s seq=%d", h.Kind, h.Name, h.Seq)})
	}
	return out
}

func BreakpointSuggestionsString(rows []BreakpointSuggestion) string {
	if len(rows) == 0 {
		return "<no breakpoint suggestions>\n"
	}
	var b strings.Builder
	b.WriteString("auto breakpoint/watchpoint suggestions\n")
	for i, s := range rows {
		fmt.Fprintf(&b, "%2d. %-16s addr=%#x", i+1, s.Kind, s.Address)
		if s.Length != 0 {
			fmt.Fprintf(&b, " len=%#x", s.Length)
		}
		if s.Mode != "" {
			fmt.Fprintf(&b, " mode=%s", s.Mode)
		}
		fmt.Fprintf(&b, " confidence=%s\n    %s\n    command: %s\n", s.Confidence, s.Reason, s.Command)
	}
	return b.String()
}

func extractHexAddresses(s string, max int) []uint64 {
	matches := pcRE.FindAllString(s, max)
	out := []uint64{}
	for _, m := range matches {
		idx := strings.LastIndex(strings.ToLower(m), "0x")
		if idx >= 0 {
			m = m[idx:]
		}
		v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(m), "0x"), 16, 64)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}

func firstNonZero64(v, def uint64) uint64 {
	if v == 0 {
		return def
	}
	return v
}

// SmokeFailureCluster groups smoke presets by the observed phase and top cause.
type SmokeFailureCluster struct {
	Key       string   `json:"key"`
	Phase     string   `json:"phase"`
	Cause     string   `json:"cause"`
	Count     int      `json:"count"`
	Presets   []string `json:"presets"`
	MinRan    int      `json:"min_ran"`
	MaxRan    int      `json:"max_ran"`
	Suggested string   `json:"suggested_query,omitempty"`
}

func ClusterSmokeFailures(rows []SmokeSummary) []SmokeFailureCluster {
	byKey := map[string]*SmokeFailureCluster{}
	for _, r := range rows {
		phase := firstNonEmpty(r.Phase, "unknown")
		cause := normalizeSmokeCause(r.TopCause)
		key := phase + "|" + cause
		c := byKey[key]
		if c == nil {
			c = &SmokeFailureCluster{Key: key, Phase: phase, Cause: cause, MinRan: r.Ran, MaxRan: r.Ran, Suggested: smokeClusterQuery(phase, cause)}
			byKey[key] = c
		}
		c.Count++
		c.Presets = append(c.Presets, r.Preset)
		if r.Ran < c.MinRan {
			c.MinRan = r.Ran
		}
		if r.Ran > c.MaxRan {
			c.MaxRan = r.Ran
		}
	}
	out := make([]SmokeFailureCluster, 0, len(byKey))
	for _, c := range byKey {
		sort.Strings(c.Presets)
		out = append(out, *c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Key < out[j].Key
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func SmokeFailureClustersString(rows []SmokeFailureCluster) string {
	if len(rows) == 0 {
		return "<no smoke failure clusters>\n"
	}
	var b strings.Builder
	b.WriteString("smoke failure clusters\n")
	for _, c := range rows {
		fmt.Fprintf(&b, "[%d] phase=%s cause=%s ran=%d..%d presets=%s\n", c.Count, c.Phase, c.Cause, c.MinRan, c.MaxRan, strings.Join(c.Presets, ","))
		if c.Suggested != "" {
			fmt.Fprintf(&b, "    query: %s\n", c.Suggested)
		}
	}
	return b.String()
}

func normalizeSmokeCause(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "none"
	}
	if i := strings.Index(s, ":"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	if len(s) > 48 {
		return s[:48]
	}
	return s
}

func smokeClusterQuery(phase, cause string) string {
	switch {
	case strings.Contains(cause, "virtqueue"):
		return "virtqueue QueueReady QueueNotify descriptor"
	case strings.Contains(cause, "panic") || strings.Contains(cause, "oops"):
		return "panic oops call trace"
	case strings.Contains(cause, "illegal"):
		return "illegal asm CSR"
	case strings.Contains(cause, "page"):
		return "page fault satp stval"
	case strings.Contains(cause, "probe") || strings.Contains(phase, "virtio"):
		return "DeviceID Status QueueReady"
	default:
		return strings.TrimSpace(phase + " " + cause)
	}
}

// DiagnosticBundle is a compact, optionally gzipped export of the most useful
// state for offline triage.  It avoids raw memory and disk contents.
type DiagnosticBundle struct {
	Manifest    ArtifactManifest       `json:"manifest"`
	Triage      TriageDashboard        `json:"triage"`
	StopCauses  []StopCauseCandidate   `json:"stop_causes"`
	Suggestions []BreakpointSuggestion `json:"breakpoint_suggestions,omitempty"`
	Smoke       []SmokeSummary         `json:"smoke,omitempty"`
	Clusters    []SmokeFailureCluster  `json:"smoke_clusters,omitempty"`
	Share       ShareBundle            `json:"share"`
	Watches     []mem.WatchpointHit    `json:"watchpoint_hits,omitempty"`
	Notes       []string               `json:"notes,omitempty"`
}

type CompressedDiagnosticBundle struct {
	Encoding         string `json:"encoding"`
	UncompressedJSON int    `json:"uncompressed_json_bytes"`
	GzipBytes        int    `json:"gzip_bytes"`
	Base64           string `json:"base64"`
}

func DiagnosticBundleJSON(bundle DiagnosticBundle) string {
	b, _ := json.MarshalIndent(bundle, "", "  ")
	return string(b)
}

func CompressDiagnosticBundleJSON(jsonText string) (CompressedDiagnosticBundle, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(jsonText)); err != nil {
		_ = gz.Close()
		return CompressedDiagnosticBundle{}, err
	}
	if err := gz.Close(); err != nil {
		return CompressedDiagnosticBundle{}, err
	}
	return CompressedDiagnosticBundle{Encoding: "gzip+base64", UncompressedJSON: len(jsonText), GzipBytes: buf.Len(), Base64: base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

func CompressedDiagnosticBundleJSONString(jsonText string) string {
	c, err := CompressDiagnosticBundleJSON(jsonText)
	if err != nil {
		return `{"error":"` + strings.ReplaceAll(err.Error(), `"`, `'`) + `"}`
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return string(b)
}
