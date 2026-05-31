//go:build js && wasm

package main

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/kitaharata/rvwasm/internal/analyze"
	"github.com/kitaharata/rvwasm/internal/boot"
	"github.com/kitaharata/rvwasm/internal/cpu"
	"github.com/kitaharata/rvwasm/internal/debugmap"
	"github.com/kitaharata/rvwasm/internal/dev"
	"github.com/kitaharata/rvwasm/internal/mem"
)

const (
	dramBase    = uint64(0x80000000)
	dramSize    = uint64(128 * 1024 * 1024)
	fwBase      = uint64(0x80000000)
	dtbAddr     = uint64(0x87e00000)
	dynamicAddr = uint64(0x87dff000)
	payloadAddr = uint64(0x80200000)
	initrdAddr  = uint64(0x84000000)
	fbAddr      = uint64(0x86000000)
	fbWidth     = 1024
	fbHeight    = 768
	fbStride    = fbWidth * 4
	fbSize      = fbStride * fbHeight
)

type Breakpoint struct {
	PC        uint64
	AfterHits uint64
	Mode      string
	Hart      int
	Hits      uint64
}

type Emulator struct {
	bus            *mem.Bus
	hart           *cpu.Hart
	harts          []*cpu.Hart
	activeHart     int
	hartCount      int
	uart           *dev.UART
	plic           *dev.PLIC
	clint          *dev.CLINT
	blk            *dev.VirtioBlock
	vcon           *dev.VirtioConsole
	net            *dev.VirtioNet
	rng            *dev.VirtioRNG
	inputDev       *dev.VirtioInput
	gpu            *dev.VirtioGPU
	syscon         *dev.Syscon
	entry          uint64
	nextAddr       uint64
	initrdBase     uint64
	initrdEnd      uint64
	bootArgs       string
	running        bool
	firmwareLoaded bool
	runToken       uint64
	runPending     uint64
	runLastSteps   int
	runLastYield   time.Time
	console        []byte
	terminalLog    []byte
	customDTB      []byte
	symbols        *debugmap.Table
	sbiShim        bool
	hsmState       map[uint64]string
	breakpoints    map[uint64]Breakpoint
	stopReason     string
	traceFilter    string
	lastSnapshot   map[string]any
	lastMemoryScan []analyze.MemoryObject
	artifacts      []analyze.ArtifactEntry
	funcs          []js.Func
}

func newEmulator() *Emulator {
	e := &Emulator{entry: fwBase, nextAddr: payloadAddr, bootArgs: "console=ttyS0 earlycon=sbi root=/dev/vda rw", hartCount: 1, hsmState: map[uint64]string{0: "started"}, breakpoints: map[uint64]Breakpoint{}}
	e.initMachine(1)
	return e
}

func (e *Emulator) currentHart() *cpu.Hart {
	if len(e.harts) == 0 {
		return e.hart
	}
	if e.activeHart < 0 || e.activeHart >= len(e.harts) {
		e.activeHart = 0
	}
	e.hart = e.harts[e.activeHart]
	return e.hart
}

func (e *Emulator) setHartPending(hart int, mask uint64, set bool) {
	if hart < 0 || hart >= len(e.harts) {
		return
	}
	e.harts[hart].SetPending(mask, set)
}

func (e *Emulator) initMachine(hartCount int) {
	if hartCount <= 0 {
		hartCount = 1
	}
	if hartCount > 8 {
		hartCount = 8
	}
	e.hartCount = hartCount
	e.activeHart = 0
	e.bus = mem.NewBus(dramBase, dramSize)
	e.console = nil
	e.terminalLog = nil
	e.stopReason = ""
	e.firmwareLoaded = false
	e.harts = make([]*cpu.Hart, hartCount)
	for i := range e.harts {
		h := cpu.NewHart(e.bus)
		h.HartID = uint64(i)
		h.SBIHandler = e.handleSBI
		h.SBIShim = e.sbiShim
		e.harts[i] = h
	}
	e.hart = e.harts[0]
	e.plic = dev.NewPLICWithContexts(hartCount*2, func(context int, set bool) {
		hart := context / 2
		if context%2 == 0 {
			e.setHartPending(hart, cpu.MIP_MEIP, set)
		} else {
			e.setHartPending(hart, cpu.MIP_SEIP, set)
		}
	})
	e.uart = dev.NewUART(func(b byte) {
		e.appendConsole(b)
	}, func(set bool) { e.plic.SetIRQ(10, set) })
	e.blk = dev.NewVirtioBlock(e.bus, nil, func(set bool) { e.plic.SetIRQ(1, set) })
	e.vcon = dev.NewVirtioConsole(e.bus, func(b byte) {
		e.appendConsole(b)
	}, func(set bool) { e.plic.SetIRQ(2, set) })
	e.net = dev.NewVirtioNet(e.bus, [6]byte{0x02, 0x72, 0x76, 0x77, 0x00, 0x01}, func(set bool) { e.plic.SetIRQ(3, set) })
	e.rng = dev.NewVirtioRNG(e.bus, func(set bool) { e.plic.SetIRQ(4, set) })
	e.inputDev = dev.NewVirtioInput(e.bus, func(set bool) { e.plic.SetIRQ(5, set) })
	e.gpu = dev.NewVirtioGPU(e.bus, func(set bool) { e.plic.SetIRQ(6, set) })
	e.gpu.SetFramebuffer(fbAddr, fbWidth, fbHeight, fbStride)
	e.syscon = dev.NewSyscon(
		func() { e.requestSystemPoweroff() },
		func() { e.requestSystemReboot() },
	)
	e.bus.AddNamedDevice(dev.SysconBase, dev.SysconSize, "syscon", e.syscon)
	e.bus.AddNamedDevice(dev.UARTBase, 0x100, "uart16550", e.uart)
	e.bus.AddNamedDevice(dev.VirtioBlockBase, 0x200, "virtio-blk", e.blk)
	e.bus.AddNamedDevice(dev.VirtioConsoleBase, 0x200, "virtio-console", e.vcon)
	e.bus.AddNamedDevice(dev.VirtioNetBase, 0x200, "virtio-net", e.net)
	e.bus.AddNamedDevice(dev.VirtioRNGBase, 0x200, "virtio-rng", e.rng)
	e.bus.AddNamedDevice(dev.VirtioInputBase, 0x200, "virtio-input", e.inputDev)
	e.bus.AddNamedDevice(dev.VirtioGPUBase, 0x200, "virtio-gpu", e.gpu)
	e.clint = dev.NewCLINTMulti(hartCount, func(hart int, mask uint64, set bool) { e.setHartPending(hart, mask, set) })
	e.bus.AddNamedDevice(dev.CLINTBase, 0x10000, "clint", e.clint)
	e.bus.AddNamedDevice(dev.PLICBase, 0x4000000, "plic", e.plic)
	if e.breakpoints == nil {
		e.breakpoints = map[uint64]Breakpoint{}
	}
	e.hsmState = map[uint64]string{}
	for i := 0; i < hartCount; i++ {
		e.hsmState[uint64(i)] = "started"
	}
	e.installDTB()
	e.resetHarts()
}

func (e *Emulator) haltAllHarts() {
	for _, h := range e.harts {
		h.Halted = true
		h.Waiting = false
	}
}

func (e *Emulator) requestSystemPoweroff() {
	e.stopReason = "syscon-poweroff"
	e.running = false
	e.runToken++
	e.haltAllHarts()
}

func (e *Emulator) requestSystemReboot() {
	e.stopReason = "syscon-reboot"
	e.running = false
	e.runToken++
	e.resetHarts()
}

func (e *Emulator) resetHarts() {
	for i, h := range e.harts {
		h.Reset(e.entry, dtbAddr)
		h.HartID = uint64(i)
		h.X[10] = uint64(i)
		h.X[11] = dtbAddr
		h.X[12] = dynamicAddr
		h.SBIShim = e.sbiShim
		h.SBIHandler = e.handleSBI
	}
	e.currentHart()
}

func (e *Emulator) runSlice(n int) int {
	if len(e.harts) == 0 {
		return e.hart.Run(n)
	}
	ran := 0
	e.stopReason = ""
	for i := 0; i < n; i++ {
		progress := false
		for _, h := range e.harts {
			if h.Halted {
				continue
			}
			if n != 1 && e.checkBreakpoint(h) {
				e.running = false
				return ran
			}
			if h.Step() {
				ran++
				progress = true
			}
			if msg := e.bus.LastWatchpoint(); msg != "" {
				e.stopReason = fmt.Sprintf("%s hart=%d pc=%#x", msg, h.HartID, h.InstPC)
				e.bus.ClearLastWatchpoint()
				e.running = false
				return ran
			}
		}
		if !progress {
			break
		}
	}
	return ran
}

func (e *Emulator) runSliceTimeboxed(maxSteps int, maxWall time.Duration) int {
	if maxSteps <= 0 {
		maxSteps = 1
	}
	if maxWall <= 0 {
		maxWall = 6 * time.Millisecond
	}
	const microChunk = 1024
	start := time.Now()
	ran := 0
	for e.running && e.anyHartAlive() && ran < maxSteps {
		chunk := microChunk
		if rem := maxSteps - ran; rem < chunk {
			chunk = rem
		}
		n := e.runSlice(chunk)
		ran += n
		if n == 0 || !e.running || e.stopReason != "" || e.firstHartError() != nil {
			break
		}
		if time.Since(start) >= maxWall {
			break
		}
	}
	return ran
}

func (e *Emulator) checkBreakpoint(h *cpu.Hart) bool {
	bp, ok := e.breakpoints[h.PC]
	if !ok {
		return false
	}
	if bp.Hart >= 0 && bp.Hart != int(h.HartID) {
		return false
	}
	if bp.Mode != "" {
		m := strings.ToLower(bp.Mode)
		want := map[string]cpu.PrivMode{"u": cpu.PrivU, "s": cpu.PrivS, "m": cpu.PrivM}[m]
		if h.Mode != want {
			return false
		}
	}
	bp.Hits++
	e.breakpoints[h.PC] = bp
	if bp.AfterHits != 0 && bp.Hits < bp.AfterHits {
		return false
	}
	e.stopReason = fmt.Sprintf("breakpoint hit hart=%d pc=%#x hits=%d", h.HartID, h.PC, bp.Hits)
	return true
}

func (e *Emulator) anyHartAlive() bool {
	for _, h := range e.harts {
		if !h.Halted {
			return true
		}
	}
	return false
}

func (e *Emulator) refreshBootRegs() {
	for i, h := range e.harts {
		h.X[10] = uint64(i)
		h.X[11] = dtbAddr
		h.X[12] = dynamicAddr
	}
}

func (e *Emulator) firstHartError() error {
	for _, h := range e.harts {
		if h.LastError != nil {
			return h.LastError
		}
	}
	return nil
}

func (e *Emulator) hartsSummary() string {
	var b strings.Builder
	for i, h := range e.harts {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "h%d:pc=%#x mode=%d halted=%v wait=%v", i, h.PC, h.Mode, h.Halted, h.Waiting)
		if h.LastError != nil {
			fmt.Fprintf(&b, " err=%v", h.LastError)
		}
	}
	return b.String()
}

func (e *Emulator) installDTB() {
	dtb := e.customDTB
	if len(dtb) == 0 {
		dtb = boot.BuildVirtDTBConfig(boot.VirtDTBConfig{
			MemBase:     dramBase,
			MemSize:     dramSize,
			TimebaseHz:  10000000,
			BootArgs:    e.bootArgs,
			InitrdStart: e.initrdBase,
			InitrdEnd:   e.initrdEnd,
			HartCount:   e.hartCount,
			Framebuffer: boot.FramebufferConfig{
				Enabled: true,
				Base:    fbAddr,
				Size:    fbSize,
				Width:   fbWidth,
				Height:  fbHeight,
				Stride:  fbStride,
				Format:  "a8r8g8b8",
			},
		})
	}
	_ = e.bus.Load(dtbAddr, dtb)
	e.installDynamicInfo()
}

func (e *Emulator) installDynamicInfo() {
	info := boot.BuildFWDynamicInfo(boot.FWDynamicInfo{
		NextAddr: e.nextAddr,
		NextMode: boot.FWDynamicNextModeS,
		Options:  0,
		BootHart: 0,
	})
	_ = e.bus.Load(dynamicAddr, info)
}

func (e *Emulator) appendConsole(b byte) {
	e.console = append(e.console, b)
	e.terminalLog = append(e.terminalLog, b)
	const maxLog = 2 * 1024 * 1024
	if len(e.terminalLog) > maxLog {
		copy(e.terminalLog, e.terminalLog[len(e.terminalLog)-maxLog:])
		e.terminalLog = e.terminalLog[:maxLog]
	}
	if len(e.console) >= 64 || b == '\n' || b == '\r' {
		e.flushConsole()
	}
}

func (e *Emulator) consoleLogString() string {
	if len(e.console) == 0 {
		return string(e.terminalLog)
	}
	buf := make([]byte, 0, len(e.terminalLog)+len(e.console))
	buf = append(buf, e.terminalLog...)
	// e.console bytes are already in terminalLog; avoid duplicating the suffix
	// when flush has not happened yet.
	return string(buf)
}

func (e *Emulator) flushConsole() {
	if len(e.console) == 0 {
		return
	}
	s := string(e.console)
	e.console = e.console[:0]
	cb := js.Global().Get("appendTerminal")
	if cb.Type() == js.TypeFunction {
		cb.Invoke(s)
	}
}

func (e *Emulator) logf(format string, args ...any) {
	cb := js.Global().Get("appendTerminal")
	if cb.Type() == js.TypeFunction {
		cb.Invoke(fmt.Sprintf(format, args...))
	}
}

func (e *Emulator) recordArtifact(role string, data []byte, loadAddr, entry uint64, elf bool, note string) {
	entryRec := analyze.NewArtifactEntry(role, data, loadAddr, entry, elf, note)
	// Replace prior record of the same role so the manifest reflects the current boot set.
	out := e.artifacts[:0]
	for _, a := range e.artifacts {
		if a.Role != role {
			out = append(out, a)
		}
	}
	e.artifacts = append(out, entryRec)
}

func (e *Emulator) currentDTBBytes() []byte {
	if len(e.customDTB) != 0 {
		out := make([]byte, len(e.customDTB))
		copy(out, e.customDTB)
		return out
	}
	return boot.BuildVirtDTBConfig(boot.VirtDTBConfig{
		MemBase:     dramBase,
		MemSize:     dramSize,
		TimebaseHz:  10000000,
		BootArgs:    e.bootArgs,
		InitrdStart: e.initrdBase,
		InitrdEnd:   e.initrdEnd,
		HartCount:   e.hartCount,
		Framebuffer: boot.FramebufferConfig{Enabled: true, Base: fbAddr, Size: fbSize, Width: fbWidth, Height: fbHeight, Stride: fbStride, Format: "a8r8g8b8"},
	})
}

func (e *Emulator) currentArtifactManifest() analyze.ArtifactManifest {
	arts := append([]analyze.ArtifactEntry{}, e.artifacts...)
	dtb := e.currentDTBBytes()
	arts = append(arts, analyze.NewArtifactEntry("dtb", dtb, dtbAddr, 0, false, func() string {
		if len(e.customDTB) != 0 {
			return "custom"
		}
		return "generated"
	}()))
	dyn := boot.BuildFWDynamicInfo(boot.FWDynamicInfo{NextAddr: e.nextAddr, NextMode: boot.FWDynamicNextModeS, Options: 0, BootHart: 0})
	arts = append(arts, analyze.NewArtifactEntry("fw_dynamic_info", dyn, dynamicAddr, 0, false, "generated"))
	return analyze.ArtifactManifest{BootArgs: e.bootArgs, HartCount: e.hartCount, NextAddr: e.nextAddr, DTBAddr: dtbAddr, DynamicInfoAddr: dynamicAddr, Artifacts: arts}
}

func (e *Emulator) loadFirmware(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "missing Uint8Array"
	}
	data := make([]byte, args[0].Get("byteLength").Int())
	js.CopyBytesToGo(data, args[0])
	loadBase := fwBase
	if len(args) >= 2 && args[1].Type() == js.TypeString {
		if v, err := strconv.ParseUint(strings.TrimPrefix(args[1].String(), "0x"), 16, 64); err == nil {
			loadBase = v
		}
	}
	img, err := mem.LoadELFOrRaw(e.bus, loadBase, data)
	if err != nil {
		return err.Error()
	}
	if tab, serr := debugmap.ParseELF(data, "firmware ELF"); serr == nil {
		e.symbols = tab
	}
	e.entry = img.Entry
	e.firmwareLoaded = true
	e.recordArtifact("firmware", data, loadBase, img.Entry, img.IsELF, "")
	e.installDTB()
	e.resetHarts()
	return fmt.Sprintf("loaded %d bytes, entry=%#x, elf=%v", len(data), img.Entry, img.IsELF)
}

