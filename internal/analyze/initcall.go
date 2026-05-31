package analyze

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type InitcallEvent struct {
	Line     int    `json:"line"`
	Category string `json:"category"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Text     string `json:"text"`
}

var initcallPatterns = []struct {
	cat string
	re  *regexp.Regexp
}{
	{"initcall-start", regexp.MustCompile(`(?i)calling\s+([A-Za-z0-9_.$+-]+)`)},
	{"initcall-return", regexp.MustCompile(`(?i)initcall\s+([A-Za-z0-9_.$+-]+).*returned\s+(-?\d+)`)},
	{"driver-probe", regexp.MustCompile(`(?i)(probe of|probing|registered|attached|detected|found)\s+([^:;,]+)`)},
	{"driver-fail", regexp.MustCompile(`(?i)(probe failed|failed to probe|error|failed|timeout|timed out)`)},
	{"virtio", regexp.MustCompile(`(?i)(virtio[-_a-z0-9]*|virtio-mmio|vda|hvc0)`)},
	{"storage", regexp.MustCompile(`(?i)(blk|vda|mmc|scsi|ata|rootfs|vfs)`)},
	{"console", regexp.MustCompile(`(?i)(console|ttyS|hvc|serial)`)},
	{"network", regexp.MustCompile(`(?i)(net|eth|IPv6|IPv4|link is)`)},
	{"graphics", regexp.MustCompile(`(?i)(drm|fbcon|framebuffer|simple-framebuffer|virtio_gpu)`)},
}

func ClassifyInitcalls(console string, max int) []InitcallEvent {
	if max <= 0 || max > 512 {
		max = 160
	}
	lines := strings.Split(console, "\n")
	out := make([]InitcallEvent, 0, max)
	seen := map[string]bool{}
	for i, raw := range lines {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		for _, p := range initcallPatterns {
			m := p.re.FindStringSubmatch(line)
			if len(m) == 0 {
				continue
			}
			name := ""
			status := ""
			if len(m) > 1 {
				name = strings.TrimSpace(m[len(m)-1])
			}
			if p.cat == "initcall-return" && len(m) > 2 {
				name = m[1]
				status = m[2]
			}
			if p.cat == "driver-fail" {
				status = "fail"
			}
			key := fmt.Sprintf("%d:%s:%s", i, p.cat, name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, InitcallEvent{Line: i + 1, Category: p.cat, Name: truncate(name, 80), Status: status, Text: truncate(line, 180)})
			if len(out) >= max {
				return out
			}
		}
	}
	return out
}

func InitcallClassifierString(console string, max int) string {
	events := ClassifyInitcalls(console, max)
	if len(events) == 0 {
		return "<no initcall/driver probe lines found in console log>\n"
	}
	var sb strings.Builder
	for _, ev := range events {
		name := ev.Name
		if name == "" {
			name = "-"
		}
		status := ev.Status
		if status == "" {
			status = "-"
		}
		fmt.Fprintf(&sb, "L%-5d %-16s %-32s %-8s %s\n", ev.Line, ev.Category, name, status, ev.Text)
	}
	return sb.String()
}

func InitcallCategoryCounts(console string) map[string]int {
	counts := map[string]int{}
	for _, ev := range ClassifyInitcalls(console, 512) {
		counts[ev.Category]++
	}
	return counts
}

func SortedInitcallCategoryCounts(console string) string {
	counts := InitcallCategoryCounts(console)
	if len(counts) == 0 {
		return "<no categories>\n"
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%-16s %d\n", k, counts[k])
	}
	return sb.String()
}

func InitcallTimelineString(console string, max int) string {
	if max <= 0 || max > 512 {
		max = 160
	}
	events := ClassifyInitcalls(console, max)
	if len(events) == 0 {
		return "<no initcall/driver probe timeline events found>\n"
	}
	var sb strings.Builder
	lastCat := ""
	for _, ev := range events {
		if ev.Category != lastCat {
			fmt.Fprintf(&sb, "\n[%s]\n", ev.Category)
			lastCat = ev.Category
		}
		name := ev.Name
		if name == "" {
			name = "-"
		}
		status := ev.Status
		if status == "" {
			status = "-"
		}
		fmt.Fprintf(&sb, "L%-5d %-32s %-8s %s\n", ev.Line, name, status, ev.Text)
	}
	return strings.TrimLeft(sb.String(), "\n")
}
