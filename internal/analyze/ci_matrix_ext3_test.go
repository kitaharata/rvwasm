package analyze

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReproZipChecksumManifest(t *testing.T) {
	insp := ReproZipInspection{
		Status:    "pass",
		ZipSHA256: strings.Repeat("a", 64),
		Files: []ReproZipFileInspection{
			{Path: "README.md", Bytes: 10, SHA256: strings.Repeat("1", 64), Required: true},
			{Path: "scripts/rvsmoke.sh", Bytes: 20, SHA256: strings.Repeat("2", 64), Required: true},
		},
	}
	m := BuildReproZipChecksumManifest(insp)
	if m.FileCount != 2 || m.RequiredCount != 2 || m.TotalBytes != 30 {
		t.Fatalf("bad counts: %+v", m)
	}
	if len(m.ManifestSHA256) != 64 {
		t.Fatalf("missing manifest hash: %q", m.ManifestSHA256)
	}
	if !strings.Contains(ReproZipChecksumManifestString(m), "README.md") {
		t.Fatalf("string output missing file")
	}
}

func TestMatrixResultAggregate(t *testing.T) {
	payload := map[string]any{
		"ci":        map[string]any{"status": "pass", "exit_code": 0, "phase": "linux"},
		"gate":      map[string]any{"status": "pass", "policy_name": "linux-boot", "checks": []any{map[string]any{"name": "trace_lines", "status": "pass"}}},
		"integrity": map[string]any{"status": "pass", "manifest": ArtifactManifest{Artifacts: []ArtifactEntry{{Role: "payload", SHA256: strings.Repeat("f", 64)}}}},
	}
	b, _ := json.Marshal(payload)
	failPayload := map[string]any{
		"ci":   map[string]any{"status": "fail", "exit_code": 1},
		"gate": map[string]any{"status": "fail", "policy_name": "strict", "checks": []any{map[string]any{"name": "top_stop_cause", "status": "fail", "observed": "illegal instruction"}}},
	}
	fb, _ := json.Marshal(failPayload)
	agg := BuildMatrixResultAggregate([]MatrixResultInput{{Name: "ok", JSON: string(b)}, {Name: "bad", JSON: string(fb)}})
	if agg.Status != "fail" || agg.Total != 2 || agg.Failed != 1 || agg.Passed != 1 {
		t.Fatalf("bad aggregate: %+v", agg)
	}
	if !strings.Contains(MatrixResultAggregateMarkdown(agg), "illegal instruction") {
		t.Fatalf("markdown missing cause")
	}
	if !strings.Contains(MatrixResultAggregateHTML(agg), "rvsmoke matrix aggregate") {
		t.Fatalf("html missing title")
	}
}
