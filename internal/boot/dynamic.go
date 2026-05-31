package boot

import "encoding/binary"

const (
	FWDynamicInfoMagic   uint64 = 0x4942534f
	FWDynamicInfoVersion uint64 = 2
	FWDynamicInfoSize           = 6 * 8
	FWDynamicNextModeS   uint64 = 1
	FWDynamicNextModeM   uint64 = 3
)

type FWDynamicInfo struct {
	NextAddr uint64
	NextMode uint64
	Options  uint64
	BootHart uint64
}

func BuildFWDynamicInfo(info FWDynamicInfo) []byte {
	buf := make([]byte, FWDynamicInfoSize)
	binary.LittleEndian.PutUint64(buf[0:], FWDynamicInfoMagic)
	binary.LittleEndian.PutUint64(buf[8:], FWDynamicInfoVersion)
	binary.LittleEndian.PutUint64(buf[16:], info.NextAddr)
	binary.LittleEndian.PutUint64(buf[24:], info.NextMode)
	binary.LittleEndian.PutUint64(buf[32:], info.Options)
	binary.LittleEndian.PutUint64(buf[40:], info.BootHart)
	return buf
}