func (e *Emulator) loadDTB(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "missing Uint8Array"
	}
	e.customDTB = make([]byte, args[0].Get("byteLength").Int())
	js.CopyBytesToGo(e.customDTB, args[0])
	e.recordArtifact("dtb-custom", e.customDTB, dtbAddr, 0, false, "custom dtb upload")
	e.installDTB()
	e.refreshBootRegs()
	return fmt.Sprintf("loaded DTB %d bytes at %#x", len(e.customDTB), dtbAddr)
}

func (e *Emulator) reset(this js.Value, args []js.Value) any {
	e.running = false
	e.runToken++
	e.console = nil
	e.terminalLog = nil
	e.resetHarts()
	e.installDTB()
	e.flushConsole()
	return e.status(this, nil)
}

func (e *Emulator) step(this js.Value, args []js.Value) any {
	n := 1
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	ran := e.runSlice(n)
	e.flushConsole()
	if err := e.firstHartError(); err != nil {
		e.logf("\n[halt] %v\n", err)
	}
	return fmt.Sprintf("ran %d hart-steps", ran)
}

func (e *Emulator) run(this js.Value, args []js.Value) any {
	if e.running {
		return "already running"
	}
	if !e.firmwareLoaded {
		return "firmware is not loaded; load OpenSBI first"
	}
	e.running = true
	e.runToken++
	token := e.runToken
	e.scheduleRunTick(token, 0)
	return "running (time-sliced)"
}

func (e *Emulator) scheduleRunTick(token uint64, delayMS int) {
	if !e.running {
		return
	}
	var cb js.Func
	e.runPending++
	cb = js.FuncOf(func(this js.Value, args []js.Value) any {
		if e.runPending > 0 {
			e.runPending--
		}
		cb.Release()
		if !e.running || token != e.runToken {
			return nil
		}
		e.runTick(token)
		return nil
	})
	js.Global().Call("setTimeout", cb, delayMS)
}

func (e *Emulator) runTick(token uint64) {
	if !e.running || token != e.runToken {
		return
	}
	// Go/Wasm runs on the browser main thread. Keep each callback short so
	// Pause, Diagnostics, console flush, and repaint can always run.
	const maxStepsPerTick = 50000
	ran := e.runSliceTimeboxed(maxStepsPerTick, 8*time.Millisecond)
	e.runLastSteps = ran
	e.runLastYield = time.Now()
	e.flushConsole()
	if err := e.firstHartError(); err != nil {
		e.logf("\n[halt] %v\n", err)
		e.running = false
		e.runToken++
		return
	}
	if !e.anyHartAlive() {
		e.running = false
		e.runToken++
		return
	}
	if !e.running || token != e.runToken {
		return
	}
	delay := 1
	if ran == 0 {
		// WFI or a stalled guest should not spin the browser event loop.
		delay = 25
	}
	e.scheduleRunTick(token, delay)
}

func (e *Emulator) pause(this js.Value, args []js.Value) any {
	e.running = false
	e.runToken++
	e.flushConsole()
	return "paused"
}

func consoleTail(s string, max int) string {
	if max <= 0 || len(s) == 0 {
		return ""
	}
	if len(s) > max {
		s = s[len(s)-max:]
	}
	return strconv.QuoteToASCII(s)
}

func (e *Emulator) instAtPC(h *cpu.Hart) string {
	if e == nil || e.bus == nil || h == nil {
		return ""
	}
	raw16, err := e.bus.ReadNoTrace(h.PC, 2)
	if err != nil {
		return "inst=<unreadable>"
	}
	if raw16&3 != 3 {
		return fmt.Sprintf("inst16=%#04x", raw16)
	}
	raw32, err := e.bus.ReadNoTrace(h.PC, 4)
	if err != nil {
		return "inst32=<unreadable>"
	}
	return fmt.Sprintf("inst32=%#08x", raw32)
}

func (e *Emulator) status(this js.Value, args []js.Value) any {
	h := e.currentHart()
	regs := h.Regs()
	parts := []string{
		fmt.Sprintf("hart=%d/%d", e.activeHart, e.hartCount),
		fmt.Sprintf("run=%v scheduled=%v pendingTicks=%d lastTickSteps=%d", e.running, e.runPending != 0, e.runPending, e.runLastSteps),
		fmt.Sprintf("pc=%#016x", h.PC),
		e.instAtPC(h),
		fmt.Sprintf("mode=%d", h.Mode),
		fmt.Sprintf("cycle=%d", h.Cycle),
		fmt.Sprintf("lastTrap=%d/%#x/int=%v", h.LastTrapCause, h.LastTrapTval, h.LastTrapInterrupt),
		fmt.Sprintf("a0=%#x", regs[10]),
		fmt.Sprintf("a1=%#x", regs[11]),
		fmt.Sprintf("a2=%#x", regs[12]),
		fmt.Sprintf("sp=%#x", regs[2]),
		fmt.Sprintf("ra=%#x", regs[1]),
	}
	if e.stopReason != "" {
		parts = append(parts, "stop="+e.stopReason)
	}
	if len(e.breakpoints) > 0 {
		parts = append(parts, fmt.Sprintf("breakpoints=%d", len(e.breakpoints)))
	}
	if wps := e.bus.WatchpointSummary(); len(wps) > 0 {
		parts = append(parts, fmt.Sprintf("writeWatchpoints=%d", len(wps)))
	}
	if rps := e.bus.ReadWatchpointSummary(); len(rps) > 0 {
		parts = append(parts, fmt.Sprintf("readWatchpoints=%d", len(rps)))
	}
	if e.blk != nil {
		parts = append(parts, fmt.Sprintf("disk=%d sectors", e.blk.CapacitySectors()))
		parts = append(parts, fmt.Sprintf("lastEcall(mode=%d ext=%#x func=%#x a0=%#x)", h.LastEcallMode, h.LastEcallExt, h.LastEcallFunc, h.LastEcallArgs[0]))
		if msg := e.blk.LastError(); msg != "" {
			parts = append(parts, "virtio-blk="+msg)
		}
	}
	if e.vcon != nil {
		if msg := e.vcon.LastError(); msg != "" {
			parts = append(parts, "virtio-console="+msg)
		}
	}
	if e.net != nil {
		if msg := e.net.LastError(); msg != "" {
			parts = append(parts, "virtio-net="+msg)
		}
	}
	if e.rng != nil {
		if msg := e.rng.LastError(); msg != "" {
			parts = append(parts, "virtio-rng="+msg)
		}
	}
	if e.inputDev != nil {
		if msg := e.inputDev.LastError(); msg != "" {
			parts = append(parts, "virtio-input="+msg)
		}
	}
	if e.gpu != nil {
		if msg := e.gpu.LastError(); msg != "" {
			parts = append(parts, "virtio-gpu="+msg)
		}
	}
	if e.initrdBase != 0 && e.initrdEnd > e.initrdBase {
		parts = append(parts, fmt.Sprintf("initrd=%#x-%#x", e.initrdBase, e.initrdEnd))
	}
	parts = append(parts, fmt.Sprintf("sbiShim=%v", e.sbiShim))
	parts = append(parts, fmt.Sprintf("fb=%#x %dx%d", fbAddr, fbWidth, fbHeight))
	parts = append(parts, fmt.Sprintf("mmioBuckets=%d", len(e.bus.AccessHistogram())))
	parts = append(parts, fmt.Sprintf("dramTrace=%v", e.bus.IsTraceDRAMAccessEnabled()))
	if tail := consoleTail(e.consoleLogString(), 80); tail != "" {
		parts = append(parts, "consoleTail="+tail)
	}
	if h.Profile {
		parts = append(parts, fmt.Sprintf("pcProfile=%d", len(h.PCProfileTop(0))))
	}
	if e.symbols != nil {
		parts = append(parts, fmt.Sprintf("pcsym=%s", e.symbols.FormatLookup(h.PC)))
	}
	if h.LastError != nil {
		parts = append(parts, "error="+h.LastError.Error())
	}
	return strings.Join(parts, " ")
}

func (e *Emulator) regs(this js.Value, args []js.Value) any {
	h := e.currentHart()
	regs := h.Regs()
	var b strings.Builder
	for i, r := range regs {
		fmt.Fprintf(&b, "x%02d=%016x", i, r)
		if i%4 == 3 {
			b.WriteByte('\n')
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func (e *Emulator) csrs(this js.Value, args []js.Value) any {
	type csrInfo struct {
		name string
		addr uint16
	}
	list := []csrInfo{
		{"mstatus", cpu.CSR_MSTATUS}, {"misa", cpu.CSR_MISA}, {"medeleg", cpu.CSR_MEDELEG}, {"mideleg", cpu.CSR_MIDELEG},
		{"mie", cpu.CSR_MIE}, {"mtvec", cpu.CSR_MTVEC}, {"mepc", cpu.CSR_MEPC}, {"mcause", cpu.CSR_MCAUSE}, {"mtval", cpu.CSR_MTVAL}, {"mip", cpu.CSR_MIP},
		{"menvcfg", cpu.CSR_MENVCFG}, {"mcounter", cpu.CSR_MCOUNTEREN},
		{"sstatus", cpu.CSR_SSTATUS}, {"sie", cpu.CSR_SIE}, {"stvec", cpu.CSR_STVEC}, {"sepc", cpu.CSR_SEPC}, {"scause", cpu.CSR_SCAUSE}, {"stval", cpu.CSR_STVAL}, {"sip", cpu.CSR_SIP}, {"satp", cpu.CSR_SATP},
		{"senvcfg", cpu.CSR_SENVCFG}, {"scounter", cpu.CSR_SCOUNTEREN},
		{"cycle", cpu.CSR_CYCLE}, {"time", cpu.CSR_TIME}, {"instret", cpu.CSR_INSTRET},
	}
	var b strings.Builder
	for _, x := range list {
		fmt.Fprintf(&b, "%-8s[%03x]=%016x\n", x.name, x.addr, e.currentHart().ReadCSR(x.addr))
	}
	return b.String()
}

func (e *Emulator) input(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "missing input string"
	}
	data := []byte(args[0].String())
	e.uart.Inject(data)
	if e.vcon != nil {
		e.vcon.Inject(data)
	}
	return fmt.Sprintf("queued %d input bytes to UART and virtio-console", len(data))
}

func (e *Emulator) loadPayload(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "missing Uint8Array"
	}
	addr := e.nextAddr
	if len(args) >= 2 && args[1].Type() == js.TypeString {
		if v, err := strconv.ParseUint(strings.TrimPrefix(args[1].String(), "0x"), 16, 64); err == nil {
			addr = v
		}
	}
	data := make([]byte, args[0].Get("byteLength").Int())
	js.CopyBytesToGo(data, args[0])
	img, err := mem.LoadELFOrRaw(e.bus, addr, data)
	if err != nil {
		return err.Error()
	}
	if tab, serr := debugmap.ParseELF(data, "payload ELF"); serr == nil {
		e.symbols = tab
	}
	e.nextAddr = img.Entry
	e.recordArtifact("payload", data, addr, img.Entry, img.IsELF, "")
	e.installDynamicInfo()
	e.refreshBootRegs()
	return fmt.Sprintf("loaded payload %d bytes at %#x, next=%#x, elf=%v", len(data), addr, e.nextAddr, img.IsELF)
}

func (e *Emulator) loadDisk(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "missing Uint8Array"
	}
	data := make([]byte, args[0].Get("byteLength").Int())
	js.CopyBytesToGo(data, args[0])
	e.blk.SetDisk(data)
	e.recordArtifact("disk", data, 0, 0, false, fmt.Sprintf("%d sectors", e.blk.CapacitySectors()))
	return fmt.Sprintf("loaded virtio-blk disk %d bytes, %d sectors", len(data), e.blk.CapacitySectors())
}

func (e *Emulator) exportDisk(this js.Value, args []js.Value) any {
	if e.blk == nil {
		return js.Null()
	}
	disk := e.blk.Disk()
	arr := js.Global().Get("Uint8Array").New(len(disk))
	js.CopyBytesToJS(arr, disk)
	return arr
}

func (e *Emulator) loadInitrd(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "missing Uint8Array"
	}
	addr := initrdAddr
	if len(args) >= 2 && args[1].Type() == js.TypeString {
		if v, err := strconv.ParseUint(strings.TrimPrefix(args[1].String(), "0x"), 16, 64); err == nil {
			addr = v
		}
	}
	data := make([]byte, args[0].Get("byteLength").Int())
	js.CopyBytesToGo(data, args[0])
	if addr < dramBase || addr+uint64(len(data)) > dtbAddr-0x100000 {
		return fmt.Sprintf("initrd range %#x-%#x would overlap reserved high-memory area", addr, addr+uint64(len(data)))
	}
	if err := e.bus.Load(addr, data); err != nil {
		return err.Error()
	}
	e.initrdBase = addr
	e.initrdEnd = addr + uint64(len(data))
	e.recordArtifact("initrd", data, addr, 0, false, "")
	e.installDTB()
	e.refreshBootRegs()
	return fmt.Sprintf("loaded initrd %d bytes at %#x-%#x", len(data), e.initrdBase, e.initrdEnd)
}

func (e *Emulator) setBootArgs(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return e.bootArgs
	}
	e.bootArgs = args[0].String()
	e.installDTB()
	e.refreshBootRegs()
	return "bootargs=" + e.bootArgs
}

func (e *Emulator) trace(this js.Value, args []js.Value) any {
	n := 64
	filter := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	if len(args) > 1 && args[1].Type() == js.TypeString {
		filter = args[1].String()
	}
	return e.currentHart().TraceStringFiltered(n, filter)
}

func (e *Emulator) setTrace(this js.Value, args []js.Value) any {
	enable := true
	if len(args) > 0 && args[0].Type() == js.TypeBoolean {
		enable = args[0].Bool()
	}
	for _, h := range e.harts {
		h.Trace = enable
	}
	return fmt.Sprintf("trace=%v", enable)
}

func (e *Emulator) setCSRTrace(this js.Value, args []js.Value) any {
	enable := true
	if len(args) > 0 && args[0].Type() == js.TypeBoolean {
		enable = args[0].Bool()
	}
	for _, h := range e.harts {
		h.SetCSRTrace(enable)
	}
	return fmt.Sprintf("csrTrace=%v", enable)
}

func (e *Emulator) csrTrace(this js.Value, args []js.Value) any {
	n := 128
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	return e.currentHart().CSRTraceString(n)
}

func (e *Emulator) csrSummary(this js.Value, args []js.Value) any {
	n := 32
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	return e.currentHart().CSRAccessSummaryString(n)
}

func (e *Emulator) clearCSRTrace(this js.Value, args []js.Value) any {
	for _, h := range e.harts {
		h.ClearCSRTrace()
	}
	return "CSR trace and summary cleared"
}

func (e *Emulator) setProfile(this js.Value, args []js.Value) any {
	enable := true
	if len(args) > 0 && args[0].Type() == js.TypeBoolean {
		enable = args[0].Bool()
	}
	for _, h := range e.harts {
		h.SetProfile(enable)
	}
	return fmt.Sprintf("pcProfile=%v", enable)
}

func (e *Emulator) clearProfile(this js.Value, args []js.Value) any {
	for _, h := range e.harts {
		h.ClearProfile()
	}
	return "PC profile cleared"
}

func (e *Emulator) pcProfile(this js.Value, args []js.Value) any {
	n := 32
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	lookup := func(pc uint64) string {
		if e.symbols != nil {
			return e.symbols.FormatLookup(pc)
		}
		return ""
	}
	return e.currentHart().PCProfileString(n, lookup)
}

func (e *Emulator) traceCompact(this js.Value, args []js.Value) any {
	n := 256
	filter := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	if len(args) > 1 && args[1].Type() == js.TypeString {
		filter = args[1].String()
	}
	return e.currentHart().TraceCompactString(n, filter)
}

func (e *Emulator) traceAnnotated(this js.Value, args []js.Value) any {
	if e.symbols == nil {
		return "<no symbols loaded>\n"
	}
	n := 256
	filter := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	if len(args) > 1 && args[1].Type() == js.TypeString {
		filter = args[1].String()
	}
	return e.symbols.AnnotateTraceText(e.currentHart().TraceStringFiltered(n, filter), n)
}

func (e *Emulator) poke(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return "usage: poke(hexAddr, hexBytes)"
	}
	addr, err := strconv.ParseUint(strings.TrimPrefix(args[0].String(), "0x"), 16, 64)
	if err != nil {
		return err.Error()
	}
	s := strings.ReplaceAll(args[1].String(), " ", "")
	data, err := hex.DecodeString(s)
	if err != nil {
		return err.Error()
	}
	if err := e.bus.Load(addr, data); err != nil {
		return err.Error()
	}
	return fmt.Sprintf("wrote %d bytes at %#x", len(data), addr)
}

func (e *Emulator) dumpMemory(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return js.Null()
	}
	addr, err := strconv.ParseUint(strings.TrimPrefix(args[0].String(), "0x"), 16, 64)
	if err != nil {
		return js.Null()
	}
	n := args[1].Int()
	if n < 0 {
		n = 0
	}
	if n > 16*1024*1024 {
		n = 16 * 1024 * 1024
	}
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		v, err := e.bus.Read(addr+uint64(i), 1)
		if err != nil {
			return js.Null()
		}
		buf[i] = byte(v)
	}
	arr := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(arr, buf)
	return arr
}

