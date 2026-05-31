# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## 개요

Go 1.23.2 `GOOS=js GOARCH=wasm` 에서 동작하는 RV64IMAC 에뮬레이터입니다. 기본은 single-hart이지만, UI에서 1〜8 hart의 협력적 스케줄링도 가능합니다. OpenSBI 1.8.1의 `fw_payload.bin` / `fw_jump.bin` / `fw_dynamic.bin` / ELF를 브라우저 UI에서 로드하여 부팅을 확인할 수 있습니다.

![rvwasm에서 OpenSBI fw_payload이 부팅된 화면](images/fw_payload.png)

OpenSBI 1.8.1 `fw_payload.bin` 이 rvwasm에서 부팅되고 다음 단계의 S-mode payload로 진입하는 예입니다.

## 구현 완료

- RV64I 기본 명령어
- M extension
- A extension의 LR/SC/AMO 최소 구현
- C extension의 일반적인 정수 명령어
- Zicsr / Zifencei 상당
- M/S/U privilege mode의 CSR/trap/mret/sret 최소 구현
  - 동기 예외의 `mepc` / `sepc` 를 faulting instruction PC로 보정
  - faulting load/CSR write가 rd를 파괴하지 않고, retire counter도 진행하지 않도록 보정
  - CSR 존재 확인, read-only CSR 쓰기 side effect 억제, `mcounteren` / `scounteren` 의 기본 반영
  - Linux측의 probing용으로 `senvcfg` / state-enable CSR stub을 추가
  - `TVM` / `TW` / `TSR` 과 `MPRV` 해제의 기본 반영
- Sv39 MMU
  - `satp` mode Bare / Sv39
  - 3단 페이지 테이블 walk
  - 4 KiB / 2 MiB / 1 GiB leaf
  - `SUM` / `MXR` / `MPRV` 의 기본 반영
  - page fault exception
  - PTE `A` / `D` bit의 자동 업데이트
- UART 16550풍 MMIO(`0x10000000`)
  - guest로부터의 출력
  - 브라우저 UI로부터의 입력 inject
  - receive interrupt
- CLINT풍 mtime/mtimecmp/msip(`0x02000000`)
  - multi-hart용의 per-hart MSIP / MTIMECMP routing
- PLIC풍 interrupt controller(`0x0c000000`)
  - priority / pending / enable / threshold
  - claim / complete
  - hart별 M/S context
- PMP enforcement
  - TOR / NA4 / NAPOT
  - R/W/X permission
  - locked entry에 의한 M-mode 제한
- OpenSBI `fw_dynamic` 용 boot info
  - dynamic info를 `0x87dff000` 에 배치
  - `a2` 에 dynamic info pointer를 설정
  - UI에서 S-mode payload / kernel을 별도 로드 가능
- virtio-mmio block device(`0x10001000`)
  - modern virtio 1.0 style MMIO register
  - split virtqueue의 read/write/flush/get-id 최소 대응
  - `FEATURES_OK` negotiation과 `VIRTIO_F_VERSION_1` 검증
  - queue reset, `DRIVER_OK` 전 notify의 무시, `NO_INTERRUPT` flag의 기본 반영
  - `VIRTIO_RING_F_INDIRECT_DESC` 와 indirect descriptor table의 처리
  - `VIRTIO_RING_F_EVENT_IDX` 의 used event에 의한 인터럽트 억제
  - UI에서 disk image를 로드 가능
  - guest가 수정한 disk image를 UI에서 다운로드 가능
- virtio-mmio console device(`0x10002000`)
  - device ID 3의 최소 console
  - queue 0 receive / queue 1 transmit
  - `VIRTIO_CONSOLE_F_SIZE` , indirect descriptor, event index의 최소 대응
  - UI 입력을 UART와 virtio-console 양쪽에 inject
- virtio-mmio net device(`0x10003000`)
  - device ID 1의 디버깅용 최소 virtio-net
  - queue 0 receive / queue 1 transmit
  - `VIRTIO_NET_F_MAC` / `VIRTIO_NET_F_STATUS` / indirect descriptor / event index의 최소 대응
  - UI에서 Ethernet frame hex를 RX inject
  - guest가 전송한 Ethernet frame을 TX log로 표시
- virtio-mmio rng device(`0x10004000`)
  - device ID 4의 최소 entropy source
  - split virtqueue, indirect descriptor, event index의 최소 대응
  - deterministic seed를 UI에서 설정 가능
- virtio-mmio input device(`0x10005000`)
  - device ID 18의 디버깅용 최소 keyboard/input device
  - event queue / status queue, indirect descriptor, event index의 최소 대응
  - UI에서 key event / raw input event를 inject 가능
- virtio-mmio gpu device(`0x10006000`)
  - device ID 16의 디버깅용 2D virtio-gpu foundation
  - control / cursor queue, indirect descriptor, event index의 최소 대응
  - `GET_DISPLAY_INFO` / `RESOURCE_CREATE_2D` / `SET_SCANOUT` / `FLUSH` 등의 기본 응답
  - Linux의 virtio-gpu probe와 초기 modeset command 관측용
- initrd / initramfs 전달
  - default load address: `0x84000000`
  - 자동 생성 DTB의 `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end` 에 반영
- bootargs 편집
  - default: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - UART / virtio-console / initramfs / verbose debug용의 preset
  - UI에서 설정하고, 자동 생성 DTB에 반영
