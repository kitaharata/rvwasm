package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ReproductionStep is a copy/paste-friendly checklist item derived from a
// diagnostic bundle. It intentionally avoids embedding raw payload bytes; it
// points at artifact roles and hashes instead.
type ReproductionStep struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Command   string   `json:"command,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
	Checks    []string `json:"checks,omitempty"`
}

type ReproductionPlan struct {
	Tool         string             `json:"tool"`
	BootArgs     string             `json:"boot_args"`
	HartCount    int                `json:"hart_count"`
	NextAddr     uint64             `json:"next_addr"`
	ArtifactPins []ArtifactEntry    `json:"artifact_pins"`
	TopStopCause string             `json:"top_stop_cause,omitempty"`
	SuggestedRun string             `json:"suggested_run,omitempty"`
	Steps        []ReproductionStep `json:"steps"`
}

func BuildReproductionPlan(bundle DiagnosticBundle, provenance ProvenanceReport, smokePreset string) ReproductionPlan {
	if smokePreset == "" {
		smokePreset = "uart-blk"
	}
	p := ReproductionPlan{
		Tool:         firstNonEmpty(provenance.Tool, "rvwasm-go1.23.2-js-wasm"),
		BootArgs:     bundle.Manifest.BootArgs,
		HartCount:    bundle.Manifest.HartCount,
		NextAddr:     bundle.Manifest.NextAddr,
		ArtifactPins: append([]ArtifactEntry(nil), bundle.Manifest.Artifacts...),
		SuggestedRun: fmt.Sprintf("preset=%s harts=%d next_addr=%#x", smokePreset, firstNonZero(bundle.Manifest.HartCount, 1), bundle.Manifest.NextAddr),
	}
	if len(bundle.StopCauses) != 0 {
		p.TopStopCause = bundle.StopCauses[0].Category
	}
	p.Steps = []ReproductionStep{
		{ID: "01-environment", Title: "Open the same rvwasm build", Command: "make serve", Rationale: "Use the same browser wasm build before comparing traces.", Checks: []string{"Go target is GOOS=js GOARCH=wasm", "Use the same rvwasm zip/build as the bundle"}},
		{ID: "02-artifacts", Title: "Load pinned artifacts", Rationale: "Reproduction depends on matching firmware/payload/initrd/disk/symbol hashes.", Checks: artifactPinChecks(bundle.Manifest.Artifacts)},
		{ID: "03-boot-config", Title: "Apply boot configuration", Command: fmt.Sprintf("bootargs=%q; harts=%d; next_addr=%#x", bundle.Manifest.BootArgs, bundle.Manifest.HartCount, bundle.Manifest.NextAddr), Checks: []string{"Set hart count before loading firmware when multi-hart is needed", "Keep SBI shim off for normal OpenSBI boot"}},
		{ID: "04-run", Title: "Run smoke preset", Command: fmt.Sprintf("Smoke matrix preset %q or Run until the first stop reason", smokePreset), Rationale: "The smoke result gives a bounded comparison even when full Linux boot is slow."},
		{ID: "05-compare", Title: "Compare diagnostic bundle", Command: "Diagnostic bundle JSON → Bundle compare", Checks: []string{"Compare provenance SHA-256 values", "Compare top stop-cause and virtqueue anomalies", "Inspect first trace PC divergence"}},
	}
	if len(bundle.Suggestions) != 0 {
		p.Steps = append(p.Steps, ReproductionStep{ID: "06-breakpoints", Title: "Apply suggested breakpoints/watchpoints", Command: "Apply auto breaks", Rationale: "Stops closer to the suspected failing condition on the next run.", Checks: suggestionChecks(bundle.Suggestions, 6)})
	}
	return p
}

func artifactPinChecks(rows []ArtifactEntry) []string {
	if len(rows) == 0 {
		return []string{"No artifacts were recorded in the bundle; load the original firmware/payload manually"}
	}
	checks := make([]string, 0, len(rows))
	for _, a := range rows {
		checks = append(checks, fmt.Sprintf("%s bytes=%d range=%#x..%#x sha256=%s", a.Role, a.Bytes, a.LoadAddr, a.EndAddr, shortHash(a.SHA256)))
	}
	return checks
}

func suggestionChecks(rows []BreakpointSuggestion, limit int) []string {
	if limit <= 0 {
		limit = 6
	}
	out := []string{}
	for i, s := range rows {
		if i >= limit {
			break
		}
		out = append(out, s.Command+"  # "+s.Reason)
	}
	return out
}

func ReproductionPlanString(p ReproductionPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "reproduction plan tool=%s harts=%d next=%#x\n", p.Tool, p.HartCount, p.NextAddr)
	if p.BootArgs != "" {
		fmt.Fprintf(&b, "bootargs: %s\n", p.BootArgs)
	}
	if p.TopStopCause != "" {
		fmt.Fprintf(&b, "top stop cause: %s\n", p.TopStopCause)
	}
	for _, s := range p.Steps {
		fmt.Fprintf(&b, "\n[%s] %s\n", s.ID, s.Title)
		if s.Command != "" {
			fmt.Fprintf(&b, "  command: %s\n", s.Command)
		}
		if s.Rationale != "" {
			fmt.Fprintf(&b, "  why: %s\n", s.Rationale)
		}
		for _, c := range s.Checks {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
	}
	return b.String()
}

func ReproductionPlanMarkdown(p ReproductionPlan) string {
	var b strings.Builder
	b.WriteString("# rvwasm reproduction plan\n\n")
	fmt.Fprintf(&b, "- Tool: `%s`\n- Harts: `%d`\n- Next address: `%#x`\n- Bootargs: `%s`\n", p.Tool, p.HartCount, p.NextAddr, p.BootArgs)
	if p.TopStopCause != "" {
		fmt.Fprintf(&b, "- Top stop cause: `%s`\n", p.TopStopCause)
	}
	b.WriteString("\n## Artifact pins\n\n")
	if len(p.ArtifactPins) == 0 {
		b.WriteString("_No artifact pins recorded._\n")
	} else {
		b.WriteString("| Role | Bytes | Range | Entry | SHA-256 |\n|---|---:|---|---|---|\n")
		for _, a := range p.ArtifactPins {
			fmt.Fprintf(&b, "| %s | %d | `%#x..%#x` | `%#x` | `%s` |\n", mdCell(a.Role), a.Bytes, a.LoadAddr, a.EndAddr, a.Entry, shortHash(a.SHA256))
		}
	}
	b.WriteString("\n## Steps\n\n")
	for _, s := range p.Steps {
		fmt.Fprintf(&b, "### %s. %s\n\n", s.ID, s.Title)
		if s.Command != "" {
			fmt.Fprintf(&b, "```text\n%s\n```\n\n", s.Command)
		}
		if s.Rationale != "" {
			fmt.Fprintf(&b, "%s\n\n", s.Rationale)
		}
		for _, c := range s.Checks {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func ReproductionPlanJSON(p ReproductionPlan) string {
	j, _ := json.MarshalIndent(p, "", "  ")
	return string(j)
}

func firstNonZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// LogSignatureSet is a stable hash summary for comparing trace/console without
// copying large logs into every handoff message.
type LogSignatureSet struct {
	TraceSHA256    string   `json:"trace_sha256,omitempty"`
	ConsoleSHA256  string   `json:"console_sha256,omitempty"`
	ManifestSHA256 string   `json:"manifest_sha256,omitempty"`
	TraceLines     int      `json:"trace_lines"`
	ConsoleLines   int      `json:"console_lines"`
	FirstPC        string   `json:"first_pc,omitempty"`
	LastPC         string   `json:"last_pc,omitempty"`
	FirstConsole   string   `json:"first_console,omitempty"`
	LastConsole    string   `json:"last_console,omitempty"`
	HotTokens      []string `json:"hot_tokens,omitempty"`
}

type LogSignatureDiff struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Kind   string `json:"kind"`
}

func BuildLogSignatureSet(trace, console string, manifest ArtifactManifest) LogSignatureSet {
	tLines := nonEmptyLines(trace)
	cLines := nonEmptyLines(console)
	firstPC, lastPC := firstLastPC(tLines)
	return LogSignatureSet{
		TraceSHA256:    shaText(trace),
		ConsoleSHA256:  shaText(console),
		ManifestSHA256: shaText(ArtifactManifestJSON(manifest)),
		TraceLines:     len(tLines),
		ConsoleLines:   len(cLines),
		FirstPC:        firstPC,
		LastPC:         lastPC,
		FirstConsole:   truncate(firstString(cLines), 160),
		LastConsole:    truncate(lastString(cLines), 160),
		HotTokens:      hotLogTokens(append(tLines, cLines...), 8),
	}
}

func LogSignatureSetString(s LogSignatureSet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "trace   sha256=%s lines=%d first_pc=%s last_pc=%s\n", shortHash(s.TraceSHA256), s.TraceLines, firstNonEmpty(s.FirstPC, "-"), firstNonEmpty(s.LastPC, "-"))
	fmt.Fprintf(&b, "console sha256=%s lines=%d\n", shortHash(s.ConsoleSHA256), s.ConsoleLines)
	fmt.Fprintf(&b, "manifest sha256=%s\n", shortHash(s.ManifestSHA256))
	if s.FirstConsole != "" || s.LastConsole != "" {
		fmt.Fprintf(&b, "console first: %s\nconsole last:  %s\n", s.FirstConsole, s.LastConsole)
	}
	if len(s.HotTokens) != 0 {
		fmt.Fprintf(&b, "hot tokens: %s\n", strings.Join(s.HotTokens, ", "))
	}
	return b.String()
}

