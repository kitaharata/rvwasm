package analyze

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestCompareArtifactManifests(t *testing.T) {
	base := ArtifactManifest{BootArgs: "a", HartCount: 1, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 3, LoadAddr: 0x80000000, EndAddr: 0x80000002, SHA256: "aaaa"}}}
	cur := ArtifactManifest{BootArgs: "b", HartCount: 2, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 4, LoadAddr: 0x80000000, EndAddr: 0x80000003, SHA256: "bbbb"}, {Role: "payload", Bytes: 1}}}
	bj, _ := json.Marshal(base)
	rows := CompareArtifactManifests(cur, string(bj))
	got := ArtifactManifestDiffString(rows)
	if !strings.Contains(got, "boot_args") || !strings.Contains(got, "payload") {
		t.Fatalf("unexpected diff: %s", got)
	}
}

func TestSuggestBreakpoints(t *testing.T) {
	causes := []StopCauseCandidate{{Score: 90, Category: "illegal-instruction", Summary: "bad insn", Evidence: []string{"trap pc=0x80201234"}}}
	hits := []mem.WatchpointHit{{Seq: 2, Kind: "write", Name: "page", Op: "write", Addr: 0x81000000, End: 0x81000007}}
	rows := SuggestBreakpoints(causes, "000 pc=0x80205678 asm=ecall", hits, 8)
	if len(rows) < 2 || rows[0].Address != 0x80201234 {
		t.Fatalf("bad suggestions: %+v", rows)
	}
	if !strings.Contains(BreakpointSuggestionsString(rows), "command:") {
		t.Fatal("missing command text")
	}
}

func TestClusterSmokeFailures(t *testing.T) {
	rows := []SmokeSummary{{Preset: "a", Ran: 10, Phase: "linux", TopCause: "virtqueue: bad"}, {Preset: "b", Ran: 20, Phase: "linux", TopCause: "virtqueue: other"}, {Preset: "c", Ran: 5, Phase: "opensbi", TopCause: "illegal-instruction: x"}}
	clusters := ClusterSmokeFailures(rows)
	if len(clusters) != 2 || clusters[0].Count != 2 {
		t.Fatalf("bad clusters: %+v", clusters)
	}
	if !strings.Contains(SmokeFailureClustersString(clusters), "QueueReady") {
		t.Fatal("missing query hint")
	}
}

func TestCompressDiagnosticBundleJSON(t *testing.T) {
	jsonText := DiagnosticBundleJSON(DiagnosticBundle{Notes: []string{"hello"}})
	c, err := CompressDiagnosticBundleJSON(jsonText)
	if err != nil || c.Encoding != "gzip+base64" || c.Base64 == "" || c.GzipBytes == 0 {
		t.Fatalf("bad compression: %+v err=%v", c, err)
	}
}
