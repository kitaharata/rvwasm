# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](README.md) | [Español](docs/README-ES.md) | [Français](docs/README-FR.md) | [Português](docs/README-PT.md) | [Deutsch](docs/README-DE.md) | [Italiano](docs/README-IT.md) | [简体中文](docs/README-ZH-CN.md) | [繁體中文](docs/README-ZH-TW.md) | [日本語](docs/README-JA.md) | [한국어](docs/README-KO.md)

## Overview

An RV64IMAC emulator running on Go 1.23.2 `GOOS=js GOARCH=wasm`. The default is a single-hart, but cooperative scheduling of 1 to 8 harts is available from the UI. You can load OpenSBI 1.8.1 `fw_payload.bin` / `fw_jump.bin` / `fw_dynamic.bin` / ELF from the browser UI to confirm booting.

[![OpenSBI fw_payload boot on rvwasm](docs/images/fw_payload.png)](https://kitaharata.github.io/rvwasm/)

OpenSBI 1.8.1 `fw_payload.bin` booting on rvwasm and entering the next-stage S-mode payload.

## Implemented Features

- RV64I base instructions
- M extension
- Minimal implementation of A extension LR/SC/AMO
- Common integer instructions of C extension
- Zicsr / Zifencei equivalents
- Minimal implementation of M/S/U privilege mode CSR/trap/mret/sret
  - Corrects synchronous exception `mepc` / `sepc` to the faulting instruction PC
  - Corrects faulting load/CSR write so as not to corrupt rd and stops the retire counter from advancing
  - CSR existence check, suppression of read-only CSR write side effects, basic reflection of `mcounteren` / `scounteren`
  - Added `senvcfg` / state-enable CSR stubs for Linux probing
  - Basic reflection of `TVM` / `TW` / `TSR` and `MPRV` clearing
- Sv39 MMU
  - `satp` mode Bare / Sv39
  - 3-level page table walk
  - 4 KiB / 2 MiB / 1 GiB leaves
  - Basic reflection of `SUM` / `MXR` / `MPRV`
  - page fault exception
  - Automatic update of PTE `A` / `D` bits
- UART 16550-style MMIO (`0x10000000`)
  - Output from the guest
  - Input injection from the browser UI
  - Receive interrupt
- CLINT-style mtime/mtimecmp/msip (`0x02000000`)
  - Per-hart MSIP / MTIMECMP routing for multi-hart
- PLIC-style interrupt controller (`0x0c000000`)
  - priority / pending / enable / threshold
  - claim / complete
  - M/S context per hart
- PMP enforcement
  - TOR / NA4 / NAPOT
  - R/W/X permissions
  - M-mode restrictions via locked entries
- OpenSBI `fw_dynamic` boot info
  - Dynamic info is placed at `0x87dff000`
  - Dynamic info pointer is set to `a2`
  - S-mode payload / kernel can be separately loaded from the UI
- virtio-mmio block device (`0x10001000`)
  - Modern virtio 1.0 style MMIO registers
  - Minimal support for split virtqueue read/write/flush/get-id
  - `FEATURES_OK` negotiation and `VIRTIO_F_VERSION_1` verification
  - Queue reset, ignoring notify before `DRIVER_OK`, basic reflection of `NO_INTERRUPT` flag
  - Handling of `VIRTIO_RING_F_INDIRECT_DESC` and indirect descriptor tables
  - Interrupt suppression via `VIRTIO_RING_F_EVENT_IDX` used event
  - Disk images can be loaded from the UI
  - Disk images modified by the guest can be downloaded from the UI
- virtio-mmio console device (`0x10002000`)
  - Minimal console with device ID 3
  - Queue 0 receive / Queue 1 transmit
  - Minimal support for `VIRTIO_CONSOLE_F_SIZE`, indirect descriptors, and event indices
  - Inject UI input to both UART and virtio-console
- virtio-mmio net device (`0x10003000`)
  - Minimal debug virtio-net with device ID 1
  - Queue 0 receive / Queue 1 transmit
  - Minimal support for `VIRTIO_NET_F_MAC` / `VIRTIO_NET_F_STATUS` / indirect descriptors / event indices
  - Inject Ethernet frame hex into RX from the UI
  - Display Ethernet frames sent by the guest as TX logs
- virtio-mmio rng device (`0x10004000`)
  - Minimal entropy source with device ID 4
  - Minimal support for split virtqueues, indirect descriptors, and event indices
  - Deterministic seed can be set from the UI
- virtio-mmio input device (`0x10005000`)
  - Minimal debug keyboard/input device with device ID 18
  - Minimal support for event queue / status queue, indirect descriptors, and event indices
  - Key events / raw input events can be injected from the UI
- virtio-mmio gpu device (`0x10006000`)
  - Minimal 2D virtio-gpu foundation for debugging with device ID 16
  - Minimal support for control / cursor queues, indirect descriptors, and event indices
  - Basic responses for `GET_DISPLAY_INFO` / `RESOURCE_CREATE_2D` / `SET_SCANOUT` / `FLUSH` etc.
  - Useful for observing Linux virtio-gpu probes and initial modeset commands
- initrd / initramfs passing
  - Default load address: `0x84000000`
  - Reflected in `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end` of the automatically generated DTB
- bootargs editing
  - Default: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - Presets for UART / virtio-console / initramfs / verbose debugging
  - Can be set from the UI and reflected in the automatically generated DTB
- Execution trace ring buffer
  - PC / instructions / traps / last trap cause/tval can be viewed in the UI
  - Text / JSON / CSV exports of CSR dumps and whole hart trace snapshots are available from the UI
  - Diagnostics displaying the last ECALL/SBI arguments, SBI BASE/TIME/IPI/RFENCE/HSM/SRST/legacy counters, traps, and virtio queue states at a glance
  - JSON export of Diagnostics / device states
  - Loads ELF / System.map symbols, displays symbols around the halted PC, name search, and automatic PC symbol resolution within panic/oops logs
  - Arbitrary SBI shim for directly testing small S-mode payloads without OpenSBI
    - Minimal short-circuit of BASE / TIME / IPI / RFENCE / HSM / SRST
    - Debug path to boot target hart's S-mode entry via HSM `hart_start`
    - Disabled by default. Not used in the normal path for running OpenSBI
  - Arbitrary physical memory ranges can be dumped from the UI
  - PC breakpoints, physical read/write watchpoints, and trace filters can be set from the UI
  - Breakpoints can specify hit counts, mode conditions, and hart conditions
  - Trace displays simplified decode mnemonics along with raw instructions
  - Breakpoint / watchpoint hits record the stop reason in status / diagnostics / trace exports
  - Collects MMIO/DRAM access histograms, allowing you to check biases in device probes and queue activities via Diagnostics / JSON
  - Saves MMIO/DRAM access timelines to the ring buffer, allowing you to check the time series of probes in raw / compact views
  - The MMIO access timeline adds register decoder names for virtio-mmio / UART / CLINT / PLIC, allowing observation in units like `QueueNotify` / `Status` / `LSR`
  - Optionally enables CSR access trace to display guest CSR read/write tails and per-CSR read/write summaries in Diagnostics / trace exports
  - Optionally enables PC hot-spot profile to view frequently executed PCs with symbols before halting
  - Diagnostic snapshot capture / diff lets you check the differences in hart/device/CSR/MMIO states before and after execution on the UI
  - Folds consecutive identical instructions, traps, and ECALL logs in the compact trace view
  - Smoke runner per boot preset can automatically execute a specified number of hart-steps of currently loaded firmware/payload and retrieve JSON results
  - Boot phase analyzer can summarize OpenSBI / Linux / panic / virtio activities / traps / PC symbols together
  - Boot timeline can display console markers and MMIO probes / statuses / QueueNotifies / PLIC claims integrated into a time series
  - Device probe analyzer can aggregate virtio/UART/PLIC/CLINT read/writes, identity registers, status negotiations, and queue notifies
  - Virtqueue inspector can display the latest states of QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify by device/queue
  - Descriptor chain visualizer traces head descriptors from the avail ring and displays NEXT / WRITE / INDIRECT descriptors along with a small buffer preview
  - Descriptor chain graph export can save and visualize virtqueue chains as Graphviz DOTs
  - Guest physical memory scanner can detect areas in DRAM resembling ELF / FDT / gzip / xz / zstd / squashfs / cpio / ext magics / OpenSBI / Linux version / BusyBox / kernel cmdline
  - Initcall / driver probe classifier can categorize Linux console log lines related to initcalls, probes, virtio, storage, consoles, networks, and graphics
  - Initcall timeline can display classified initcall / driver probe lines in time-series groups
  - Reads DWARF line tables from ELFs with symbols, displaying file:line near the current PC, DWARF file summaries, and symbol+line annotations for trace PCs
  - Panic summary automatically extracts lines around panic/oops/fault in the console log and resolves addresses with loaded symbols
  - Boot analysis JSON can collectively export timelines / device probes / virtqueues / panic summaries
  - Trace replay report can summarize the number of steps/traps/ecalls/SBI shims, hot mnemonics, and trap causes in the trace
  - Trace baseline compare can compare the PC/instruction/trap differences between a previously saved trace and the current trace from the beginning
  - Trace baseline can be saved/loaded to/from the browser's localStorage
  - Boot regression report/JSON, as well as Markdown/HTML report exports, can bulk-save trace stats, boot events, device probes, virtqueues, memory objects, and initcall counts
  - Virtqueue snapshot can display queue setups and descriptor chains simultaneously
  - Virtqueue anomaly detector can detect missing ready queue addresses, descriptor loops, invalid indirect lengths, out-of-DRAM buffers, etc.
  - Virtqueue anomaly hints can display repair hints such as QueueNum / QueueDesc / QueueReady / descriptor alignment for each detection result
  - Integrated diagnostic query can cross-search consoles / traces / CSR traces / MMIO timelines / virtqueue anomalies / memory indices using the same query
  - Diagnostic query presets allow batch searching for panics, virtio negotiations, QueueReady/Notifies, satp/mstatus, traps, and rootfs
  - Share report MD/JSON/HTML allows sharing boot regressions, virtqueue hints/triage, memory indices, query presets, jump hints, and query hits in a self-contained format
  - Triage dashboard / stop-cause ranking can display panics, traps, page/access faults, virtqueue anomalies, and stalled device probes in candidate order
  - Stop-cause evidence displays ranking rationales, score breakdowns, recommended diagnostic queries, and next actions
  - Triage dashboard baseline can be saved to localStorage to compare status/phase/device/anomaly/memory counts with the current dashboard
  - Diagnostic preset baseline can be saved to localStorage to compare the difference with the current preset hit count
  - Redacted share report MD/JSON/HTML can output shareable reports with IPs/MACs/emails redacted
  - Redaction options JSON allows toggling the replacement of IPs/MACs/emails/long hex addresses from the UI
  - Memory object dump can verify hex + ASCII around memory index/search hits
  - Memory range dump can specify an arbitrary DRAM address and byte length to hex + ASCII dump / export JSON
  - Memory scan snapshot/diff can verify ELF/FDT/initrd/rootfs fragment candidates that increased/decreased before and after execution
  - Memory index can group nearby ELF/FDT/initrd/kernel/rootfs signatures by range to create an index
  - Extracts Linux `dmesg`-style logs from UART / virtio-console outputs and resolves panic/oops addresses with loaded symbols
- simple-framebuffer
  - Automatically adds `0x86000000`, 1024x768, `a8r8g8b8` to `/chosen/framebuffer@86000000` in the generated DTB
  - Draws the framebuffer to a UI Canvas, and RGBA raw dumps / PNGs can be downloaded
  - The 2D resource backing for virtio-gpu can be copied to the simple-framebuffer upon `TRANSFER_TO_HOST_2D` / `RESOURCE_FLUSH`
- DRAM `0x80000000`, 128 MiB
- Automatic minimal virt DTB generation with virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT, or load DTB from UI
  - Added `sifive,plic-1.0.0` / `sifive,clint0` compatible and virtio `dma-coherent`
  - Generates `cpu@N` and `interrupts-extended` according to the Hart count

## Usage

```bash
make serve
```

Open `http://localhost:8080` in your browser, select OpenSBI firmware, then click `Load firmware` → `Run`.

If you want to test virtio-console as the Linux console, you can change the bootargs to something like `console=hvc0 earlycon=sbi root=/dev/vda rw`. By default, it uses UART (`ttyS0`) as usual.

To analyze a halted PC, load a Linux `System.map` or an ELF with symbols using `Load symbols`, then use `Symbols @ PC` / `Diagnostics` / `Search symbols`. If the ELF with symbols contains DWARF line tables, you can also check file:line using `DWARF lines @ PC`. `DWARF file summary` shows the number of lines per file contained in the line table. If the firmware/payload is an ELF with symbols, it automatically imports the symbol table. `Annotated trace` annotates `pc=` in the trace with symbols/DWARF lines. `Download trace` saves a trace snapshot for all harts. You can also select JSON/CSV formats. The JSON trace includes symbol/source information if symbols exist. Enter strings like `trap`, `ecall`, `sbi-shim`, `pc=`, or `virtio` in `Trace filter` to narrow down the trace tail/export, access timeline, and compact view. `Compact trace` folds consecutive identical instructions, traps, and ECALLs. If you paste a panic/oops log and click `Analyze log symbols`, it resolves 64-bit PC-like addresses in the log using loaded symbols.

`Trace replay report` generates statistics for the current trace, and `Trace baseline compare` pastes a saved trace to compare PC/instruction/trap differences with the current trace from the beginning. `Save current trace as baseline` / `Load saved baseline` keeps the baseline in the browser's localStorage. `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` are regression check reports that consolidate the boot timeline, device probes, virtqueues, memory scanner, initcall classification, and trace statistics. You can also check the increase or decrease of ELF/FDT/initrd/rootfs candidates in guest memory by doing `Capture memory scan` → Run → `Diff memory scan`. `DWARF source context` displays symbols and DWARF file:lines around the current PC together.

`Boot phase` summarizes the current progress from console logs, MMIO histograms, traps, and symbol information. `Boot timeline` aligns console milestones and MMIO probes / statuses / QueueNotifies in a time series. `Device probe` aggregates register accesses and negotiations of virtio and others, and `Virtqueue inspect` displays queue setups and notify statuses by device/queue. `Descriptor chains` reads descriptor chains from the avail ring of the queue and displays indirect descriptors and buffer previews. `Descriptor DOT` / `Download DOT` outputs the same chain as Graphviz DOT. `Virtqueue anomalies` detects inconsistencies in queue setups and descriptor chains, and `Anomaly hints` displays the next check point for each inconsistency. `Integrated diagnostic query` cross-searches consoles / traces / CSR traces / MMIO timelines / virtqueue anomalies / memory indices using words like `virtio QueueReady`, `panic`, `satp`, `0x80200000`. `Share report MD/JSON/HTML` is a shareable bundle adding anomaly hints/triage, memory indices, memory jump hints, query presets, and query hits to the boot regression report. HTML can be saved as a self-contained file with embedded JSON. `Diagnostic query presets` batches searches related to panics, virtio statuses, QueueReady/QueueNotify, satp/mstatus, traps, and rootfs. `Save query` / `Load query` saves diagnostic queries to browser localStorage. `Memory scan` searches for ELF/FDT/initrd/kernel/rootfs fragment candidates in DRAM, and `Memory index` groups nearby signatures by range. `Memory search` searches memory indices using strings or `0x...` addresses, and `Memory jumps` displays useful jump destination candidates such as ELF/FDT/Linux/OpenSBI/cmdline/rootfs. `Initcall classifier` / `Initcall timeline` classifies and timestamps Linux initcall/driver probe style logs. `Panic summary` extracts lines around panic/oops/faults and resolves addresses if symbols are present. `Boot analysis JSON` saves these together. `Dmesg extract` extracts only Linux-style lines from UART / virtio-console outputs. `Decoded MMIO` displays the latest MMIO accesses with register names.

`Triage dashboard` combines stop-cause rankings, virtqueue anomaly severity, device probes, and query bookmarks into a single-screen text. `Stop-cause ranking` prioritizes kernel panics, oops, illegal instructions, page/access faults, virtqueue abnormalities, and stalled device probes from consoles/traces/statuses. `Stop-cause evidence` displays the rationale for the ranking, score breakdown, recommended queries, and next check points. You can compare differences in status/phase/device/anomaly/memory counts on the dashboard by doing `Save triage baseline` → Run → `Triage diff`. `Save preset baseline` → Run → `Compare preset baseline` allows you to check whether the hit count of preset queries like panic/virtio/satp/rootfs has increased or decreased since the last time. `Memory dump hits` hex/ASCII dumps around memory index hits using diagnostic queries or trace filters. `Memory range dump` specifies an arbitrary address/length to directly hex/ASCII dump the DRAM. `Redacted share MD/JSON/HTML` replaces emails / MACs / IPv4s with `<email>` / `<mac>` / `<ipv4>` before sharing. `Redaction options JSON` toggles `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex`.

`Smoke preset` resets the currently selected boot preset and executes only the specified steps. `Smoke matrix` sequentially executes a preset list like `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` and lists the execution steps, last phase, and stop-cause candidates for each preset.

PC breakpoints are added by entering the hex value of a physical/virtual PC into the `PC breakpoint` field under `Breakpoints / watchpoints`. `Run` / `Step 1k` stops at a breakpoint, and `Step` advances exactly 1 instruction even if the current PC is a breakpoint. Write watchpoints detect bus writes to a physical address range, and read watchpoints are a simple feature to detect bus reads. They are useful for checking MMIO probes, framebuffer writes, and references to specific structures. `Access timeline` / `Compact access` displays recent DRAM/MMIO accesses in a compressed time series. `PC profile on` aggregates hot PCs, and `Capture snapshot` → Run → `Diff snapshot` allows checking diagnostic differences before and after execution.

simple-framebuffer prepares a 1024x768x32bpp memory at `0x86000000` and places it as `simple-framebuffer` in `/chosen/framebuffer@86000000` of the automatically generated DTB. If simplefb is usable on the Linux side, it can be displayed on the Canvas with `Render framebuffer`.

virtio-net does not connect to a real network from the browser; it is a packet-level debug device. Enter Ethernet frame hex into `virtio-net debug` to inject it into RX, and frames sent by the guest to the TX queue can be verified in `Show TX frames`. To have it recognized on the Linux side, execute commands like `ip link set dev eth0 up` on the guest side as necessary.

virtio-rng is a verification device presenting a deterministic PRNG as a guest entropy source. To maintain reproducibility, the default seed is fixed and can be changed via `Set deterministic seed` in the UI.

virtio-gpu is a minimal device for observing Linux's virtio-gpu driver probes and 2D resource setup. Instead of real GPU acceleration, it tracks modeset / scanout / flush commands arriving in the control queue and outputs the states to Diagnostics. Because it also copies from resource backing memory to the simple-framebuffer, you can verify the result of the guest flushing a 2D resource via `Render framebuffer` / PNG export. `UPDATE_CURSOR` / `MOVE_CURSOR` in the cursor queue are also recorded as states.

`SBI shim on` is for debugging S-mode payloads directly without OpenSBI. Keep it disabled for normal experiments using `fw_dynamic.bin` / `fw_payload.bin`.

If you want to test Multi-hart, please set the `Hart count` before loading firmware. Since changing settings entails a machine reset, it assumes you will reload the firmware / payload / disk afterward. `View hart` allows switching the registers / CSRs / traces of the target hart being displayed.

When using `fw_dynamic.bin`, load the S-mode payload / kernel around `0x80200000` via `Load payload` as necessary. The emulator places dynamic info at `0x87dff000` and sets its address to `a2`.

For Linux experiments, you can use either of the following:

- `Load disk`: Pass a raw disk image like rootfs as virtio-blk. Default bootargs are `root=/dev/vda rw`.
- `Load initrd`: Place initramfs at `0x84000000` and reflect the initrd range in the generated DTB. Change bootargs to `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` etc., if necessary.

Example using pre-distributed RISC-V binaries of OpenSBI 1.8.1:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# Load fw_dynamic.bin / fw_payload.bin / fw_jump.bin from the extracted files via the browser
```

If building OpenSBI locally, prepare a RISC-V toolchain like `riscv64-unknown-elf-` and build with `PLATFORM=generic`.

### Development Commands

```bash
go test ./...
make wasm
make serve
```

## Note

This implementation incrementally features the functions needed to investigate OpenSBI initialization, S-mode payload transition, and Linux boot. For Linux boot, PMP, Sv39, virtio-blk, virtio-console, virtio-net, virtio-rng, virtio-input, virtio-gpu, initrd, simple-framebuffer, trap/CSR accuracy, CSR trace/summary, MMIO histogram/timeline/register decoders, boot phase/timeline analyzer, device probe analyzer, virtqueue inspector/descriptor chain visualizer/DOT export/snapshot/anomaly detector/anomaly hints/anomaly triage, guest memory scanner/index/diff/search/jump hints/dump helpers, integrated diagnostic query/query presets/preset baseline comparison, triage dashboard/stop-cause ranking, share report bundle/HTML/redaction, initcall classifier/timeline, DWARF line lookup/source context/trace annotation, panic summary, dmesg extractor, trace replay/compare, boot regression reports, PC profiling, snapshot diff, boot smoke runner/smoke matrix, triage baseline diff, stop-cause evidence, editable redaction, and memory range dump have been added. The main unimplemented or simplified parts include an accurate cycle/time model, AIA/IMSIC, real network connection via tap/WebSocket bridges, full virgl/DRM/GPU acceleration, strict WARL/WPRI behaviors for all CSRs, and true parallel execution using multiple workers. Multi-hart is a cooperative scheduling within a single wasm worker.

## Diagnostics and Regression Aids

Enhanced shareability for smoke matrices and diagnostic queries.

- `Smoke matrix MD/HTML` saves smoke matrix results as Markdown / self-contained HTML.
- `Save smoke baseline` → Run → `Compare smoke baseline` lets you check the differences in phase, execution steps, and top stop-causes for each preset.
- `Stop checklist` creates a checklist of specific action items to check next based on the stop-cause ranking.
- `CSR/MMIO bookmarks` extracts only the crucial CSR / MMIO / trace hits from the integrated diagnostic query results.
- `Watchpoint hits` displays the read/write watchpoint hit history in a time series. `Clear hit timeline` clears only the history.
- `Artifact manifest` lists the currently loaded firmware / payload / disk / initrd / symbols and the generated DTB / dynamic info ranges, entries, and SHA-256 hashes.

### Regression Handoff Aids

- `Manifest diff` / `Manifest diff JSON` compares the current boot artifact manifest with a baseline saved in localStorage and displays differences in bootargs, hart counts, load ranges, entries, ELF detection, and SHA-256 hashes.
- `Auto break/watch suggestions` generates candidates for PC breakpoints / read watchpoints / write watchpoints to set for the next run based on the stop-cause evidence, recent trace PCs, and watchpoint hit timelines.
- `Smoke clusters` / `Smoke clusters JSON` clusters the smoke matrix preset results by phase and top stop-cause, grouping presets with the same failure type.
- `Diagnostic bundle JSON` is a self-contained JSON grouping the manifest, triage dashboard, stop-causes, breakpoint suggestions, share bundle, and watchpoint hits.
- `Compressed bundle JSON` is the above diagnostic bundle converted to gzip+base64. Use this when you want to reduce the size before pasting into issues or chats.

### Handoff / Provenance Aids

- `Decode bundle` extracts a pasted `Diagnostic bundle JSON` or `Compressed bundle JSON`, or raw gzip+base64.
- `Bundle compare` / `Bundle compare JSON` compares a pasted past bundle with the current bundle and displays differences in triage phases, top stop-causes, manifests, artifact hashes, smoke clusters, watchpoint hits, and suggestion counts.
- `Provenance` / `Provenance JSON` summarizes the SHA-256 hashes of the manifest, trace, console, and diagnostic bundle, trace line counts, console byte counts, and top stop-causes. Can be used for verifying reproducibility or as evidence attached to issues.
- `Handoff MD` summarizes provenance, top stop-causes, auto break/watch suggestions, stop checklists, baseline diffs, and artifact manifests into Markdown.
- `Apply auto breaks` bulk-applies the top candidates of auto break/watch suggestions to the current emulator. A utility for quickly setting up stop positions or suspicious MMIO/DRAM ranges before re-running.

### Reproduction / Signature / Headless Handoff

- `Repro plan` / `Repro MD` / `Repro JSON`
  - Generates reproduction steps from diagnostic bundles, provenance, and artifact manifests.
  - Lists the roles, sizes, load ranges, and SHA-256 hashes of firmware / payload / initrd / disk / symbols as artifact pins.
  - Documents smoke presets, bootargs, hart counts, next_addr, and recommended break/watch conditions into steps.
- `Log signature` / `Log signature JSON`
  - Creates a lightweight summary from the SHA-256 hashes of traces / consoles / manifests, trace line counts, first/last PCs, console first/last lines, and hot tokens.
  - Allows you to compare "Is this the same log?" or "What changed?" without pasting raw traces.
- `Save log signature` / `Load log signature` / `Compare log signature`
  - Saves the log signature baseline to browser localStorage and compares it with the current signature.
  - Displays differences in trace hashes, console hashes, manifest hashes, line counts, last PCs, and last console lines.
- `Auto break verify`
  - Displays a confirmation summary before applying auto breakpoint/watchpoint suggestions.
  - Emits warnings for duplicate suggestions or suspicious PC ranges.
- `Headless smoke script`
  - Generates a shell script skeleton for CI/handoff from the current artifact manifest, bootargs, hart counts, smoke presets, and step counts.
  - Intended to fix artifact pins and preset matrices before adding browser harnesses like Playwright to the execution environment.

#### Headless / CI Aids

To make repro/signature handoff easier to handle in CI or issues, the following have been added.

- `Bundle integrity` / `Integrity JSON` checks the consistency between the diagnostic bundle and the artifact manifest, categorizing discrepancies in artifact roles, SHA-256 hashes, load ranges, suggestions, and smoke results as `error`, `warn`, or `info`.
- `Repro validation` / `Repro validation JSON` verifies whether the current reproduction plan matches the bundle's bootargs, hart counts, next_addr, artifact pins, top stop-causes, and log signatures.
- `CI summary` / `CI summary JSON` consolidates bundle integrity, trace/console signatures, smoke results, and stop-causes, outputting a summary that makes pass/warn/fail judgments easier in CI.
- `Headless runner spec` / `Runner spec JSON` generates presets, steps, artifact pins, and recommended commands for inspection with `go run ./cmd/rvsmoke ...`.
- Added `cmd/rvsmoke`. It can read diagnostic bundles / artifact manifests outside the browser and output artifact hashes, bundle integrity, CI summaries, and runner specs in text / JSON / Markdown.

Example:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

`rvsmoke` currently performs reproducibility inspections and CI summary generations for bundle/manifest and artifact hashes. CPU execution itself will continue to use the smoke matrix on the browser js/wasm side.

#### rvsmoke CI Gate / JUnit / SARIF

`cmd/rvsmoke` is a helper CLI for inspecting exported diagnostic bundles / manifests in CI. By materializing headless execution, it can output baseline bundle comparisons, CI gate policies, JUnit XMLs, SARIFs, and self-contained HTML reports.

Example:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -baseline previous-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -policy rvwasm-ci-policy.json \
  -junit rvwasm-junit.xml \
  -sarif rvwasm.sarif \
  -html rvwasm-ci.html \
  -out md > rvwasm-ci-summary.md
```

Example of policy JSON:

```json
{
  "name": "linux-boot-regression",
  "max_integrity_errors": 0,
  "max_integrity_warnings": 2,
  "max_virtqueue_anomalies": 0,
  "max_smoke_failures": 0,
  "min_trace_lines": 100,
  "require_artifacts": ["firmware", "payload"],
  "fail_on_top_cause_contains": ["panic", "oops", "illegal", "virtqueue"],
  "warn_on_missing_baseline": true,
  "treat_warnings_as_failures": false
}
```

`-out html` prints self-contained HTML to stdout, `-out junit` for JUnit XML, and `-out sarif` for SARIF JSON. If `-junit` / `-html` / `-sarif` are specified simultaneously, they save to their respective files in addition to the stdout format. CI gates normalize artifact manifests, trace/console signatures, baseline diffs, virtqueue anomalies, and smoke results into `pass`, `warn`, or `fail`.

#### rvsmoke Policy Templates / Bundle Trend Compare

To ease initial CI gate introduction and multiple regression comparisons, policy templates, action checklists, and bundle trend compares have been added to `rvsmoke` and the browser UI.

- `CI policy templates` / `Policy templates JSON` display built-in policies: `default`, `strict`, `linux-boot`, `artifact-only`, and `lenient`.
- `Policy template JSON` saves the specified template as a JSON ready to drop into CI.
- `CI gate` / `CI gate JSON` applies a policy template to the current browser state and displays pass/warn/fail gate checks.
- `CI checklist` / `CI checklist JSON` transforms gate failures, bundle integrity, and artifact diffs into actionable checklists.
- `rvsmoke -compare name=bundle.json` aligns multiple bundles chronologically and outputs trend reports showing changes in phases, top stop-causes, artifact hashes, and smoke clusters.

Example of policy template generation:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

Example of comparing multiple bundles:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

`-policy-template` serves as the default policy when `-policy` is not specified. If `-policy` is specified, the file's JSON takes precedence.

## rvsmoke CI Integration

Expanded CI/handoff aids for `rvsmoke`.

- `rvsmoke -print-github-actions linux-boot` can generate GitHub Actions workflow YAMLs.
- `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` can output workflows to files.
- `rvsmoke -policy-tree policy-tree.md` can save CI gates / bundle integrity / baseline drifts as cause trees.
- `rvsmoke -history history.txt` can save phase / stop-cause / artifact drift aggregations of multiple bundle trends.
- `rvsmoke -repro-zip rvwasm-minimal-repro.zip` can generate minimal reproduction packages containing READMEs, diagnostic bundles, manifests, runner specs, policies, CI summaries, and verification scripts.

Example:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -github-actions rvwasm-smoke.yml \
  -policy-tree policy-tree.md \
  -history history.txt \
  -repro-zip rvwasm-minimal-repro.zip \
  -out md > rvwasm-ci-summary.md
```

`-repro-zip` does not embed raw firmware/kernels/disks. It embeds SHA-256 pins and manifest ranges in the bundle, expecting artifacts to be verified by the recipient.

### CI Repro ZIP Inspection / Matrix Workflow Continuation

Added functions to `rvsmoke` and browser UI to inspect the handoff of minimal reproduction packages, and outputs for GitHub Actions matrices / trend visualization.

- `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` can inspect the ZIP generated by `-repro-zip` without extracting it. Verifies required files, unsafe paths, `diagnostic-bundle.json` / `manifest.json` matches, `ci-policy.json`, and `scripts/rvsmoke.sh`.
- `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` can generate GitHub Actions matrix workflow YAMLs per preset.
- `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` can output matrix workflows to files.
- `rvsmoke -trend-csv rvwasm-trend.csv` and `-trend-chart-json rvwasm-trend-chart.json` can save bundle trends into CSV / JSON for easy external graphing.
- Added `Minimal repro ZIP`, `Inspect repro ZIP`, `Repro ZIP JSON`, `Matrix workflow YAML`, `Trend chart JSON`, and `Trend CSV` to the Browser UI.

Example:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# Save inspection results as JSON
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# Convert trends of current bundle and previous bundle to CSV/JSON
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` can run standalone. If specified alongside `-bundle`, it includes ZIP inspection results in the normal CI summary and treats failures as CI summary failures.

### CI Matrix Aggregation / Checksum Manifest Continuation

Enhanced CI artifact handoffs for `rvsmoke`.

- `-repro-checksums rvwasm-repro-checksums.json` can save deterministic checksum manifests for files inside the ZIP based on `-inspect-repro-zip` results.
- By specifying multiple `-matrix-result name=rvsmoke-output.json`, you can aggregate `rvsmoke -out json` results from multiple presets / multiple jobs.
- `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` can save matrix results as text / JSON / self-contained HTML.
- `-trend-html rvwasm-trend.html` can save bundle trend reports as standalone HTMLs.

Example:

```bash
# Save contents of minimal reproduction ZIP and checksum manifest
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# Aggregate rvsmoke JSONs from multiple matrix jobs
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -matrix-result uart=artifacts/uart/rvsmoke.json \
  -matrix-result simplefb=artifacts/simplefb/rvsmoke.json \
  -matrix-summary rvwasm-matrix.txt \
  -matrix-summary-json rvwasm-matrix.json \
  -matrix-summary-html rvwasm-matrix.html \
  -trend-html rvwasm-trend.html \
  -out md > rvwasm-ci-summary.md
```

Matrix aggregates summarize CI statuses, gate failure/warning counts, artifact mismatches, and top stop-causes per job. It is a utility to easily view overall failure trends in the final aggregation job even when GitHub Actions matrix jobs are split.

#### CI / Release Handoff Aids

Enhanced CI artifact management and release handoffs for `rvsmoke`.

- `-artifact-index rvwasm-artifacts.json` summarizes the paths, bytes, and SHA-256 hashes of generated CI artifacts like JUnit / SARIF / HTML / trend / matrix / repro checksums.
- `-release-manifest rvwasm-release.json` bundles diagnostic bundles, log signatures, CI gates, matrix aggregates, flake reports, artifact indices, and repro checksum verifications into one handoff manifest.
- `-release-html rvwasm-release.html` outputs a self-contained HTML with navigation to Summary / Artifacts / Matrix / Checksums / JSON.
- `-verify-repro-checksums baseline-repro-checksums.json` compares the checksum manifest of the currently inspected minimal repro ZIP with a baseline to detect missing / changed / extra entries.
- `-matrix-flakes`, `-matrix-flakes-json`, and `-matrix-flakes-html` normalize multiple matrix results like `uart#1` / `uart#2` to detect if the same preset is flaking between pass/fail.

Example:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -verify-repro-checksums previous-repro-checksums.json \
  -matrix-result uart#1=artifacts/uart1/rvsmoke.json \
  -matrix-result uart#2=artifacts/uart2/rvsmoke.json \
  -matrix-flakes rvwasm-flakes.txt \
  -artifact-index rvwasm-artifacts.json \
  -release-manifest rvwasm-release.json \
  -release-html rvwasm-release.html \
  -out md > rvwasm-ci-summary.md
```

## Release Handoff and Verification

Added metadata outputs to `rvsmoke` for handing off CI results to other machines, other repositories, or reviewers.

### SBOM / Provenance Extension

#### SBOM-lite Dependency Inventory

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

This dependency list is intended to be in a small, deterministic format. It reads `go.mod` and records module paths, Go versions, direct `require` lines, `replace` targets, and artifact types included in the CI artifact index.

When running `rvsmoke` from another working directory, specify `-go-mod /path/to/go.mod`.

#### Provenance Attestation

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

The attestation is a JSON payload inspired by in-toto / SLSA. It is not a signature by itself, but because it has a stable SHA-256, it can be used as a target for external CI tooling to sign.

#### Release Handoff ZIP

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

The release handoff ZIP includes metadata only.

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

It does not embed firmware, kernels, initrds, or disk images. Large artifacts are kept as SHA-256 pins in the manifest.

#### Release Handoff ZIP Inspection

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

The inspector checks the ZIP without extracting it, for required files, dangerous paths, duplicate paths, JSON parsability, and basic consistency between releases / indices / SBOMs / attestations.

### Release Verification

In addition to creating release handoff ZIPs, verification-oriented outputs have been added.

- `-verify-attestation` / `-verify-attestation-text` confirm whether the deterministic provenance attestation hash, release materials, and CI artifact subjects match the generated release manifest, SBOM-lite inventory, and artifact index.
- `-sbom-baseline`, `-sbom-diff`, and `-sbom-diff-json` compare the current SBOM-lite dependency inventory with a saved baseline.
- `-compare-release-zip-inspection`, `-release-zip-compare`, and `-release-zip-compare-json` compare the currently inspected release handoff ZIP with past inspection JSONs.
- `-retention-manifest` / `-retention-text` generate a CI artifact retention manifest containing paths, kinds, bytes, SHA-256, retention days, expiry times, and reasons.
- `-release-verification-html` outputs HTML with navigation summarizing release statuses, attestation verifications, SBOM diffs, release ZIP comparisons, and retention information.

Example:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-zip rvwasm-release-handoff.zip \
  -verify-attestation rvwasm-attestation-verify.json \
  -sbom-baseline previous-sbom.json \
  -sbom-diff rvwasm-sbom-diff.txt \
  -retention-manifest rvwasm-retention.json \
  -release-verification-html rvwasm-release-verification.html \
  -out md > rvwasm-ci-summary.md
```

### Release Audit Gate

A final release audit layer has been added on top of release verification. It summarizes provenance attestation verifications, SBOM-lite diffs, release ZIP comparisons, artifact retention expiries, matrix flake statuses, and release manifest statuses into a single score and gate report.

Main flags:

- `-list-release-verify-policies` lists built-in release audit policies.
- `-print-release-verify-policy strict` outputs a policy JSON template.
- `-release-verify-template default|strict|lenient|archive` selects a built-in policy.
- `-release-verify-policy policy.json` loads a custom release audit policy.
- `-retention-audit` / `-retention-audit-json` writes out expiry and minimum-retention inspection results.
- `-release-score` / `-release-score-json` writes out a release verification score from 0 to 100.
- `-release-gate` / `-release-gate-json` writes out policy gate results.
- `-release-audit` / `-release-audit-json` / `-release-audit-html` writes out an integrated audit report.

Example:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-verify-template strict \
  -release-gate rvwasm-release-gate.txt \
  -release-score rvwasm-release-score.txt \
  -retention-audit rvwasm-retention-audit.txt \
  -release-audit-html rvwasm-release-audit.html \
  -out md > rvwasm-ci-summary.md
```

The strict policy treats non-passing release manifests, failed attestation / SBOM / ZIP checks, expired artifacts, and artifacts below the set minimum retention days as failures. The default policy is suitable for daily checks such as nightly handoffs, allowing warnings but failing the CI for clear verification failures.

#### Release Audit Diff / Waiver / TODO Handoff

The `rvsmoke` release-audit path supports comparing the current audit with a past audit, applying time-limited waivers to known issues, and generating checklists for un-waivered tasks.

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-audit-baseline previous-release-audit.json \
  -release-waivers release-waivers.json \
  -release-audit-diff rvwasm-release-audit-diff.txt \
  -release-waiver-report rvwasm-release-waivers.txt \
  -release-todo rvwasm-release-todo.md \
  -release-audit-nav-html rvwasm-release-audit.html \
  -out md > rvwasm-ci-summary.md
```

You can create a waiver template with the following command:

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

Waivers are used to handle known, temporary release-audit findings. Each rule has an ID, arbitrary kind / name / status / substring matchers, an owner, a reason, and an `expires_at` timestamp. Expired waivers are reported but are not used to suppress issues.

#### Release Decision / Evidence Bundle

Added final handoff aids to be used after executing release audits.

- `-waiver-calendar`, `-waiver-calendar-json`, and `-waiver-calendar-html` display the expiry, owner, match counts, and expired / expiring-soon states for each waiver.
- `-release-changelog` and `-release-changelog-json` summarize audit diffs, waiver states, TODO counts, and waiver expiry states as human-readable changelogs.
- `-final-decision` and `-final-decision-json` generate final `go`, `go-with-watch`, and `no-go` decisions containing blocking items and next actions.
- `-release-evidence-zip` writes out a small evidence bundle containing audits, waiver reports, TODO lists, waiver calendars, changelogs, and final decisions.
- `-inspect-release-evidence-zip` inspects evidence bundles without extracting them for required files, dangerous paths, duplicate entries, and JSON parsability.
- `-dry-run` calculates reports without writing optional output files.
- `-exit-code-mode never` outputs results even in cases where it would normally fail with a gate failure.

Example:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-waivers release-waivers.json \
  -waiver-calendar-html rvwasm-waivers.html \
  -release-changelog rvwasm-release-changelog.md \
  -final-decision rvwasm-final-decision.txt \
  -release-evidence-zip rvwasm-release-evidence.zip \
  -out md > rvwasm-ci-summary.md
```

Example of inspecting an evidence bundle in CI:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## License

This project is licensed under the BSD 2-Clause License. See the [LICENSE](LICENSE) file for details.

SPDX-License-Identifier: BSD-2-Clause