- 실행 트레이스 ring buffer
  - PC / 명령어 / trap / 마지막 trap cause/tval을 UI에서 확인 가능
  - UI에서 CSR dump와 모든 hart trace snapshot의 text / JSON / CSV export가 가능
  - 마지막 ECALL/SBI 인수, SBI BASE/TIME/IPI/RFENCE/HSM/SRST/legacy counters, trap, virtio queue 상태를 한데 모아 표시하는 Diagnostics
  - Diagnostics / device state의 JSON export
  - ELF / System.map symbols를 읽어들여, 정지 PC 주변의 symbol 표시, 이름 검색, panic/oops 로그 내 PC의 자동 symbol 해석이 가능
  - OpenSBI가 없는 작은 S-mode payload를 직접 테스트하기 위한 임의 SBI shim
    - BASE / TIME / IPI / RFENCE / HSM / SRST의 최소 short-circuit
    - HSM `hart_start` 로 대상 hart의 S-mode 엔트리를 기동하는 디버깅용 경로
    - 기본값은 무효. OpenSBI를 구동하는 일반 경로에서는 사용하지 않음
  - UI에서 임의 물리 메모리 범위를 dump 가능
  - PC breakpoint, 물리 read/write watchpoint, trace filter를 UI에서 설정 가능
  - breakpoint는 hit count, mode 조건, hart 조건을 지정 가능
  - trace는 raw instruction에 더해 간이 decode mnemonic을 표시
  - breakpoint / watchpoint hit 시에는 stop reason을 status / diagnostics / trace export에 기록
  - MMIO/DRAM access histogram을 수집하여, device probe나 queue activity의 편향을 Diagnostics / JSON으로 확인 가능
  - MMIO/DRAM access timeline을 ring buffer에 저장하여, raw / compact 표시로 probe의 시계열을 확인 가능
  - MMIO access timeline에는 virtio-mmio / UART / CLINT / PLIC의 register decoder 이름을 부여하여, `QueueNotify` / `Status` / `LSR` 등의 단위로 확인 가능
  - CSR access trace를 임의로 활성화하여, guest의 CSR read/write tail과 CSR별 read/write summary를 Diagnostics / trace export에 표시 가능
  - PC hot-spot profile을 임의로 활성화하여, 정지 전에 실행 횟수가 많았던 PC를 symbol과 함께 확인 가능
  - diagnostic snapshot capture / diff를 통해, 실행 전후의 hart/device/CSR/MMIO 상태 차이를 UI에서 확인 가능
  - compact trace 표시로 동종 명령어・trap・ECALL의 연속 로그를 접기 가능
  - boot preset별 smoke runner로, 현재 로드된 firmware/payload를 지정된 hart-step만큼 자동 실행하고, 결과 JSON을 취득 가능
  - boot phase analyzer로 OpenSBI / Linux / panic / virtio activity / trap / PC symbol을 한데 모아 표시 가능
  - boot timeline으로 console marker와 MMIO probe / status / QueueNotify / PLIC claim을 시계열로 통합 표시 가능
  - device probe analyzer로 virtio/UART/PLIC/CLINT의 read/write, identity register, status negotiation, queue notify를 집계 가능
  - virtqueue inspector로 QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify의 최근 상태를 device/queue별로 표시 가능
  - descriptor chain visualizer로 avail ring에서 head descriptor를 따라가, NEXT / WRITE / INDIRECT descriptor와 작은 buffer preview를 표시 가능
  - descriptor chain graph export로 virtqueue chain을 Graphviz DOT으로 저장・시각화 가능
  - guest physical memory scanner로 DRAM 내의 ELF / FDT / gzip / xz / zstd / squashfs / cpio / ext magic / OpenSBI / Linux version / BusyBox / kernel cmdline으로 보이는 영역을 검출 가능
  - initcall / driver probe classifier로 Linux console log의 initcall, probe, virtio, storage, console, network, graphics 관련 줄을 분류 가능
  - initcall timeline으로 분류된 initcall / driver probe 줄을 시계열 그룹으로 표시 가능
  - symbol이 포함된 ELF의 DWARF line table을 읽어들여, 현재 PC 근처의 file:line, DWARF file summary, trace PC의 symbol+line 주석을 표시 가능
  - panic summary로 console log 내의 panic/oops/fault 주변 줄을 자동 추출하고, 로드된 symbols로 address를 해석 가능
  - boot analysis JSON으로 timeline / device probe / virtqueue / panic summary를 한꺼번에 export 가능
  - trace replay report로 trace의 step/trap/ecall/SBI shim 건수, hot mnemonic, trap cause를 요약 가능
  - trace baseline compare로 이전에 저장한 trace와 현재 trace의 PC/명령어/trap 차이를 처음부터 비교 가능
  - trace baseline을 브라우저 localStorage에 저장/로드 가능
  - boot regression report/JSON에 더해 Markdown/HTML report export로 trace stats, boot events, device probe, virtqueue, memory object, initcall counts를 일괄 저장 가능
  - virtqueue snapshot으로 queue setup과 descriptor chain을 동시에 표시 가능
  - virtqueue anomaly detector로 ready queue의 address 누락, descriptor loop, indirect 길이 오류, DRAM 외부 buffer 등을 검출 가능
  - virtqueue anomaly hints로 검출 결과마다 QueueNum / QueueDesc / QueueReady / descriptor alignment 등의 수정 힌트를 표시 가능
  - integrated diagnostic query로 console / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory index를 동일한 query로 교차 검색 가능
  - diagnostic query presets를 사용하여 panic, virtio negotiation, QueueReady/Notify, satp/mstatus, trap, rootfs 관련을 일괄 검색 가능
  - share report MD/JSON/HTML로 boot regression, virtqueue hints/triage, memory index, query presets, jump hints, query hits를 자체 완결 형식으로 공유 가능
  - triage dashboard / stop-cause ranking으로 panic, trap, page/access fault, virtqueue anomaly, device probe 정체를 후보 순으로 표시 가능
  - stop-cause evidence로 랭킹 근거, score breakdown, 권장 diagnostic query, 다음 조치를 표시 가능
  - triage dashboard baseline을 localStorage에 저장하여, 현재 dashboard와 status/phase/device/anomaly/memory counts를 비교 가능
  - diagnostic preset baseline을 localStorage에 저장하여, 현재의 preset hit 수와의 차이를 비교 가능
  - redacted share report MD/JSON/HTML로 IP/MAC/email을 가린 공유용 보고서를 출력 가능
  - redaction options JSON으로 IP/MAC/email/long hex address의 치환 여부를 UI에서 조정 가능
  - memory object dump로 memory index/search의 hit 주변을 hex + ASCII로 확인 가능
  - memory range dump로 임의의 DRAM address와 byte length를 지정하여 hex + ASCII dump / JSON export 가능
  - memory scan snapshot/diff로 실행 전후에 증감한 ELF/FDT/initrd/rootfs 단편 후보를 확인 가능
  - memory index로 인접한 ELF/FDT/initrd/kernel/rootfs signature를 범위별로 묶어 색인화 가능
  - UART / virtio-console 출력에서 Linux `dmesg` 풍 로그를 추출하고, 로드된 symbols로 panic/oops address를 해석 가능
