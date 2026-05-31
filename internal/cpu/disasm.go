package cpu

import "fmt"

func disasm(raw uint32, width int) string {
	if width == 16 {
		return disasmC(uint16(raw))
	}
	op := raw & 0x7f
	rd := (raw >> 7) & 0x1f
	f3 := (raw >> 12) & 7
	rs1 := (raw >> 15) & 0x1f
	rs2 := (raw >> 20) & 0x1f
	f7 := (raw >> 25) & 0x7f
	x := func(r uint32) string { return fmt.Sprintf("x%d", r) }
	switch op {
	case 0x37:
		return fmt.Sprintf("lui %s,%#x", x(rd), immU(raw))
	case 0x17:
		return fmt.Sprintf("auipc %s,%#x", x(rd), immU(raw))
	case 0x6f:
		return fmt.Sprintf("jal %s,%+d", x(rd), int64(immJ(raw)))
	case 0x67:
		return fmt.Sprintf("jalr %s,%d(%s)", x(rd), int64(immI(raw)), x(rs1))
	case 0x63:
		m := map[uint32]string{0: "beq", 1: "bne", 4: "blt", 5: "bge", 6: "bltu", 7: "bgeu"}[f3]
		if m == "" {
			return fmt.Sprintf("unknown.%08x", raw)
		}
		return fmt.Sprintf("%s %s,%s,%+d", m, x(rs1), x(rs2), int64(immB(raw)))
	case 0x03:
		m := map[uint32]string{0: "lb", 1: "lh", 2: "lw", 3: "ld", 4: "lbu", 5: "lhu", 6: "lwu"}[f3]
		if m == "" {
			return fmt.Sprintf("unknown.%08x", raw)
		}
		return fmt.Sprintf("%s %s,%d(%s)", m, x(rd), int64(immI(raw)), x(rs1))
	case 0x23:
		m := map[uint32]string{0: "sb", 1: "sh", 2: "sw", 3: "sd"}[f3]
		if m == "" {
			return fmt.Sprintf("unknown.%08x", raw)
		}
		return fmt.Sprintf("%s %s,%d(%s)", m, x(rs2), int64(immS(raw)), x(rs1))
	case 0x13:
		shamt := (raw >> 20) & 0x3f
		switch f3 {
		case 0:
			return fmt.Sprintf("addi %s,%s,%d", x(rd), x(rs1), int64(immI(raw)))
		case 2:
			return fmt.Sprintf("slti %s,%s,%d", x(rd), x(rs1), int64(immI(raw)))
		case 3:
			return fmt.Sprintf("sltiu %s,%s,%d", x(rd), x(rs1), int64(immI(raw)))
		case 4:
			return fmt.Sprintf("xori %s,%s,%d", x(rd), x(rs1), int64(immI(raw)))
		case 6:
			return fmt.Sprintf("ori %s,%s,%d", x(rd), x(rs1), int64(immI(raw)))
		case 7:
			return fmt.Sprintf("andi %s,%s,%d", x(rd), x(rs1), int64(immI(raw)))
		case 1:
			return fmt.Sprintf("slli %s,%s,%d", x(rd), x(rs1), shamt)
		case 5:
			if f7 == 0x20 {
				return fmt.Sprintf("srai %s,%s,%d", x(rd), x(rs1), shamt)
			}
			return fmt.Sprintf("srli %s,%s,%d", x(rd), x(rs1), shamt)
		}
	case 0x1b:
		shamt := (raw >> 20) & 0x1f
		switch f3 {
		case 0:
			return fmt.Sprintf("addiw %s,%s,%d", x(rd), x(rs1), int64(immI(raw)))
		case 1:
			return fmt.Sprintf("slliw %s,%s,%d", x(rd), x(rs1), shamt)
		case 5:
			if f7 == 0x20 {
				return fmt.Sprintf("sraiw %s,%s,%d", x(rd), x(rs1), shamt)
			}
			return fmt.Sprintf("srliw %s,%s,%d", x(rd), x(rs1), shamt)
		}
	case 0x33:
		if f7 == 1 {
			return disasmM("", rd, rs1, rs2, f3, x)
		}
		m := map[[2]uint32]string{{0, 0}: "add", {0x20, 0}: "sub", {0, 1}: "sll", {0, 2}: "slt", {0, 3}: "sltu", {0, 4}: "xor", {0, 5}: "srl", {0x20, 5}: "sra", {0, 6}: "or", {0, 7}: "and"}[[2]uint32{f7, f3}]
		if m != "" {
			return fmt.Sprintf("%s %s,%s,%s", m, x(rd), x(rs1), x(rs2))
		}
	case 0x3b:
		if f7 == 1 {
			return disasmM("w", rd, rs1, rs2, f3, x)
		}
		m := map[[2]uint32]string{{0, 0}: "addw", {0x20, 0}: "subw", {0, 1}: "sllw", {0, 5}: "srlw", {0x20, 5}: "sraw"}[[2]uint32{f7, f3}]
		if m != "" {
			return fmt.Sprintf("%s %s,%s,%s", m, x(rd), x(rs1), x(rs2))
		}
	case 0x0f:
		if f3 == 1 {
			return "fence.i"
		}
		return "fence"
	case 0x73:
		if f3 == 0 {
			switch raw {
			case 0x00000073:
				return "ecall"
			case 0x00100073:
				return "ebreak"
			case 0x30200073:
				return "mret"
			case 0x10200073:
				return "sret"
			case 0x10500073:
				return "wfi"
			}
		}
		csr := (raw >> 20) & 0xfff
		m := map[uint32]string{1: "csrrw", 2: "csrrs", 3: "csrrc", 5: "csrrwi", 6: "csrrsi", 7: "csrrci"}[f3]
		if m != "" {
			return fmt.Sprintf("%s %s,%#x,%s", m, x(rd), csr, x(rs1))
		}
	}
	return fmt.Sprintf("unknown.%0*x", width/4, raw)
}

