package analyze

import (
	"fmt"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

// VirtqueueChainsDOT returns a Graphviz DOT representation of recently
// notified virtqueue descriptor chains. The graph is intentionally text-only
// so it can be copied from the browser into graphviz, vscode, or web viewers.
func VirtqueueChainsDOT(b *mem.Bus, events []mem.AccessEvent, maxHeads int) string {
	chains := VirtqueueChains(b, events, maxHeads)
	var sb strings.Builder
	sb.WriteString("digraph virtqueue_chains {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=record,fontname=\"monospace\",fontsize=10];\n")
	if len(chains) == 0 {
		sb.WriteString("  empty [label=\"no ready virtqueue descriptor chains found\"];\n")
		sb.WriteString("}\n")
		return sb.String()
	}
	for ci, c := range chains {
		clusterName := fmt.Sprintf("cluster_%d", ci)
		fmt.Fprintf(&sb, "  subgraph %s {\n", clusterName)
		fmt.Fprintf(&sb, "    label=%q;\n", fmt.Sprintf("%s q=%d slot=%d head=%d avail=%d used=%d", c.Device, c.Queue, c.RingSlot, c.Head, c.AvailIdx, c.UsedIdx))
		fmt.Fprintf(&sb, "    meta_%d [shape=box,label=%q];\n", ci, dotEscape(fmt.Sprintf("notifySeq=%d ready=%v indirect=%v", c.LastNotifyAt, c.Ready, c.Indirect)))
		prev := fmt.Sprintf("meta_%d", ci)
		for di, d := range c.Descriptors {
			node := fmt.Sprintf("c%d_d%d", ci, di)
			dir := "R"
			if d.Writable {
				dir = "W"
			}
			flags := []string{}
			if d.Flags&vqDescFNext != 0 {
				flags = append(flags, "NEXT")
			}
			if d.Flags&vqDescFWrite != 0 {
				flags = append(flags, "WRITE")
			}
			if d.Flags&vqDescFIndirect != 0 {
				flags = append(flags, "INDIRECT")
			}
			if len(flags) == 0 {
				flags = append(flags, "0")
			}
			label := fmt.Sprintf("{%03d %s|addr=%#x|len=%d|flags=%s|next=%d", d.Index, dir, d.Addr, d.Len, strings.Join(flags, "+"), d.Next)
			if d.Preview != "" {
				label += "|" + d.Preview
			}
			label += "}"
			fmt.Fprintf(&sb, "    %s [label=%q];\n", node, dotEscape(label))
			fmt.Fprintf(&sb, "    %s -> %s;\n", prev, node)
			prev = node
		}
		if c.Error != "" {
			errNode := fmt.Sprintf("c%d_err", ci)
			fmt.Fprintf(&sb, "    %s [shape=box,color=red,label=%q];\n", errNode, dotEscape("error: "+c.Error))
			fmt.Fprintf(&sb, "    %s -> %s [color=red];\n", prev, errNode)
		}
		sb.WriteString("  }\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

func dotEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
