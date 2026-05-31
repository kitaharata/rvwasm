package analyze

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DependencyModule is a compact SBOM-style module row parsed from go.mod.
type DependencyModule struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
	Replace string `json:"replace,omitempty"`
	Direct  bool   `json:"direct,omitempty"`
}

// DependencyInventory is intentionally SPDX-lite: it is stable, easy to diff,
// and does not require network access. It records Go module metadata plus the
// release artifacts that were produced by rvsmoke.
type DependencyInventory struct {
	SchemaVersion string             `json:"schema_version"`
	Status        string             `json:"status"`
	GoVersion     string             `json:"go_version,omitempty"`
	ModulePath    string             `json:"module_path,omitempty"`
	Modules       []DependencyModule `json:"modules,omitempty"`
	ArtifactKinds map[string]int     `json:"artifact_kinds,omitempty"`
	ArtifactFiles int                `json:"artifact_files,omitempty"`
	Warnings      []string           `json:"warnings,omitempty"`
	InventoryHash string             `json:"inventory_sha256,omitempty"`
}

func BuildDependencyInventory(goModText string, index CIArtifactIndex) DependencyInventory {
	inv := DependencyInventory{SchemaVersion: "rvwasm.sbom-lite.v1", Status: "ok", ArtifactKinds: map[string]int{}, ArtifactFiles: index.FileCount}
	inv.Modules, inv.ModulePath, inv.GoVersion, inv.Warnings = parseGoModInventory(goModText)
	for k, v := range index.Kinds {
		inv.ArtifactKinds[k] = v
	}
	if strings.TrimSpace(goModText) == "" {
		inv.Status = "warn"
		inv.Warnings = append(inv.Warnings, "go.mod text was not supplied")
	}
	if inv.ModulePath == "" && strings.TrimSpace(goModText) != "" {
		inv.Status = "warn"
		inv.Warnings = append(inv.Warnings, "module directive not found in go.mod")
	}
	sort.Slice(inv.Modules, func(i, j int) bool { return inv.Modules[i].Path < inv.Modules[j].Path })
	sort.Strings(inv.Warnings)
	clone := inv
	clone.InventoryHash = ""
	b, _ := json.Marshal(clone)
	sum := sha256.Sum256(b)
	inv.InventoryHash = hex.EncodeToString(sum[:])
	return inv
}

func parseGoModInventory(text string) ([]DependencyModule, string, string, []string) {
	modules := []DependencyModule{}
	warnings := []string{}
	modulePath := ""
	goVersion := ""
	inRequire := false
	inReplace := false
	replaces := map[string]string{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "module ") {
			modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			continue
		}
		if strings.HasPrefix(line, "go ") {
			goVersion = strings.TrimSpace(strings.TrimPrefix(line, "go "))
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if strings.HasPrefix(line, "replace (") {
			inReplace = true
			continue
		}
		if line == ")" {
			inRequire = false
			inReplace = false
			continue
		}
		if strings.HasPrefix(line, "require ") {
			mod, ok := parseRequireLine(strings.TrimSpace(strings.TrimPrefix(line, "require ")))
			if ok {
				modules = append(modules, mod)
			} else {
				warnings = append(warnings, "cannot parse require line: "+line)
			}
			continue
		}
		if inRequire {
			mod, ok := parseRequireLine(line)
			if ok {
				modules = append(modules, mod)
			} else {
				warnings = append(warnings, "cannot parse require line: "+line)
			}
			continue
		}
		if strings.HasPrefix(line, "replace ") || inReplace {
			repl := strings.TrimSpace(strings.TrimPrefix(line, "replace "))
			left, right, ok := strings.Cut(repl, "=>")
			if ok {
				leftFields := strings.Fields(left)
				if len(leftFields) > 0 {
					replaces[leftFields[0]] = strings.TrimSpace(right)
				}
			} else if repl != "" {
				warnings = append(warnings, "cannot parse replace line: "+line)
			}
		}
	}
	for i := range modules {
		if r := replaces[modules[i].Path]; r != "" {
			modules[i].Replace = r
		}
	}
	return modules, modulePath, goVersion, warnings
}

func parseRequireLine(line string) (DependencyModule, bool) {
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return DependencyModule{}, false
	}
	return DependencyModule{Path: fields[0], Version: fields[1], Direct: true}, true
}

func DependencyInventoryJSON(inv DependencyInventory) string {
	b, _ := json.MarshalIndent(inv, "", "  ")
	return string(b)
}