- simple-framebuffer
  - `0x86000000` , 1024x768, `a8r8g8b8` 을 자동 생성 DTB의 `/chosen/framebuffer@86000000` 에 추가
  - UI의 Canvas에 framebuffer를 렌더링, RGBA raw dump와 PNG를 다운로드 가능
  - virtio-gpu의 2D resource backing을 `TRANSFER_TO_HOST_2D` / `RESOURCE_FLUSH` 시에 simple-framebuffer로 복사 가능
- DRAM `0x80000000` , 128 MiB
- virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT가 포함된 최소 virt DTB 자동 생성, 또는 UI에서 DTB 로드
  - `sifive,plic-1.0.0` / `sifive,clint0` compatible과 virtio `dma-coherent` 를 추가
  - Hart count에 맞추어 `cpu@N` 과 `interrupts-extended` 를 생성

## 사용법

```bash
make serve
```

브라우저에서 `http://localhost:8080` 을 열고, OpenSBI의 firmware를 선택한 후 `Load firmware` → `Run` .

virtio-console을 Linux console로 테스트할 경우에는, bootargs를 `console=hvc0 earlycon=sbi root=/dev/vda rw` 등으로 변경할 수 있습니다. 기본적으로는 기존과 동일하게 UART(`ttyS0`)를 사용합니다.

정지 PC의 해석을 위해서는, Linux의 `System.map` 또는 symbol이 포함된 ELF를 `Load symbols` 로 읽어들인 후 `Symbols @ PC` / `Diagnostics` / `Search symbols` 를 사용합니다. symbol이 포함된 ELF에 DWARF line table이 포함되어 있는 경우는 `DWARF lines @ PC` 로 file:line도 확인할 수 있습니다. `DWARF file summary` 는 line table에 포함된 파일별 줄 수를 표시합니다. firmware/payload가 symbol이 포함된 ELF일 경우에는, 자동으로 symbol table을 가져옵니다. `Annotated trace` 는 trace 내의 `pc=` 를 symbols/DWARF line으로 주석 처리합니다. `Download trace` 로 모든 hart의 trace snapshot을 저장할 수 있습니다. JSON/CSV 형식도 선택 가능합니다. JSON trace에는 symbols가 있으면 symbol/source 정보도 포함합니다. `Trace filter` 에는 `trap` , `ecall` , `sbi-shim` , `pc=` , `virtio` 등의 문자열을 넣어 trace tail/export, access timeline, compact 표시를 필터링할 수 있습니다. `Compact trace` 는 연속되는 동종 명령어・trap・ECALL을 접어줍니다. panic/oops 로그를 붙여넣고 `Analyze log symbols` 를 누르면, 로그 내의 64-bit PC 형식의 주소를 로드된 symbols로 해석합니다.

`Trace replay report` 는 현재의 trace를 통계화하고, `Trace baseline compare` 는 저장된 trace를 붙여넣어 현재 trace와의 차이를 PC/명령어/trap 단위로 처음부터 비교합니다. `Save current trace as baseline` / `Load saved baseline` 은 브라우저 localStorage에 baseline을 유지합니다. `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` 은 boot timeline, device probe, virtqueue, memory scanner, initcall 분류, trace 통계를 요약한 회귀 확인용 보고서입니다. `Capture memory scan` → 실행 → `Diff memory scan` 으로, guest memory 내의 ELF/FDT/initrd/rootfs 후보의 증감도 확인할 수 있습니다. `DWARF source context` 는 현재 PC 주변의 symbol과 DWARF file:line을 한꺼번에 표시합니다.

`Boot phase` 는 console log, MMIO histogram, trap, symbol 정보로부터 현재의 진행 상황을 요약합니다. `Boot timeline` 은 console의 milestone과 MMIO의 probe / status / QueueNotify를 시계열로 나열합니다. `Device probe` 는 virtio 등의 register access와 negotiation을 집계하고, `Virtqueue inspect` 는 queue setup과 notify 상태를 device/queue별로 표시합니다. `Descriptor chains` 는 queue의 avail ring에서 descriptor chain을 읽어, indirect descriptor나 buffer preview를 표시합니다. `Descriptor DOT` / `Download DOT` 는 동일한 chain을 Graphviz DOT으로 출력합니다. `Virtqueue anomalies` 는 queue setup이나 descriptor chain의 불일치를 검출하고, `Anomaly hints` 는 각각의 불일치에 대한 다음 확인 포인트를 표시합니다. `Integrated diagnostic query` 는 `virtio QueueReady` , `panic` , `satp` , `0x80200000` 등의 단어로 console / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory index를 교차 검색합니다. `Share report MD/JSON/HTML` 은 boot regression report에 anomaly hints/triage, memory index, memory jump hints, query presets, query hits를 더한 공유용 bundle입니다. HTML은 JSON을 내장한 자체 완결 파일로 저장할 수 있습니다. `Diagnostic query presets` 는 panic, virtio status, QueueReady/QueueNotify, satp/mstatus, trap, rootfs 관련을 일괄 검색합니다. `Save query` / `Load query` 는 diagnostic query를 browser localStorage에 저장합니다. `Memory scan` 은 DRAM 내의 ELF/FDT/initrd/kernel/rootfs 단편 후보를 찾고, `Memory index` 는 인접한 signature를 범위별로 묶어줍니다. `Memory search` 는 memory index를 문자열 또는 `0x...` address로 검색하고, `Memory jumps` 는 ELF/FDT/Linux/OpenSBI/cmdline/rootfs 등의 유용한 점프 목적지 후보를 표시합니다. `Initcall classifier` / `Initcall timeline` 은 Linux의 initcall/driver probe풍 로그를 분류・시계열화합니다. `Panic summary` 는 panic/oops/fault 주변 줄을 추출하고, symbols가 있으면 address를 해석합니다. `Boot analysis JSON` 은 이것들을 모아서 저장합니다. `Dmesg extract` 는 UART / virtio-console에 출력된 로그에서 Linux풍의 줄만을 추출합니다. `Decoded MMIO` 는 최근의 MMIO access를 register 이름과 함께 표시합니다.