func LogSignatureSetJSON(s LogSignatureSet) string {
	j, _ := json.MarshalIndent(s, "", "  ")
	return string(j)
}

func CompareLogSignatures(current LogSignatureSet, baselineText string) []LogSignatureDiff {
	baselineText = strings.TrimSpace(baselineText)
	if baselineText == "" {
		return []LogSignatureDiff{{Field: "baseline", Kind: "missing", After: "paste rvwasm-log-signature.json"}}
	}
	var base LogSignatureSet
	if err := json.Unmarshal([]byte(baselineText), &base); err != nil {
		return []LogSignatureDiff{{Field: "baseline", Kind: "parse-error", Before: err.Error()}}
	}
	rows := []LogSignatureDiff{}
	add := func(field, before, after string) {
		if before != after {
			rows = append(rows, LogSignatureDiff{Field: field, Before: before, After: after, Kind: "changed"})
		}
	}
	add("trace_sha256", base.TraceSHA256, current.TraceSHA256)
	add("console_sha256", base.ConsoleSHA256, current.ConsoleSHA256)
	add("manifest_sha256", base.ManifestSHA256, current.ManifestSHA256)
	add("trace_lines", strconv.Itoa(base.TraceLines), strconv.Itoa(current.TraceLines))
	add("console_lines", strconv.Itoa(base.ConsoleLines), strconv.Itoa(current.ConsoleLines))
	add("first_pc", base.FirstPC, current.FirstPC)
	add("last_pc", base.LastPC, current.LastPC)
	add("last_console", base.LastConsole, current.LastConsole)
	if strings.Join(base.HotTokens, "|") != strings.Join(current.HotTokens, "|") {
		rows = append(rows, LogSignatureDiff{Field: "hot_tokens", Before: strings.Join(base.HotTokens, ","), After: strings.Join(current.HotTokens, ","), Kind: "changed"})
	}
	if len(rows) == 0 {
		rows = append(rows, LogSignatureDiff{Field: "signature", Kind: "unchanged"})
	}
	return rows
}