func parseHexOrDec(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	base := 16
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 0
	}
	return strconv.ParseUint(s, base, 64)
}

func (e *Emulator) addBreakpoint(this js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return "usage: breakpoint(hexPC [, afterHits] [, mode=u|s|m] [, hart])"
	}
	pc, err := parseHexOrDec(args[0].String())
	if err != nil {
		return err.Error()
	}
	bp := Breakpoint{PC: pc, Hart: -1}
	if len(args) > 1 && args[1].Type() == js.TypeString && args[1].String() != "" {
		if v, err := parseHexOrDec(args[1].String()); err == nil {
			bp.AfterHits = v
		} else {
			return err.Error()
		}
	}
	if len(args) > 2 && args[2].Type() == js.TypeString {
		m := strings.ToLower(strings.TrimSpace(args[2].String()))
		if m != "" && m != "u" && m != "s" && m != "m" {
			return "mode must be u, s, m or empty"
		}
		bp.Mode = m
	}
	if len(args) > 3 && args[3].Type() == js.TypeNumber {
		bp.Hart = args[3].Int()
	}
	if e.breakpoints == nil {
		e.breakpoints = map[uint64]Breakpoint{}
	}
	if old, ok := e.breakpoints[pc]; ok {
		bp.Hits = old.Hits
	}
	e.breakpoints[pc] = bp
	return fmt.Sprintf("breakpoint added at %#x afterHits=%d mode=%q hart=%d", pc, bp.AfterHits, bp.Mode, bp.Hart)
}

func (e *Emulator) clearBreakpoints(this js.Value, args []js.Value) any {
	e.breakpoints = map[uint64]Breakpoint{}
	e.stopReason = ""
	return "breakpoints cleared"
}

func (e *Emulator) breakpointsString(this js.Value, args []js.Value) any {
	if len(e.breakpoints) == 0 {
		return "<no breakpoints>\n"
	}
	var b strings.Builder
	for pc, bp := range e.breakpoints {
		fmt.Fprintf(&b, "%#016x hits=%d after=%d", pc, bp.Hits, bp.AfterHits)
		if bp.Mode != "" {
			fmt.Fprintf(&b, " mode=%s", bp.Mode)
		}
		if bp.Hart >= 0 {
			fmt.Fprintf(&b, " hart=%d", bp.Hart)
		}
		if e.symbols != nil {
			fmt.Fprintf(&b, " %s", e.symbols.FormatLookup(pc))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (e *Emulator) addWatchpoint(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return "usage: watchpoint(hexAddr, hexLen)"
	}
	addr, err := parseHexOrDec(args[0].String())
	if err != nil {
		return err.Error()
	}
	length, err := parseHexOrDec(args[1].String())
	if err != nil {
		return err.Error()
	}
	if length == 0 {
		return "watchpoint length must be non-zero"
	}
	e.bus.AddWriteWatchpoint(addr, length, fmt.Sprintf("%#x+%#x", addr, length))
	return fmt.Sprintf("write watchpoint added at %#x+%#x", addr, length)
}

func (e *Emulator) clearWatchpoints(this js.Value, args []js.Value) any {
	e.bus.ClearWriteWatchpoints()
	e.stopReason = ""
	return "write watchpoints cleared"
}

func (e *Emulator) watchpointsString(this js.Value, args []js.Value) any {
	wps := e.bus.WatchpointSummary()
	if len(wps) == 0 {
		return "<no write watchpoints>\n"
	}
	var b strings.Builder
	for _, w := range wps {
		fmt.Fprintf(&b, "%#016x..%#016x %s\n", w.Base, w.Base+w.Size-1, w.Name)
	}
	return b.String()
}

func (e *Emulator) addReadWatchpoint(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return "usage: readWatchpoint(hexAddr, hexLen)"
	}
	addr, err := parseHexOrDec(args[0].String())
	if err != nil {
		return err.Error()
	}
	length, err := parseHexOrDec(args[1].String())
	if err != nil {
		return err.Error()
	}
	if length == 0 {
		return "read watchpoint length must be non-zero"
	}
	e.bus.AddReadWatchpoint(addr, length, fmt.Sprintf("%#x+%#x", addr, length))
	return fmt.Sprintf("read watchpoint added at %#x+%#x", addr, length)
}

func (e *Emulator) clearReadWatchpoints(this js.Value, args []js.Value) any {
	e.bus.ClearReadWatchpoints()
	e.stopReason = ""
	return "read watchpoints cleared"
}

func (e *Emulator) readWatchpointsString(this js.Value, args []js.Value) any {
	wps := e.bus.ReadWatchpointSummary()
	if len(wps) == 0 {
		return "<no read watchpoints>\n"
	}
	var b strings.Builder
	for _, w := range wps {
		fmt.Fprintf(&b, "%#016x..%#016x %s\n", w.Base, w.Base+w.Size-1, w.Name)
	}
	return b.String()
}

func (e *Emulator) setTraceFilter(this js.Value, args []js.Value) any {
	if len(args) > 0 && args[0].Type() == js.TypeString {
		e.traceFilter = args[0].String()
	}
	return "traceFilter=" + e.traceFilter
}

func (e *Emulator) setTraceDRAMAccess(this js.Value, args []js.Value) any {
	enable := true
	if len(args) > 0 && args[0].Type() == js.TypeBoolean {
		enable = args[0].Bool()
	}
	e.bus.SetTraceDRAMAccess(enable)
	return fmt.Sprintf("traceDRAMAccess=%v", enable)
}

func (e *Emulator) framebufferInfo(this js.Value, args []js.Value) any {
	return fmt.Sprintf("{\"base\":\"%#x\",\"width\":%d,\"height\":%d,\"stride\":%d,\"format\":\"a8r8g8b8\"}", fbAddr, fbWidth, fbHeight, fbStride)
}

func (e *Emulator) framebufferRGBA(this js.Value, args []js.Value) any {
	buf := make([]byte, fbWidth*fbHeight*4)
	for y := 0; y < fbHeight; y++ {
		row := fbAddr + uint64(y*fbStride)
		for x := 0; x < fbWidth; x++ {
			v, err := e.bus.Read(row+uint64(x*4), 4)
			if err != nil {
				return js.Null()
			}
			// simple-framebuffer format a8r8g8b8 is stored little-endian as B,G,R,A.
			dst := (y*fbWidth + x) * 4
			buf[dst+0] = byte(v >> 16)
			buf[dst+1] = byte(v >> 8)
			buf[dst+2] = byte(v)
			buf[dst+3] = byte(v >> 24)
			if buf[dst+3] == 0 {
				buf[dst+3] = 0xff
			}
		}
	}
	arr := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(arr, buf)
	return arr
}

func (e *Emulator) framebufferPNG(this js.Value, args []js.Value) any {
	img := image.NewRGBA(image.Rect(0, 0, fbWidth, fbHeight))
	for y := 0; y < fbHeight; y++ {
		row := fbAddr + uint64(y*fbStride)
		for x := 0; x < fbWidth; x++ {
			v, err := e.bus.Read(row+uint64(x*4), 4)
			if err != nil {
				return js.Null()
			}
			a := byte(v >> 24)
			if a == 0 {
				a = 0xff
			}
			img.SetRGBA(x, y, color.RGBA{R: byte(v >> 16), G: byte(v >> 8), B: byte(v), A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return js.Null()
	}
	arr := js.Global().Get("Uint8Array").New(buf.Len())
	js.CopyBytesToJS(arr, buf.Bytes())
	return arr
}

func (e *Emulator) injectInputEvent(this js.Value, args []js.Value) any {
	if e.inputDev == nil {
		return "virtio-input not present"
	}
	if len(args) < 3 {
		return "usage: injectInputEvent(type, code, value)"
	}
	typ, err := parseHexOrDec(args[0].String())
	if err != nil {
		return err.Error()
	}
	code, err := parseHexOrDec(args[1].String())
	if err != nil {
		return err.Error()
	}
	val, err := strconv.ParseInt(strings.TrimSpace(args[2].String()), 0, 32)
	if err != nil {
		return err.Error()
	}
	e.inputDev.InjectEvent(uint16(typ), uint16(code), int32(val))
	return fmt.Sprintf("queued virtio-input event type=%#x code=%#x value=%d", typ, code, val)
}

func (e *Emulator) injectInputKey(this js.Value, args []js.Value) any {
	if e.inputDev == nil {
		return "virtio-input not present"
	}
	if len(args) < 1 {
		return "usage: injectInputKey(code [, down])"
	}
	code, err := parseHexOrDec(args[0].String())
	if err != nil {
		return err.Error()
	}
	down := true
	if len(args) > 1 && args[1].Type() == js.TypeBoolean {
		down = args[1].Bool()
	}
	e.inputDev.InjectKey(uint16(code), down)
	return fmt.Sprintf("queued virtio-input key code=%#x down=%v", code, down)
}

func (e *Emulator) clearFramebuffer(this js.Value, args []js.Value) any {
	zero := make([]byte, fbSize)
	for i := 3; i < len(zero); i += 4 {
		zero[i] = 0xff
	}
	if err := e.bus.Load(fbAddr, zero); err != nil {
		return err.Error()
	}
	return fmt.Sprintf("framebuffer cleared at %#x (%dx%d)", fbAddr, fbWidth, fbHeight)
}

func (e *Emulator) diagnostics(this js.Value, args []js.Value) any {
	h := e.currentHart()
	var b strings.Builder
	fmt.Fprintf(&b, "[status] %s\n", e.status(this, nil))
	fmt.Fprintf(&b, "[harts] %s\n", e.hartsSummary())
	if e.stopReason != "" {
		fmt.Fprintf(&b, "[stop] %s\n", e.stopReason)
	}
	fmt.Fprintf(&b, "[breakpoints] %s", e.breakpointsString(js.Value{}, nil))
	fmt.Fprintf(&b, "[write-watchpoints] %s", e.watchpointsString(js.Value{}, nil))
	fmt.Fprintf(&b, "[read-watchpoints] %s", e.readWatchpointsString(js.Value{}, nil))
	fmt.Fprintf(&b, "[framebuffer] base=%#x width=%d height=%d stride=%d format=a8r8g8b8\n", fbAddr, fbWidth, fbHeight, fbStride)
	fmt.Fprintf(&b, "[access-histogram]\n%s", e.bus.AccessHistogramString())
	fmt.Fprintf(&b, "[boot-phase]\n%s", e.bootPhaseText())
	fmt.Fprintf(&b, "[boot-timeline]\n%s", e.bootTimeline(js.Value{}, []js.Value{js.ValueOf(48)}))
	fmt.Fprintf(&b, "[device-probe]\n%s", e.deviceProbe(js.Value{}, nil))
	fmt.Fprintf(&b, "[virtqueue-inspect]\n%s", e.virtqueueInspect(js.Value{}, nil))
	fmt.Fprintf(&b, "[virtqueue-chains]\n%s", e.virtqueueChains(js.Value{}, []js.Value{js.ValueOf(8)}))
	fmt.Fprintf(&b, "[virtqueue-anomalies]\n%s", e.virtqueueAnomalies(js.Value{}, []js.Value{js.ValueOf(8)}))
	fmt.Fprintf(&b, "[virtqueue-dot]\n%s", e.virtqueueGraphDOT(js.Value{}, []js.Value{js.ValueOf(8)}))
	fmt.Fprintf(&b, "[guest-memory-scan]\n%s", e.guestMemoryScan(js.Value{}, []js.Value{js.ValueOf(16)}))
	fmt.Fprintf(&b, "[guest-memory-index]\n%s", e.guestMemoryIndex(js.Value{}, []js.Value{js.ValueOf(16)}))
	fmt.Fprintf(&b, "[initcall-classifier]\n%s", e.initcallClassifier(js.Value{}, []js.Value{js.ValueOf(120)}))
	fmt.Fprintf(&b, "[initcall-timeline]\n%s", e.initcallTimeline(js.Value{}, []js.Value{js.ValueOf(120)}))
	fmt.Fprintf(&b, "[panic-summary]\n%s", e.panicSummary(js.Value{}, []js.Value{js.ValueOf(80)}))
	fmt.Fprintf(&b, "[dmesg-extract]\n%s", e.dmesg(js.Value{}, []js.Value{js.ValueOf(80)}))
	if tl := e.bus.AccessTimelineCompactString(32, e.traceFilter); tl != "" {
		fmt.Fprintf(&b, "[access-timeline-compact filter=%q]\n%s", e.traceFilter, tl)
	}
	fmt.Fprintf(&b, "[boot-phase]\n%s", e.bootPhaseText())
	if ps := h.PCProfileString(16, func(pc uint64) string {
		if e.symbols != nil {
			return e.symbols.FormatLookup(pc)
		}
		return ""
	}); !strings.Contains(ps, "<empty") {
		fmt.Fprintf(&b, "[pc-profile]\n%s", ps)
	}
	if cs := h.CSRAccessSummaryString(16); !strings.Contains(cs, "<empty") {
		fmt.Fprintf(&b, "[csr-summary]\n%s", cs)
	}
	if e.clint != nil {
		fmt.Fprintf(&b, "[clint] %s\n", e.clint.DebugString())
	}
	fmt.Fprintf(&b, "[sbi/ecall] hart=%d cycle=%d mode=%d class=%s ext=%#x func=%#x args=%#x,%#x,%#x,%#x,%#x,%#x\n",
		h.HartID, h.LastEcallCycle, h.LastEcallMode, h.LastSBIClass, h.LastEcallExt, h.LastEcallFunc,
		h.LastEcallArgs[0], h.LastEcallArgs[1], h.LastEcallArgs[2], h.LastEcallArgs[3], h.LastEcallArgs[4], h.LastEcallArgs[5])
	fmt.Fprintf(&b, "[sbi/counts] %s shim=%v hsm=%v\n", h.SBIObservationString(), e.sbiShim, e.hsmState)
	fmt.Fprintf(&b, "[trap] cause=%d tval=%#x interrupt=%v mepc=%#x sepc=%#x\n", h.LastTrapCause, h.LastTrapTval, h.LastTrapInterrupt, h.ReadCSR(cpu.CSR_MEPC), h.ReadCSR(cpu.CSR_SEPC))
	if e.blk != nil {
		fmt.Fprintf(&b, "[virtio-blk] %s\n", e.blk.DebugString())
	}
	if e.vcon != nil {
		fmt.Fprintf(&b, "[virtio-console] %s\n", e.vcon.DebugString())
	}
	if e.net != nil {
		fmt.Fprintf(&b, "[virtio-net] %s\n", e.net.DebugString())
	}
	if e.rng != nil {
		fmt.Fprintf(&b, "[virtio-rng] %s\n", e.rng.DebugString())
	}
	if e.inputDev != nil {
		fmt.Fprintf(&b, "[virtio-input] %s\n", e.inputDev.DebugString())
	}
	if e.gpu != nil {
		fmt.Fprintf(&b, "[virtio-gpu] %s\n", e.gpu.DebugString())
	}
	if e.symbols != nil {
		fmt.Fprintf(&b, "[symbols] %s", e.symbols.Around(h.PC, 4))
		fmt.Fprintf(&b, "[dwarf-line-summary]\n%s", e.symbols.LineSummary(16))
	}
	if tr := h.TraceStringFiltered(32, e.traceFilter); tr != "" {
		fmt.Fprintf(&b, "[trace-tail filter=%q]\n%s", e.traceFilter, tr)
	}
	if ct := h.CSRTraceString(32); ct != "" {
		fmt.Fprintf(&b, "[csr-trace-tail]\n%s", ct)
	}
	return b.String()
}

const (
	sbiSuccess         int64  = 0
	sbiErrFailed       int64  = -1
	sbiErrNotSupported int64  = -2
	sbiExtBase         uint64 = 0x10
	sbiExtTime         uint64 = 0x54494d45
	sbiExtIPI          uint64 = 0x735049
	sbiExtRFence       uint64 = 0x52464e43
	sbiExtHSM          uint64 = 0x48534d
	sbiExtSRST         uint64 = 0x53525354
)

func (e *Emulator) handleSBI(h *cpu.Hart, ext, fn uint64, args [6]uint64) (bool, int64, uint64) {
	switch ext {
	case sbiExtBase:
		switch fn {
		case 0: // get spec version, report SBI v2.0 style value
			return true, sbiSuccess, 0x02000000
		case 1: // get impl id
			return true, sbiSuccess, 0x525657 // "RVW"
		case 2: // get impl version
			return true, sbiSuccess, 1
		case 3: // probe extension
			switch args[0] {
			case sbiExtBase, sbiExtTime, sbiExtIPI, sbiExtRFence, sbiExtHSM, sbiExtSRST:
				return true, sbiSuccess, 1
			default:
				return true, sbiSuccess, 0
			}
		case 4: // get machine vendor id
			return true, sbiSuccess, h.ReadCSR(cpu.CSR_MVENDORID)
		case 5: // get machine arch id
			return true, sbiSuccess, h.ReadCSR(cpu.CSR_MARCHID)
		case 6: // get machine impl id
			return true, sbiSuccess, h.ReadCSR(cpu.CSR_MIMPID)
		default:
			return true, sbiErrNotSupported, 0
		}
	case sbiExtTime:
		if fn == 0 {
			if e.clint != nil {
				e.clint.SetTimerCompareHart(int(h.HartID), args[0])
			}
			return true, sbiSuccess, 0
		}
		return true, sbiErrNotSupported, 0
	case sbiExtIPI:
		if fn == 0 {
			if e.clint != nil {
				for hartID := 0; hartID < e.hartCount; hartID++ {
					if hartMaskIncludes(args[0], args[1], uint64(hartID)) {
						e.clint.SetSoftwareInterruptHart(hartID, true)
					}
				}
			}
			return true, sbiSuccess, 0
		}
		return true, sbiErrNotSupported, 0
	case sbiExtRFence:
		// The interpreter is strongly ordered; for debug shim purposes all remote
		// fence variants complete after validating the hart mask shape.
		if fn <= 6 {
			return true, sbiSuccess, 0
		}
		return true, sbiErrNotSupported, 0
	case sbiExtHSM:
		hartID := args[0]
		switch fn {
		case 0: // hart_start(hartid, start_addr, opaque)
			if hartID >= uint64(len(e.harts)) {
				e.hsmState[hartID] = "absent"
				return true, sbiErrNotSupported, 0
			}
			target := e.harts[hartID]
			target.Halted = false
			target.Waiting = false
			target.Mode = cpu.PrivS
			target.PC = args[1]
			target.InstPC = args[1]
			target.X[10] = hartID
			target.X[11] = args[2]
			e.hsmState[hartID] = "started"
			return true, sbiSuccess, 0
		case 1: // hart_stop
			e.hsmState[h.HartID] = "stopped"
			h.Halted = true
			return true, sbiSuccess, 0
		case 2: // hart_get_status
			if hartID >= uint64(len(e.harts)) {
				return true, sbiSuccess, 3 // absent
			}
			if e.harts[hartID].Halted {
				return true, sbiSuccess, 1 // stopped
			}
			return true, sbiSuccess, 0 // started
		case 3: // hart_suspend
			h.Waiting = true
			e.hsmState[h.HartID] = "suspended"
			return true, sbiSuccess, 0
		default:
			return true, sbiErrNotSupported, 0
		}
	case sbiExtSRST:
		if fn == 0 {
			e.running = false
			h.Halted = true
			return true, sbiSuccess, 0
		}
		return true, sbiErrNotSupported, 0
	}
	return false, sbiErrNotSupported, 0
}

func hartMaskIncludes(mask, base, hartID uint64) bool {
	if base == ^uint64(0) {
		return mask&(uint64(1)<<hartID) != 0
	}
	if hartID < base || hartID >= base+64 {
		return false
	}
	return mask&(uint64(1)<<(hartID-base)) != 0
}

func (e *Emulator) loadSymbols(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "missing Uint8Array"
	}
	data := make([]byte, args[0].Get("byteLength").Int())
	js.CopyBytesToGo(data, args[0])
	source := "symbols"
	if len(args) >= 2 && args[1].Type() == js.TypeString && args[1].String() != "" {
		source = args[1].String()
	}
	tab, err := debugmap.Parse(data, source)
	if err != nil {
		return err.Error()
	}
	e.symbols = tab
	e.recordArtifact("symbols", data, 0, 0, false, source)
	return fmt.Sprintf("loaded %d symbols, %d DWARF lines from %s", tab.Count(), tab.LineCount(), source)
}

func (e *Emulator) symbolsAround(this js.Value, args []js.Value) any {
	if e.symbols == nil {
		return "<no symbols loaded>\n"
	}
	pc := e.currentHart().PC
	if len(args) >= 1 && args[0].Type() == js.TypeString && args[0].String() != "" {
		if v, err := strconv.ParseUint(strings.TrimPrefix(args[0].String(), "0x"), 16, 64); err == nil {
			pc = v
		}
	}
	return e.symbols.Around(pc, 8)
}

func (e *Emulator) symbolSearch(this js.Value, args []js.Value) any {
	if e.symbols == nil {
		return "<no symbols loaded>\n"
	}
	query := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	return e.symbols.Search(query, 64)
}

func (e *Emulator) analyzeLog(this js.Value, args []js.Value) any {
	if e.symbols == nil {
		return "<no symbols loaded>\n"
	}
	text := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		text = args[0].String()
	}
	if strings.TrimSpace(text) == "" {
		text = e.consoleLogString()
	}
	return e.symbols.AnalyzeLog(text, 128)
}

func (e *Emulator) setSBIShim(this js.Value, args []js.Value) any {
	enable := true
	if len(args) > 0 && args[0].Type() == js.TypeBoolean {
		enable = args[0].Bool()
	}
	e.sbiShim = enable
	for _, h := range e.harts {
		h.SBIShim = enable
		h.SBIHandler = e.handleSBI
	}
	return fmt.Sprintf("sbiShim=%v", enable)
}

func (e *Emulator) setHartCount(this js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeNumber {
		return fmt.Sprintf("hartCount=%d", e.hartCount)
	}
	if e.running {
		return "pause before changing hart count"
	}
	n := args[0].Int()
	if n < 1 {
		n = 1
	}
	if n > 8 {
		n = 8
	}
	e.initMachine(n)
	return fmt.Sprintf("hartCount=%d (machine reset; reload firmware/payload/disk if needed)", e.hartCount)
}

func (e *Emulator) setActiveHart(this js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeNumber {
		return fmt.Sprintf("activeHart=%d", e.activeHart)
	}
	n := args[0].Int()
	if n < 0 || n >= len(e.harts) {
		return fmt.Sprintf("hart %d out of range", n)
	}
	e.activeHart = n
	e.currentHart()
	return fmt.Sprintf("activeHart=%d", e.activeHart)
}

func (e *Emulator) injectNetHex(this js.Value, args []js.Value) any {
	if e.net == nil {
		return "virtio-net not present"
	}
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return "missing hex ethernet frame"
	}
	if err := e.net.InjectHex(args[0].String()); err != nil {
		return err.Error()
	}
	return "queued virtio-net RX frame"
}

func (e *Emulator) netTxLog(this js.Value, args []js.Value) any {
	if e.net == nil {
		return ""
	}
	log := e.net.TxFramesHex()
	if log == "" {
		return "<no TX frames captured>\n"
	}
	return log
}

func (e *Emulator) exportTrace(this js.Value, args []js.Value) any {
	var b strings.Builder
	fmt.Fprintf(&b, "[status] %s\n", e.status(this, nil))
	fmt.Fprintf(&b, "[harts] %s\n", e.hartsSummary())
	if e.stopReason != "" {
		fmt.Fprintf(&b, "[stop] %s\n", e.stopReason)
	}
	fmt.Fprintf(&b, "[breakpoints] %s", e.breakpointsString(js.Value{}, nil))
	fmt.Fprintf(&b, "[write-watchpoints] %s", e.watchpointsString(js.Value{}, nil))
	fmt.Fprintf(&b, "[read-watchpoints] %s", e.readWatchpointsString(js.Value{}, nil))
	fmt.Fprintf(&b, "[framebuffer] base=%#x width=%d height=%d stride=%d format=a8r8g8b8\n", fbAddr, fbWidth, fbHeight, fbStride)
	fmt.Fprintf(&b, "[access-histogram]\n%s", e.bus.AccessHistogramString())
	fmt.Fprintf(&b, "[boot-phase]\n%s", e.bootPhaseText())
	fmt.Fprintf(&b, "[boot-timeline]\n%s", e.bootTimeline(js.Value{}, []js.Value{js.ValueOf(48)}))
	fmt.Fprintf(&b, "[device-probe]\n%s", e.deviceProbe(js.Value{}, nil))
	fmt.Fprintf(&b, "[virtqueue-inspect]\n%s", e.virtqueueInspect(js.Value{}, nil))
	fmt.Fprintf(&b, "[virtqueue-chains]\n%s", e.virtqueueChains(js.Value{}, []js.Value{js.ValueOf(8)}))
	fmt.Fprintf(&b, "[virtqueue-anomalies]\n%s", e.virtqueueAnomalies(js.Value{}, []js.Value{js.ValueOf(8)}))
	fmt.Fprintf(&b, "[virtqueue-dot]\n%s", e.virtqueueGraphDOT(js.Value{}, []js.Value{js.ValueOf(8)}))
	fmt.Fprintf(&b, "[guest-memory-scan]\n%s", e.guestMemoryScan(js.Value{}, []js.Value{js.ValueOf(16)}))
	fmt.Fprintf(&b, "[guest-memory-index]\n%s", e.guestMemoryIndex(js.Value{}, []js.Value{js.ValueOf(16)}))
	fmt.Fprintf(&b, "[initcall-classifier]\n%s", e.initcallClassifier(js.Value{}, []js.Value{js.ValueOf(120)}))
	fmt.Fprintf(&b, "[initcall-timeline]\n%s", e.initcallTimeline(js.Value{}, []js.Value{js.ValueOf(120)}))
	fmt.Fprintf(&b, "[panic-summary]\n%s", e.panicSummary(js.Value{}, []js.Value{js.ValueOf(80)}))
	fmt.Fprintf(&b, "[dmesg-extract]\n%s", e.dmesg(js.Value{}, []js.Value{js.ValueOf(80)}))
	if e.symbols != nil {
		fmt.Fprintf(&b, "[dwarf-line-summary]\n%s", e.symbols.LineSummary(32))
	}
	fmt.Fprintf(&b, "[virtqueue-dot]\n%s", e.virtqueueGraphDOT(js.Value{}, []js.Value{js.ValueOf(8)}))
	for i, h := range e.harts {
		traceText := h.TraceStringFiltered(256, e.traceFilter)
		fmt.Fprintf(&b, "\n[hart %d trace filter=%q]\n%s", i, e.traceFilter, traceText)
		if e.symbols != nil {
			fmt.Fprintf(&b, "[hart %d annotated-trace]\n%s", i, e.symbols.AnnotateTraceText(traceText, 256))
		}
		if ct := h.CSRTraceString(256); ct != "" {
			fmt.Fprintf(&b, "[hart %d csr-trace]\n%s", i, ct)
		}
	}
	return b.String()
}

var traceLinePCRe = regexp.MustCompile(`\bpc=([0-9a-fA-F]{8,16})\b`)

func traceLinePC(line string) (uint64, bool) {
	m := traceLinePCRe.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0, false
	}
	pc, err := strconv.ParseUint(m[1], 16, 64)
	return pc, err == nil
}

