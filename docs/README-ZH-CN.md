# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## 概述

这是一个运行在Go 1.23.2 `GOOS=js GOARCH=wasm` 上的RV64IMAC模拟器。默认是single-hart，但也可以通过UI进行1〜8 hart的协作调度。可以从浏览器UI加载OpenSBI 1.8.1的 `fw_payload.bin` / `fw_jump.bin` / `fw_dynamic.bin` / ELF并确认启动。

[![OpenSBI fw_payload在rvwasm上启动的画面](images/fw_payload.png)](https://kitaharata.github.io/rvwasm/)

OpenSBI 1.8.1 `fw_payload.bin` 在rvwasm上启动，并进入下一阶段S-mode payload的示例。

## 已实现

- RV64I基本指令
- M extension
- A extension的LR/SC/AMO最小实现
- C extension的常见整数指令
- 相当于Zicsr / Zifencei
- M/S/U privilege mode的CSR/trap/mret/sret最小实现
  - 将同步异常的 `mepc` / `sepc` 修正为faulting instruction PC
  - 修正faulting load/CSR write，使其不破坏rd，也不推进retire counter
  - CSR存在检查、抑制read-only CSR写入side effect、基本反映 `mcounteren` / `scounteren`
  - 为Linux侧的probing添加 `senvcfg` / state-enable CSR stub
  - 基本反映 `TVM` / `TW` / `TSR` 和解除 `MPRV`
- Sv39 MMU
  - `satp` mode Bare / Sv39
  - 3级页表walk
  - 4 KiB / 2 MiB / 1 GiB leaf
  - 基本反映 `SUM` / `MXR` / `MPRV`
  - page fault exception
  - 自动更新PTE `A` / `D` bit
- 类UART 16550风MMIO（`0x10000000`）
  - 来自guest的输出
  - 从浏览器UI进行输入inject
  - receive interrupt
- 类CLINT风mtime/mtimecmp/msip（`0x02000000`）
  - 面向multi-hart的per-hart MSIP / MTIMECMP routing
- 类PLIC风interrupt controller（`0x0c000000`）
  - priority / pending / enable / threshold
  - claim / complete
  - 每个hart的M/S context
- PMP enforcement
  - TOR / NA4 / NAPOT
  - R/W/X permission
  - 通过locked entry限制M-mode
- 供OpenSBI `fw_dynamic` 使用的boot info
  - 将dynamic info配置在 `0x87dff000`
  - 在 `a2` 中设置dynamic info pointer
  - 可从UI单独加载S-mode payload / kernel
- virtio-mmio block device（`0x10001000`）
  - modern virtio 1.0 style MMIO register
  - split virtqueue的read/write/flush/get-id最小对应
  - `FEATURES_OK` negotiation与 `VIRTIO_F_VERSION_1` 验证
  - queue reset、忽略 `DRIVER_OK` 前的notify、基本反映 `NO_INTERRUPT` flag
  - `VIRTIO_RING_F_INDIRECT_DESC` 与indirect descriptor table的处理
  - 依据 `VIRTIO_RING_F_EVENT_IDX` 的used event抑制中断
  - 可从UI加载disk image
  - 可从UI下载guest修改后的disk image
- virtio-mmio console device（`0x10002000`）
  - device ID 3的最小console
  - queue 0 receive / queue 1 transmit
  - 最小对应 `VIRTIO_CONSOLE_F_SIZE` 、indirect descriptor、event index
  - 将UI输入inject到UART和virtio-console两者中
- virtio-mmio net device（`0x10003000`）
  - device ID 1的调试用最小virtio-net
  - queue 0 receive / queue 1 transmit
  - 最小对应 `VIRTIO_NET_F_MAC` / `VIRTIO_NET_F_STATUS` / indirect descriptor / event index
  - 从UI将Ethernet frame hex进行RX inject
  - 将guest发送的Ethernet frame作为TX log显示
- virtio-mmio rng device（`0x10004000`）
  - device ID 4的最小entropy source
  - 最小对应split virtqueue、indirect descriptor、event index
  - 可从UI设置deterministic seed
- virtio-mmio input device（`0x10005000`）
  - device ID 18的调试用最小keyboard/input device
  - 最小对应event queue / status queue、indirect descriptor、event index
  - 可从UI进行key event / raw input event的inject
- virtio-mmio gpu device（`0x10006000`）
  - device ID 16的调试用2D virtio-gpu foundation
  - 最小对应control / cursor queue、indirect descriptor、event index
  - 针对 `GET_DISPLAY_INFO` / `RESOURCE_CREATE_2D` / `SET_SCANOUT` / `FLUSH` 等的基本响应
  - 用于观测Linux的virtio-gpu probe和初始modeset command
- initrd / initramfs传递
  - default load address: `0x84000000`
  - 反映到自动生成DTB的 `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end`
- bootargs编辑
  - default: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - 供UART / virtio-console / initramfs / verbose debug使用的preset
  - 从UI设置并反映到自动生成DTB中
- 执行追踪ring buffer
  - 可在UI中查看PC / 指令 / trap / 最后的trap cause/tval
  - 可从UI进行CSR dump和所有hart trace snapshot的text / JSON / CSV export
  - 汇总显示最后ECALL/SBI参数、SBI BASE/TIME/IPI/RFENCE/HSM/SRST/legacy counters、trap、virtio queue状态的Diagnostics
  - Diagnostics / device state的JSON export
  - 加载ELF / System.map symbols，支持显示停止PC周围的symbol、名称搜索、自动解析panic/oops日志内PC的symbol
  - 用于直接测试无OpenSBI的小型S-mode payload的任意SBI shim
    - BASE / TIME / IPI / RFENCE / HSM / SRST的最小short-circuit
    - 通过HSM `hart_start` 启动目标hart的S-mode入口的调试路径
    - 默认无效。在运行OpenSBI的常规路径中不使用
  - 可从UI对任意物理内存范围进行dump
  - 可从UI设置PC breakpoint、物理read/write watchpoint、trace filter
  - breakpoint可指定hit count、mode条件、hart条件
  - trace在raw instruction之外显示简化的decode mnemonic
  - breakpoint / watchpoint hit时将stop reason记录到status / diagnostics / trace export
  - 收集MMIO/DRAM access histogram，可在Diagnostics / JSON中确认device probe或queue activity的偏差
  - 将MMIO/DRAM access timeline保存到ring buffer中，可用raw / compact显示确认probe的时间序列
  - 在MMIO access timeline中附加virtio-mmio / UART / CLINT / PLIC的register decoder名称，能够以 `QueueNotify` / `Status` / `LSR` 等为单位进行确认
  - 可选择启用CSR access trace，在Diagnostics / trace export中显示guest的CSR read/write tail及各CSR的read/write summary
  - 可选择启用PC hot-spot profile，带有symbol确认停止前执行次数较多的PC
  - 通过diagnostic snapshot capture / diff，可在UI中确认执行前后的hart/device/CSR/MMIO状态差异
  - 在compact trace显示中可折叠连续的同类指令・trap・ECALL日志
  - 依据各boot preset的smoke runner，自动执行当前已加载firmware/payload指定的hart-step，并获取结果JSON
  - 在boot phase analyzer中统一显示OpenSBI / Linux / panic / virtio activity / trap / PC symbol
  - 在boot timeline中将console marker与MMIO probe / status / QueueNotify / PLIC claim整合为时间序列显示
  - 在device probe analyzer中统计virtio/UART/PLIC/CLINT的read/write、identity register、status negotiation、queue notify
  - 在virtqueue inspector中按device/queue显示QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify的最近状态
  - 在descriptor chain visualizer中从avail ring追踪head descriptor，显示NEXT / WRITE / INDIRECT descriptor及小型buffer preview
  - 在descriptor chain graph export中可将virtqueue chain保存・可视化为Graphviz DOT
  - 在guest physical memory scanner中检测DRAM内类似ELF / FDT / gzip / xz / zstd / squashfs / cpio / ext magic / OpenSBI / Linux version / BusyBox / kernel cmdline的区域
  - 在initcall / driver probe classifier中对Linux console log的initcall、probe、virtio、storage、console、network、graphics相关行进行分类
  - 在initcall timeline中以时间序列分组显示已分类的initcall / driver probe行
  - 加载带symbol的ELF的DWARF line table，显示当前PC附近的file:line、DWARF file summary、trace PC的symbol+line注释
  - 在panic summary中自动提取console log内的panic/oops/fault周边行，用已加载的symbols解析address
  - 通过boot analysis JSON一次性export timeline / device probe / virtqueue / panic summary
  - 在trace replay report中摘要trace的step/trap/ecall/SBI shim件数、hot mnemonic、trap cause
  - 在trace baseline compare中将上次保存的trace与当前trace的PC/指令/trap差异从头开始比对
  - 可在浏览器localStorage中保存/加载trace baseline
  - 除boot regression report/JSON外，可通过Markdown/HTML report export统一保存trace stats、boot events、device probe、virtqueue、memory object、initcall counts
  - 在virtqueue snapshot中同时显示queue setup与descriptor chain
  - 在virtqueue anomaly detector中检测ready queue的address缺失、descriptor loop、indirect长度错误、DRAM外buffer等
  - 在virtqueue anomaly hints中按检测结果显示QueueNum / QueueDesc / QueueReady / descriptor alignment等修复提示
  - 通过integrated diagnostic query，使用相同query横向搜索console / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory index
  - 使用diagnostic query presets一次性搜索panic、virtio negotiation、QueueReady/Notify、satp/mstatus、trap、rootfs相关内容
  - 通过share report MD/JSON/HTML，以自包含形式分享boot regression、virtqueue hints/triage、memory index、query presets、jump hints、query hits
  - 在triage dashboard / stop-cause ranking中按候补顺序显示panic、trap、page/access fault、virtqueue anomaly、device probe停滞
  - 在stop-cause evidence中显示排名依据、score breakdown、推荐diagnostic query、下一步行动
  - 将triage dashboard baseline保存到localStorage，对比当前dashboard与status/phase/device/anomaly/memory counts
  - 将diagnostic preset baseline保存到localStorage，对比与当前preset hit数的差异
  - 通过redacted share report MD/JSON/HTML输出隐藏了IP/MAC/email的分享用报告
  - 在redaction options JSON中从UI调整是否替换IP/MAC/email/long hex address
  - 在memory object dump中用hex + ASCII确认memory index/search的hit周边
  - 在memory range dump中指定任意DRAM address与byte length进行hex + ASCII dump / JSON export
  - 在memory scan snapshot/diff中确认执行前后增减的ELF/FDT/initrd/rootfs片段候补
  - 在memory index中将相邻的ELF/FDT/initrd/kernel/rootfs signature按范围汇总并索引化
  - 从UART / virtio-console输出中提取类似Linux `dmesg` 的日志，用已加载的symbols解析panic/oops address
- simple-framebuffer
  - 将 `0x86000000` , 1024x768, `a8r8g8b8` 添加到自动生成DTB的 `/chosen/framebuffer@86000000`
  - 在UI的Canvas绘制framebuffer，可下载RGBA raw dump与PNG
  - 在 `TRANSFER_TO_HOST_2D` / `RESOURCE_FLUSH` 时将virtio-gpu的2D resource backing复制到simple-framebuffer
- DRAM `0x80000000` , 128 MiB
- 自动生成带virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT的最小virt DTB，或从UI加载DTB
  - 添加 `sifive,plic-1.0.0` / `sifive,clint0` compatible与virtio `dma-coherent`
  - 根据Hart count生成 `cpu@N` 与 `interrupts-extended`

## 使用方法

```bash
make serve
```

在浏览器中打开 `http://localhost:8080` ，选择OpenSBI的firmware后点击 `Load firmware` → `Run` 。

若要将virtio-console作为Linux console进行测试，可将bootargs修改为 `console=hvc0 earlycon=sbi root=/dev/vda rw` 等。默认仍按常规使用UART（`ttyS0`）。

解析停止PC时，请在 `Load symbols` 中加载Linux的 `System.map` 或带symbol的ELF后，使用 `Symbols @ PC` / `Diagnostics` / `Search symbols` 。如果带symbol的ELF包含DWARF line table，也可以在 `DWARF lines @ PC` 中确认file:line。 `DWARF file summary` 显示line table中包含的各个文件的行数。如果firmware/payload是带symbol的ELF，也会自动导入symbol table。 `Annotated trace` 会用symbols/DWARF line在trace内的 `pc=` 处进行注释。可通过 `Download trace` 保存所有hart的trace snapshot。也可选择JSON/CSV格式。JSON trace中如果存在symbols则会包含symbol/source信息。在 `Trace filter` 中输入 `trap` 、 `ecall` 、 `sbi-shim` 、 `pc=` 、 `virtio` 等字符串可以过滤trace tail/export、access timeline、compact显示。 `Compact trace` 会折叠连续的同类指令・trap・ECALL。粘贴panic/oops日志并点击 `Analyze log symbols` ，将用已加载的symbols解析日志中类似64-bit PC的地址。

`Trace replay report` 会统计当前的trace， `Trace baseline compare` 粘贴已保存的trace与当前trace的PC/指令/trap差异从头开始比对。 `Save current trace as baseline` / `Load saved baseline` 会在浏览器localStorage中保存baseline。 `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` 是汇总了boot timeline、device probe、virtqueue、memory scanner、initcall分类、trace统计的回归确认用报告。通过 `Capture memory scan` →执行→ `Diff memory scan` ，也能确认guest memory内ELF/FDT/initrd/rootfs候补的增减。 `DWARF source context` 会将当前PC周边的symbol与DWARF file:line合并显示。

`Boot phase` 从console log、MMIO histogram、trap、symbol信息摘要当前的进度。 `Boot timeline` 将console的milestone与MMIO的probe / status / QueueNotify按时间序列排列。 `Device probe` 统计virtio等的register access与negotiation， `Virtqueue inspect` 按device/queue显示queue setup与notify状态。 `Descriptor chains` 从queue的avail ring读取descriptor chain，并显示indirect descriptor或buffer preview。 `Descriptor DOT` / `Download DOT` 将该chain作为Graphviz DOT输出。 `Virtqueue anomalies` 检测queue setup或descriptor chain的不一致， `Anomaly hints` 针对各不一致显示下一步检查点。 `Integrated diagnostic query` 用 `virtio QueueReady` 、 `panic` 、 `satp` 、 `0x80200000` 等词汇横向搜索console / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory index。 `Share report MD/JSON/HTML` 是在boot regression report中加入anomaly hints/triage、memory index、memory jump hints、query presets、query hits的分享用bundle。HTML可作为内嵌了JSON的自包含文件保存。 `Diagnostic query presets` 集中搜索panic、virtio status、QueueReady/QueueNotify、satp/mstatus、trap、rootfs相关内容。 `Save query` / `Load query` 将diagnostic query保存到browser localStorage。 `Memory scan` 在DRAM内寻找ELF/FDT/initrd/kernel/rootfs片段候补， `Memory index` 将相邻的signature按范围汇总。 `Memory search` 用字符串或 `0x...` address搜索memory index， `Memory jumps` 显示ELF/FDT/Linux/OpenSBI/cmdline/rootfs等有用的跳转目标候补。 `Initcall classifier` / `Initcall timeline` 对Linux的initcall/driver probe风日志进行分类并按时间序列化。 `Panic summary` 提取panic/oops/fault周边行，如有symbols则解析address。 `Boot analysis JSON` 将这些内容集中保存。 `Dmesg extract` 从UART / virtio-console输出中仅提取类似Linux风格的行。 `Decoded MMIO` 附带register名称显示最近的MMIO access。

`Triage dashboard` 将stop-cause ranking、virtqueue anomaly severity、device probe、query bookmarks整合为一屏用的文本。 `Stop-cause ranking` 从console/trace/status提取kernel panic、oops、illegal instruction、page/access fault、virtqueue异常、device probe停滞并带优先级排序。 `Stop-cause evidence` 显示排名依据、score breakdown、推荐query、下一步检查点。通过 `Save triage baseline` →执行→ `Triage diff` 可比对dashboard的status/phase/device/anomaly/memory counts差异。通过 `Save preset baseline` →执行→ `Compare preset baseline` ，可确认panic/virtio/satp/rootfs等preset query hit数相比上次是否有所增减。 `Memory dump hits` 使用diagnostic query或trace filter对memory index hit周边进行hex/ASCII dump。 `Memory range dump` 指定任意address/length直接对DRAM进行hex/ASCII dump。 `Redacted share MD/JSON/HTML` 在分享前将email / MAC / IPv4替换为 `<email>` / `<mac>` / `<ipv4>` 。在 `Redaction options JSON` 中可切换 `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex` 。

`Smoke preset` 重置选中的boot preset并仅执行指定step。 `Smoke matrix` 依次执行如 `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` 等preset list，并列出各preset的执行step、最后phase、stop-cause候补。

PC breakpoint通过在 `Breakpoints / watchpoints` 的 `PC breakpoint` 填入物理/虚拟PC的hex值添加。 `Run` / `Step 1k` 在breakpoint处停止， `Step` 即使当前PC是breakpoint也只步进1条指令。write watchpoint是针对物理地址范围的bus write、read watchpoint是检测bus read的简易功能。可用于检查MMIO probe或framebuffer写入、特定结构体的引用。 `Access timeline` / `Compact access` 将最近的DRAM/MMIO access按时间序列・压缩显示。 `PC profile on` 统计hot PC，通过 `Capture snapshot` →执行→ `Diff snapshot` 可确认执行前后的诊断差异。

simple-framebuffer在 `0x86000000` 准备1024x768x32bpp的内存，并作为 `simple-framebuffer` 载入自动生成DTB的 `/chosen/framebuffer@86000000` 。若Linux侧可使用simplefb，则可通过 `Render framebuffer` 显示在Canvas上。

virtio-net并非让浏览器连接真实网络，而是packet-level的调试设备。在 `virtio-net debug` 中输入Ethernet frame的hex并注入RX，guest发送到TX queue的frame在 `Show TX frames` 中确认。若要在Linux侧识别，根据需要在guest侧执行 `ip link set dev eth0 up` 等。

virtio-rng是作为guest entropy source展示deterministic PRNG的验证用设备。为保持可复现性默认seed是固定的，可从UI的 `Set deterministic seed` 进行更改。

virtio-gpu是为观测Linux的virtio-gpu driver probe与2D resource setup的最小设备。并非真正的GPU acceleration，而是追踪进入control queue的modeset / scanout / flush系命令，并在Diagnostics中输出状态。由于也执行从resource backing memory到simple-framebuffer的复制，可通过 `Render framebuffer` / PNG export确认guest对2D resource进行flush的结果。cursor queue的 `UPDATE_CURSOR` / `MOVE_CURSOR` 也会作为状态记录。

`SBI shim on` 用于不使用OpenSBI直接运行S-mode payload的调试。在通常的 `fw_dynamic.bin` / `fw_payload.bin` 实验中请保持无效。

若要尝试Multi-hart，请在加载firmware前设置 `Hart count` 。更改设置会伴随machine reset，因此前提是更改后重新加载firmware / payload / disk。在 `View hart` 中可切换显示目标hart的register / CSR / trace。

若使用 `fw_dynamic.bin` ，根据需要在 `Load payload` 中将S-mode payload / kernel加载到 `0x80200000` 附近。模拟器将dynamic info放置在 `0x87dff000` ，并在 `a2` 中设置其地址。

在Linux实验中可使用以下任意一种:

- `Load disk`: 将rootfs等raw disk image作为virtio-blk传递。默认bootargs为 `root=/dev/vda rw` 。
- `Load initrd`: 将initramfs放置在 `0x84000000` ，并在自动生成DTB中反映initrd范围。如果需要，将bootargs修改为 `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` 等。

使用OpenSBI 1.8.1已发布的RISC-V二进制文件的示例:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# 将解压后的 fw_dynamic.bin / fw_payload.bin / fw_jump.bin 等从浏览器加载
```

若是本地构建OpenSBI，请准备 `riscv64-unknown-elf-` 等RISC-V toolchain并使用 `PLATFORM=generic` 进行构建。

### 开发用命令

```bash
go test ./...
make wasm
make serve
```

## 注意

该实现分阶段具备了调查OpenSBI初始化、S-mode payload迁移、Linux boot所需的功能。面向Linux boot虽添加了PMP、Sv39、virtio-blk、virtio-console、virtio-net、virtio-rng、virtio-input、virtio-gpu、initrd、simple-framebuffer、trap/CSR精度、CSR trace/summary、MMIO histogram/timeline/register decoder、boot phase/timeline analyzer、device probe analyzer、virtqueue inspector/descriptor chain visualizer/DOT export/snapshot/anomaly detector/anomaly hints/anomaly triage、guest memory scanner/index/diff/search/jump hints/dump helper、integrated diagnostic query/query presets/preset baseline comparison、triage dashboard/stop-cause ranking、share report bundle/HTML/redaction、initcall classifier/timeline、DWARF line lookup/source context/trace annotation、panic summary、dmesg extractor、trace replay/compare、boot regression report、PC profile、snapshot diff、boot smoke runner/smoke matrix、triage baseline diff、stop-cause evidence、editable redaction、memory range dump，但主要未实现・简化的部分为准确的cycle/time模型、AIA/IMSIC、tap/WebSocket bridge等真实网络连接、真正的virgl/DRM/GPU acceleration、所有CSR的严格WARL/WPRI动作、使用多个worker的真正并行执行。multi-hart是单一wasm worker内的协作调度。

## 诊断・回归辅助

强化了smoke matrix与诊断查询的分享性。

- `Smoke matrix MD/HTML` 将smoke matrix结果保存为Markdown / 自包含HTML。
- 通过 `Save smoke baseline` →执行→ `Compare smoke baseline` 可确认各preset的phase、执行step、top stop-cause的差异。
- `Stop checklist` 根据stop-cause ranking，将下一步需要确认的具体操作项目转化为检查清单。
- `CSR/MMIO bookmarks` 从integrated diagnostic query的结果中仅提取CSR / MMIO / trace的重要命中。
- `Watchpoint hits` 按时间序列显示read/write watchpoint的hit历史。通过 `Clear hit timeline` 可仅清除历史。
- `Artifact manifest` 将当前已加载的firmware / payload / disk / initrd / symbols及生成DTB / dynamic info的range、entry、SHA-256进行列表化。

### regression handoff辅助

- `Manifest diff` / `Manifest diff JSON` 将当前的boot artifact manifest与保存在localStorage的baseline进行比较，显示bootargs、hart数、load range、entry、ELF判定、SHA-256的差异。
- `Auto break/watch suggestions` 根据stop-cause evidence、最近的trace PC、watchpoint hit timeline，生成下次执行时应设置的PC breakpoint / read watchpoint / write watchpoint候补。
- `Smoke clusters` / `Smoke clusters JSON` 按照phase与top stop-cause对smoke matrix的preset结果进行聚类，将相同失败类型的preset归纳在一起。
- `Diagnostic bundle JSON` 是汇总了manifest、triage dashboard、stop-cause、breakpoint suggestions、share bundle、watchpoint hits的自包含JSON。
- `Compressed bundle JSON` 是将上述diagnostic bundle转化为gzip+base64。用于在贴到issue或chat前缩小体积。

### handoff / provenance辅助

- `Decode bundle` 粘贴并解压 `Diagnostic bundle JSON` 或 `Compressed bundle JSON` ，或gzip+base64本体。
- `Bundle compare` / `Bundle compare JSON` 将粘贴的过去bundle与当前bundle比较，显示triage phase、top stop-cause、manifest、artifact hash、smoke cluster、watchpoint hit数、suggestion数的差异。
- `Provenance` / `Provenance JSON` 汇总manifest、trace、console、diagnostic bundle的SHA-256、trace line数、console byte数、top stop-cause。可用作可复现性确认或附加到issue的依据。
- `Handoff MD` 将provenance、top stop-cause、auto break/watch suggestions、stop checklist、baseline diff、artifact manifest汇总为Markdown。
- `Apply auto breaks` 将auto break/watch suggestions的高位候补一次性应用到当前的emulator中。用于再次执行前快速设置停止位置或可疑的MMIO/DRAM范围的辅助。

### reproduction / signature / headless handoff

- `Repro plan` / `Repro MD` / `Repro JSON`
  - 根据diagnostic bundle、provenance、artifact manifest生成复现步骤。
  - 将firmware / payload / initrd / disk / symbols的role、size、load range、SHA-256作为artifact pin列出。
  - 将smoke preset、bootargs、hart count、next_addr、推荐break/watch条件步骤化。
- `Log signature` / `Log signature JSON`
  - 将trace / console / manifest的SHA-256、trace line count、first/last PC、console first/last line、hot token轻量化为summary。
  - 不粘贴raw trace也能比较“是否是同一个日志”“哪里发生了变化”。
- `Save log signature` / `Load log signature` / `Compare log signature`
  - 在browser localStorage中保存log signature baseline，并与当前的signature比较。
  - 显示trace hash、console hash、manifest hash、line count、last PC、last console line的差异。
- `Auto break verify`
  - 显示应用auto breakpoint/watchpoint suggestion前后的确认用summary。
  - 针对duplicate suggestion或可疑的PC range发出warning。
- `Headless smoke script`
  - 根据当前的artifact manifest、bootargs、hart count、smoke presets、step count生成供CI/handoff使用的shell script skeleton。
  - 作为向执行环境添加Playwright等browser harness的前置阶段，用途是固定artifact pin与preset matrix。

#### headless / CI辅助

为了让repro/signature handoff在CI或issue中易于处理，增加了以下功能。

- `Bundle integrity` / `Integrity JSON` 检查diagnostic bundle与artifact manifest的整合性，将artifact role、SHA-256、load range、suggestion、smoke result的不一致分类为 `error` / `warn` / `info` 。
- `Repro validation` / `Repro validation JSON` 确认当前的reproduction plan是否与bundle的bootargs、hart count、next_addr、artifact pins、top stop-cause、log signature一致。
- `CI summary` / `CI summary JSON` 汇总bundle integrity、trace/console signature、smoke result、stop-cause，输出在CI中便于判定pass/warn/fail的summary。
- `Headless runner spec` / `Runner spec JSON` 生成供 `go run ./cmd/rvsmoke ...` 检查用的preset、steps、artifact pin、推荐command。
- 添加了 `cmd/rvsmoke` 。可在浏览器外读取diagnostic bundle / artifact manifest，以text / JSON / Markdown输出artifact hash、bundle integrity、CI summary、runner spec。

示例:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

`rvsmoke` 目前负责对bundle/manifest与artifact hash进行复现性检查及CI summary生成。CPU执行本身仍继续使用浏览器js/wasm侧的smoke matrix。

#### rvsmoke CI gate / JUnit / SARIF

`cmd/rvsmoke` 是在CI侧检查由浏览器export出的diagnostic bundle / manifest的辅助CLI。由于headless执行的实体化，可输出baseline bundle比较、CI gate policy、JUnit XML、SARIF、self-contained HTML report。

示例:

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

policy JSON的示例:

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

`-out html` 向标准输出自包含HTML， `-out junit` 为JUnit XML， `-out sarif` 为SARIF JSON。若同时指定 `-junit` / `-html` / `-sarif` ，除标准输出的格式外也会保存至各文件。CI gate将artifact manifest、trace/console signature、baseline diff、virtqueue anomaly、smoke result汇总并正规化为 `pass` / `warn` / `fail` 。

#### rvsmoke policy templates / bundle trend compare

为了方便CI gate的初步引入与多次回归比较，在 `rvsmoke` 及浏览器UI中添加了policy template、action checklist、bundle trend compare。

- `CI policy templates` / `Policy templates JSON` 显示 `default` 、 `strict` 、 `linux-boot` 、 `artifact-only` 、 `lenient` 等内置policy。
- `Policy template JSON` 将指定的template直接保存为可放入CI的JSON。
- `CI gate` / `CI gate JSON` 对当前的browser状态应用policy template，显示pass/warn/fail的gate check。
- `CI checklist` / `CI checklist JSON` 根据gate failure、bundle integrity、artifact diff，将下一步需要确认的项目转化为checklist。
- `rvsmoke -compare name=bundle.json` 将多个bundle按时间序列排列，将phase、top stop-cause、artifact hash、smoke cluster的变化作为trend report输出。

Policy template的生成示例:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

多个bundle的比较示例:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

未指定 `-policy` 时， `-policy-template` 作为default policy使用。若指定了 `-policy` ，则优先使用文件的JSON。

## rvsmoke CI联动

扩展了 `rvsmoke` 的CI/handoff辅助功能。

- 能够通过 `rvsmoke -print-github-actions linux-boot` 生成GitHub Actions workflow YAML。
- 能够通过 `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` 将workflow输出到文件。
- 能够通过 `rvsmoke -policy-tree policy-tree.md` 将CI gate / bundle integrity / baseline drift作为原因树保存。
- 能够通过 `rvsmoke -history history.txt` 保存多个bundle trend的phase / stop-cause / artifact drift统计。
- 能够通过 `rvsmoke -repro-zip rvwasm-minimal-repro.zip` ，生成包含README、diagnostic bundle、manifest、runner spec、policy、CI summary、验证script的最小复现包。

示例:

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

`-repro-zip` 不打包raw firmware/kernel/disk。前提是将bundle内的SHA-256 pin与manifest range一同打包，在分享对象处核对artifact。

### CI复现ZIP检查 / matrix workflow继续

在 `rvsmoke` 及browser UI中，添加了检查最小复现包交接的功能，以及用于GitHub Actions matrix / trend可视化的输出。

- 通过 `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` ，无需解压即可检查 `-repro-zip` 生成的ZIP。确认必须文件、unsafe path、 `diagnostic-bundle.json` / `manifest.json` 的一致性、 `ci-policy.json` 、 `scripts/rvsmoke.sh` 。
- 通过 `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` ，可针对各个preset生成GitHub Actions matrix workflow YAML。
- 通过 `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` 可将matrix workflow输出到文件。
- 通过 `rvsmoke -trend-csv rvwasm-trend.csv` 与 `-trend-chart-json rvwasm-trend-chart.json` ，可将bundle trend保存到易于在外部图表化的CSV / JSON。
- 在Browser UI中添加了 `Minimal repro ZIP` 、 `Inspect repro ZIP` 、 `Repro ZIP JSON` 、 `Matrix workflow YAML` 、 `Trend chart JSON` 、 `Trend CSV` 。

示例:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# 保存检查结果为JSON
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# 将current bundle与previous bundle的trend转化为CSV/JSON
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` 也可以单独运行。若与 `-bundle` 同时指定，则在常规的CI summary中也会包含ZIP inspection结果，如果检查为 `fail` 则CI summary也会视为failure。

### CI matrix聚合 / checksum manifest继续

强化了 `rvsmoke` 的CI artifact交接。

- 通过 `-repro-checksums rvwasm-repro-checksums.json` ，可根据 `-inspect-repro-zip` 的结果将ZIP内文件的deterministic checksum manifest进行保存。
- 指定多个 `-matrix-result name=rvsmoke-output.json` ，能够聚合多个preset / 多个job的 `rvsmoke -out json` 结果。
- 通过 `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` ，可将matrix结果保存为text / JSON / self-contained HTML。
- 通过 `-trend-html rvwasm-trend.html` 可将bundle trend report保存为单体HTML。

示例:

```bash
# 保存最小复现ZIP的内容与checksum manifest
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# 聚合多个matrix job的rvsmoke JSON
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

matrix aggregate汇总了各job的CI status、gate failure/warning数、artifact mismatch、top stop-cause。这是为了在GitHub Actions的matrix job分开时，也能在最后的聚合job中便于查看整体失败趋势的辅助功能。

#### CI / release handoff辅助

强化了 `rvsmoke` 的CI artifact管理与release handoff。

- `-artifact-index rvwasm-artifacts.json` 汇总了JUnit / SARIF / HTML / trend / matrix / repro checksum等已生成的CI artifact的path、bytes、SHA-256。
- `-release-manifest rvwasm-release.json` 将diagnostic bundle、log signature、CI gate、matrix aggregate、flake report、artifact index、repro checksum verification汇总为一个handoff manifest。
- `-release-html rvwasm-release.html` 输出带有navigation的自包含HTML，能够跳转至Summary / Artifacts / Matrix / Checksums / JSON。
- `-verify-repro-checksums baseline-repro-checksums.json` 将当前检查的minimal repro ZIP的checksum manifest与baseline进行比较，检测missing / changed / extra。
- `-matrix-flakes` 、 `-matrix-flakes-json` 、 `-matrix-flakes-html` 正规化了如 `uart#1` / `uart#2` 等多次matrix结果，检测同一preset是否在pass/fail之间波动。

示例:

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

## 发布交接・验证

为将CI结果交接给其他机器、其他仓库或代码审查负责人，在 `rvsmoke` 中添加了metadata输出。

### SBOM / provenance扩展

#### SBOM-lite依赖项列表

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

该依赖项列表旨在采用小型且deterministic的格式。读取 `go.mod` ，并记录module path、Go version、直接的 `require` 行、 `replace` 目标、以及CI artifact index中包含的artifact类型。

当从另一个working directory运行 `rvsmoke` 时，请指定 `-go-mod /path/to/go.mod` 。

#### provenance attestation

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

attestation是参考in-toto / SLSA的JSON payload。这本身并非签名，但因拥有稳定的SHA-256，可用作外部CI tooling的签名对象。

#### 发布交接ZIP

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

release handoff ZIP中仅包含metadata。

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

不嵌入firmware、kernel、initrd、disk image。大型artifact在manifest侧保留SHA-256 pin。

#### 发布交接ZIP的检查

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

inspector在不解压ZIP的情况下，检查必须文件、危险的path、重复的path、JSON是否可parse、以及release / index / SBOM / attestation的基本整合性。

### 发布验证

除了创建release handoff ZIP之外，还添加了用于验证的输出。

- `-verify-attestation` / `-verify-attestation-text` 确认deterministic provenance attestation hash、release materials、CI artifact subjects是否与已生成的release manifest、SBOM-lite inventory、artifact index相匹配。
- `-sbom-baseline` 、 `-sbom-diff` 、 `-sbom-diff-json` 将当前的SBOM-lite dependency inventory与已保存的baseline进行比较。
- `-compare-release-zip-inspection` 、 `-release-zip-compare` 、 `-release-zip-compare-json` 将当前inspected release handoff ZIP与过去的inspection JSON进行比较。
- `-retention-manifest` / `-retention-text` 生成包含path、kind、bytes、SHA-256、retention days、expiry time、reason的CI artifact retention manifest。
- `-release-verification-html` 输出整合了release status、attestation verification、SBOM diff、release ZIP comparison、retention信息且附带navigation的HTML。

示例:

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

### 发布audit gate

在release verification之上添加了最终的release audit layer。将provenance attestation verification、SBOM-lite diff、release ZIP comparison、artifact retention expiry、matrix flake status、release manifest status汇总为1个score与gate report。

主要flag:

- `-list-release-verify-policies` 列出内置的release audit policy。
- `-print-release-verify-policy strict` 输出policy JSON template。
- `-release-verify-template default|strict|lenient|archive` 选择内置policy。
- `-release-verify-policy policy.json` 读取custom release audit policy。
- `-retention-audit` / `-retention-audit-json` 写出expiry与minimum-retention的检查结果。
- `-release-score` / `-release-score-json` 写出0〜100的release verification score。
- `-release-gate` / `-release-gate-json` 写出policy gate result。
- `-release-audit` / `-release-audit-json` / `-release-audit-html` 写出统合的audit report。

示例:

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

strict policy会将非pass的release manifest、失败的attestation / SBOM / ZIP check、已过期的artifact、低于设置的minimum retention days的artifact视为失败。default policy适合nightly handoff等日常确认，在允许warning的同时能将明确的verification failure作为CI失败处理。

#### release audit diff / waiver / TODO交接

`rvsmoke` 的release-audit path支持当前audit与过去audit的比较、对已知issue应用带期限的waiver、以及生成未waiver作业的checklist。

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

waiver template可通过以下命令创建。

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

waiver用于处理已知且暂时的release-audit finding。各rule拥有ID、任意的kind / name / status / substring matcher、owner、reason、 `expires_at` timestamp。过期的waiver将被报告，但不会用于抑制issue。

#### release decision / evidence bundle

添加了在执行release audit后使用的最终交接辅助功能。

- `-waiver-calendar` 、 `-waiver-calendar-json` 、 `-waiver-calendar-html` 可显示各waiver的expiry、owner、match count、expired / expiring-soon状态。
- `-release-changelog` 、 `-release-changelog-json` 将audit diff、waiver state、TODO count、waiver expiry state摘要为便于人类阅读的changelog。
- `-final-decision` 、 `-final-decision-json` 生成包含blocking item与next action的最终 `go` 、 `go-with-watch` 、 `no-go` decision。
- `-release-evidence-zip` 写出包含audit、waiver report、TODO list、waiver calendar、changelog、final decision的小型evidence bundle。
- `-inspect-release-evidence-zip` 在不解压evidence bundle的情况下，检查必须文件、危险的path、重复的entry、JSON是否可parse。
- `-dry-run` 仅计算report，不写出optional output file。
- `-exit-code-mode never` 在通常会因gate failure而失败的情况下，依然输出结果。

示例:

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

在CI中检查evidence bundle的示例:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## 许可证

本项目采用BSD 2-Clause License授权。详情请参阅[LICENSE](../LICENSE)文件。

SPDX-License-Identifier: BSD-2-Clause