func LogSignatureDiffString(rows []LogSignatureDiff) string {
	var b strings.Builder
	b.WriteString("log signature baseline diff\n")
	for _, r := range rows {
		switch r.Kind {
		case "unchanged", "missing", "parse-error":
			fmt.Fprintf(&b, "%s %s %s\n", r.Kind, r.Field, firstNonEmpty(r.After, r.Before))
		default:
			fmt.Fprintf(&b, "* %-16s %s -> %s\n", r.Field, shortMaybeHash(r.Before), shortMaybeHash(r.After))
		}
	}
	return b.String()
}

func LogSignatureDiffJSON(rows []LogSignatureDiff) string {
	j, _ := json.MarshalIndent(rows, "", "  ")
	return string(j)
}

func nonEmptyLines(s string) []string {
	out := []string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func firstLastPC(lines []string) (string, string) {
	first, last := "", ""
	for _, line := range lines {
		addrs := extractHexAddresses(line, 4)
		if len(addrs) == 0 || !strings.Contains(strings.ToLower(line), "pc") {
			continue
		}
		pc := fmt.Sprintf("%#x", addrs[0])
		if first == "" {
			first = pc
		}
		last = pc
	}
	return first, last
}

func firstString(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0]
}

func lastString(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[len(rows)-1]
}