func (e *Emulator) exportTraceJSON(this js.Value, args []js.Value) any {
	type row struct {
		Hart   int    `json:"hart"`
		Line   int    `json:"line"`
		Text   string `json:"text"`
		Symbol string `json:"symbol,omitempty"`
		Source string `json:"source,omitempty"`
	}
	var rows []row
	for i, h := range e.harts {
		lines := h.TraceLinesFiltered(1024, e.traceFilter)
		for j, line := range lines {
			r := row{Hart: i, Line: j, Text: line}
			if e.symbols != nil {
				if pc, ok := traceLinePC(line); ok {
					r.Symbol = e.symbols.FormatLookup(pc)
					if src := e.symbols.FormatLine(pc); src != "<no DWARF line>" {
						r.Source = src
					}
				}
			}
			rows = append(rows, r)
		}
	}
	b, err := json.MarshalIndent(map[string]any{
		"status":       e.status(this, nil),
		"trace_filter": e.traceFilter,
		"rows":         rows,
	}, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) exportTraceCSV(this js.Value, args []js.Value) any {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"hart", "line", "text"})
	for i, h := range e.harts {
		lines := h.TraceLinesFiltered(1024, e.traceFilter)
		for j, line := range lines {
			_ = w.Write([]string{strconv.Itoa(i), strconv.Itoa(j), line})
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err.Error()
	}
	return b.String()
}

func (e *Emulator) snapshot() map[string]any {
	h := e.currentHart()
	ss := map[string]any{
		"status":      e.status(js.Value{}, nil),
		"hart_count":  e.hartCount,
		"active_hart": e.activeHart,
		"bootargs":    e.bootArgs,
		"entry":       fmt.Sprintf("%#x", e.entry),
		"next_addr":   fmt.Sprintf("%#x", e.nextAddr),
		"dtb_addr":    fmt.Sprintf("%#x", dtbAddr),
		"initrd": map[string]any{
			"start": fmt.Sprintf("%#x", e.initrdBase),
			"end":   fmt.Sprintf("%#x", e.initrdEnd),
		},
		"selected_hart": map[string]any{
			"id":              h.HartID,
			"pc":              fmt.Sprintf("%#x", h.PC),
			"mode":            h.Mode,
			"cycle":           h.Cycle,
			"instret":         h.Instret,
			"halted":          h.Halted,
			"waiting":         h.Waiting,
			"last_trap_cause": h.LastTrapCause,
			"last_trap_tval":  fmt.Sprintf("%#x", h.LastTrapTval),
			"last_ecall_ext":  fmt.Sprintf("%#x", h.LastEcallExt),
			"last_ecall_func": fmt.Sprintf("%#x", h.LastEcallFunc),
			"last_sbi_class":  h.LastSBIClass,
		},
		"hsm":          e.hsmState,
		"sbi_shim":     e.sbiShim,
		"stop_reason":  e.stopReason,
		"trace_filter": e.traceFilter,
		"framebuffer": map[string]any{
			"base":   fmt.Sprintf("%#x", fbAddr),
			"width":  fbWidth,
			"height": fbHeight,
			"stride": fbStride,
			"format": "a8r8g8b8",
		},
		"breakpoints":             len(e.breakpoints),
		"write_watchpoints":       len(e.bus.WatchpointSummary()),
		"read_watchpoints":        len(e.bus.ReadWatchpointSummary()),
		"watchpoint_hits":         e.bus.WatchpointHits(64),
		"artifact_manifest":       e.currentArtifactManifest(),
		"access_histogram":        e.bus.AccessHistogram(),
		"access_timeline":         e.bus.AccessTimeline(64),
		"boot_phase":              e.bootPhaseSummary(),
		"dmesg_tail":              e.dmesgLines(40),
		"boot_timeline":           analyze.BootTimeline(e.consoleLogString(), e.bus.AccessTimeline(1024), 96),
		"device_probe":            analyze.DeviceProbeSummary(e.bus.AccessTimeline(1024)),
		"virtqueues":              analyze.VirtqueueSummary(e.bus.AccessTimeline(1024)),
		"virtqueue_chains":        analyze.VirtqueueChains(e.bus, e.bus.AccessTimeline(1024), 8),
		"memory_objects":          analyze.ScanMemoryObjects(e.bus, 16),
		"memory_index":            analyze.BuildMemoryIndex(e.bus, 16, 0x20000),
		"memory_object_counts":    analyze.MemoryObjectTypeCounts(analyze.ScanMemoryObjects(e.bus, 16)),
		"trace_replay_stats":      analyze.TraceStatsForText(e.combinedTraceText(2048)),
		"boot_regression":         analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram()),
		"virtqueue_snapshot":      analyze.BuildVirtqueueSnapshot(e.bus, e.bus.AccessTimeline(1024), 8),
		"virtqueue_anomaly_hints": analyze.VirtqueueAnomalyHints(analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16)),
		"diagnostic_query_hits":   analyze.QueryDiagnostics(e.traceFilter, e.consoleLogString(), e.combinedTraceText(2048), e.combinedCSRTraceText(1024), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 64),
		"initcall_counts":         analyze.InitcallCategoryCounts(e.consoleLogString()),
		"csr_access_counts":       h.CSRAccessCounts(),
		"pc_profile_top":          h.PCProfileTop(32),
	}
	if e.blk != nil {
		ss["virtio_blk"] = e.blk.DebugString()
	}
	if e.vcon != nil {
		ss["virtio_console"] = e.vcon.DebugString()
	}
	if e.net != nil {
		ss["virtio_net"] = e.net.DebugString()
	}
	if e.rng != nil {
		ss["virtio_rng"] = e.rng.DebugString()
	}
	if e.inputDev != nil {
		ss["virtio_input"] = e.inputDev.DebugString()
	}
	if e.gpu != nil {
		ss["virtio_gpu"] = e.gpu.DebugString()
	}
	if e.clint != nil {
		ss["clint"] = e.clint.DebugString()
	}
	if e.symbols != nil {
		ss["pc_symbol"] = e.symbols.FormatLookup(h.PC)
		ss["pc_line"] = e.symbols.FormatLine(h.PC)
		ss["dwarf_lines"] = e.symbols.LineCount()
	}
	return ss
}

