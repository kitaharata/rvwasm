package mem

import (
	"strings"
	"testing"
)

func TestWatchpointHitsTimeline(t *testing.T) {
	b := NewBus(0x80000000, 0x1000)
	b.AddWriteWatchpoint(0x80000010, 4, "wp")
	if err := b.Write(0x80000010, 4, 0x1234); err != nil {
		t.Fatal(err)
	}
	hits := b.WatchpointHits(8)
	if len(hits) != 1 || hits[0].Name != "wp" || hits[0].Kind != "write" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
	if !strings.Contains(b.WatchpointHitsString(8, "wp"), "wp") {
		t.Fatalf("missing hit string")
	}
	b.ClearWatchpointHits()
	if len(b.WatchpointHits(8)) != 0 {
		t.Fatalf("expected cleared hits")
	}
}
