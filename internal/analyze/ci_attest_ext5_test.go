package analyze

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"testing"
)

func TestDependencyInventoryParsesGoMod(t *testing.T) {
	idx := BuildCIArtifactIndex([]CIArtifactInput{{Path: "rvsmoke.html", Kind: "html", Data: []byte("x")}})
	inv := BuildDependencyInventory("module example.com/rv\n\ngo 1.23.2\n\nrequire (\n example.com/a v1.2.3\n)\nreplace example.com/a => ../a\n", idx)
	if inv.ModulePath != "example.com/rv" || inv.GoVersion != "1.23.2" || len(inv.Modules) != 1 {
		t.Fatalf("bad inventory: %+v", inv)
	}
	if inv.Modules[0].Replace != "../a" || inv.ArtifactKinds["html"] != 1 || inv.InventoryHash == "" {
		t.Fatalf("missing inventory details: %+v", inv)
	}
}

func TestProvenanceAttestationAndReleaseZipInspection(t *testing.T) {
	idx := BuildCIArtifactIndex([]CIArtifactInput{{Path: "rvsmoke.json", Kind: "json", Data: []byte(`{"ok":true}`)}})
	inv := BuildDependencyInventory("module rvwasm\ngo 1.23.2\n", idx)
	rel := ReleaseBundleManifest{Status: "pass", BundleSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ManifestSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ArtifactIndex: idx}
	att := BuildProvenanceAttestation(rel, inv, idx, "2026-05-30T00:00:00Z")
	if att.AttestationHash == "" || len(att.Subjects) != 1 || len(att.Materials) < 2 {
		t.Fatalf("bad attestation: %+v", att)
	}
	if att.PredicateType != RvwasmProvenancePredicateV1 {
		t.Fatalf("unexpected predicate type: %q", att.PredicateType)
	}
	files := ReleaseHandoffPackageFiles(rel, idx, inv, att)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, text := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	insp, err := InspectReleaseHandoffZipBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if insp.Status != "pass" || len(insp.Missing) != 0 {
		t.Fatalf("bad inspection: %+v", insp)
	}
	if !json.Valid([]byte(ReleaseHandoffPackageInspectionJSON(insp))) {
		t.Fatal("inspection json invalid")
	}
}