func (e *Emulator) bootPhaseSummary() map[string]any {
	log := strings.ToLower(e.consoleLogString())
	h := e.currentHart()
	hist := map[string]mem.AccessBucket{}
	for _, b := range e.bus.AccessHistogram() {
		hist[b.Name] = b
	}
	phase := map[string]any{
		"firmware_loaded": e.firmwareLoaded,
		"opensbi_seen":    strings.Contains(log, "opensbi"),
		"linux_seen":      strings.Contains(log, "linux version") || strings.Contains(log, "kernel command line"),
		"panic_seen":      strings.Contains(log, "kernel panic") || strings.Contains(log, "oops") || strings.Contains(log, "unable to handle"),
		"uart_activity":   hist["uart16550"].ReadOps + hist["uart16550"].WriteOps,
		"plic_activity":   hist["plic"].ReadOps + hist["plic"].WriteOps,
		"clint_activity":  hist["clint"].ReadOps + hist["clint"].WriteOps,
		"virtio_blk":      hist["virtio-blk"].ReadOps + hist["virtio-blk"].WriteOps,
		"virtio_console":  hist["virtio-console"].ReadOps + hist["virtio-console"].WriteOps,
		"virtio_net":      hist["virtio-net"].ReadOps + hist["virtio-net"].WriteOps,
		"virtio_rng":      hist["virtio-rng"].ReadOps + hist["virtio-rng"].WriteOps,
		"virtio_input":    hist["virtio-input"].ReadOps + hist["virtio-input"].WriteOps,
		"virtio_gpu":      hist["virtio-gpu"].ReadOps + hist["virtio-gpu"].WriteOps,
		"last_pc":         fmt.Sprintf("%#x", h.PC),
		"last_mode":       h.Mode,
		"last_trap":       h.LastTrapCause,
		"last_tval":       fmt.Sprintf("%#x", h.LastTrapTval),
		"stop_reason":     e.stopReason,
	}
	if e.symbols != nil {
		phase["last_pc_symbol"] = e.symbols.FormatLookup(h.PC)
	}
	return phase
}

func (e *Emulator) bootPhaseText() string {
	p := e.bootPhaseSummary()
	order := []string{"firmware_loaded", "opensbi_seen", "linux_seen", "panic_seen", "uart_activity", "plic_activity", "clint_activity", "virtio_blk", "virtio_console", "virtio_net", "virtio_rng", "virtio_input", "virtio_gpu", "last_pc", "last_pc_symbol", "last_mode", "last_trap", "last_tval", "stop_reason"}
	var b strings.Builder
	for _, k := range order {
		if v, ok := p[k]; ok {
			fmt.Fprintf(&b, "%-16s %v\n", k, v)
		}
	}
	return b.String()
}

func (e *Emulator) bootPhase(this js.Value, args []js.Value) any { return e.bootPhaseText() }

var dmesgLineRe = regexp.MustCompile(`^\s*(\[[ 0-9.]+\]|Linux version|Kernel command line|OF:|SBI|sbi|virtio|random:|Serial:|console\s|tty|input:|fbcon|simple-framebuffer|VFS:|EXT4-fs|Freeing|Run /init|init:|BUG:|Oops|Call Trace|Kernel panic|Unable to handle|CPU:|pc\s*:|epc\s*:)`)

func (e *Emulator) dmesgLines(limit int) []string {
	if limit <= 0 || limit > 512 {
		limit = 128
	}
	lines := strings.Split(e.consoleLogString(), "\n")
	out := make([]string, 0, limit)
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if dmesgLineRe.MatchString(line) || strings.Contains(strings.ToLower(line), "panic") || strings.Contains(strings.ToLower(line), "oops") {
			out = append(out, line)
			if len(out) > limit {
				out = out[len(out)-limit:]
			}
		}
	}
	return out
}

func (e *Emulator) dmesg(this js.Value, args []js.Value) any {
	limit := 160
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	lines := e.dmesgLines(limit)
	var b strings.Builder
	fmt.Fprintf(&b, "dmesg extract: %d lines from %d bytes of console log\n", len(lines), len(e.consoleLogString()))
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if e.symbols != nil && len(lines) != 0 {
		b.WriteString("\n[symbols]\n")
		b.WriteString(e.symbols.AnalyzeLog(strings.Join(lines, "\n"), 64))
	}
	if len(lines) == 0 {
		b.WriteString("<no Linux-style lines captured yet>\n")
	}
	return b.String()
}

func (e *Emulator) bootTimeline(this js.Value, args []js.Value) any {
	limit := 128
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	return analyze.BootTimelineString(e.consoleLogString(), e.bus.AccessTimeline(1024), limit)
}

func (e *Emulator) deviceProbe(this js.Value, args []js.Value) any {
	return analyze.DeviceProbeString(e.bus.AccessTimeline(1024))
}

func (e *Emulator) virtqueueInspect(this js.Value, args []js.Value) any {
	return analyze.VirtqueueString(e.bus.AccessTimeline(1024))
}
func (e *Emulator) virtqueueChains(this js.Value, args []js.Value) any {
	maxHeads := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	return analyze.VirtqueueChainsString(e.bus, e.bus.AccessTimeline(1024), maxHeads)
}

func (e *Emulator) virtqueueGraphDOT(this js.Value, args []js.Value) any {
	maxHeads := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	return analyze.VirtqueueChainsDOT(e.bus, e.bus.AccessTimeline(1024), maxHeads)
}

func (e *Emulator) guestMemoryScan(this js.Value, args []js.Value) any {
	maxPerType := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxPerType = args[0].Int()
	}
	return "summary:\n" + analyze.MemoryObjectSummaryString(e.bus, maxPerType) + "\nobjects:\n" + analyze.ScanMemoryObjectsString(e.bus, maxPerType)
}

func (e *Emulator) initcallClassifier(this js.Value, args []js.Value) any {
	max := 160
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		max = args[0].Int()
	}
	var b strings.Builder
	b.WriteString("initcall / driver probe classifier:\n")
	b.WriteString(analyze.InitcallClassifierString(e.consoleLogString(), max))
	b.WriteString("\ncategory counts:\n")
	b.WriteString(analyze.SortedInitcallCategoryCounts(e.consoleLogString()))
	return b.String()
}

func (e *Emulator) initcallTimeline(this js.Value, args []js.Value) any {
	max := 160
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		max = args[0].Int()
	}
	return analyze.InitcallTimelineString(e.consoleLogString(), max)
}

func (e *Emulator) dwarfLinesAround(this js.Value, args []js.Value) any {
	if e.symbols == nil {
		return "<no symbols loaded>\n"
	}
	pc := e.currentHart().PC
	if len(args) >= 1 && args[0].Type() == js.TypeString && args[0].String() != "" {
		if v, err := strconv.ParseUint(strings.TrimPrefix(args[0].String(), "0x"), 16, 64); err == nil {
			pc = v
		}
	}
	return e.symbols.AroundLine(pc, 6)
}

func (e *Emulator) dwarfLineSummary(this js.Value, args []js.Value) any {
	if e.symbols == nil {
		return "<no symbols loaded>\n"
	}
	max := 32
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		max = args[0].Int()
	}
	return e.symbols.LineSummary(max)
}

func (e *Emulator) panicSummary(this js.Value, args []js.Value) any {
	limit := 120
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	return analyze.PanicSummary(e.consoleLogString(), e.symbols, limit)
}

func (e *Emulator) bootAnalysisJSON(this js.Value, args []js.Value) any {
	data := map[string]any{
		"timeline":                analyze.BootTimeline(e.consoleLogString(), e.bus.AccessTimeline(1024), 128),
		"device_probe":            analyze.DeviceProbeSummary(e.bus.AccessTimeline(1024)),
		"virtqueues":              analyze.VirtqueueSummary(e.bus.AccessTimeline(1024)),
		"virtqueue_chain":         analyze.VirtqueueChains(e.bus, e.bus.AccessTimeline(1024), 8),
		"virtqueue_chain_dot":     analyze.VirtqueueChainsDOT(e.bus, e.bus.AccessTimeline(1024), 8),
		"virtqueue_anomalies":     analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 8),
		"virtqueue_anomaly_hints": analyze.VirtqueueAnomalyHints(analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16)),
		"diagnostic_query_hits":   analyze.QueryDiagnostics(e.traceFilter, e.consoleLogString(), e.combinedTraceText(2048), e.combinedCSRTraceText(1024), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 64),
		"memory_objects":          analyze.ScanMemoryObjects(e.bus, 16),
		"memory_index":            analyze.BuildMemoryIndex(e.bus, 16, 0x20000),
		"memory_object_counts":    analyze.MemoryObjectTypeCounts(analyze.ScanMemoryObjects(e.bus, 16)),
		"initcalls":               analyze.ClassifyInitcalls(e.consoleLogString(), 160),
		"initcall_timeline":       analyze.InitcallTimelineString(e.consoleLogString(), 160),
		"panic_text":              analyze.PanicSummary(e.consoleLogString(), e.symbols, 120),
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) mmioDecodedTimeline(this js.Value, args []js.Value) any {
	limit := 256
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	var b strings.Builder
	for _, ev := range e.bus.AccessTimeline(limit) {
		if ev.Name == "dram" {
			continue
		}
		op := "R"
		if ev.Write {
			op = "W"
		}
		fmt.Fprintf(&b, "%06d %s %-15s %-22s addr=%#x size=%d val=%#x\n", ev.Seq, op, ev.Name, ev.Reg, ev.Addr, ev.Size, ev.Value)
	}
	if b.Len() == 0 {
		return "<no MMIO accesses recorded>\n"
	}
	return b.String()
}

func (e *Emulator) accessHistogram(this js.Value, args []js.Value) any {
	return e.bus.AccessHistogramString()
}

func (e *Emulator) clearAccessHistogram(this js.Value, args []js.Value) any {
	e.bus.ClearAccessHistogram()
	return "access histogram cleared"
}

func (e *Emulator) accessTimeline(this js.Value, args []js.Value) any {
	n := 256
	filter := ""
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	if len(args) > 1 && args[1].Type() == js.TypeString {
		filter = args[1].String()
	}
	return e.bus.AccessTimelineString(n, filter)
}

func (e *Emulator) accessTimelineCompact(this js.Value, args []js.Value) any {
	n := 256
	filter := ""
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		n = args[0].Int()
	}
	if len(args) > 1 && args[1].Type() == js.TypeString {
		filter = args[1].String()
	}
	return e.bus.AccessTimelineCompactString(n, filter)
}

func (e *Emulator) clearAccessTimeline(this js.Value, args []js.Value) any {
	e.bus.ClearAccessTimeline()
	return "access timeline cleared"
}

func (e *Emulator) runSmokePreset(this js.Value, args []js.Value) any {
	preset := "current"
	if len(args) > 0 && args[0].Type() == js.TypeString && args[0].String() != "" {
		preset = args[0].String()
	}
	steps := 200000
	if len(args) > 1 && args[1].Type() == js.TypeNumber && args[1].Int() > 0 {
		steps = args[1].Int()
	}
	switch preset {
	case "uart-blk":
		e.bootArgs = "console=ttyS0 earlycon=sbi root=/dev/vda rw"
	case "hvc-blk":
		e.bootArgs = "console=hvc0 earlycon=sbi root=/dev/vda rw"
	case "uart-initrd":
		e.bootArgs = "console=ttyS0 earlycon=sbi root=/dev/ram0 rw"
	case "hvc-initrd":
		e.bootArgs = "console=hvc0 earlycon=sbi root=/dev/ram0 rw"
	case "simplefb":
		e.bootArgs = "console=ttyS0 earlycon=sbi video=simplefb:1024x768-32 nokaslr loglevel=8 root=/dev/vda rw"
	}
	e.installDTB()
	e.resetHarts()
	e.bus.ClearAccessHistogram()
	ran := 0
	for ran < steps && e.anyHartAlive() {
		chunk := 1000
		if steps-ran < chunk {
			chunk = steps - ran
		}
		n := e.runSlice(chunk)
		ran += n
		if n == 0 || e.stopReason != "" || e.firstHartError() != nil {
			break
		}
	}
	ss := e.snapshot()
	ss["smoke_preset"] = preset
	ss["smoke_steps_requested"] = steps
	ss["smoke_steps_ran"] = ran
	b, err := json.MarshalIndent(ss, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) runSmokePresetCore(preset string, steps int) string {
	args := []js.Value{js.ValueOf(preset), js.ValueOf(steps)}
	return e.runSmokePreset(js.Value{}, args).(string)
}

func (e *Emulator) runSmokeMatrix(this js.Value, args []js.Value) any {
	presets := []string{"uart-blk", "hvc-blk", "uart-initrd", "hvc-initrd", "simplefb"}
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		presets = nil
		for _, p := range strings.Split(args[0].String(), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				presets = append(presets, p)
			}
		}
	}
	steps := 50000
	if len(args) > 1 && args[1].Type() == js.TypeNumber && args[1].Int() > 0 {
		steps = args[1].Int()
	}
	rows := []analyze.SmokeSummary{}
	for _, preset := range presets {
		text := e.runSmokePresetCore(preset, steps)
		var snap map[string]any
		_ = json.Unmarshal([]byte(text), &snap)
		report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(1024), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
		anoms := analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16)
		causes := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(1024), report, anoms, 1)
		top := ""
		if len(causes) != 0 {
			top = causes[0].Category + ": " + causes[0].Summary
		}
		phase := "unknown"
		if len(report.BootEvents) != 0 {
			phase = report.BootEvents[len(report.BootEvents)-1].Phase
		}
		ran := 0
		if v, ok := snap["smoke_steps_ran"].(float64); ok {
			ran = int(v)
		}
		requested := steps
		if v, ok := snap["smoke_steps_requested"].(float64); ok {
			requested = int(v)
		}
		rows = append(rows, analyze.SmokeSummary{Preset: preset, Ran: ran, Requested: requested, Status: e.status(js.Value{}, nil).(string), Phase: phase, TopCause: top})
	}
	return analyze.SmokeSummaryString(rows)
}

func (e *Emulator) runSmokeMatrixJSON(this js.Value, args []js.Value) any {
	presets := []string{"uart-blk", "hvc-blk", "uart-initrd", "hvc-initrd", "simplefb"}
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		presets = nil
		for _, p := range strings.Split(args[0].String(), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				presets = append(presets, p)
			}
		}
	}
	steps := 50000
	if len(args) > 1 && args[1].Type() == js.TypeNumber && args[1].Int() > 0 {
		steps = args[1].Int()
	}
	rows := []analyze.SmokeSummary{}
	for _, preset := range presets {
		text := e.runSmokePresetCore(preset, steps)
		var snap map[string]any
		_ = json.Unmarshal([]byte(text), &snap)
		report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(1024), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
		anoms := analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16)
		causes := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(1024), report, anoms, 1)
		top := ""
		if len(causes) != 0 {
			top = causes[0].Category + ": " + causes[0].Summary
		}
		phase := "unknown"
		if len(report.BootEvents) != 0 {
			phase = report.BootEvents[len(report.BootEvents)-1].Phase
		}
		ran := 0
		if v, ok := snap["smoke_steps_ran"].(float64); ok {
			ran = int(v)
		}
		requested := steps
		if v, ok := snap["smoke_steps_requested"].(float64); ok {
			requested = int(v)
		}
		rows = append(rows, analyze.SmokeSummary{Preset: preset, Ran: ran, Requested: requested, Status: e.status(js.Value{}, nil).(string), Phase: phase, TopCause: top})
	}
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func smokePresetsFromArgs(args []js.Value, defaultSteps int) ([]string, int) {
	presets := []string{"uart-blk", "hvc-blk", "uart-initrd", "hvc-initrd", "simplefb"}
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		presets = nil
		for _, p := range strings.Split(args[0].String(), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				presets = append(presets, p)
			}
		}
	}
	steps := defaultSteps
	if len(args) > 1 && args[1].Type() == js.TypeNumber && args[1].Int() > 0 {
		steps = args[1].Int()
	}
	return presets, steps
}

func (e *Emulator) runSmokeRows(presets []string, steps int) []analyze.SmokeSummary {
	rows := []analyze.SmokeSummary{}
	for _, preset := range presets {
		text := e.runSmokePresetCore(preset, steps)
		var snap map[string]any
		_ = json.Unmarshal([]byte(text), &snap)
		report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(1024), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
		anoms := analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16)
		causes := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(1024), report, anoms, 1)
		top := ""
		if len(causes) != 0 {
			top = causes[0].Category + ": " + causes[0].Summary
		}
		phase := "unknown"
		if len(report.BootEvents) != 0 {
			phase = report.BootEvents[len(report.BootEvents)-1].Phase
		}
		ran := 0
		if v, ok := snap["smoke_steps_ran"].(float64); ok {
			ran = int(v)
		}
		requested := steps
		if v, ok := snap["smoke_steps_requested"].(float64); ok {
			requested = int(v)
		}
		rows = append(rows, analyze.SmokeSummary{Preset: preset, Ran: ran, Requested: requested, Status: e.status(js.Value{}, nil).(string), Phase: phase, TopCause: top})
	}
	return rows
}

func (e *Emulator) smokeMatrixMarkdown(this js.Value, args []js.Value) any {
	presets, steps := smokePresetsFromArgs(args, 50000)
	return analyze.SmokeSummaryMarkdown(e.runSmokeRows(presets, steps))
}