func DependencyInventoryString(inv DependencyInventory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "dependency inventory status=%s module=%s go=%s modules=%d artifacts=%d sha=%s\n", inv.Status, firstNonEmpty(inv.ModulePath, "-"), firstNonEmpty(inv.GoVersion, "-"), len(inv.Modules), inv.ArtifactFiles, shortHash(inv.InventoryHash))
	if len(inv.ArtifactKinds) != 0 {
		keys := make([]string, 0, len(inv.ArtifactKinds))
		for k := range inv.ArtifactKinds {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  artifact kind %-12s %d\n", k, inv.ArtifactKinds[k])
		}
	}
	for _, w := range inv.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	for _, m := range inv.Modules {
		repl := ""
		if m.Replace != "" {
			repl = " replace=" + m.Replace
		}
		fmt.Fprintf(&b, "  %s %s%s\n", m.Path, m.Version, repl)
	}
	return b.String()
}

// ProvenanceAttestation is an in-toto/SLSA-inspired statement. It is not a
// cryptographic signature; it is a deterministic attestation payload whose JSON
// can be hashed, stored, and signed by external CI if desired.
type ProvenanceSubject struct {
	Name, Digest, Kind string
	Bytes              int `json:"bytes,omitempty"`
}

const RvwasmProvenancePredicateV1 = "https://github.com/kitaharata/rvwasm/blob/main/docs/provenance-v1.md"

type ProvenanceAttestation struct {
	SchemaVersion   string              `json:"schema_version"`
	PredicateType   string              `json:"predicate_type"`
	Builder         string              `json:"builder"`
	Invocation      map[string]string   `json:"invocation,omitempty"`
	Materials       []ProvenanceSubject `json:"materials,omitempty"`
	Subjects        []ProvenanceSubject `json:"subjects,omitempty"`
	GeneratedAt     string              `json:"generated_at,omitempty"`
	AttestationHash string              `json:"attestation_sha256,omitempty"`
}

func BuildProvenanceAttestation(release ReleaseBundleManifest, inv DependencyInventory, index CIArtifactIndex, generatedAt string) ProvenanceAttestation {
	if strings.TrimSpace(generatedAt) == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	a := ProvenanceAttestation{SchemaVersion: "rvwasm.attestation.v1", PredicateType: RvwasmProvenancePredicateV1, Builder: "rvsmoke", GeneratedAt: generatedAt, Invocation: map[string]string{"module": inv.ModulePath, "go": inv.GoVersion, "release_status": release.Status}}
	if release.BundleSHA256 != "" {
		a.Materials = append(a.Materials, ProvenanceSubject{Name: "diagnostic-bundle", Kind: "bundle", Digest: release.BundleSHA256})
	}
	if release.ManifestSHA256 != "" {
		a.Materials = append(a.Materials, ProvenanceSubject{Name: "artifact-manifest", Kind: "manifest", Digest: release.ManifestSHA256})
	}
	if inv.InventoryHash != "" {
		a.Materials = append(a.Materials, ProvenanceSubject{Name: "dependency-inventory", Kind: "sbom-lite", Digest: inv.InventoryHash})
	}
	for _, e := range index.Entries {
		a.Subjects = append(a.Subjects, ProvenanceSubject{Name: e.Path, Kind: e.Kind, Digest: e.SHA256, Bytes: e.Bytes})
	}
	sort.Slice(a.Materials, func(i, j int) bool { return a.Materials[i].Name < a.Materials[j].Name })
	sort.Slice(a.Subjects, func(i, j int) bool { return a.Subjects[i].Name < a.Subjects[j].Name })
	clone := a
	clone.AttestationHash = ""
	b, _ := json.Marshal(clone)
	sum := sha256.Sum256(b)
	a.AttestationHash = hex.EncodeToString(sum[:])
	return a
}

func ProvenanceAttestationJSON(a ProvenanceAttestation) string {
	b, _ := json.MarshalIndent(a, "", "  ")
	return string(b)
}
func ProvenanceAttestationString(a ProvenanceAttestation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "provenance attestation builder=%s subjects=%d materials=%d sha=%s\n", a.Builder, len(a.Subjects), len(a.Materials), shortHash(a.AttestationHash))
	for _, m := range a.Materials {
		fmt.Fprintf(&b, "  material %-20s kind=%-10s sha=%s\n", m.Name, m.Kind, shortHash(m.Digest))
	}
	for _, s := range a.Subjects {
		fmt.Fprintf(&b, "  subject  %-28s kind=%-10s bytes=%d sha=%s\n", s.Name, s.Kind, s.Bytes, shortHash(s.Digest))
	}
	return b.String()
}

