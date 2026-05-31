package analyze

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/mem"
)

type MemoryObject struct {
	Type   string `json:"type"`
	Addr   uint64 `json:"addr"`
	Size   uint64 `json:"size,omitempty"`
	Detail string `json:"detail"`
}

var memorySignatures = []struct {
	typ string
	sig []byte
}{
	{"elf", []byte{0x7f, 'E', 'L', 'F'}},
	{"fdt", []byte{0xd0, 0x0d, 0xfe, 0xed}},
	{"gzip", []byte{0x1f, 0x8b}},
	{"xz", []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}},
	{"zstd", []byte{0x28, 0xb5, 0x2f, 0xfd}},
	{"bzip2", []byte("BZh")},
	{"squashfs", []byte("hsqs")},
	{"romfs", []byte("-rom1fs-")},
	{"cpio-newc", []byte("070701")},
	{"cpio-crc", []byte("070702")},
	{"cpio-trailer", []byte("TRAILER!!!")},
	{"ext4-super-magic", []byte{0x53, 0xef}},
	{"opensbi-string", []byte("OpenSBI")},
	{"u-boot-string", []byte("U-Boot")},
	{"linux-version-string", []byte("Linux version")},
	{"kernel-cmdline-string", []byte("Kernel command line")},
	{"busybox-string", []byte("BusyBox")},
	{"riscv-virtio-string", []byte("riscv-virtio")},
	{"root-arg-string", []byte("root=")},
	{"init-arg-string", []byte("init=")},
	{"rdinit-arg-string", []byte("rdinit=")},
}

func ScanMemoryObjects(b *mem.Bus, maxPerType int) []MemoryObject {
	if b == nil || len(b.DRAM) == 0 {
		return nil
	}
	if maxPerType <= 0 || maxPerType > 256 {
		maxPerType = 32
	}
	out := make([]MemoryObject, 0)
	for _, sig := range memorySignatures {
		pos := 0
		count := 0
		for count < maxPerType {
			i := bytes.Index(b.DRAM[pos:], sig.sig)
			if i < 0 {
				break
			}
			off := pos + i
			addr := b.DRAMBase + uint64(off)
			obj := MemoryObject{Type: sig.typ, Addr: addr, Detail: signatureDetail(sig.typ, b.DRAM[off:])}
			if sig.typ == "fdt" && off+8 <= len(b.DRAM) {
				obj.Size = uint64(binary.BigEndian.Uint32(b.DRAM[off+4 : off+8]))
			}
			if sig.typ == "elf" && off+0x20 <= len(b.DRAM) {
				obj.Detail = elfDetail(b.DRAM[off:])
			}
			if sig.typ == "ext4-super-magic" {
				obj.Detail = ext4Detail(addr)
			}
			out = append(out, obj)
			count++
			pos = off + len(sig.sig)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Addr == out[j].Addr {
			return out[i].Type < out[j].Type
		}
		return out[i].Addr < out[j].Addr
	})
	return out
}

func MemoryObjectTypeCounts(objs []MemoryObject) map[string]int {
	out := map[string]int{}
	for _, o := range objs {
		out[o.Type]++
	}
	return out
}

func MemoryObjectSummaryString(b *mem.Bus, maxPerType int) string {
	objs := ScanMemoryObjects(b, maxPerType)
	if len(objs) == 0 {
		return "<no known guest memory objects found>\n"
	}
	counts := MemoryObjectTypeCounts(objs)
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%-22s %d\n", k, counts[k])
	}
	return sb.String()
}

func ScanMemoryObjectsString(b *mem.Bus, maxPerType int) string {
	objs := ScanMemoryObjects(b, maxPerType)
	if len(objs) == 0 {
		return "<no known guest memory objects found>\n"
	}
	var sb strings.Builder
	for _, o := range objs {
		if o.Size != 0 {
			fmt.Fprintf(&sb, "%#016x %-22s size=%#x %s\n", o.Addr, o.Type, o.Size, o.Detail)
		} else {
			fmt.Fprintf(&sb, "%#016x %-22s %s\n", o.Addr, o.Type, o.Detail)
		}
	}
	return sb.String()
}

func signatureDetail(typ string, data []byte) string {
	switch typ {
	case "opensbi-string", "u-boot-string", "linux-version-string", "kernel-cmdline-string", "busybox-string", "riscv-virtio-string", "root-arg-string", "init-arg-string", "rdinit-arg-string":
		return quoteLine(data, 200)
	case "gzip":
		if len(data) >= 10 {
			return fmt.Sprintf("method=%d flags=%#x", data[2], data[3])
		}
	case "xz":
		return "XZ stream candidate"
	case "zstd":
		return "Zstandard frame candidate"
	case "bzip2":
		if len(data) >= 4 {
			return fmt.Sprintf("bzip2 level=%c", data[3])
		}
	case "squashfs":
		return "SquashFS superblock candidate"
	case "romfs":
		return "romfs image candidate"
	case "cpio-newc", "cpio-crc":
		return "initramfs/newc candidate"
	case "cpio-trailer":
		return "end of cpio archive marker"
	}
	return ""
}

func ext4Detail(sigAddr uint64) string {
	// ext2/3/4 magic lives at superblock offset 0x38. The superblock itself
	// is normally 1024 bytes from the start of the filesystem image, so this
	// points at a filesystem image candidate without assuming partition layout.
	if sigAddr >= 0x438 {
		return fmt.Sprintf("ext filesystem magic; possible image base %#x", sigAddr-0x438)
	}
	return "ext filesystem magic"
}

func elfDetail(data []byte) string {
	if len(data) < 0x20 {
		return "ELF header"
	}
	cls := "unknown"
	if data[4] == 1 {
		cls = "ELF32"
	} else if data[4] == 2 {
		cls = "ELF64"
	}
	end := "unknown-endian"
	if data[5] == 1 {
		end = "little-endian"
	} else if data[5] == 2 {
		end = "big-endian"
	}
	machine := uint16(0)
	if len(data) >= 20 {
		machine = binary.LittleEndian.Uint16(data[18:20])
	}
	return fmt.Sprintf("%s %s machine=%#x", cls, end, machine)
}

func quoteLine(data []byte, max int) string {
	if max <= 0 || max > 4096 {
		max = 160
	}
	end := 0
	for end < len(data) && end < max {
		c := data[end]
		if c == 0 || c == '\n' || c == '\r' {
			break
		}
		end++
	}
	return fmt.Sprintf("%q", string(data[:end]))
}
