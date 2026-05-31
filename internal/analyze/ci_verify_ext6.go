package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

// AttestationVerification records deterministic checks for an rvsmoke
// provenance attestation. The attestation is not a signature; this verifier
// checks that the payload hash, release materials and CI artifact subjects are
// internally consistent.
type AttestationVerification struct {
	Status       string   `json:"status"`
	Checked      int      `json:"checked"`
	ExpectedHash string   `json:"expected_hash,omitempty"`
	ActualHash   string   `json:"actual_hash,omitempty"`
	Missing      []string `json:"missing,omitempty"`
	Mismatch     []string `json:"mismatch,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

func VerifyProvenanceAttestation(att ProvenanceAttestation, release ReleaseBundleManifest, inv DependencyInventory, index CIArtifactIndex) AttestationVerification {
	v := AttestationVerification{Status: "pass"}
	clone := att
	clone.AttestationHash = ""
	b, _ := json.Marshal(clone)
	sum := sha256.Sum256(b)
	v.ExpectedHash = hex.EncodeToString(sum[:])
	v.ActualHash = att.AttestationHash
	v.Checked++
	if att.AttestationHash == "" {
		v.Missing = append(v.Missing, "attestation_sha256")
	} else if !strings.EqualFold(att.AttestationHash, v.ExpectedHash) {
		v.Mismatch = append(v.Mismatch, "attestation_sha256 does not match deterministic payload hash")
	}
	if att.SchemaVersion == "" {
		v.Missing = append(v.Missing, "schema_version")
	} else {
		v.Checked++
	}
	if att.Builder == "" {
		v.Missing = append(v.Missing, "builder")
	} else {
		v.Checked++
	}

	materials := map[string]ProvenanceSubject{}
	for _, m := range att.Materials {
		materials[m.Name] = m
	}
	expectedMaterials := map[string]string{}
	if release.BundleSHA256 != "" {
		expectedMaterials["diagnostic-bundle"] = release.BundleSHA256
	}
	if release.ManifestSHA256 != "" {
		expectedMaterials["artifact-manifest"] = release.ManifestSHA256
	}
	if inv.InventoryHash != "" {
		expectedMaterials["dependency-inventory"] = inv.InventoryHash
	}
	for name, digest := range expectedMaterials {
		got, ok := materials[name]
		if !ok {
			v.Missing = append(v.Missing, "material:"+name)
			continue
		}
		v.Checked++
		if !strings.EqualFold(got.Digest, digest) {
			v.Mismatch = append(v.Mismatch, fmt.Sprintf("material %s digest %s != %s", name, shortHash(got.Digest), shortHash(digest)))
		}
	}

	subjects := map[string]ProvenanceSubject{}
	for _, s := range att.Subjects {
		subjects[s.Name] = s
	}
	for _, e := range index.Entries {
		got, ok := subjects[e.Path]
		if !ok {
			v.Missing = append(v.Missing, "subject:"+e.Path)
			continue
		}
		v.Checked++
		if !strings.EqualFold(got.Digest, e.SHA256) || got.Bytes != e.Bytes {
			v.Mismatch = append(v.Mismatch, fmt.Sprintf("subject %s bytes/sha mismatch", e.Path))
		}
	}
	if len(index.Entries) == 0 {
		v.Warnings = append(v.Warnings, "no CI artifacts were available as attestation subjects")
	}
	sort.Strings(v.Missing)
	sort.Strings(v.Mismatch)
	sort.Strings(v.Warnings)
	if len(v.Missing)+len(v.Mismatch) != 0 {
		v.Status = "fail"
	} else if len(v.Warnings) != 0 {
		v.Status = "warn"
	}
	return v
}

func AttestationVerificationJSON(v AttestationVerification) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
func AttestationVerificationString(v AttestationVerification) string {
	var b strings.Builder
	fmt.Fprintf(&b, "attestation verification status=%s checked=%d expected=%s actual=%s\n", v.Status, v.Checked, shortHash(v.ExpectedHash), shortHash(v.ActualHash))
	for _, x := range v.Missing {
		fmt.Fprintf(&b, "missing: %s\n", x)
	}
	for _, x := range v.Mismatch {
		fmt.Fprintf(&b, "mismatch: %s\n", x)
	}
	for _, x := range v.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", x)
	}
	return b.String()
}

// DependencyInventoryDiff compares the current SBOM-lite inventory with a
// baseline inventory JSON. It focuses on module and artifact-kind drift.
type DependencyInventoryDiff struct {
	Status             string   `json:"status"`
	ModulePathChanged  string   `json:"module_path_changed,omitempty"`
	GoVersionChanged   string   `json:"go_version_changed,omitempty"`
	AddedModules       []string `json:"added_modules,omitempty"`
	RemovedModules     []string `json:"removed_modules,omitempty"`
	ChangedModules     []string `json:"changed_modules,omitempty"`
	ArtifactKindChange []string `json:"artifact_kind_change,omitempty"`
	Issues             []string `json:"issues,omitempty"`
}

func CompareDependencyInventory(current DependencyInventory, baselineText string) DependencyInventoryDiff {
	d := DependencyInventoryDiff{Status: "pass"}
	baselineText = strings.TrimSpace(baselineText)
	if baselineText == "" {
		d.Status = "warn"
		d.Issues = append(d.Issues, "no baseline dependency inventory provided")
		return d
	}
	var base DependencyInventory
	if err := json.Unmarshal([]byte(baselineText), &base); err != nil {
		d.Status = "fail"
		d.Issues = append(d.Issues, "cannot parse baseline dependency inventory: "+err.Error())
		return d
	}
	if base.ModulePath != current.ModulePath {
		d.ModulePathChanged = fmt.Sprintf("%s→%s", firstNonEmpty(base.ModulePath, "-"), firstNonEmpty(current.ModulePath, "-"))
	}
	if base.GoVersion != current.GoVersion {
		d.GoVersionChanged = fmt.Sprintf("%s→%s", firstNonEmpty(base.GoVersion, "-"), firstNonEmpty(current.GoVersion, "-"))
	}
	bm := map[string]DependencyModule{}
	cm := map[string]DependencyModule{}
	for _, m := range base.Modules {
		bm[m.Path] = m
	}
	for _, m := range current.Modules {
		cm[m.Path] = m
	}
	for p, c := range cm {
		b, ok := bm[p]
		if !ok {
			d.AddedModules = append(d.AddedModules, fmt.Sprintf("%s %s", p, c.Version))
			continue
		}
		if b.Version != c.Version || b.Replace != c.Replace {
			d.ChangedModules = append(d.ChangedModules, fmt.Sprintf("%s %s→%s", p, firstNonEmpty(b.Version, "-"), firstNonEmpty(c.Version, "-")))
		}
	}
	for p, b := range bm {
		if _, ok := cm[p]; !ok {
			d.RemovedModules = append(d.RemovedModules, fmt.Sprintf("%s %s", p, b.Version))
		}
	}
	keys := map[string]bool{}
	for k := range base.ArtifactKinds {
		keys[k] = true
	}
	for k := range current.ArtifactKinds {
		keys[k] = true
	}
	for k := range keys {
		if base.ArtifactKinds[k] != current.ArtifactKinds[k] {
			d.ArtifactKindChange = append(d.ArtifactKindChange, fmt.Sprintf("%s %d→%d", k, base.ArtifactKinds[k], current.ArtifactKinds[k]))
		}
	}
	sort.Strings(d.AddedModules)
	sort.Strings(d.RemovedModules)
	sort.Strings(d.ChangedModules)
	sort.Strings(d.ArtifactKindChange)
	sort.Strings(d.Issues)
	if d.ModulePathChanged != "" || len(d.RemovedModules) != 0 {
		d.Status = "fail"
	} else if d.GoVersionChanged != "" || len(d.AddedModules)+len(d.ChangedModules)+len(d.ArtifactKindChange) != 0 {
		d.Status = "warn"
	}
	return d
}

func DependencyInventoryDiffJSON(d DependencyInventoryDiff) string {
	b, _ := json.MarshalIndent(d, "", "  ")
	return string(b)
}
func DependencyInventoryDiffString(d DependencyInventoryDiff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "dependency inventory diff status=%s added=%d removed=%d changed=%d artifact_kinds=%d\n", d.Status, len(d.AddedModules), len(d.RemovedModules), len(d.ChangedModules), len(d.ArtifactKindChange))
	if d.ModulePathChanged != "" {
		fmt.Fprintf(&b, "module path: %s\n", d.ModulePathChanged)
	}
	if d.GoVersionChanged != "" {
		fmt.Fprintf(&b, "go version: %s\n", d.GoVersionChanged)
	}
	for _, x := range d.Issues {
		fmt.Fprintf(&b, "issue: %s\n", x)
	}
	for _, x := range d.AddedModules {
		fmt.Fprintf(&b, "added: %s\n", x)
	}
	for _, x := range d.RemovedModules {
		fmt.Fprintf(&b, "removed: %s\n", x)
	}
	for _, x := range d.ChangedModules {
		fmt.Fprintf(&b, "changed: %s\n", x)
	}
	for _, x := range d.ArtifactKindChange {
		fmt.Fprintf(&b, "artifact-kind: %s\n", x)
	}
	return b.String()
}

type ReleaseHandoffPackageComparison struct {
	Status  string   `json:"status"`
	Checked int      `json:"checked"`
	Missing []string `json:"missing,omitempty"`
	Changed []string `json:"changed,omitempty"`
	Extra   []string `json:"extra,omitempty"`
	Issues  []string `json:"issues,omitempty"`
}

func CompareReleaseHandoffPackageInspection(current ReleaseHandoffPackageInspection, baselineText string) ReleaseHandoffPackageComparison {
	c := ReleaseHandoffPackageComparison{Status: "pass"}
	baselineText = strings.TrimSpace(baselineText)
	if baselineText == "" {
		c.Status = "warn"
		c.Issues = append(c.Issues, "no baseline release handoff inspection provided")
		return c
	}
	var base ReleaseHandoffPackageInspection
	if err := json.Unmarshal([]byte(baselineText), &base); err != nil {
		c.Status = "fail"
		c.Issues = append(c.Issues, "cannot parse baseline release handoff inspection: "+err.Error())
		return c
	}
	bm := map[string]ReleaseHandoffFile{}
	cm := map[string]ReleaseHandoffFile{}
	for _, f := range base.Files {
		bm[f.Path] = f
	}
	for _, f := range current.Files {
		cm[f.Path] = f
	}
	for p, bf := range bm {
		cf, ok := cm[p]
		if !ok {
			c.Missing = append(c.Missing, p)
			continue
		}
		c.Checked++
		if bf.SHA256 != cf.SHA256 || bf.Bytes != cf.Bytes || bf.Required != cf.Required {
			c.Changed = append(c.Changed, fmt.Sprintf("%s bytes %d→%d sha %s→%s", p, bf.Bytes, cf.Bytes, shortHash(bf.SHA256), shortHash(cf.SHA256)))
		}
	}
	for p := range cm {
		if _, ok := bm[p]; !ok {
			c.Extra = append(c.Extra, p)
		}
	}
	sort.Strings(c.Missing)
	sort.Strings(c.Changed)
	sort.Strings(c.Extra)
	sort.Strings(c.Issues)
	if len(c.Missing)+len(c.Changed) != 0 {
		c.Status = "fail"
	} else if len(c.Extra) != 0 {
		c.Status = "warn"
	}
	return c
}

func ReleaseHandoffPackageComparisonJSON(c ReleaseHandoffPackageComparison) string {
	b, _ := json.MarshalIndent(c, "", "  ")
	return string(b)
}
func ReleaseHandoffPackageComparisonString(c ReleaseHandoffPackageComparison) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release handoff comparison status=%s checked=%d missing=%d changed=%d extra=%d\n", c.Status, c.Checked, len(c.Missing), len(c.Changed), len(c.Extra))
	for _, x := range c.Issues {
		fmt.Fprintf(&b, "issue: %s\n", x)
	}
	for _, x := range c.Missing {
		fmt.Fprintf(&b, "missing: %s\n", x)
	}
	for _, x := range c.Changed {
		fmt.Fprintf(&b, "changed: %s\n", x)
	}
	for _, x := range c.Extra {
		fmt.Fprintf(&b, "extra: %s\n", x)
	}
	return b.String()
}

type RetentionEntry struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Bytes      int    `json:"bytes"`
	SHA256     string `json:"sha256"`
	RetainDays int    `json:"retain_days"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Reason     string `json:"reason,omitempty"`
}
type RetentionManifest struct {
	SchemaVersion  string           `json:"schema_version"`
	Status         string           `json:"status"`
	GeneratedAt    string           `json:"generated_at,omitempty"`
	ReleaseStatus  string           `json:"release_status,omitempty"`
	EntryCount     int              `json:"entry_count"`
	TotalBytes     int              `json:"total_bytes"`
	Entries        []RetentionEntry `json:"entries,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
	ManifestSHA256 string           `json:"manifest_sha256,omitempty"`
}

func BuildRetentionManifest(index CIArtifactIndex, release ReleaseBundleManifest, generatedAt string) RetentionManifest {
	rm := RetentionManifest{SchemaVersion: "rvwasm.retention.v1", Status: "ok", GeneratedAt: generatedAt, ReleaseStatus: release.Status}
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
		rm.GeneratedAt = generatedAt
	}
	t, _ := time.Parse(time.RFC3339, generatedAt)
	for _, e := range index.Entries {
		days, reason := retentionDaysForKind(e.Kind, release.Status)
		exp := ""
		if !t.IsZero() && days > 0 {
			exp = t.Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
		}
		rm.Entries = append(rm.Entries, RetentionEntry{Path: e.Path, Kind: e.Kind, Bytes: e.Bytes, SHA256: e.SHA256, RetainDays: days, ExpiresAt: exp, Reason: reason})
		rm.EntryCount++
		rm.TotalBytes += e.Bytes
	}
	if len(rm.Entries) == 0 {
		rm.Status = "warn"
		rm.Warnings = append(rm.Warnings, "no CI artifacts available for retention manifest")
	}
	sort.Slice(rm.Entries, func(i, j int) bool { return rm.Entries[i].Path < rm.Entries[j].Path })
	clone := rm
	clone.ManifestSHA256 = ""
	b, _ := json.Marshal(clone)
	sum := sha256.Sum256(b)
	rm.ManifestSHA256 = hex.EncodeToString(sum[:])
	return rm
}

func retentionDaysForKind(kind, releaseStatus string) (int, string) {
	k := strings.ToLower(kind)
	failed := strings.EqualFold(releaseStatus, "fail")
	switch k {
	case "sarif", "junit":
		if failed {
			return 180, "failed CI evidence"
		}
		return 90, "CI evidence"
	case "html", "markdown":
		if failed {
			return 180, "human-readable failure report"
		}
		return 60, "human-readable report"
	case "zip":
		return 365, "handoff package"
	case "json":
		if failed {
			return 180, "machine-readable diagnostics"
		}
		return 90, "machine-readable diagnostics"
	case "workflow":
		return 365, "workflow provenance"
	default:
		if failed {
			return 90, "debug artifact"
		}
		return 30, "default retention"
	}
}

func RetentionManifestJSON(r RetentionManifest) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
func RetentionManifestString(r RetentionManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "retention manifest status=%s entries=%d bytes=%d sha=%s\n", r.Status, r.EntryCount, r.TotalBytes, shortHash(r.ManifestSHA256))
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "  %-34s kind=%-10s retain=%dd expires=%s sha=%s\n", e.Path, e.Kind, e.RetainDays, firstNonEmpty(e.ExpiresAt, "-"), shortHash(e.SHA256))
	}
	return b.String()
}

func ReleaseVerificationHTML(release ReleaseBundleManifest, attv AttestationVerification, sbomDiff DependencyInventoryDiff, zipCmp ReleaseHandoffPackageComparison, retention RetentionManifest) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm release verification</title><style>body{font-family:system-ui,sans-serif;margin:2rem;line-height:1.4}nav{position:sticky;top:0;background:white;border-bottom:1px solid #ddd;padding:.5rem 0}nav a{margin-right:1rem}pre{background:#f7f7f7;padding:1rem;overflow:auto}.fail{background:#ffe6e6}.warn{background:#fff6cc}.pass,.ok{background:#e8ffe8}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:.35rem .5rem}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	b.WriteString("<nav><a href=\"#summary\">Summary</a><a href=\"#attestation\">Attestation</a><a href=\"#sbom\">SBOM diff</a><a href=\"#zip\">Release ZIP</a><a href=\"#retention\">Retention</a></nav>")
	fmt.Fprintf(&b, "<h1 id=\"summary\">rvwasm release verification</h1><p>Release <code class=%q>%s</code>; bundle <code>%s</code>; manifest <code>%s</code></p>", html.EscapeString(release.Status), html.EscapeString(release.Status), html.EscapeString(shortHash(release.BundleSHA256)), html.EscapeString(shortHash(release.ManifestSHA256)))
	fmt.Fprintf(&b, "<p>Attestation <code class=%q>%s</code>; SBOM diff <code class=%q>%s</code>; ZIP compare <code class=%q>%s</code>; retention <code class=%q>%s</code></p>", html.EscapeString(attv.Status), html.EscapeString(attv.Status), html.EscapeString(sbomDiff.Status), html.EscapeString(sbomDiff.Status), html.EscapeString(zipCmp.Status), html.EscapeString(zipCmp.Status), html.EscapeString(retention.Status), html.EscapeString(retention.Status))
	b.WriteString("<h2 id=\"attestation\">Attestation verification</h2><pre>")
	b.WriteString(html.EscapeString(AttestationVerificationString(attv)))
	b.WriteString("</pre>")
	b.WriteString("<h2 id=\"sbom\">SBOM diff</h2><pre>")
	b.WriteString(html.EscapeString(DependencyInventoryDiffString(sbomDiff)))
	b.WriteString("</pre>")
	b.WriteString("<h2 id=\"zip\">Release handoff ZIP comparison</h2><pre>")
	b.WriteString(html.EscapeString(ReleaseHandoffPackageComparisonString(zipCmp)))
	b.WriteString("</pre>")
	b.WriteString("<h2 id=\"retention\">Retention manifest</h2><pre>")
	b.WriteString(html.EscapeString(RetentionManifestString(retention)))
	b.WriteString("</pre>")
	return b.String()
}