func hotLogTokens(lines []string, limit int) []string {
	counts := map[string]int{}
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, tok := range []string{"panic", "oops", "trap", "ecall", "virtio", "queuenotify", "queueready", "satp", "mstatus", "fault", "illegal", "opensbi", "linux"} {
			if strings.Contains(lower, tok) {
				counts[tok]++
			}
		}
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	if len(keys) > limit {
		keys = keys[:limit]
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s:%d", k, counts[k]))
	}
	return out
}

func shortMaybeHash(s string) string {
	if len(s) >= 32 {
		if _, err := hex.DecodeString(s[:32]); err == nil {
			return shortHash(s)
		}
	}
	return truncate(s, 64)
}

// AppliedSuggestionReport explains what auto-break/watch suggestions currently
// look like after application. It is a light validation layer for handoff flows.
type AppliedSuggestionReport struct {
	Requested int      `json:"requested"`
	Commands  []string `json:"commands"`
	Warnings  []string `json:"warnings,omitempty"`
}

func BuildAppliedSuggestionReport(rows []BreakpointSuggestion, limit int) AppliedSuggestionReport {
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	r := AppliedSuggestionReport{Requested: limit}
	seen := map[string]bool{}
	for i := 0; i < limit; i++ {
		cmd := rows[i].Command
		if cmd == "" {
			cmd = rows[i].Kind
		}
		r.Commands = append(r.Commands, cmd)
		if seen[cmd] {
			r.Warnings = append(r.Warnings, "duplicate suggestion: "+cmd)
		}
		seen[cmd] = true
		if rows[i].Kind == "pc-breakpoint" && rows[i].Address < 0x80000000 {
			r.Warnings = append(r.Warnings, fmt.Sprintf("PC breakpoint below DRAM/OpenSBI area: %#x", rows[i].Address))
		}
	}
	if len(r.Commands) == 0 {
		r.Warnings = append(r.Warnings, "no suggestions available")
	}
	return r
}

func AppliedSuggestionReportString(r AppliedSuggestionReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "applied suggestion verification requested=%d commands=%d\n", r.Requested, len(r.Commands))
	for _, cmd := range r.Commands {
		fmt.Fprintf(&b, "  - %s\n", cmd)
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "  warning: %s\n", w)
	}
	return b.String()
}

func AppliedSuggestionReportJSON(r AppliedSuggestionReport) string {
	j, _ := json.MarshalIndent(r, "", "  ")
	return string(j)
}

func HeadlessSmokeScript(manifest ArtifactManifest, presets []string, steps int) string {
	if len(presets) == 0 {
		presets = []string{"uart-blk", "hvc-blk", "uart-initrd", "hvc-initrd", "simplefb"}
	}
	if steps <= 0 {
		steps = 200000
	}
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	b.WriteString("# Generated by rvwasm. This script records the intended CI smoke matrix.\n")
	b.WriteString("# It assumes the rvwasm web bundle is already built with: GOOS=js GOARCH=wasm go build -trimpath -buildvcs=false -ldflags='-s -w' -o web/riscv.wasm ./cmd/rvwasm\n")
	fmt.Fprintf(&b, "BOOTARGS=%q\nHARTS=%d\nNEXT_ADDR=%#x\nSTEPS=%d\nPRESETS=(%s)\n\n", manifest.BootArgs, firstNonZero(manifest.HartCount, 1), manifest.NextAddr, steps, shellArray(presets))
	b.WriteString("cat <<'INFO'\nRequired artifact pins:\n")
	for _, a := range manifest.Artifacts {
		fmt.Fprintf(&b, "- %s bytes=%d sha256=%s load=%#x entry=%#x\n", a.Role, a.Bytes, a.SHA256, a.LoadAddr, a.Entry)
	}
	b.WriteString("INFO\n\n")
	b.WriteString("echo 'Start a static server: python3 -m http.server 8080 -d web'\n")
	b.WriteString("echo 'Run each preset in the browser UI or a Playwright harness, then export Diagnostic bundle JSON.'\n")
	b.WriteString("for p in \"${PRESETS[@]}\"; do\n  echo \"SMOKE preset=$p steps=$STEPS bootargs=$BOOTARGS\"\ndone\n")
	return b.String()
}

func shellArray(rows []string) string {
	quoted := make([]string, 0, len(rows))
	for _, r := range rows {
		quoted = append(quoted, strconv.Quote(r))
	}
	return strings.Join(quoted, " ")
}

func StableTextSHA256(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