// ReleaseHandoffPackageInspection validates a higher-level release handoff zip
// containing release manifest, artifact index, SBOM-lite inventory and
// provenance attestation. This is separate from the minimal reproduction package.
type ReleaseHandoffFile struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Required bool   `json:"required"`
}
type ReleaseHandoffPackageInspection struct {
	Status     string               `json:"status"`
	ZipSHA256  string               `json:"zip_sha256,omitempty"`
	Files      []ReleaseHandoffFile `json:"files,omitempty"`
	Missing    []string             `json:"missing,omitempty"`
	Unexpected []string             `json:"unexpected,omitempty"`
	Issues     []ReproZipCheck      `json:"issues,omitempty"`
	Summary    []string             `json:"summary,omitempty"`
}

func ReleaseHandoffPackageFiles(release ReleaseBundleManifest, index CIArtifactIndex, inv DependencyInventory, att ProvenanceAttestation) map[string]string {
	return map[string]string{
		"README.md":                   releaseHandoffReadme(release, inv, att),
		"release-manifest.json":       ReleaseBundleManifestJSON(release),
		"ci-artifact-index.json":      CIArtifactIndexJSON(index),
		"dependency-inventory.json":   DependencyInventoryJSON(inv),
		"provenance-attestation.json": ProvenanceAttestationJSON(att),
		"release.html":                ReleaseBundleManifestHTML(release, MatrixResultAggregate{}, MatrixFlakeReport{}),
	}
}

func releaseHandoffReadme(release ReleaseBundleManifest, inv DependencyInventory, att ProvenanceAttestation) string {
	var b strings.Builder
	b.WriteString("# rvwasm release handoff\n\n")
	fmt.Fprintf(&b, "- Release status: `%s`\n- Bundle SHA-256: `%s`\n- Manifest SHA-256: `%s`\n- Dependency inventory SHA-256: `%s`\n- Attestation SHA-256: `%s`\n\n", release.Status, release.BundleSHA256, release.ManifestSHA256, inv.InventoryHash, att.AttestationHash)
	b.WriteString("This package contains metadata only. Firmware, kernel, disk and initrd bytes are referenced by SHA-256 pins in the release manifest and diagnostic bundle; they are not embedded here.\n")
	return b.String()
}

func InspectReleaseHandoffZipBytes(data []byte) (ReleaseHandoffPackageInspection, error) {
	r := ReleaseHandoffPackageInspection{ZipSHA256: shaBytes(data)}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		r.Status = "fail"
		r.Issues = append(r.Issues, ReproZipCheck{Name: "zip-open", Status: "fail", Detail: err.Error()})
		return r, err
	}
	required := map[string]bool{"README.md": true, "release-manifest.json": true, "ci-artifact-index.json": true, "dependency-inventory.json": true, "provenance-attestation.json": true, "release.html": true}
	seen := map[string]bool{}
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		unsafe := strings.HasPrefix(name, "/") || strings.Contains(name, "../") || name == ".." || strings.HasPrefix(name, "..")
		if unsafe {
			r.Issues = append(r.Issues, ReproZipCheck{Name: "path-safety:" + name, Status: "fail", Detail: "unsafe zip path"})
		}
		if seen[name] {
			r.Issues = append(r.Issues, ReproZipCheck{Name: "duplicate:" + name, Status: "warn", Detail: "duplicate file path"})
		}
		seen[name] = true
		rc, err := f.Open()
		if err != nil {
			r.Issues = append(r.Issues, ReproZipCheck{Name: "read:" + name, Status: "fail", Detail: err.Error()})
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		_ = rc.Close()
		if err != nil {
			r.Issues = append(r.Issues, ReproZipCheck{Name: "read:" + name, Status: "fail", Detail: err.Error()})
			continue
		}
		req := required[name]
		r.Files = append(r.Files, ReleaseHandoffFile{Path: name, Bytes: int64(len(b)), SHA256: shaBytes(b), Required: req})
		if !req {
			r.Unexpected = append(r.Unexpected, name)
		}
		switch name {
		case "release-manifest.json":
			var x ReleaseBundleManifest
			if err := json.Unmarshal(b, &x); err != nil {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "release-manifest", Status: "fail", Detail: err.Error()})
			} else {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "release-manifest", Status: "pass", Detail: x.Status})
			}
		case "ci-artifact-index.json":
			var x CIArtifactIndex
			if err := json.Unmarshal(b, &x); err != nil {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "artifact-index", Status: "fail", Detail: err.Error()})
			} else {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "artifact-index", Status: "pass", Detail: fmt.Sprintf("files=%d", x.FileCount)})
			}
		case "dependency-inventory.json":
			var x DependencyInventory
			if err := json.Unmarshal(b, &x); err != nil {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "dependency-inventory", Status: "fail", Detail: err.Error()})
			} else {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "dependency-inventory", Status: "pass", Detail: x.Status})
			}
		case "provenance-attestation.json":
			var x ProvenanceAttestation
			if err := json.Unmarshal(b, &x); err != nil {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "provenance-attestation", Status: "fail", Detail: err.Error()})
			} else {
				r.Issues = append(r.Issues, ReproZipCheck{Name: "provenance-attestation", Status: "pass", Detail: shortHash(x.AttestationHash)})
			}
		}
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
	for p := range required {
		if !seen[p] {
			r.Missing = append(r.Missing, p)
		}
	}
	sort.Strings(r.Missing)
	sort.Strings(r.Unexpected)
	if len(r.Missing) != 0 {
		r.Issues = append(r.Issues, ReproZipCheck{Name: "required-files", Status: "fail", Detail: strings.Join(r.Missing, ", ")})
	} else {
		r.Issues = append(r.Issues, ReproZipCheck{Name: "required-files", Status: "pass", Detail: "all required files present"})
	}
	r.Status = statusFromChecks(r.Issues)
	r.Summary = []string{fmt.Sprintf("files=%d missing=%d unexpected=%d", len(r.Files), len(r.Missing), len(r.Unexpected))}
	return r, nil
}