`Triage dashboard` 는 stop-cause ranking, virtqueue anomaly severity, device probe, query bookmarks를 한 화면용 텍스트로 요약합니다. `Stop-cause ranking` 은 console/trace/status에서 kernel panic, oops, illegal instruction, page/access fault, virtqueue 이상, device probe 정체를 우선순위에 따라 나열합니다. `Stop-cause evidence` 는 랭킹의 근거, score breakdown, 권장 query, 다음 확인 포인트를 표시합니다. `Save triage baseline` → 실행 → `Triage diff` 로 dashboard의 status/phase/device/anomaly/memory counts의 차이를 비교할 수 있습니다. `Save preset baseline` → 실행 → `Compare preset baseline` 으로, panic/virtio/satp/rootfs 등의 preset query hit 수가 이전보다 증감했는지 확인할 수 있습니다. `Memory dump hits` 는 diagnostic query 또는 trace filter를 사용하여 memory index hit 주변을 hex/ASCII dump합니다. `Memory range dump` 는 임의의 address/length를 지정하여 DRAM을 직접 hex/ASCII dump합니다. `Redacted share MD/JSON/HTML` 은 공유 전에 email / MAC / IPv4를 `<email>` / `<mac>` / `<ipv4>` 로 치환합니다. `Redaction options JSON` 에서는 `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex` 를 전환할 수 있습니다.

`Smoke preset` 은 선택 중인 boot preset을 reset하고 지정된 step만큼 실행합니다. `Smoke matrix` 는 `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` 등의 preset list를 순서대로 실행하여, 각 preset의 실행 step, 마지막 phase, stop-cause 후보를 목록화합니다.

PC breakpoint는 `Breakpoints / watchpoints` 의 `PC breakpoint` 에 물리/가상 PC의 hex 값을 넣어 추가합니다. `Run` / `Step 1k` 는 breakpoint에서 정지하며, `Step` 은 현재 PC가 breakpoint이더라도 1명령어만 진행합니다. write watchpoint는 물리 주소 범위에 대한 bus write, read watchpoint는 bus read를 검출하는 간이 기능입니다. MMIO probe나 framebuffer 쓰기, 특정 구조체의 참조 확인에 사용할 수 있습니다. `Access timeline` / `Compact access` 는 최근의 DRAM/MMIO access를 시계열・압축 표시합니다. `PC profile on` 은 hot PC를 집계하며, `Capture snapshot` → 실행 → `Diff snapshot` 으로 실행 전후의 진단 차이를 확인할 수 있습니다.

simple-framebuffer는 `0x86000000` 에 1024x768x32bpp의 메모리를 준비하고, 자동 생성 DTB의 `/chosen/framebuffer@86000000` 에 `simple-framebuffer` 로 올립니다. Linux측에서 simplefb를 사용할 수 있는 경우에는 `Render framebuffer` 로 Canvas에 표시할 수 있습니다.

virtio-net은 브라우저에서 실제 네트워크로 연결하는 것이 아니라, packet-level의 디버그 디바이스입니다. `virtio-net debug` 에 Ethernet frame의 hex를 넣어 RX에 주입하고, guest가 TX queue로 보낸 frame은 `Show TX frames` 로 확인합니다. Linux측에서 인식시킬 경우에는 필요에 따라 `ip link set dev eth0 up` 등을 guest측에서 실행합니다.

virtio-rng은 deterministic PRNG를 guest entropy source로 보여주는 검증용 디바이스입니다. 재현성을 유지하기 위해 기본 seed는 고정되며, UI의 `Set deterministic seed` 에서 변경할 수 있습니다.

virtio-gpu는 Linux의 virtio-gpu driver probe와 2D resource setup을 관측하기 위한 최소 디바이스입니다. 실제 GPU acceleration이 아니라, control queue로 들어오는 modeset / scanout / flush계 명령을 추적하여, Diagnostics에 상태를 출력합니다. resource backing memory에서 simple-framebuffer로의 복사도 수행하므로, guest가 2D resource를 flush한 결과를 `Render framebuffer` / PNG export로 확인할 수 있습니다. cursor queue의 `UPDATE_CURSOR` / `MOVE_CURSOR` 도 상태로서 기록합니다.

`SBI shim on` 은 OpenSBI를 사용하지 않고 S-mode payload를 직접 실행시키는 디버그용입니다. 일반적인 `fw_dynamic.bin` / `fw_payload.bin` 실험에서는 비활성 상태로 유지해 주십시오.