func (e *Emulator) smokeMatrixHTML(this js.Value, args []js.Value) any {
	presets, steps := smokePresetsFromArgs(args, 50000)
	return analyze.SmokeSummaryHTML(e.runSmokeRows(presets, steps))
}

func (e *Emulator) compareSmokeMatrix(this js.Value, args []js.Value) any {
	baselineText := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baselineText = args[0].String()
	}
	presets, steps := smokePresetsFromArgs(args[1:], 50000)
	var baseline []analyze.SmokeSummary
	if strings.TrimSpace(baselineText) != "" {
		if err := json.Unmarshal([]byte(baselineText), &baseline); err != nil {
			return "baseline JSON parse error: " + err.Error()
		}
	}
	return analyze.SmokeMatrixDiffString(analyze.CompareSmokeSummaries(e.runSmokeRows(presets, steps), baseline))
}

func (e *Emulator) compareSmokeMatrixJSON(this js.Value, args []js.Value) any {
	baselineText := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baselineText = args[0].String()
	}
	presets, steps := smokePresetsFromArgs(args[1:], 50000)
	var baseline []analyze.SmokeSummary
	if strings.TrimSpace(baselineText) != "" {
		if err := json.Unmarshal([]byte(baselineText), &baseline); err != nil {
			return `{"error":` + strconv.Quote(err.Error()) + `}`
		}
	}
	rows := analyze.CompareSmokeSummaries(e.runSmokeRows(presets, steps), baseline)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) watchpointHits(this js.Value, args []js.Value) any {
	limit := 128
	filter := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	if len(args) > 1 && args[1].Type() == js.TypeString {
		filter = args[1].String()
	}
	return e.bus.WatchpointHitsString(limit, filter)
}

func (e *Emulator) watchpointHitsJSON(this js.Value, args []js.Value) any {
	limit := 256
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	b, err := json.MarshalIndent(e.bus.WatchpointHits(limit), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) clearWatchpointHits(this js.Value, args []js.Value) any {
	e.bus.ClearWatchpointHits()
	return "watchpoint hit timeline cleared"
}

func (e *Emulator) combinedTraceText(limit int) string {
	if limit <= 0 {
		limit = 1024
	}
	var b strings.Builder
	for i, h := range e.harts {
		fmt.Fprintf(&b, "[hart %d]\n", i)
		b.WriteString(h.TraceStringFiltered(limit, e.traceFilter))
	}
	return b.String()
}

func (e *Emulator) traceReplayReport(this js.Value, args []js.Value) any {
	limit := 1024
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	var b strings.Builder
	for i, h := range e.harts {
		fmt.Fprintf(&b, "[hart %d]\n", i)
		b.WriteString(analyze.TraceStatsString(h.TraceStringFiltered(limit, e.traceFilter)))
	}
	return b.String()
}

func (e *Emulator) traceCompare(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	if strings.TrimSpace(baseline) == "" {
		return "<paste a baseline trace first>\n"
	}
	maxDiffs := 32
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		maxDiffs = args[1].Int()
	}
	return analyze.CompareTraceTextString(baseline, e.combinedTraceText(4096), maxDiffs)
}

func (e *Emulator) bootRegressionReport(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	return analyze.BootRegressionReportString(report)
}

func (e *Emulator) bootRegressionJSON(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) virtqueueSnapshot(this js.Value, args []js.Value) any {
	maxHeads := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	return analyze.VirtqueueSnapshotString(e.bus, e.bus.AccessTimeline(1024), maxHeads)
}

func (e *Emulator) captureMemoryScan(this js.Value, args []js.Value) any {
	maxPerType := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxPerType = args[0].Int()
	}
	e.lastMemoryScan = analyze.ScanMemoryObjects(e.bus, maxPerType)
	return fmt.Sprintf("memory scan snapshot captured: %d objects", len(e.lastMemoryScan))
}

func (e *Emulator) diffMemoryScan(this js.Value, args []js.Value) any {
	if e.lastMemoryScan == nil {
		return "<no captured memory scan>\n"
	}
	maxPerType := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxPerType = args[0].Int()
	}
	return analyze.DiffMemoryObjectsString(e.lastMemoryScan, analyze.ScanMemoryObjects(e.bus, maxPerType))
}

func (e *Emulator) dwarfSourceContext(this js.Value, args []js.Value) any {
	if e.symbols == nil {
		return "<no symbols loaded>\n"
	}
	pc := e.currentHart().PC
	if len(args) > 0 && args[0].Type() == js.TypeString && args[0].String() != "" {
		if v, err := strconv.ParseUint(strings.TrimPrefix(args[0].String(), "0x"), 16, 64); err == nil {
			pc = v
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "PC %#x\n", pc)
	b.WriteString(e.symbols.Around(pc, 4))
	b.WriteByte('\n')
	b.WriteString(e.symbols.AroundLine(pc, 10))
	return b.String()
}

func (e *Emulator) bootRegressionMarkdown(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	return analyze.BootRegressionReportMarkdown(report)
}

func (e *Emulator) bootRegressionHTML(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	return analyze.BootRegressionReportHTML(report)
}

func (e *Emulator) virtqueueAnomalies(this js.Value, args []js.Value) any {
	maxHeads := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	return analyze.VirtqueueAnomaliesString(e.bus, e.bus.AccessTimeline(1024), maxHeads)
}

func (e *Emulator) virtqueueAnomaliesJSON(this js.Value, args []js.Value) any {
	maxHeads := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	b, err := json.MarshalIndent(analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), maxHeads), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) guestMemoryIndex(this js.Value, args []js.Value) any {
	maxPerType := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxPerType = args[0].Int()
	}
	return analyze.MemoryIndexString(e.bus, maxPerType)
}

func (e *Emulator) guestMemoryIndexJSON(this js.Value, args []js.Value) any {
	maxPerType := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxPerType = args[0].Int()
	}
	b, err := json.MarshalIndent(analyze.BuildMemoryIndex(e.bus, maxPerType, 0x20000), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) combinedCSRTraceText(limit int) string {
	if limit <= 0 {
		limit = 512
	}
	var b strings.Builder
	for i, h := range e.harts {
		text := h.CSRTraceString(limit)
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "[hart %d csr]\n", i)
		b.WriteString(text)
	}
	return b.String()
}

func (e *Emulator) diagnosticQuery(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	if strings.TrimSpace(query) == "" {
		return "<enter a diagnostic query or trace filter>\n"
	}
	return analyze.QueryDiagnosticsString(query, e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 96)
}

func (e *Emulator) diagnosticQueryJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	hits := analyze.QueryDiagnostics(query, e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 256)
	b, err := json.MarshalIndent(map[string]any{"query": strings.TrimSpace(query), "hits": hits}, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) virtqueueAnomalyHints(this js.Value, args []js.Value) any {
	maxHeads := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	return analyze.VirtqueueAnomalyHintsString(analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), maxHeads))
}

func (e *Emulator) virtqueueAnomalyHintsJSON(this js.Value, args []js.Value) any {
	maxHeads := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	b, err := json.MarshalIndent(analyze.VirtqueueAnomalyHints(analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), maxHeads)), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) shareReportMarkdown(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	bundle := analyze.BuildShareBundle(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	return analyze.ShareBundleMarkdown(bundle)
}

func (e *Emulator) shareReportJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	bundle := analyze.BuildShareBundle(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	b, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) diagnosticQueryPresets(this js.Value, args []js.Value) any {
	return analyze.DiagnosticQueryPresetResultsString(e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 12)
}

func (e *Emulator) diagnosticQueryPresetsJSON(this js.Value, args []js.Value) any {
	rows := analyze.DiagnosticQueryPresetResults(e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 24)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) virtqueueAnomalyTriage(this js.Value, args []js.Value) any {
	maxHeads := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	return analyze.VirtqueueAnomalyTriageString(analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), maxHeads))
}

func (e *Emulator) virtqueueAnomalyTriageJSON(this js.Value, args []js.Value) any {
	maxHeads := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		maxHeads = args[0].Int()
	}
	b, err := json.MarshalIndent(analyze.VirtqueueAnomalyTriage(analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), maxHeads)), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) memoryIndexSearch(this js.Value, args []js.Value) any {
	query := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	if strings.TrimSpace(query) == "" {
		query = e.traceFilter
	}
	return analyze.SearchMemoryIndexString(analyze.BuildMemoryIndex(e.bus, 32, 0x20000), query, 80)
}

func (e *Emulator) memoryJumpHints(this js.Value, args []js.Value) any {
	return analyze.MemoryJumpHintsString(analyze.BuildMemoryIndex(e.bus, 32, 0x20000), 24)
}

func (e *Emulator) memoryJumpHintsJSON(this js.Value, args []js.Value) any {
	b, err := json.MarshalIndent(analyze.MemoryJumpHints(analyze.BuildMemoryIndex(e.bus, 32, 0x20000), 64), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) shareReportHTML(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	bundle := analyze.BuildShareBundle(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	return analyze.ShareBundleHTML(bundle)
}

func (e *Emulator) triageDashboard(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	d := analyze.BuildTriageDashboard(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	return analyze.TriageDashboardString(d)
}

func (e *Emulator) triageDashboardJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	d := analyze.BuildTriageDashboard(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) currentTriageDashboard(query string) analyze.TriageDashboard {
	return analyze.BuildTriageDashboard(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
}

func (e *Emulator) redactionOptionsFromArgs(args []js.Value, index int) analyze.RedactionOptions {
	opt := analyze.DefaultRedactionOptions()
	if len(args) > index && args[index].Type() == js.TypeString {
		opt = analyze.RedactionOptionsFromJSON(args[index].String(), opt)
	}
	return opt
}

func (e *Emulator) stopCauseEvidence(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	rows := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), report, analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), 12)
	return analyze.StopCauseEvidenceString(rows)
}

func (e *Emulator) stopCauseEvidenceJSON(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	rows := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), report, analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), 32)
	b, err := json.MarshalIndent(analyze.ExplainStopCauses(rows), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) triageDashboardDiff(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	query := e.traceFilter
	if len(args) > 1 && args[1].Type() == js.TypeString {
		query = args[1].String()
	}
	return analyze.TriageDashboardDiffString(baseline, e.currentTriageDashboard(query))
}

func (e *Emulator) triageDashboardDiffJSON(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	query := e.traceFilter
	if len(args) > 1 && args[1].Type() == js.TypeString {
		query = args[1].String()
	}
	rows := analyze.DiffTriageDashboardJSON(baseline, e.currentTriageDashboard(query))
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) memoryRangeDump(this js.Value, args []js.Value) any {
	addr := e.currentHart().PC
	if len(args) > 0 && args[0].Type() == js.TypeString && args[0].String() != "" {
		addr = analyze.HexToUint64(args[0].String(), addr)
	}
	length := 256
	if len(args) > 1 && args[1].Type() == js.TypeNumber && args[1].Int() > 0 {
		length = args[1].Int()
	}
	return analyze.DumpMemoryRangeString(e.bus, addr, length)
}

func (e *Emulator) memoryRangeDumpJSON(this js.Value, args []js.Value) any {
	addr := e.currentHart().PC
	if len(args) > 0 && args[0].Type() == js.TypeString && args[0].String() != "" {
		addr = analyze.HexToUint64(args[0].String(), addr)
	}
	length := 256
	if len(args) > 1 && args[1].Type() == js.TypeNumber && args[1].Int() > 0 {
		length = args[1].Int()
	}
	return analyze.MarshalMemoryRangeDump(analyze.DumpMemoryRange(e.bus, addr, length))
}

func (e *Emulator) stopCauseRanking(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	rows := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), report, analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), 12)
	d := analyze.TriageDashboard{Status: e.status(js.Value{}, nil).(string), Phase: "manual", TopCandidates: rows}
	return analyze.TriageDashboardString(d)
}

func (e *Emulator) stopCauseRankingJSON(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	rows := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), report, analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), 32)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) currentPresetResults() []analyze.DiagnosticQueryPresetResult {
	return analyze.DiagnosticQueryPresetResults(e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 24)
}

func (e *Emulator) diagnosticPresetBaselineJSON(this js.Value, args []js.Value) any {
	b, err := json.MarshalIndent(e.currentPresetResults(), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) compareDiagnosticPresetBaseline(this js.Value, args []js.Value) any {
	baselineText := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baselineText = args[0].String()
	}
	var baseline []analyze.DiagnosticQueryPresetResult
	if strings.TrimSpace(baselineText) != "" {
		if err := json.Unmarshal([]byte(baselineText), &baseline); err != nil {
			return "baseline JSON parse error: " + err.Error()
		}
	}
	return analyze.QueryPresetComparisonString(analyze.CompareDiagnosticQueryPresets(e.currentPresetResults(), baseline))
}

func (e *Emulator) compareDiagnosticPresetBaselineJSON(this js.Value, args []js.Value) any {
	baselineText := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baselineText = args[0].String()
	}
	var baseline []analyze.DiagnosticQueryPresetResult
	if strings.TrimSpace(baselineText) != "" {
		if err := json.Unmarshal([]byte(baselineText), &baseline); err != nil {
			return `{"error":` + strconv.Quote(err.Error()) + `}`
		}
	}
	rows := analyze.CompareDiagnosticQueryPresets(e.currentPresetResults(), baseline)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) memoryObjectDump(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	if strings.TrimSpace(query) == "" {
		query = "linux"
	}
	return analyze.DumpMemoryIndexHitsString(e.bus, analyze.BuildMemoryIndex(e.bus, 32, 0x20000), query, 8, 256)
}

func (e *Emulator) memoryObjectDumpJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	rows := analyze.DumpMemoryIndexHits(e.bus, analyze.BuildMemoryIndex(e.bus, 32, 0x20000), query, 16, 512)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) shareReportRedactedMarkdown(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	opt := e.redactionOptionsFromArgs(args, 1)
	bundle := analyze.BuildShareBundle(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	return analyze.RedactedShareBundleMarkdown(bundle, opt)
}

func (e *Emulator) shareReportRedactedJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	opt := e.redactionOptionsFromArgs(args, 1)
	bundle := analyze.BuildShareBundle(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	return analyze.RedactedShareBundleJSON(bundle, opt)
}

func (e *Emulator) shareReportRedactedHTML(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	opt := e.redactionOptionsFromArgs(args, 1)
	bundle := analyze.BuildShareBundle(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	return analyze.RedactSensitiveText(analyze.ShareBundleHTML(bundle), opt)
}

func (e *Emulator) redactionOptions(this js.Value, args []js.Value) any {
	return analyze.RedactionOptionsString(e.redactionOptionsFromArgs(args, 0))
}

func (e *Emulator) stopCauseChecklist(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	rows := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), report, analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), 12)
	return analyze.StopCauseChecklistString(rows)
}

func (e *Emulator) stopCauseChecklistJSON(this js.Value, args []js.Value) any {
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	rows := analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), report, analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), 32)
	b, err := json.MarshalIndent(analyze.StopCauseChecklist(rows), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) diagnosticBookmarks(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	hits := analyze.QueryDiagnostics(query, e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 256)
	return analyze.QueryBookmarkSetString(analyze.BuildQueryBookmarkSet(query, hits, 8))
}

func (e *Emulator) diagnosticBookmarksJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	hits := analyze.QueryDiagnostics(query, e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus.AccessTimeline(1024), analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), analyze.BuildMemoryIndex(e.bus, 16, 0x20000), 256)
	set := analyze.BuildQueryBookmarkSet(query, hits, 16)
	b, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) artifactManifest(this js.Value, args []js.Value) any {
	return analyze.ArtifactManifestString(e.currentArtifactManifest())
}

func (e *Emulator) artifactManifestJSON(this js.Value, args []js.Value) any {
	return analyze.ArtifactManifestJSON(e.currentArtifactManifest())
}

func (e *Emulator) currentStopCauses(limit int) []analyze.StopCauseCandidate {
	if limit <= 0 {
		limit = 12
	}
	report := analyze.BuildBootRegressionReport(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram())
	return analyze.RankStopCauses(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), report, analyze.VirtqueueAnomalies(e.bus, e.bus.AccessTimeline(1024), 16), limit)
}

func (e *Emulator) artifactManifestDiff(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	return analyze.ArtifactManifestDiffString(analyze.CompareArtifactManifests(e.currentArtifactManifest(), baseline))
}

func (e *Emulator) artifactManifestDiffJSON(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	rows := analyze.CompareArtifactManifests(e.currentArtifactManifest(), baseline)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) autoBreakpointSuggestions(this js.Value, args []js.Value) any {
	limit := 16
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	rows := analyze.SuggestBreakpoints(e.currentStopCauses(12), e.combinedTraceText(4096), e.bus.WatchpointHits(128), limit)
	return analyze.BreakpointSuggestionsString(rows)
}

func (e *Emulator) autoBreakpointSuggestionsJSON(this js.Value, args []js.Value) any {
	limit := 32
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	rows := analyze.SuggestBreakpoints(e.currentStopCauses(12), e.combinedTraceText(4096), e.bus.WatchpointHits(128), limit)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) smokeFailureClusters(this js.Value, args []js.Value) any {
	presets, steps := smokePresetsFromArgs(args, 50000)
	return analyze.SmokeFailureClustersString(analyze.ClusterSmokeFailures(e.runSmokeRows(presets, steps)))
}