func ReleaseHandoffPackageInspectionJSON(r ReleaseHandoffPackageInspection) string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
func ReleaseHandoffPackageInspectionString(r ReleaseHandoffPackageInspection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release handoff zip status=%s files=%d missing=%d unexpected=%d sha=%s\n", r.Status, len(r.Files), len(r.Missing), len(r.Unexpected), shortHash(r.ZipSHA256))
	for _, s := range r.Summary {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	for _, c := range r.Issues {
		fmt.Fprintf(&b, "  %-28s %-5s %s\n", c.Name, c.Status, c.Detail)
	}
	return b.String()
}

func ReleaseHandoffPackageHTML(r ReleaseHandoffPackageInspection) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><title>rvwasm release handoff inspection</title><style>body{font-family:system-ui,sans-serif;margin:2rem}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:.35rem .5rem}.fail{background:#ffe6e6}.warn{background:#fff6cc}.pass{background:#e8ffe8}@media(max-width:720px){html{box-sizing:border-box;-webkit-text-size-adjust:100%}*,*:before,*:after{box-sizing:inherit}img,svg,canvas{max-width:100%;height:auto}body{margin:1rem!important;font-size:15px;line-height:1.45}nav{position:static!important;overflow-x:auto;white-space:nowrap;padding:.5rem 0}nav a{display:inline-block;margin:.25rem .75rem .25rem 0}table{display:block;max-width:100%;width:100%;overflow-x:auto;-webkit-overflow-scrolling:touch}td,th{white-space:normal;overflow-wrap:anywhere}pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;-webkit-overflow-scrolling:touch;padding:.75rem!important}code{overflow-wrap:anywhere}button,input,select,textarea{max-width:100%;font:inherit}button{min-height:40px}h1{font-size:1.55rem}h2{font-size:1.25rem}}</style>")
	fmt.Fprintf(&b, "<h1>rvwasm release handoff inspection</h1><p>Status: <code>%s</code>; zip <code>%s</code></p>", html.EscapeString(r.Status), html.EscapeString(shortHash(r.ZipSHA256)))
	b.WriteString("<table><thead><tr><th>File</th><th>Bytes</th><th>SHA-256</th><th>Required</th></tr></thead><tbody>")
	for _, f := range r.Files {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%d</td><td><code>%s</code></td><td>%v</td></tr>", html.EscapeString(f.Path), f.Bytes, html.EscapeString(shortHash(f.SHA256)), f.Required)
	}
	b.WriteString("</tbody></table><h2>Checks</h2><ul>")
	for _, c := range r.Issues {
		fmt.Fprintf(&b, "<li class=%q><b>%s</b>: %s %s</li>", html.EscapeString(c.Status), html.EscapeString(c.Name), html.EscapeString(c.Status), html.EscapeString(c.Detail))
	}
	b.WriteString("</ul><h2>JSON</h2><pre>")
	b.WriteString(html.EscapeString(ReleaseHandoffPackageInspectionJSON(r)))
	b.WriteString("</pre>")
	return b.String()
}