Multi-hart를 테스트할 경우에는, firmware를 로드하기 전에 `Hart count` 를 설정해 주십시오. 설정 변경은 machine reset을 수반하므로, 변경 후에 firmware / payload / disk를 다시 로드하는 것을 전제로 합니다. `View hart` 로 표시 대상 hart의 register / CSR / trace를 전환할 수 있습니다.

`fw_dynamic.bin` 을 사용할 경우에는, 필요에 따라 `Load payload` 로 S-mode payload / kernel을 `0x80200000` 근처에 읽어들입니다. 에뮬레이터는 dynamic info를 `0x87dff000` 에 두고, `a2` 에 해당 주소를 설정합니다.

Linux 실험에서는 다음 중 하나를 사용할 수 있습니다:

- `Load disk`: rootfs 등의 raw disk image를 virtio-blk로 전달. 기본 bootargs는 `root=/dev/vda rw` .
- `Load initrd`: initramfs를 `0x84000000` 에 두고, 자동 생성 DTB에 initrd 범위를 반영. 필요하다면 bootargs를 `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` 등으로 변경.

OpenSBI 1.8.1의 배포된 RISC-V 바이너리를 사용하는 예:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# 압축 해제 후의 fw_dynamic.bin / fw_payload.bin / fw_jump.bin 등을 브라우저에서 로드
```

OpenSBI를 로컬에서 빌드할 경우에는, `riscv64-unknown-elf-` 등의 RISC-V toolchain을 준비하여 `PLATFORM=generic` 으로 빌드해 주십시오.

### 개발용 명령어

```bash
go test ./...
make wasm
make serve
```

## 주의

이 구현은 OpenSBI의 초기화, S-mode payload 이행, Linux boot 조사에 필요한 기능을 단계적으로 갖추고 있습니다. Linux boot를 향해 PMP, Sv39, virtio-blk, virtio-console, virtio-net, virtio-rng, virtio-input, virtio-gpu, initrd, simple-framebuffer, trap/CSR 정확도, CSR trace/summary, MMIO histogram/timeline/register decoder, boot phase/timeline analyzer, device probe analyzer, virtqueue inspector/descriptor chain visualizer/DOT export/snapshot/anomaly detector/anomaly hints/anomaly triage, guest memory scanner/index/diff/search/jump hints/dump helper, integrated diagnostic query/query presets/preset baseline comparison, triage dashboard/stop-cause ranking, share report bundle/HTML/redaction, initcall classifier/timeline, DWARF line lookup/source context/trace annotation, panic summary, dmesg extractor, trace replay/compare, boot regression report, PC profile, snapshot diff, boot smoke runner/smoke matrix, triage baseline diff, stop-cause evidence, editable redaction, memory range dump는 추가하였으나, 주요 미구현・간략화된 부분은 정확한 cycle/time 모델, AIA/IMSIC, tap/WebSocket bridge 등의 실제 네트워크 연결, 본격적인 virgl/DRM/GPU acceleration, 모든 CSR의 엄밀한 WARL/WPRI 동작, 여러 worker를 사용한 진정한 병렬 실행입니다. multi-hart는 단일 wasm worker 내의 협력적 스케줄링입니다.

## 진단・회귀 보조

smoke matrix와 진단 쿼리의 공유성을 강화하였습니다.

- `Smoke matrix MD/HTML` 은 smoke matrix 결과를 Markdown / 자체 완결 HTML로 저장합니다.
- `Save smoke baseline` → 실행 → `Compare smoke baseline` 으로 preset별 phase, 실행 step, top stop-cause의 차이를 확인할 수 있습니다.
- `Stop checklist` 는 stop-cause ranking으로부터, 다음으로 확인해야 할 구체적인 작업 항목을 체크리스트화합니다.
- `CSR/MMIO bookmarks` 는 integrated diagnostic query의 결과로부터 CSR / MMIO / trace의 주요 히트만을 추출합니다.
- `Watchpoint hits` 는 read/write watchpoint의 hit 이력을 시계열로 표시합니다. `Clear hit timeline` 으로 이력만을 삭제할 수 있습니다.
- `Artifact manifest` 는 현재 로드된 firmware / payload / disk / initrd / symbols와, 생성 DTB / dynamic info의 range, entry, SHA-256을 목록화합니다.

### regression handoff 보조

- `Manifest diff` / `Manifest diff JSON` 은 현재의 boot artifact manifest와 localStorage에 저장한 baseline을 비교하여, bootargs, hart 수, load range, entry, ELF 판정, SHA-256의 차이를 표시합니다.
- `Auto break/watch suggestions` 는 stop-cause evidence, 최근 trace PC, watchpoint hit timeline으로부터, 다음 실행 시에 두어야 할 PC breakpoint / read watchpoint / write watchpoint 후보를 생성합니다.
- `Smoke clusters` / `Smoke clusters JSON` 은 smoke matrix의 preset 결과를 phase와 top stop-cause로 클러스터링하여, 동일한 실패 유형의 preset을 하나로 묶습니다.
- `Diagnostic bundle JSON` 은 manifest, triage dashboard, stop-cause, breakpoint suggestions, share bundle, watchpoint hits를 모은 자체 완결 JSON입니다.
- `Compressed bundle JSON` 은 위의 diagnostic bundle을 gzip+base64로 변환한 것입니다. issue나 chat에 붙여넣기 전에 크기를 줄이고 싶을 때 사용합니다.

### handoff / provenance 보조

- `Decode bundle` 은 `Diagnostic bundle JSON` 이나 `Compressed bundle JSON` , 또는 gzip+base64 본체를 붙여넣어 전개합니다.
- `Bundle compare` / `Bundle compare JSON` 은 붙여넣은 과거 bundle과 현재 bundle을 비교하여, triage phase, top stop-cause, manifest, artifact hash, smoke cluster, watchpoint hit 수, suggestion 수의 차이를 표시합니다.
- `Provenance` / `Provenance JSON` 은 manifest, trace, console, diagnostic bundle의 SHA-256, trace line 수, console byte 수, top stop-cause를 요약합니다. 재현성 확인이나 issue 첨부의 근거로 사용할 수 있습니다.
- `Handoff MD` 는 provenance, top stop-cause, auto break/watch suggestions, stop checklist, baseline diff, artifact manifest를 Markdown으로 요약합니다.
- `Apply auto breaks` 는 auto break/watch suggestions의 상위 후보를 현재의 emulator에 일괄 적용합니다. 재실행 전에 정지 위치나 의심스러운 MMIO/DRAM 범위를 빠르게 설정하기 위한 보조 기능입니다.

### reproduction / signature / headless handoff

- `Repro plan` / `Repro MD` / `Repro JSON`
  - diagnostic bundle, provenance, artifact manifest로부터 재현 절차를 생성합니다.
  - firmware / payload / initrd / disk / symbols의 role, size, load range, SHA-256을 artifact pin으로 열거합니다.
  - smoke preset, bootargs, hart count, next_addr, 권장 break/watch 조건을 절차화합니다.
- `Log signature` / `Log signature JSON`
  - trace / console / manifest의 SHA-256, trace line count, first/last PC, console first/last line, hot token을 경량 summary화합니다.
  - raw trace를 붙여넣지 않고도 "동일한 로그인지" "어디가 변경되었는지"를 비교할 수 있습니다.
- `Save log signature` / `Load log signature` / `Compare log signature`
  - browser localStorage에 log signature baseline을 저장하고, 현재의 signature와 비교합니다.
  - trace hash, console hash, manifest hash, line count, last PC, last console line의 차이를 표시합니다.
- `Auto break verify`
  - auto breakpoint/watchpoint suggestion을 적용하기 전후의 확인용 summary를 표시합니다.
  - duplicate suggestion이나 수상한 PC range에 대한 warning을 출력합니다.
- `Headless smoke script`
  - 현재의 artifact manifest, bootargs, hart count, smoke presets, step count로부터 CI/handoff용의 shell script skeleton을 생성합니다.
  - 실행 환경에 Playwright 등의 browser harness를 추가하기 전 단계로서, artifact pin과 preset matrix를 고정하는 용도입니다.

#### headless / CI 보조

repro/signature handoff를 CI나 issue에서 다루기 쉽게 하기 위해, 다음을 추가하였습니다.

- `Bundle integrity` / `Integrity JSON` 은 diagnostic bundle과 artifact manifest의 정합성을 검사하여, artifact role, SHA-256, load range, suggestion, smoke result의 불일치를 `error` / `warn` / `info` 로 분류합니다.
- `Repro validation` / `Repro validation JSON` 은 현재의 reproduction plan이 bundle의 bootargs, hart count, next_addr, artifact pins, top stop-cause, log signature와 일치하는지를 확인합니다.
- `CI summary` / `CI summary JSON` 은 bundle integrity, trace/console signature, smoke result, stop-cause를 모아, CI에서 pass/warn/fail을 판정하기 쉬운 summary를 출력합니다.
- `Headless runner spec` / `Runner spec JSON` 은 `go run ./cmd/rvsmoke ...` 로 검사하기 위한 preset, steps, artifact pin, 권장 command를 생성합니다.
- `cmd/rvsmoke` 를 추가했습니다. 브라우저 외부에서 diagnostic bundle / artifact manifest를 읽어들여, artifact hash, bundle integrity, CI summary, runner spec을 text / JSON / Markdown으로 출력할 수 있습니다.

예:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

`rvsmoke` 는 현재, bundle/manifest와 artifact hash의 재현성 검사・CI summary 생성을 수행합니다. CPU 실행 자체는 계속해서 브라우저 js/wasm 측의 smoke matrix를 사용합니다.

#### rvsmoke CI gate / JUnit / SARIF

`cmd/rvsmoke` 는 브라우저에서 export한 diagnostic bundle / manifest를 CI측에서 검사하는 보조 CLI입니다. headless 실행의 실체화를 통해, baseline bundle 비교, CI gate policy, JUnit XML, SARIF, self-contained HTML report를 출력할 수 있습니다.

예:

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

policy JSON의 예:

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

`-out html` 은 표준 출력으로 자체 완결 HTML, `-out junit` 은 JUnit XML, `-out sarif` 는 SARIF JSON을 출력합니다. `-junit` / `-html` / `-sarif` 를 동시에 지정하면, 표준 출력 형식과는 별개로 각 파일도 저장합니다. CI gate는 artifact manifest, trace/console signature, baseline diff, virtqueue anomaly, smoke result를 통합하여 `pass` / `warn` / `fail` 로 정규화합니다.

#### rvsmoke policy templates / bundle trend compare

CI gate의 초기 도입과 여러 번의 regression 비교를 쉽게 하기 위해, `rvsmoke` 와 브라우저 UI에 policy template, action checklist, bundle trend compare를 추가했습니다.

- `CI policy templates` / `Policy templates JSON` 은 `default` , `strict` , `linux-boot` , `artifact-only` , `lenient` 의 내장 policy를 표시합니다.
- `Policy template JSON` 은 지정한 template을 그대로 CI에 적용할 수 있는 JSON으로 저장합니다.
- `CI gate` / `CI gate JSON` 은 현재의 browser 상태에 대하여 policy template을 적용하고, pass/warn/fail의 gate check를 표시합니다.
- `CI checklist` / `CI checklist JSON` 은 gate failure, bundle integrity, artifact diff로부터 다음으로 확인해야 할 항목을 checklist화합니다.
- `rvsmoke -compare name=bundle.json` 은 여러 bundle을 시계열로 나열하고, phase, top stop-cause, artifact hash, smoke cluster의 변화를 trend report로 출력합니다.

Policy template의 생성 예:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

여러 bundle의 비교 예:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

`-policy-template` 은 `-policy` 를 지정하지 않을 경우의 default policy로 사용됩니다. `-policy` 를 지정한 경우에는 파일의 JSON이 우선됩니다.

## rvsmoke CI 연동

`rvsmoke` 의 CI/handoff 보조를 확장하고 있습니다.

- `rvsmoke -print-github-actions linux-boot` 로 GitHub Actions workflow YAML을 생성할 수 있습니다.
- `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` 로 workflow를 파일 출력할 수 있습니다.
- `rvsmoke -policy-tree policy-tree.md` 로 CI gate / bundle integrity / baseline drift를 원인 트리로 저장할 수 있습니다.
- `rvsmoke -history history.txt` 로 여러 bundle trend의 phase / stop-cause / artifact drift 집계를 저장할 수 있습니다.
- `rvsmoke -repro-zip rvwasm-minimal-repro.zip` 으로, README, diagnostic bundle, manifest, runner spec, policy, CI summary, 검증 script를 포함한 최소 재현 패키지를 생성할 수 있습니다.

예:

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

`-repro-zip` 은 raw firmware/kernel/disk를 동봉하지 않습니다. bundle 내의 SHA-256 pin과 manifest range를 동봉하고, 공유처에서 artifact를 대조하는 것을 전제로 합니다.

### CI 재현 ZIP 검사 / matrix workflow 계속

`rvsmoke` 와 browser UI에, 최소 재현 패키지의 전달을 검사하는 기능과, GitHub Actions matrix / trend 시각화용 출력을 추가했습니다.

- `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` 으로, `-repro-zip` 이 생성한 ZIP을 압축 해제하지 않고 검사할 수 있습니다. 필수 파일, unsafe path, `diagnostic-bundle.json` / `manifest.json` 의 일치, `ci-policy.json` , `scripts/rvsmoke.sh` 를 확인합니다.
- `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` 로, preset별 GitHub Actions matrix workflow YAML을 생성할 수 있습니다.
- `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` 로 matrix workflow를 파일 출력할 수 있습니다.
- `rvsmoke -trend-csv rvwasm-trend.csv` 와 `-trend-chart-json rvwasm-trend-chart.json` 으로, bundle trend를 외부에서 그래프화하기 쉬운 CSV / JSON으로 저장할 수 있습니다.
- Browser UI에 `Minimal repro ZIP` , `Inspect repro ZIP` , `Repro ZIP JSON` , `Matrix workflow YAML` , `Trend chart JSON` , `Trend CSV` 를 추가했습니다.

예:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# JSON으로 검사 결과를 저장
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# current bundle과 previous bundle의 trend를 CSV/JSON화
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` 은 단독으로도 실행할 수 있습니다. `-bundle` 과 동시에 지정한 경우에는 일반적인 CI summary에 ZIP inspection 결과도 포함하며, 검사가 `fail` 일 경우에는 CI summary도 failure로 처리합니다.