func (e *Emulator) smokeFailureClustersJSON(this js.Value, args []js.Value) any {
	presets, steps := smokePresetsFromArgs(args, 50000)
	rows := analyze.ClusterSmokeFailures(e.runSmokeRows(presets, steps))
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) currentDiagnosticBundle(query string, smokeRows []analyze.SmokeSummary) analyze.DiagnosticBundle {
	share := analyze.BuildShareBundle(e.status(js.Value{}, nil).(string), e.consoleLogString(), e.combinedTraceText(4096), e.combinedCSRTraceText(2048), e.bus, e.bus.AccessTimeline(1024), e.bus.AccessHistogram(), query)
	stops := e.currentStopCauses(16)
	return analyze.DiagnosticBundle{
		Manifest:    e.currentArtifactManifest(),
		Triage:      e.currentTriageDashboard(query),
		StopCauses:  stops,
		Suggestions: analyze.SuggestBreakpoints(stops, e.combinedTraceText(4096), e.bus.WatchpointHits(128), 24),
		Smoke:       smokeRows,
		Clusters:    analyze.ClusterSmokeFailures(smokeRows),
		Share:       share,
		Watches:     e.bus.WatchpointHits(128),
		Notes:       []string{"generated by rvwasm browser emulator", "bundle omits raw DRAM/disk bytes"},
	}
}

func (e *Emulator) diagnosticBundleJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	var smoke []analyze.SmokeSummary
	if len(args) > 1 && args[1].Type() == js.TypeString && strings.TrimSpace(args[1].String()) != "" {
		presets, steps := smokePresetsFromArgs(args[1:], 25000)
		smoke = e.runSmokeRows(presets, steps)
	}
	return analyze.DiagnosticBundleJSON(e.currentDiagnosticBundle(query, smoke))
}

func (e *Emulator) diagnosticBundleCompressedJSON(this js.Value, args []js.Value) any {
	jsonText := e.diagnosticBundleJSON(this, args).(string)
	return analyze.CompressedDiagnosticBundleJSONString(jsonText)
}

func (e *Emulator) decodeDiagnosticBundle(this js.Value, args []js.Value) any {
	if len(args) == 0 || args[0].Type() != js.TypeString {
		return `{"error":"missing compressed or plain diagnostic bundle text"}`
	}
	return analyze.DecodeDiagnosticBundleJSONString(args[0].String())
}

func (e *Emulator) compareDiagnosticBundleBaseline(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	query := e.traceFilter
	if len(args) > 1 && args[1].Type() == js.TypeString {
		query = args[1].String()
	}
	rows := analyze.CompareDiagnosticBundles(e.currentDiagnosticBundle(query, nil), baseline)
	return analyze.DiagnosticBundleDiffString(rows)
}

func (e *Emulator) compareDiagnosticBundleBaselineJSON(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	query := e.traceFilter
	if len(args) > 1 && args[1].Type() == js.TypeString {
		query = args[1].String()
	}
	rows := analyze.CompareDiagnosticBundles(e.currentDiagnosticBundle(query, nil), baseline)
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) currentProvenance(query string) analyze.ProvenanceReport {
	bundleJSON := analyze.DiagnosticBundleJSON(e.currentDiagnosticBundle(query, nil))
	return analyze.BuildProvenanceReport("rvwasm-go1.23.2-js-wasm", time.Now().UTC().Format(time.RFC3339), e.currentArtifactManifest(), e.combinedTraceText(4096), e.consoleLogString(), bundleJSON, e.currentStopCauses(8))
}

func (e *Emulator) provenanceReport(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	return analyze.ProvenanceReportString(e.currentProvenance(query))
}

func (e *Emulator) provenanceReportJSON(this js.Value, args []js.Value) any {
	query := e.traceFilter
	if len(args) > 0 && args[0].Type() == js.TypeString {
		query = args[0].String()
	}
	return analyze.ProvenanceReportJSON(e.currentProvenance(query))
}

func (e *Emulator) regressionHandoffMarkdown(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	query := e.traceFilter
	if len(args) > 1 && args[1].Type() == js.TypeString {
		query = args[1].String()
	}
	bundle := e.currentDiagnosticBundle(query, nil)
	prov := e.currentProvenance(query)
	return analyze.RegressionHandoffMarkdown(bundle, prov, baseline)
}

func (e *Emulator) applyAutoBreakpointSuggestions(this js.Value, args []js.Value) any {
	limit := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	if limit <= 0 || limit > 64 {
		limit = 8
	}
	suggestions := analyze.SuggestBreakpoints(e.currentStopCauses(12), e.combinedTraceText(4096), e.bus.WatchpointHits(128), limit)
	applied := 0
	var b strings.Builder
	for _, s := range suggestions {
		switch s.Kind {
		case "pc-breakpoint":
			if e.breakpoints == nil {
				e.breakpoints = map[uint64]Breakpoint{}
			}
			old := e.breakpoints[s.Address]
			bp := Breakpoint{PC: s.Address, Hart: -1, Hits: old.Hits, Mode: strings.ToLower(strings.TrimSpace(s.Mode))}
			if bp.Mode != "" && bp.Mode != "u" && bp.Mode != "s" && bp.Mode != "m" {
				bp.Mode = ""
			}
			e.breakpoints[s.Address] = bp
			fmt.Fprintf(&b, "breakpoint %#x reason=%s\n", s.Address, s.Reason)
			applied++
		case "read-watchpoint":
			length := s.Length
			if length == 0 {
				length = 8
			}
			e.bus.AddReadWatchpoint(s.Address, length, "auto:"+s.Reason)
			fmt.Fprintf(&b, "read watchpoint %#x+%#x reason=%s\n", s.Address, length, s.Reason)
			applied++
		case "write-watchpoint":
			length := s.Length
			if length == 0 {
				length = 8
			}
			e.bus.AddWriteWatchpoint(s.Address, length, "auto:"+s.Reason)
			fmt.Fprintf(&b, "write watchpoint %#x+%#x reason=%s\n", s.Address, length, s.Reason)
			applied++
		}
	}
	if applied == 0 {
		return "no breakpoint/watchpoint suggestions were applied\n"
	}
	return fmt.Sprintf("applied %d auto break/watch suggestions\n%s", applied, b.String())
}

func (e *Emulator) reproductionPlan(this js.Value, args []js.Value) any {
	preset := "uart-blk"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		preset = args[0].String()
	}
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	return analyze.ReproductionPlanString(analyze.BuildReproductionPlan(bundle, e.currentProvenance(e.traceFilter), preset))
}

func (e *Emulator) reproductionPlanJSON(this js.Value, args []js.Value) any {
	preset := "uart-blk"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		preset = args[0].String()
	}
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	return analyze.ReproductionPlanJSON(analyze.BuildReproductionPlan(bundle, e.currentProvenance(e.traceFilter), preset))
}

func (e *Emulator) reproductionPlanMarkdown(this js.Value, args []js.Value) any {
	preset := "uart-blk"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		preset = args[0].String()
	}
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	return analyze.ReproductionPlanMarkdown(analyze.BuildReproductionPlan(bundle, e.currentProvenance(e.traceFilter), preset))
}

func (e *Emulator) logSignature(this js.Value, args []js.Value) any {
	return analyze.LogSignatureSetString(analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest()))
}

func (e *Emulator) logSignatureJSON(this js.Value, args []js.Value) any {
	return analyze.LogSignatureSetJSON(analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest()))
}

func (e *Emulator) compareLogSignature(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	rows := analyze.CompareLogSignatures(analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest()), baseline)
	return analyze.LogSignatureDiffString(rows)
}

func (e *Emulator) compareLogSignatureJSON(this js.Value, args []js.Value) any {
	baseline := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		baseline = args[0].String()
	}
	rows := analyze.CompareLogSignatures(analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest()), baseline)
	return analyze.LogSignatureDiffJSON(rows)
}

func (e *Emulator) autoBreakpointApplyReport(this js.Value, args []js.Value) any {
	limit := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	suggestions := analyze.SuggestBreakpoints(e.currentStopCauses(12), e.combinedTraceText(4096), e.bus.WatchpointHits(128), limit)
	return analyze.AppliedSuggestionReportString(analyze.BuildAppliedSuggestionReport(suggestions, limit))
}

func (e *Emulator) autoBreakpointApplyReportJSON(this js.Value, args []js.Value) any {
	limit := 8
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		limit = args[0].Int()
	}
	suggestions := analyze.SuggestBreakpoints(e.currentStopCauses(12), e.combinedTraceText(4096), e.bus.WatchpointHits(128), limit)
	return analyze.AppliedSuggestionReportJSON(analyze.BuildAppliedSuggestionReport(suggestions, limit))
}

func (e *Emulator) headlessSmokeScript(this js.Value, args []js.Value) any {
	steps := 200000
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		steps = args[0].Int()
	}
	var presets []string
	if len(args) > 1 && args[1].Type() == js.TypeString {
		for _, p := range strings.Split(args[1].String(), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				presets = append(presets, p)
			}
		}
	}
	return analyze.HeadlessSmokeScript(e.currentArtifactManifest(), presets, steps)
}

func (e *Emulator) headlessRunnerSpec(this js.Value, args []js.Value) any {
	steps := 200000
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		steps = args[0].Int()
	}
	presets := presetsFromStringArg(args, 1)
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	return analyze.HeadlessSmokeRunnerSpecString(analyze.BuildHeadlessSmokeRunnerSpec(bundle, presets, steps))
}

func (e *Emulator) headlessRunnerSpecJSON(this js.Value, args []js.Value) any {
	steps := 200000
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		steps = args[0].Int()
	}
	presets := presetsFromStringArg(args, 1)
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	return analyze.HeadlessSmokeRunnerSpecJSON(analyze.BuildHeadlessSmokeRunnerSpec(bundle, presets, steps))
}

func (e *Emulator) bundleIntegrity(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	return analyze.BundleIntegrityReportString(analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle)))
}

func (e *Emulator) bundleIntegrityJSON(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	return analyze.BundleIntegrityReportJSON(analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle)))
}

func (e *Emulator) reproValidation(this js.Value, args []js.Value) any {
	preset := "uart-blk"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		preset = args[0].String()
	}
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	plan := analyze.BuildReproductionPlan(bundle, e.currentProvenance(e.traceFilter), preset)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	return analyze.ReproductionValidationReportString(analyze.ValidateReproductionPlan(bundle, plan, sig))
}

func (e *Emulator) reproValidationJSON(this js.Value, args []js.Value) any {
	preset := "uart-blk"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		preset = args[0].String()
	}
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	plan := analyze.BuildReproductionPlan(bundle, e.currentProvenance(e.traceFilter), preset)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	return analyze.ReproductionValidationReportJSON(analyze.ValidateReproductionPlan(bundle, plan, sig))
}

func (e *Emulator) ciSummary(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	integrity := analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle))
	return analyze.CISummaryString(analyze.BuildCISummary(bundle, sig, integrity))
}

func (e *Emulator) ciSummaryJSON(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	integrity := analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle))
	return analyze.CISummaryJSON(analyze.BuildCISummary(bundle, sig, integrity))
}

func (e *Emulator) ciGateReport(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	integrity := analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle))
	policyName := "default"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		policyName = args[0].String()
	}
	policy := analyze.DefaultCIGatePolicy()
	if t, ok := analyze.CIGatePolicyTemplateByName(policyName); ok {
		policy = t.Policy
	}
	return analyze.CIGateReportString(analyze.BuildCIGateReport(bundle, integrity, sig, nil, policy))
}

func (e *Emulator) ciGateReportJSON(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	integrity := analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle))
	policyName := "default"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		policyName = args[0].String()
	}
	policy := analyze.DefaultCIGatePolicy()
	if t, ok := analyze.CIGatePolicyTemplateByName(policyName); ok {
		policy = t.Policy
	}
	return analyze.CIGateReportJSON(analyze.BuildCIGateReport(bundle, integrity, sig, nil, policy))
}

func (e *Emulator) ciActionChecklist(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	integrity := analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle))
	policy := analyze.DefaultCIGatePolicy()
	gate := analyze.BuildCIGateReport(bundle, integrity, sig, nil, policy)
	return analyze.CIActionChecklistString(analyze.BuildCIActionChecklist(gate, integrity, nil))
}

func (e *Emulator) ciActionChecklistJSON(this js.Value, args []js.Value) any {
	bundle := e.currentDiagnosticBundle(e.traceFilter, nil)
	sig := analyze.BuildLogSignatureSet(e.combinedTraceText(4096), e.consoleLogString(), e.currentArtifactManifest())
	integrity := analyze.BuildBundleIntegrityReport(bundle, analyze.DiagnosticBundleJSON(bundle))
	policy := analyze.DefaultCIGatePolicy()
	gate := analyze.BuildCIGateReport(bundle, integrity, sig, nil, policy)
	return analyze.CIActionChecklistJSON(analyze.BuildCIActionChecklist(gate, integrity, nil))
}

func (e *Emulator) githubActionsMatrixWorkflow(this js.Value, args []js.Value) any {
	policy := "linux-boot"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		policy = args[0].String()
	}
	presets := presetsFromStringArg(args, 1)
	return analyze.GitHubActionsMatrixWorkflowYAML(policy, presets)
}

func (e *Emulator) inspectReproZip(this js.Value, args []js.Value) any {
	if len(args) == 0 || args[0].Length() == 0 {
		return "<no repro zip bytes>\n"
	}
	buf := make([]byte, args[0].Length())
	js.CopyBytesToGo(buf, args[0])
	insp, _ := analyze.InspectMinimalReproZipBytes(buf)
	return analyze.InspectMinimalReproZipString(insp)
}

func (e *Emulator) inspectReproZipJSON(this js.Value, args []js.Value) any {
	if len(args) == 0 || args[0].Length() == 0 {
		return `{}`
	}
	buf := make([]byte, args[0].Length())
	js.CopyBytesToGo(buf, args[0])
	insp, _ := analyze.InspectMinimalReproZipBytes(buf)
	return analyze.InspectMinimalReproZipJSON(insp)
}

func (e *Emulator) bundleTrendChartJSON(this js.Value, args []js.Value) any {
	items := []analyze.NamedDiagnosticBundle{}
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		base, raw, err := analyze.ParseDiagnosticBundleText(args[0].String())
		if err == nil {
			items = append(items, analyze.NamedDiagnosticBundle{Name: "baseline", Bundle: base, Raw: raw})
		}
	}
	cur := e.currentDiagnosticBundle(e.traceFilter, nil)
	items = append(items, analyze.NamedDiagnosticBundle{Name: "current", Bundle: cur, Raw: analyze.DiagnosticBundleJSON(cur)})
	return analyze.BundleTrendChartDataJSON(analyze.BuildBundleTrendChartData(analyze.BuildBundleTrendReport(items)))
}

func (e *Emulator) bundleTrendCSV(this js.Value, args []js.Value) any {
	items := []analyze.NamedDiagnosticBundle{}
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		base, raw, err := analyze.ParseDiagnosticBundleText(args[0].String())
		if err == nil {
			items = append(items, analyze.NamedDiagnosticBundle{Name: "baseline", Bundle: base, Raw: raw})
		}
	}
	cur := e.currentDiagnosticBundle(e.traceFilter, nil)
	items = append(items, analyze.NamedDiagnosticBundle{Name: "current", Bundle: cur, Raw: analyze.DiagnosticBundleJSON(cur)})
	return analyze.BundleTrendCSV(analyze.BuildBundleTrendReport(items))
}

func (e *Emulator) ciPolicyTemplates(this js.Value, args []js.Value) any {
	return analyze.CIGatePolicyTemplateListString()
}

func (e *Emulator) ciPolicyTemplatesJSON(this js.Value, args []js.Value) any {
	return analyze.CIGatePolicyTemplatesJSON()
}

func (e *Emulator) ciPolicyTemplateJSON(this js.Value, args []js.Value) any {
	name := "default"
	if len(args) > 0 && args[0].Type() == js.TypeString && strings.TrimSpace(args[0].String()) != "" {
		name = args[0].String()
	}
	return analyze.CIGatePolicyTemplateJSON(name)
}

func presetsFromStringArg(args []js.Value, idx int) []string {
	var presets []string
	if len(args) > idx && args[idx].Type() == js.TypeString {
		for _, p := range strings.Split(args[idx].String(), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				presets = append(presets, p)
			}
		}
	}
	return presets
}

func (e *Emulator) exportDiagnosticsJSON(this js.Value, args []js.Value) any {
	b, err := json.MarshalIndent(e.snapshot(), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *Emulator) captureSnapshot(this js.Value, args []js.Value) any {
	e.lastSnapshot = e.snapshot()
	return "diagnostic snapshot captured"
}

func flattenSnapshot(prefix string, v any, out map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, v2 := range x {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenSnapshot(key, v2, out)
		}
	case map[uint64]string:
		for k, v2 := range x {
			key := fmt.Sprintf("%s.%d", prefix, k)
			out[key] = v2
		}
	default:
		out[prefix] = fmt.Sprint(x)
	}
}

