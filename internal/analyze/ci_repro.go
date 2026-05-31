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

// BundleIntegrityIssue is a machine-readable consistency problem found in a
// diagnostic bundle. Severity is one of error, warn, or info.
type BundleIntegrityIssue struct {
	Severity string `json:"severity"`
	Area     string `json:"area"`
	Field    string `json:"field,omitempty"`
	Detail   string `json:"detail"`
	Hint     string `json:"hint,omitempty"`
}

type BundleIntegrityReport struct {
	Status         string                 `json:"status"`
	BundleSHA256   string                 `json:"bundle_sha256,omitempty"`
	ManifestSHA256 string                 `json:"manifest_sha256,omitempty"`
	TopStopCause   string                 `json:"top_stop_cause,omitempty"`
	Counts         map[string]int         `json:"counts"`
	Artifacts      []ArtifactIntegrityRow `json:"artifacts,omitempty"`
	Issues         []BundleIntegrityIssue `json:"issues,omitempty"`
}

type ArtifactIntegrityRow struct {
	Role       string `json:"role"`
	Bytes      int    `json:"bytes"`
	Range      string `json:"range,omitempty"`
	Entry      string `json:"entry,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	LooksValid bool   `json:"looks_valid"`
}

func BuildBundleIntegrityReport(bundle DiagnosticBundle, rawText string) BundleIntegrityReport {
	r := BundleIntegrityReport{Counts: map[string]int{}}
	if strings.TrimSpace(rawText) != "" {
		r.BundleSHA256 = shaText(rawText)
	} else {
		r.BundleSHA256 = shaText(DiagnosticBundleJSON(bundle))
	}
	r.ManifestSHA256 = shaText(ArtifactManifestJSON(bundle.Manifest))
	if len(bundle.StopCauses) != 0 {
		r.TopStopCause = bundle.StopCauses[0].Category
	}
	add := func(sev, area, field, detail, hint string) {
		r.Issues = append(r.Issues, BundleIntegrityIssue{Severity: sev, Area: area, Field: field, Detail: detail, Hint: hint})
		r.Counts[sev]++
	}
	if bundle.Manifest.HartCount <= 0 {
		add("warn", "manifest", "hart_count", "hart_count is not positive", "use 1 unless multi-hart reproduction is required")
	}
	if strings.TrimSpace(bundle.Manifest.BootArgs) == "" {
		add("info", "manifest", "boot_args", "bootargs are empty", "record bootargs before sharing the bundle")
	}
	seenRole := map[string]bool{}
	if len(bundle.Manifest.Artifacts) == 0 {
		add("warn", "artifact", "artifacts", "no artifact pins recorded", "load firmware/payload/disk/initrd before exporting the bundle")
	}
	for _, a := range bundle.Manifest.Artifacts {
		role := strings.TrimSpace(a.Role)
		if role == "" {
			add("error", "artifact", "role", "artifact has empty role", "set role to firmware, payload, disk, initrd, symbols, or dtb")
		}
		if seenRole[role] {
			add("warn", "artifact", role, "duplicate artifact role", "keep one pin per role or make role names unique")
		}
		seenRole[role] = true
		validHash := looksLikeSHA256(a.SHA256)
		validRange := a.Bytes == 0 || a.EndAddr >= a.LoadAddr
		if a.Bytes > 0 && !validHash {
			add("error", "artifact", role, "missing or malformed SHA-256", "re-export artifact manifest after loading the artifact")
		}
		if !validRange {
			add("error", "artifact", role, "end address is before load address", "check loader address arithmetic")
		}
		expectedLen := uint64(0)
		if a.Bytes > 0 {
			expectedLen = uint64(a.Bytes)
		}
		if expectedLen != 0 && a.EndAddr >= a.LoadAddr && a.EndAddr-a.LoadAddr+1 != expectedLen {
			add("warn", "artifact", role, "range length does not match byte size", "verify artifact load range in manifest")
		}
		r.Artifacts = append(r.Artifacts, ArtifactIntegrityRow{Role: role, Bytes: a.Bytes, Range: fmt.Sprintf("%#x..%#x", a.LoadAddr, a.EndAddr), Entry: fmt.Sprintf("%#x", a.Entry), SHA256: shortHash(a.SHA256), LooksValid: validHash && validRange})
	}
	if len(bundle.Suggestions) == 0 && len(bundle.StopCauses) != 0 {
		add("info", "suggestions", "breakpoints", "no auto break/watch suggestions in bundle", "generate suggestions before handoff")
	}
	for i, s := range bundle.Suggestions {
		if s.Kind == "" || s.Address == 0 {
			add("warn", "suggestions", strconv.Itoa(i), "suggestion is missing kind or address", "regenerate auto break/watch suggestions")
		}
		if s.Command == "" {
			add("info", "suggestions", strconv.Itoa(i), "suggestion has no copy/paste command", "use newer bundle format if possible")
		}
	}
	for _, sm := range bundle.Smoke {
		if sm.Requested > 0 && sm.Ran > sm.Requested {
			add("warn", "smoke", sm.Preset, "ran steps exceed requested steps", "check smoke runner accounting")
		}
	}
	if r.Counts["error"] > 0 {
		r.Status = "error"
	} else if r.Counts["warn"] > 0 {
		r.Status = "warn"
	} else {
		r.Status = "ok"
	}
	return r
}

func BundleIntegrityReportString(r BundleIntegrityReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bundle integrity status=%s errors=%d warnings=%d infos=%d\n", r.Status, r.Counts["error"], r.Counts["warn"], r.Counts["info"])
	if r.BundleSHA256 != "" {
		fmt.Fprintf(&b, "bundle sha256=%s manifest sha256=%s\n", shortHash(r.BundleSHA256), shortHash(r.ManifestSHA256))
	}
	if r.TopStopCause != "" {
		fmt.Fprintf(&b, "top stop cause: %s\n", r.TopStopCause)
	}
	if len(r.Artifacts) != 0 {
		b.WriteString("artifacts:\n")
		for _, a := range r.Artifacts {
			ok := "bad"
			if a.LooksValid {
				ok = "ok"
			}
			fmt.Fprintf(&b, "  %-10s %-3s bytes=%-10d range=%s sha=%s\n", a.Role, ok, a.Bytes, a.Range, a.SHA256)
		}
	}
	if len(r.Issues) != 0 {
		b.WriteString("issues:\n")
		for _, it := range r.Issues {
			fmt.Fprintf(&b, "  %-5s %-12s %-14s %s\n", it.Severity, it.Area, it.Field, it.Detail)
			if it.Hint != "" {
				fmt.Fprintf(&b, "        hint: %s\n", it.Hint)
			}
		}
	}
	return b.String()
}

func BundleIntegrityReportJSON(r BundleIntegrityReport) string {
	j, _ := json.MarshalIndent(r, "", "  ")
	return string(j)
}

type ReproductionValidationCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

type ReproductionValidationReport struct {
	Status string                        `json:"status"`
	Passed int                           `json:"passed"`
	Warned int                           `json:"warned"`
	Failed int                           `json:"failed"`
	Checks []ReproductionValidationCheck `json:"checks"`
}

func ValidateReproductionPlan(bundle DiagnosticBundle, plan ReproductionPlan, sig LogSignatureSet) ReproductionValidationReport {
	r := ReproductionValidationReport{}
	add := func(name, status, detail, expected, actual string) {
		r.Checks = append(r.Checks, ReproductionValidationCheck{Name: name, Status: status, Detail: detail, Expected: expected, Actual: actual})
		switch status {
		case "pass":
			r.Passed++
		case "fail":
			r.Failed++
		default:
			r.Warned++
		}
	}
	if strings.TrimSpace(plan.BootArgs) == strings.TrimSpace(bundle.Manifest.BootArgs) {
		add("bootargs", "pass", "bootargs match bundle manifest", bundle.Manifest.BootArgs, plan.BootArgs)
	} else {
		add("bootargs", "fail", "bootargs differ from bundle manifest", bundle.Manifest.BootArgs, plan.BootArgs)
	}
	if firstNonZero(plan.HartCount, 1) == firstNonZero(bundle.Manifest.HartCount, 1) {
		add("hart-count", "pass", "hart count matches", strconv.Itoa(bundle.Manifest.HartCount), strconv.Itoa(plan.HartCount))
	} else {
		add("hart-count", "fail", "hart count differs", strconv.Itoa(bundle.Manifest.HartCount), strconv.Itoa(plan.HartCount))
	}
	if plan.NextAddr == bundle.Manifest.NextAddr {
		add("next-addr", "pass", "next address matches", fmt.Sprintf("%#x", bundle.Manifest.NextAddr), fmt.Sprintf("%#x", plan.NextAddr))
	} else {
		add("next-addr", "warn", "next address differs", fmt.Sprintf("%#x", bundle.Manifest.NextAddr), fmt.Sprintf("%#x", plan.NextAddr))
	}
	pinByRole := map[string]ArtifactEntry{}
	for _, p := range plan.ArtifactPins {
		pinByRole[p.Role] = p
	}
	for _, a := range bundle.Manifest.Artifacts {
		p, ok := pinByRole[a.Role]
		if !ok {
			add("artifact:"+a.Role, "fail", "artifact missing from reproduction plan", shortHash(a.SHA256), "")
			continue
		}
		if p.SHA256 == a.SHA256 && p.Bytes == a.Bytes {
			add("artifact:"+a.Role, "pass", "artifact pin matches", shortHash(a.SHA256), shortHash(p.SHA256))
		} else {
			add("artifact:"+a.Role, "fail", "artifact pin differs", artifactBrief(a), artifactBrief(p))
		}
	}
	top := ""
	if len(bundle.StopCauses) != 0 {
		top = bundle.StopCauses[0].Category
	}
	if top == "" && plan.TopStopCause == "" {
		add("top-stop-cause", "warn", "no top stop cause recorded", "", "")
	} else if top == plan.TopStopCause {
		add("top-stop-cause", "pass", "top stop cause matches", top, plan.TopStopCause)
	} else {
		add("top-stop-cause", "warn", "top stop cause differs", top, plan.TopStopCause)
	}
	if sig.ManifestSHA256 != "" {
		expected := shaText(ArtifactManifestJSON(bundle.Manifest))
		if sig.ManifestSHA256 == expected {
			add("log-signature-manifest", "pass", "log signature was produced from the same manifest", shortHash(expected), shortHash(sig.ManifestSHA256))
		} else {
			add("log-signature-manifest", "warn", "log signature manifest hash differs", shortHash(expected), shortHash(sig.ManifestSHA256))
		}
	}
	if r.Failed != 0 {
		r.Status = "fail"
	} else if r.Warned != 0 {
		r.Status = "warn"
	} else {
		r.Status = "pass"
	}
	return r
}

func ReproductionValidationReportString(r ReproductionValidationReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "reproduction validation status=%s pass=%d warn=%d fail=%d\n", r.Status, r.Passed, r.Warned, r.Failed)
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "  %-5s %-24s %s\n", c.Status, c.Name, c.Detail)
		if c.Expected != "" || c.Actual != "" {
			fmt.Fprintf(&b, "        expected=%s actual=%s\n", c.Expected, c.Actual)
		}
	}
	return b.String()
}

func ReproductionValidationReportJSON(r ReproductionValidationReport) string {
	j, _ := json.MarshalIndent(r, "", "  ")
	return string(j)
}

type CISummary struct {
	Status        string         `json:"status"`
	ExitCode      int            `json:"exit_code"`
	TopStopCause  string         `json:"top_stop_cause,omitempty"`
	Phase         string         `json:"phase,omitempty"`
	BundleSHA256  string         `json:"bundle_sha256,omitempty"`
	TraceSHA256   string         `json:"trace_sha256,omitempty"`
	ConsoleSHA256 string         `json:"console_sha256,omitempty"`
	Smoke         []SmokeSummary `json:"smoke,omitempty"`
	Counts        map[string]int `json:"counts,omitempty"`
	Messages      []string       `json:"messages,omitempty"`
}

func BuildCISummary(bundle DiagnosticBundle, sig LogSignatureSet, integrity BundleIntegrityReport) CISummary {
	s := CISummary{Status: "pass", Counts: map[string]int{}, BundleSHA256: integrity.BundleSHA256, TraceSHA256: sig.TraceSHA256, ConsoleSHA256: sig.ConsoleSHA256, Smoke: append([]SmokeSummary(nil), bundle.Smoke...)}
	if len(bundle.StopCauses) != 0 {
		s.TopStopCause = bundle.StopCauses[0].Category
	}
	s.Phase = bundle.Triage.Phase
	if s.Phase == "" && len(bundle.Smoke) != 0 {
		s.Phase = bundle.Smoke[0].Phase
	}
	s.Counts["integrity_errors"] = integrity.Counts["error"]
	s.Counts["integrity_warnings"] = integrity.Counts["warn"]
	s.Counts["stop_causes"] = len(bundle.StopCauses)
	s.Counts["virtqueue_anomalies"] = len(bundle.Share.Anomalies)
	for _, sm := range bundle.Smoke {
		if strings.Contains(strings.ToLower(sm.TopCause), "panic") || strings.Contains(strings.ToLower(sm.TopCause), "fault") || strings.Contains(strings.ToLower(sm.TopCause), "illegal") || strings.Contains(strings.ToLower(sm.TopCause), "virtqueue") {
			s.Counts["smoke_failures"]++
		}
	}
	if integrity.Counts["error"] > 0 || s.Counts["smoke_failures"] > 0 || strings.Contains(strings.ToLower(s.TopStopCause), "panic") || strings.Contains(strings.ToLower(s.TopStopCause), "illegal") {
		s.Status = "fail"
		s.ExitCode = 1
	} else if integrity.Counts["warn"] > 0 || s.Counts["virtqueue_anomalies"] > 0 || s.TopStopCause != "" {
		s.Status = "warn"
	}
	if s.TopStopCause != "" {
		s.Messages = append(s.Messages, "top stop-cause: "+s.TopStopCause)
	}
	if integrity.Counts["error"] != 0 || integrity.Counts["warn"] != 0 {
		s.Messages = append(s.Messages, fmt.Sprintf("bundle integrity: %d errors, %d warnings", integrity.Counts["error"], integrity.Counts["warn"]))
	}
	if len(bundle.Suggestions) != 0 {
		s.Messages = append(s.Messages, fmt.Sprintf("%d auto break/watch suggestions available", len(bundle.Suggestions)))
	}
	return s
}

func CISummaryString(s CISummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ci summary status=%s exit=%d phase=%s top=%s\n", s.Status, s.ExitCode, firstNonEmpty(s.Phase, "-"), firstNonEmpty(s.TopStopCause, "-"))
	if s.BundleSHA256 != "" || s.TraceSHA256 != "" || s.ConsoleSHA256 != "" {
		fmt.Fprintf(&b, "sha bundle=%s trace=%s console=%s\n", shortHash(s.BundleSHA256), shortHash(s.TraceSHA256), shortHash(s.ConsoleSHA256))
	}
	keys := make([]string, 0, len(s.Counts))
	for k := range s.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-22s %d\n", k, s.Counts[k])
	}
	for _, m := range s.Messages {
		fmt.Fprintf(&b, "  - %s\n", m)
	}
	if len(s.Smoke) != 0 {
		b.WriteString("smoke:\n")
		for _, sm := range s.Smoke {
			fmt.Fprintf(&b, "  %-12s ran=%d/%d phase=%s top=%s\n", sm.Preset, sm.Ran, sm.Requested, sm.Phase, sm.TopCause)
		}
	}
	return b.String()
}

func CISummaryJSON(s CISummary) string {
	j, _ := json.MarshalIndent(s, "", "  ")
	return string(j)
}

type HeadlessSmokeRunnerSpec struct {
	Tool      string          `json:"tool"`
	Steps     int             `json:"steps"`
	Presets   []string        `json:"presets"`
	BootArgs  string          `json:"boot_args"`
	HartCount int             `json:"hart_count"`
	NextAddr  uint64          `json:"next_addr"`
	Artifacts []ArtifactEntry `json:"artifacts"`
	Commands  []string        `json:"commands"`
	Notes     []string        `json:"notes,omitempty"`
}

func BuildHeadlessSmokeRunnerSpec(bundle DiagnosticBundle, presets []string, steps int) HeadlessSmokeRunnerSpec {
	if len(presets) == 0 {
		presets = []string{"uart-blk", "hvc-blk", "uart-initrd", "hvc-initrd", "simplefb"}
	}
	if steps <= 0 {
		steps = 200000
	}
	spec := HeadlessSmokeRunnerSpec{Tool: "rvsmoke", Steps: steps, Presets: append([]string(nil), presets...), BootArgs: bundle.Manifest.BootArgs, HartCount: firstNonZero(bundle.Manifest.HartCount, 1), NextAddr: bundle.Manifest.NextAddr, Artifacts: append([]ArtifactEntry(nil), bundle.Manifest.Artifacts...)}
	spec.Commands = []string{
		"go run ./cmd/rvsmoke -bundle rvwasm-diagnostic-bundle.json -out text",
		fmt.Sprintf("go run ./cmd/rvsmoke -bundle rvwasm-diagnostic-bundle.json -steps %d -presets %s -out json", steps, strings.Join(presets, ",")),
	}
	for _, a := range bundle.Manifest.Artifacts {
		if a.SHA256 != "" {
			spec.Commands = append(spec.Commands, fmt.Sprintf("go run ./cmd/rvsmoke -bundle rvwasm-diagnostic-bundle.json -artifact %s=/path/to/%s", a.Role, safeArtifactFilename(a.Role)))
		}
	}
	spec.Notes = []string{"rvsmoke validates bundle/manifest reproducibility metadata and artifact hashes", "browser execution is still required for full js/wasm CPU smoke until a native headless emulator loop is introduced"}
	return spec
}

func HeadlessSmokeRunnerSpecString(s HeadlessSmokeRunnerSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "headless smoke runner spec tool=%s steps=%d harts=%d next=%#x\n", s.Tool, s.Steps, s.HartCount, s.NextAddr)
	fmt.Fprintf(&b, "presets: %s\nbootargs: %s\n", strings.Join(s.Presets, ","), s.BootArgs)
	if len(s.Artifacts) != 0 {
		b.WriteString("artifacts:\n")
		for _, a := range s.Artifacts {
			fmt.Fprintf(&b, "  %-10s bytes=%d sha256=%s\n", a.Role, a.Bytes, shortHash(a.SHA256))
		}
	}
	b.WriteString("commands:\n")
	for _, c := range s.Commands {
		fmt.Fprintf(&b, "  %s\n", c)
	}
	for _, n := range s.Notes {
		fmt.Fprintf(&b, "note: %s\n", n)
	}
	return b.String()
}

func HeadlessSmokeRunnerSpecJSON(s HeadlessSmokeRunnerSpec) string {
	j, _ := json.MarshalIndent(s, "", "  ")
	return string(j)
}

func HeadlessSmokeRunnerMarkdown(s HeadlessSmokeRunnerSpec) string {
	var b strings.Builder
	b.WriteString("# rvwasm headless smoke runner\n\n")
	fmt.Fprintf(&b, "- Tool: `%s`\n- Steps: `%d`\n- Presets: `%s`\n- Harts: `%d`\n- Next address: `%#x`\n- Bootargs: `%s`\n\n", s.Tool, s.Steps, strings.Join(s.Presets, ","), s.HartCount, s.NextAddr, s.BootArgs)
	b.WriteString("## Artifact pins\n\n")
	for _, a := range s.Artifacts {
		fmt.Fprintf(&b, "- `%s`: bytes=%d sha256=`%s`\n", a.Role, a.Bytes, a.SHA256)
	}
	b.WriteString("\n## Commands\n\n")
	for _, c := range s.Commands {
		fmt.Fprintf(&b, "```bash\n%s\n```\n", c)
	}
	return b.String()
}

func VerifyArtifactBytes(role string, data []byte, manifest ArtifactManifest) (ArtifactIntegrityRow, bool) {
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	for _, a := range manifest.Artifacts {
		if a.Role == role {
			ok := strings.EqualFold(a.SHA256, actual) && (a.Bytes == 0 || a.Bytes == len(data))
			return ArtifactIntegrityRow{Role: role, Bytes: len(data), Range: fmt.Sprintf("%#x..%#x", a.LoadAddr, a.EndAddr), Entry: fmt.Sprintf("%#x", a.Entry), SHA256: shortHash(actual), LooksValid: ok}, ok
		}
	}
	return ArtifactIntegrityRow{Role: role, Bytes: len(data), SHA256: shortHash(actual), LooksValid: false}, false
}

func looksLikeSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func safeArtifactFilename(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	role = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, role)
	if role == "" {
		return "artifact.bin"
	}
	return role + ".bin"
}
