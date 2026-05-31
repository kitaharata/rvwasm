package boot

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestVirtDTBIncludesInitrdAndBootargs(t *testing.T) {
	dtb := BuildVirtDTBConfig(VirtDTBConfig{
		MemBase:     0x80000000,
		MemSize:     128 * 1024 * 1024,
		TimebaseHz:  10000000,
		BootArgs:    "console=ttyS0 root=/dev/ram0 rw",
		InitrdStart: 0x84000000,
		InitrdEnd:   0x84100000,
	})
	for _, needle := range [][]byte{
		[]byte("console=ttyS0 root=/dev/ram0 rw"),
		[]byte("linux,initrd-start"),
		[]byte("linux,initrd-end"),
		[]byte("virtio_mmio@10002000"),
		[]byte("virtio_mmio@10003000"),
		[]byte("virtio_mmio@10004000"),
		[]byte("virtio_mmio@10005000"),
		[]byte("local-mac-address"),
	} {
		if !bytes.Contains(dtb, needle) {
			t.Fatalf("DTB missing %q", needle)
		}
	}
	var start [8]byte
	binary.BigEndian.PutUint64(start[:], 0x84000000)
	if !bytes.Contains(dtb, start[:]) {
		t.Fatalf("DTB missing big-endian initrd start")
	}
}

func TestBuildVirtDTBMultiHartMentionsCPUs(t *testing.T) {
	dtb := BuildVirtDTBConfig(VirtDTBConfig{HartCount: 2})
	if !bytes.Contains(dtb, []byte("cpu@0")) || !bytes.Contains(dtb, []byte("cpu@1")) {
		t.Fatalf("multi-hart cpu nodes missing")
	}
}

func TestVirtDTBIncludesSimpleFramebuffer(t *testing.T) {
	dtb := BuildVirtDTBConfig(VirtDTBConfig{
		Framebuffer: FramebufferConfig{
			Enabled: true,
			Base:    0x86000000,
			Size:    1024 * 768 * 4,
			Width:   1024,
			Height:  768,
			Stride:  1024 * 4,
			Format:  "a8r8g8b8",
		},
	})
	for _, needle := range [][]byte{
		[]byte("framebuffer@86000000"),
		[]byte("simple-framebuffer"),
		[]byte("a8r8g8b8"),
	} {
		if !bytes.Contains(dtb, needle) {
			t.Fatalf("DTB missing %q", needle)
		}
	}
}

func TestVirtDTBContainsVirtioGPUNode(t *testing.T) {
	dtb := BuildVirtDTBConfig(VirtDTBConfig{})
	if !bytes.Contains(dtb, []byte("virtio_mmio@10006000")) {
		t.Fatalf("DTB missing virtio-gpu MMIO node")
	}
}

func TestVirtDTBIncludesSysconResetNodes(t *testing.T) {
	dtb := BuildVirtDTBConfig(VirtDTBConfig{})
	for _, needle := range [][]byte{
		[]byte("test@100000"),
		[]byte("sifive,test1"),
		[]byte("syscon"),
		[]byte("syscon-reboot"),
		[]byte("syscon-poweroff"),
		[]byte("regmap"),
		[]byte("offset"),
		[]byte("value"),
	} {
		if !bytes.Contains(dtb, needle) {
			t.Fatalf("DTB missing %q", needle)
		}
	}
	var rebootValue [4]byte
	binary.BigEndian.PutUint32(rebootValue[:], sysconRebootValue)
	if !bytes.Contains(dtb, rebootValue[:]) {
		t.Fatalf("DTB missing syscon reboot value")
	}
	var poweroffValue [4]byte
	binary.BigEndian.PutUint32(poweroffValue[:], sysconPoweroffValue)
	if !bytes.Contains(dtb, poweroffValue[:]) {
		t.Fatalf("DTB missing syscon poweroff value")
	}
}