func (e *Emulator) diffSnapshot(this js.Value, args []js.Value) any {
	if e.lastSnapshot == nil {
		return "<no captured snapshot>\n"
	}
	before := map[string]string{}
	after := map[string]string{}
	flattenSnapshot("", e.lastSnapshot, before)
	flattenSnapshot("", e.snapshot(), after)
	keys := make(map[string]bool, len(before)+len(after))
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		if before[k] != after[k] {
			fmt.Fprintf(&b, "%s: %s -> %s\n", k, before[k], after[k])
		}
	}
	if b.Len() == 0 {
		return "<no snapshot differences>\n"
	}
	return b.String()
}

func (e *Emulator) setRNGSeed(this js.Value, args []js.Value) any {
	if e.rng == nil {
		return "virtio-rng not present"
	}
	var seed uint64
	if len(args) > 0 && args[0].Type() == js.TypeString && args[0].String() != "" {
		if v, err := strconv.ParseUint(strings.TrimPrefix(args[0].String(), "0x"), 16, 64); err == nil {
			seed = v
		} else {
			return err.Error()
		}
	}
	e.rng.SetSeed(seed)
	return fmt.Sprintf("virtio-rng seed=%#x", seed)
}

func (e *Emulator) export(name string, fn func(js.Value, []js.Value) any) {
	f := js.FuncOf(fn)
	e.funcs = append(e.funcs, f)
	js.Global().Set(name, f)
}

func main() {
	e := newEmulator()
	e.export("rvwasmLoadFirmware", e.loadFirmware)
	e.export("rvwasmLoadPayload", e.loadPayload)
	e.export("rvwasmLoadDisk", e.loadDisk)
	e.export("rvwasmExportDisk", e.exportDisk)
	e.export("rvwasmLoadInitrd", e.loadInitrd)
	e.export("rvwasmSetBootArgs", e.setBootArgs)
	e.export("rvwasmLoadDTB", e.loadDTB)
	e.export("rvwasmReset", e.reset)
	e.export("rvwasmRun", e.run)
	e.export("rvwasmPause", e.pause)
	e.export("rvwasmStep", e.step)
	e.export("rvwasmStatus", e.status)
	e.export("rvwasmRegs", e.regs)
	e.export("rvwasmCSRs", e.csrs)
	e.export("rvwasmDiagnostics", e.diagnostics)
	e.export("rvwasmLoadSymbols", e.loadSymbols)
	e.export("rvwasmSymbolsAround", e.symbolsAround)
	e.export("rvwasmSymbolSearch", e.symbolSearch)
	e.export("rvwasmAnalyzeLog", e.analyzeLog)
	e.export("rvwasmSetSBIShim", e.setSBIShim)
	e.export("rvwasmSetHartCount", e.setHartCount)
	e.export("rvwasmSetActiveHart", e.setActiveHart)
	e.export("rvwasmInjectNetHex", e.injectNetHex)
	e.export("rvwasmNetTxLog", e.netTxLog)
	e.export("rvwasmExportTrace", e.exportTrace)
	e.export("rvwasmExportTraceJSON", e.exportTraceJSON)
	e.export("rvwasmExportTraceCSV", e.exportTraceCSV)
	e.export("rvwasmExportDiagnosticsJSON", e.exportDiagnosticsJSON)
	e.export("rvwasmArtifactManifest", e.artifactManifest)
	e.export("rvwasmArtifactManifestJSON", e.artifactManifestJSON)
	e.export("rvwasmArtifactManifestDiff", e.artifactManifestDiff)
	e.export("rvwasmArtifactManifestDiffJSON", e.artifactManifestDiffJSON)
	e.export("rvwasmAutoBreakpointSuggestions", e.autoBreakpointSuggestions)
	e.export("rvwasmAutoBreakpointSuggestionsJSON", e.autoBreakpointSuggestionsJSON)
	e.export("rvwasmDiagnosticBundleJSON", e.diagnosticBundleJSON)
	e.export("rvwasmDiagnosticBundleCompressedJSON", e.diagnosticBundleCompressedJSON)
	e.export("rvwasmDecodeDiagnosticBundle", e.decodeDiagnosticBundle)
	e.export("rvwasmCompareDiagnosticBundle", e.compareDiagnosticBundleBaseline)
	e.export("rvwasmCompareDiagnosticBundleJSON", e.compareDiagnosticBundleBaselineJSON)
	e.export("rvwasmProvenanceReport", e.provenanceReport)
	e.export("rvwasmProvenanceReportJSON", e.provenanceReportJSON)
	e.export("rvwasmRegressionHandoffMarkdown", e.regressionHandoffMarkdown)
	e.export("rvwasmReproductionPlan", e.reproductionPlan)
	e.export("rvwasmReproductionPlanJSON", e.reproductionPlanJSON)
	e.export("rvwasmReproductionPlanMarkdown", e.reproductionPlanMarkdown)
	e.export("rvwasmLogSignature", e.logSignature)
	e.export("rvwasmLogSignatureJSON", e.logSignatureJSON)
	e.export("rvwasmCompareLogSignature", e.compareLogSignature)
	e.export("rvwasmCompareLogSignatureJSON", e.compareLogSignatureJSON)
	e.export("rvwasmAutoBreakpointApplyReport", e.autoBreakpointApplyReport)
	e.export("rvwasmAutoBreakpointApplyReportJSON", e.autoBreakpointApplyReportJSON)
	e.export("rvwasmHeadlessSmokeScript", e.headlessSmokeScript)
	e.export("rvwasmHeadlessRunnerSpec", e.headlessRunnerSpec)
	e.export("rvwasmHeadlessRunnerSpecJSON", e.headlessRunnerSpecJSON)
	e.export("rvwasmBundleIntegrity", e.bundleIntegrity)
	e.export("rvwasmBundleIntegrityJSON", e.bundleIntegrityJSON)
	e.export("rvwasmReproValidation", e.reproValidation)
	e.export("rvwasmReproValidationJSON", e.reproValidationJSON)
	e.export("rvwasmCISummary", e.ciSummary)
	e.export("rvwasmCISummaryJSON", e.ciSummaryJSON)
	e.export("rvwasmCIGateReport", e.ciGateReport)
	e.export("rvwasmCIGateReportJSON", e.ciGateReportJSON)
	e.export("rvwasmCIActionChecklist", e.ciActionChecklist)
	e.export("rvwasmCIActionChecklistJSON", e.ciActionChecklistJSON)
	e.export("rvwasmGitHubActionsMatrixWorkflow", e.githubActionsMatrixWorkflow)
	e.export("rvwasmInspectReproZip", e.inspectReproZip)
	e.export("rvwasmInspectReproZipJSON", e.inspectReproZipJSON)
	e.export("rvwasmBundleTrendChartJSON", e.bundleTrendChartJSON)
	e.export("rvwasmBundleTrendCSV", e.bundleTrendCSV)
	e.export("rvwasmCIPolicyTemplates", e.ciPolicyTemplates)
	e.export("rvwasmCIPolicyTemplatesJSON", e.ciPolicyTemplatesJSON)
	e.export("rvwasmCIPolicyTemplateJSON", e.ciPolicyTemplateJSON)
	e.export("rvwasmApplyAutoBreakpointSuggestions", e.applyAutoBreakpointSuggestions)
	e.export("rvwasmSetRNGSeed", e.setRNGSeed)
	e.export("rvwasmPoke", e.poke)
	e.export("rvwasmDumpMemory", e.dumpMemory)
	e.export("rvwasmAddBreakpoint", e.addBreakpoint)
	e.export("rvwasmClearBreakpoints", e.clearBreakpoints)
	e.export("rvwasmBreakpoints", e.breakpointsString)
	e.export("rvwasmAddWatchpoint", e.addWatchpoint)
	e.export("rvwasmClearWatchpoints", e.clearWatchpoints)
	e.export("rvwasmWatchpoints", e.watchpointsString)
	e.export("rvwasmAddReadWatchpoint", e.addReadWatchpoint)
	e.export("rvwasmClearReadWatchpoints", e.clearReadWatchpoints)
	e.export("rvwasmReadWatchpoints", e.readWatchpointsString)
	e.export("rvwasmWatchpointHits", e.watchpointHits)
	e.export("rvwasmWatchpointHitsJSON", e.watchpointHitsJSON)
	e.export("rvwasmClearWatchpointHits", e.clearWatchpointHits)
	e.export("rvwasmSetTraceFilter", e.setTraceFilter)
	e.export("rvwasmSetTraceDRAMAccess", e.setTraceDRAMAccess)
	e.export("rvwasmFramebufferInfo", e.framebufferInfo)
	e.export("rvwasmFramebufferRGBA", e.framebufferRGBA)
	e.export("rvwasmFramebufferPNG", e.framebufferPNG)
	e.export("rvwasmInjectInputEvent", e.injectInputEvent)
	e.export("rvwasmInjectInputKey", e.injectInputKey)
	e.export("rvwasmClearFramebuffer", e.clearFramebuffer)
	e.export("rvwasmInput", e.input)
	e.export("rvwasmTrace", e.trace)
	e.export("rvwasmSetTrace", e.setTrace)
	e.export("rvwasmSetCSRTrace", e.setCSRTrace)
	e.export("rvwasmCSRTrace", e.csrTrace)
	e.export("rvwasmCSRSummary", e.csrSummary)
	e.export("rvwasmClearCSRTrace", e.clearCSRTrace)
	e.export("rvwasmSetProfile", e.setProfile)
	e.export("rvwasmClearProfile", e.clearProfile)
	e.export("rvwasmPCProfile", e.pcProfile)
	e.export("rvwasmTraceCompact", e.traceCompact)
	e.export("rvwasmTraceAnnotated", e.traceAnnotated)
	e.export("rvwasmAccessHistogram", e.accessHistogram)
	e.export("rvwasmClearAccessHistogram", e.clearAccessHistogram)
	e.export("rvwasmAccessTimeline", e.accessTimeline)
	e.export("rvwasmAccessTimelineCompact", e.accessTimelineCompact)
	e.export("rvwasmClearAccessTimeline", e.clearAccessTimeline)
	e.export("rvwasmCaptureSnapshot", e.captureSnapshot)
	e.export("rvwasmDiffSnapshot", e.diffSnapshot)
	e.export("rvwasmRunSmokePreset", e.runSmokePreset)
	e.export("rvwasmRunSmokeMatrix", e.runSmokeMatrix)
	e.export("rvwasmRunSmokeMatrixJSON", e.runSmokeMatrixJSON)
	e.export("rvwasmSmokeMatrixMarkdown", e.smokeMatrixMarkdown)
	e.export("rvwasmSmokeMatrixHTML", e.smokeMatrixHTML)
	e.export("rvwasmCompareSmokeMatrix", e.compareSmokeMatrix)
	e.export("rvwasmCompareSmokeMatrixJSON", e.compareSmokeMatrixJSON)
	e.export("rvwasmSmokeFailureClusters", e.smokeFailureClusters)
	e.export("rvwasmSmokeFailureClustersJSON", e.smokeFailureClustersJSON)
	e.export("rvwasmBootPhase", e.bootPhase)
	e.export("rvwasmDmesg", e.dmesg)
	e.export("rvwasmMMIODecodedTimeline", e.mmioDecodedTimeline)
	e.export("rvwasmBootTimeline", e.bootTimeline)
	e.export("rvwasmDeviceProbe", e.deviceProbe)
	e.export("rvwasmVirtqueueInspect", e.virtqueueInspect)
	e.export("rvwasmVirtqueueChains", e.virtqueueChains)
	e.export("rvwasmVirtqueueGraphDOT", e.virtqueueGraphDOT)
	e.export("rvwasmGuestMemoryScan", e.guestMemoryScan)
	e.export("rvwasmGuestMemoryIndex", e.guestMemoryIndex)
	e.export("rvwasmGuestMemoryIndexJSON", e.guestMemoryIndexJSON)
	e.export("rvwasmInitcallClassifier", e.initcallClassifier)
	e.export("rvwasmInitcallTimeline", e.initcallTimeline)
	e.export("rvwasmDwarfLines", e.dwarfLinesAround)
	e.export("rvwasmDwarfLineSummary", e.dwarfLineSummary)
	e.export("rvwasmPanicSummary", e.panicSummary)
	e.export("rvwasmBootAnalysisJSON", e.bootAnalysisJSON)
	e.export("rvwasmTraceReplayReport", e.traceReplayReport)
	e.export("rvwasmTraceCompare", e.traceCompare)
	e.export("rvwasmBootRegressionReport", e.bootRegressionReport)
	e.export("rvwasmBootRegressionJSON", e.bootRegressionJSON)
	e.export("rvwasmBootRegressionMarkdown", e.bootRegressionMarkdown)
	e.export("rvwasmBootRegressionHTML", e.bootRegressionHTML)
	e.export("rvwasmVirtqueueSnapshot", e.virtqueueSnapshot)
	e.export("rvwasmVirtqueueAnomalies", e.virtqueueAnomalies)
	e.export("rvwasmVirtqueueAnomaliesJSON", e.virtqueueAnomaliesJSON)
	e.export("rvwasmVirtqueueAnomalyHints", e.virtqueueAnomalyHints)
	e.export("rvwasmVirtqueueAnomalyHintsJSON", e.virtqueueAnomalyHintsJSON)
	e.export("rvwasmDiagnosticQuery", e.diagnosticQuery)
	e.export("rvwasmDiagnosticQueryJSON", e.diagnosticQueryJSON)
	e.export("rvwasmDiagnosticBookmarks", e.diagnosticBookmarks)
	e.export("rvwasmDiagnosticBookmarksJSON", e.diagnosticBookmarksJSON)
	e.export("rvwasmShareReportMarkdown", e.shareReportMarkdown)
	e.export("rvwasmShareReportJSON", e.shareReportJSON)
	e.export("rvwasmShareReportHTML", e.shareReportHTML)
	e.export("rvwasmTriageDashboard", e.triageDashboard)
	e.export("rvwasmTriageDashboardJSON", e.triageDashboardJSON)
	e.export("rvwasmStopCauseRanking", e.stopCauseRanking)
	e.export("rvwasmStopCauseRankingJSON", e.stopCauseRankingJSON)
	e.export("rvwasmStopCauseEvidence", e.stopCauseEvidence)
	e.export("rvwasmStopCauseEvidenceJSON", e.stopCauseEvidenceJSON)
	e.export("rvwasmStopCauseChecklist", e.stopCauseChecklist)
	e.export("rvwasmStopCauseChecklistJSON", e.stopCauseChecklistJSON)
	e.export("rvwasmTriageDashboardDiff", e.triageDashboardDiff)
	e.export("rvwasmTriageDashboardDiffJSON", e.triageDashboardDiffJSON)
	e.export("rvwasmMemoryRangeDump", e.memoryRangeDump)
	e.export("rvwasmMemoryRangeDumpJSON", e.memoryRangeDumpJSON)
	e.export("rvwasmDiagnosticPresetBaselineJSON", e.diagnosticPresetBaselineJSON)
	e.export("rvwasmCompareDiagnosticPresetBaseline", e.compareDiagnosticPresetBaseline)
	e.export("rvwasmCompareDiagnosticPresetBaselineJSON", e.compareDiagnosticPresetBaselineJSON)
	e.export("rvwasmMemoryObjectDump", e.memoryObjectDump)
	e.export("rvwasmMemoryObjectDumpJSON", e.memoryObjectDumpJSON)
	e.export("rvwasmShareReportRedactedMarkdown", e.shareReportRedactedMarkdown)
	e.export("rvwasmShareReportRedactedJSON", e.shareReportRedactedJSON)
	e.export("rvwasmShareReportRedactedHTML", e.shareReportRedactedHTML)
	e.export("rvwasmRedactionOptions", e.redactionOptions)
	e.export("rvwasmDiagnosticQueryPresets", e.diagnosticQueryPresets)
	e.export("rvwasmDiagnosticQueryPresetsJSON", e.diagnosticQueryPresetsJSON)
	e.export("rvwasmVirtqueueAnomalyTriage", e.virtqueueAnomalyTriage)
	e.export("rvwasmVirtqueueAnomalyTriageJSON", e.virtqueueAnomalyTriageJSON)
	e.export("rvwasmMemoryIndexSearch", e.memoryIndexSearch)
	e.export("rvwasmMemoryJumpHints", e.memoryJumpHints)
	e.export("rvwasmMemoryJumpHintsJSON", e.memoryJumpHintsJSON)
	e.export("rvwasmCaptureMemoryScan", e.captureMemoryScan)
	e.export("rvwasmDiffMemoryScan", e.diffMemoryScan)
	e.export("rvwasmDwarfSourceContext", e.dwarfSourceContext)
	js.Global().Set("rvwasmReady", js.ValueOf(true))
	e.refreshBootRegs()
	e.logf("rvwasm ready: load OpenSBI fw_payload.bin/fw_jump.bin/fw_dynamic.bin; optional payload/disk/net supported\n")
	select {}
}