### CI matrix 집계 / checksum manifest 계속

`rvsmoke` 의 CI artifact 전달을 강화했습니다.

- `-repro-checksums rvwasm-repro-checksums.json` 으로, `-inspect-repro-zip` 의 결과로부터 ZIP 내 파일의 deterministic checksum manifest를 저장할 수 있습니다.
- `-matrix-result name=rvsmoke-output.json` 을 여러 개 지정하여, 여러 preset / 여러 job의 `rvsmoke -out json` 결과를 집계할 수 있습니다.
- `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` 로 matrix 결과를 text / JSON / self-contained HTML로 저장할 수 있습니다.
- `-trend-html rvwasm-trend.html` 로 bundle trend report를 단일 HTML로 저장할 수 있습니다.

예:

```bash
# 최소 재현 ZIP의 내용과 checksum manifest를 저장
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# 여러 matrix job의 rvsmoke JSON을 집계
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

matrix aggregate는 job별 CI status, gate failure/warning 수, artifact mismatch, top stop-cause를 요약합니다. GitHub Actions의 matrix job이 분리되어 있는 경우에도, 마지막 집계 job에서 전체적인 실패 경향을 쉽게 파악하기 위한 보조 기능입니다.

#### CI / release handoff 보조

`rvsmoke` 의 CI artifact 관리와 release handoff를 강화했습니다.

- `-artifact-index rvwasm-artifacts.json` 은 JUnit / SARIF / HTML / trend / matrix / repro checksum 등, 생성된 CI artifact의 path, bytes, SHA-256을 요약합니다.
- `-release-manifest rvwasm-release.json` 은 diagnostic bundle, log signature, CI gate, matrix aggregate, flake report, artifact index, repro checksum verification을 1개의 handoff manifest로 통합합니다.
- `-release-html rvwasm-release.html` 은 Summary / Artifacts / Matrix / Checksums / JSON으로 이동할 수 있는 navigation이 포함된 자체 완결 HTML을 출력합니다.
- `-verify-repro-checksums baseline-repro-checksums.json` 은 현재 검사한 minimal repro ZIP의 checksum manifest를 baseline과 비교하여, missing / changed / extra를 검출합니다.
- `-matrix-flakes` , `-matrix-flakes-json` , `-matrix-flakes-html` 은 `uart#1` / `uart#2` 와 같은 여러 번의 matrix 결과를 정규화하여, 동일한 preset이 pass/fail 사이에서 흔들리고 있는지를 검출합니다.

