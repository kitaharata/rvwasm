package dev

import "testing"

func TestSysconPoweroffAndRebootCallbacks(t *testing.T) {
	var poweroff, reboot int
	s := NewSyscon(func() { poweroff++ }, func() { reboot++ })
	if err := s.Write(SysconBase, 4, uint64(SysconPoweroffValue)); err != nil {
		t.Fatal(err)
	}
	if poweroff != 1 || reboot != 0 {
		t.Fatalf("poweroff=%d reboot=%d", poweroff, reboot)
	}
	if got, _ := s.Read(SysconBase, 4); got != uint64(SysconPoweroffValue) {
		t.Fatalf("value after poweroff write = %#x", got)
	}
	if err := s.Write(SysconBase, 4, uint64(SysconRebootValue)); err != nil {
		t.Fatal(err)
	}
	if poweroff != 1 || reboot != 1 {
		t.Fatalf("poweroff=%d reboot=%d", poweroff, reboot)
	}
}