func disasmM(suffix string, rd, rs1, rs2, f3 uint32, x func(uint32) string) string {
	base := []string{"mul", "mulh", "mulhsu", "mulhu", "div", "divu", "rem", "remu"}
	if int(f3) >= len(base) {
		return "unknown.m"
	}
	m := base[f3]
	if suffix == "w" {
		switch f3 {
		case 0:
			m = "mulw"
		case 4:
			m = "divw"
		case 5:
			m = "divuw"
		case 6:
			m = "remw"
		case 7:
			m = "remuw"
		default:
			return "unknown.mw"
		}
	}
	return fmt.Sprintf("%s %s,%s,%s", m, x(rd), x(rs1), x(rs2))
}

func disasmC(inst uint16) string {
	op := inst & 3
	funct3 := (inst >> 13) & 7
	switch op {
	case 0:
		switch funct3 {
		case 0:
			return "c.addi4spn"
		case 2:
			return "c.lw"
		case 3:
			return "c.ld"
		case 6:
			return "c.sw"
		case 7:
			return "c.sd"
		}
	case 1:
		switch funct3 {
		case 0:
			return "c.addi"
		case 1:
			return "c.addiw/jal"
		case 2:
			return "c.li"
		case 3:
			return "c.lui/addi16sp"
		case 4:
			return "c.misc-alu"
		case 5:
			return "c.j"
		case 6:
			return "c.beqz"
		case 7:
			return "c.bnez"
		}
	case 2:
		switch funct3 {
		case 0:
			return "c.slli"
		case 2:
			return "c.lwsp"
		case 3:
			return "c.ldsp"
		case 4:
			return "c.jr/mv/ebreak/jalr/add"
		case 6:
			return "c.swsp"
		case 7:
			return "c.sdsp"
		}
	}
	return fmt.Sprintf("c.unknown.%04x", inst)
}

// DisasmForTest exposes the trace decoder to unit tests without making it part
// of the emulator's public API.
func DisasmForTest(raw uint32, width int) string { return disasm(raw, width) }