예:

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

## 릴리스 인계・검증

CI 결과를 다른 머신, 다른 리포지토리, 리뷰 담당자에게 전달하기 위한 metadata 출력을 `rvsmoke` 에 추가했습니다.

### SBOM / provenance 확장

#### SBOM-lite 의존성 목록

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

이 의존성 목록은 작고 deterministic한 형식을 의도하고 있습니다. `go.mod` 를 읽어들여, module path, Go version, 직접적인 `require` 줄, `replace` 대상, CI artifact index에 포함된 artifact 종류를 기록합니다.

`rvsmoke` 를 다른 working directory에서 실행할 경우에는, `-go-mod /path/to/go.mod` 를 지정해 주십시오.

#### provenance attestation

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

attestation은 in-toto / SLSA를 참고한 JSON payload입니다. 이것 자체는 서명이 아니지만, 안정적인 SHA-256을 가지므로, 외부 CI tooling에서 서명할 대상으로 사용할 수 있습니다.

#### 릴리스 인계 ZIP

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

release handoff ZIP에는 metadata만을 포함합니다.

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

firmware, kernel, initrd, disk image는 포함하지 않습니다. 크기가 큰 artifact는 manifest 측에 SHA-256 pin으로 유지합니다.

#### 릴리스 인계 ZIP의 검사

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

inspector는 ZIP을 압축 해제하지 않고, 필수 파일, 위험한 path, 중복된 path, JSON parse 가능 여부, release / index / SBOM / attestation의 기본적인 정합성을 검사합니다.

### 릴리스 검증

release handoff ZIP의 생성에 더해, 검증 목적의 출력을 추가했습니다.

- `-verify-attestation` / `-verify-attestation-text` 는, deterministic provenance attestation hash, release materials, CI artifact subjects가, 생성된 release manifest, SBOM-lite inventory, artifact index와 일치하는지 확인합니다.
- `-sbom-baseline` , `-sbom-diff` , `-sbom-diff-json` 은, 현재의 SBOM-lite dependency inventory를 저장된 baseline과 비교합니다.
- `-compare-release-zip-inspection` , `-release-zip-compare` , `-release-zip-compare-json` 은, 현재의 inspected release handoff ZIP을 과거의 inspection JSON과 비교합니다.
- `-retention-manifest` / `-retention-text` 는, path, kind, bytes, SHA-256, retention days, expiry time, reason을 포함한 CI artifact retention manifest를 생성합니다.
- `-release-verification-html` 은, release status, attestation verification, SBOM diff, release ZIP comparison, retention 정보를 통합한 navigation이 포함된 HTML을 출력합니다.

예:

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

### 릴리스 audit gate

release verification 위에 최종 release audit layer를 추가했습니다. provenance attestation verification, SBOM-lite diff, release ZIP comparison, artifact retention expiry, matrix flake status, release manifest status를 1개의 score와 gate report로 통합합니다.

주요 flag:

- `-list-release-verify-policies` 는 내장 release audit policy를 목록으로 표시합니다.
- `-print-release-verify-policy strict` 는 policy JSON template을 출력합니다.
- `-release-verify-template default|strict|lenient|archive` 는 내장 policy를 선택합니다.
- `-release-verify-policy policy.json` 은 custom release audit policy를 읽어들입니다.
- `-retention-audit` / `-retention-audit-json` 은 expiry와 minimum-retention의 검사 결과를 출력합니다.
- `-release-score` / `-release-score-json` 은 0〜100의 release verification score를 출력합니다.
- `-release-gate` / `-release-gate-json` 은 policy gate result를 출력합니다.
- `-release-audit` / `-release-audit-json` / `-release-audit-html` 은 통합 audit report를 출력합니다.

예:

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

strict policy는, pass가 아닌 release manifest, 실패한 attestation / SBOM / ZIP check, 기한이 만료된 artifact, 설정된 minimum retention days에 미치지 못하는 artifact를 실패로 간주합니다. default policy는 nightly handoff 등의 일상적인 확인에 적합하며, warning은 허용하면서, 명확한 verification failure는 CI 실패로 처리할 수 있습니다.

#### release audit diff / waiver / TODO 인계

`rvsmoke` 의 release-audit path는, 현재 audit과 과거 audit의 비교, 알려진 issue에 대한 기한부 waiver 적용, 미 waiver 작업의 checklist 생성을 지원합니다.

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

waiver template은 다음 명령어로 생성할 수 있습니다.

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

waiver는, 알려진 일시적인 release-audit finding을 다루기 위한 것입니다. 각 rule은 ID, 임의의 kind / name / status / substring matcher, owner, reason, `expires_at` timestamp를 가집니다. 기한이 만료된 waiver는 보고되지만, issue를 억제하는 데에는 사용되지 않습니다.

#### release decision / evidence bundle

release audit 실행 후에 사용하는 최종 인계 보조 기능을 추가했습니다.

- `-waiver-calendar` , `-waiver-calendar-json` , `-waiver-calendar-html` 은, 각 waiver의 expiry, owner, match count, expired / expiring-soon 상태를 표시합니다.
- `-release-changelog` , `-release-changelog-json` 은, audit diff, waiver state, TODO count, waiver expiry state를 사람이 읽기 쉬운 changelog로 요약합니다.
- `-final-decision` , `-final-decision-json` 은, blocking item과 next action을 포함한 최종 `go` , `go-with-watch` , `no-go` decision을 생성합니다.
- `-release-evidence-zip` 은, audit, waiver report, TODO list, waiver calendar, changelog, final decision을 포함한 작은 evidence bundle을 출력합니다.
- `-inspect-release-evidence-zip` 은, evidence bundle을 압축 해제하지 않고, 필수 파일, 위험한 path, 중복된 entry, JSON parse 가능 여부를 검사합니다.
- `-dry-run` 은 optional output file을 출력하지 않고 report를 계산합니다.
- `-exit-code-mode never` 는, 일반적으로 gate failure로 인해 실패할 경우에도 결과를 출력합니다.

예:

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

CI에서 evidence bundle을 검사하는 예:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## 라이선스

이 프로젝트는 BSD 2-Clause License에 따라 라이선스가 부여됩니다. 자세한 내용은 [LICENSE](../LICENSE) 파일을 참조하세요.

SPDX-License-Identifier: BSD-2-Clause
