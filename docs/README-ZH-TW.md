# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## 概述

這是一個運行在Go 1.23.2 `GOOS=js GOARCH=wasm` 上的RV64IMAC模擬器。預設是single-hart，但也可以透過UI進行1〜8 hart的協作調度。可以從瀏覽器UI載入OpenSBI 1.8.1的 `fw_payload.bin` / `fw_jump.bin` / `fw_dynamic.bin` / ELF並確認啟動。

![OpenSBI fw_payload在rvwasm上啟動的畫面](images/fw_payload.png)

OpenSBI 1.8.1 `fw_payload.bin` 在rvwasm上啟動，並進入下一階段S-mode payload的範例。

## 已實現

- RV64I基本指令
- M extension
- A extension的LR/SC/AMO最小實現
- C extension的常見整數指令
- 相當於Zicsr / Zifencei
- M/S/U privilege mode的CSR/trap/mret/sret最小實現
  - 將同步異常的 `mepc` / `sepc` 修正為faulting instruction PC
  - 修正faulting load/CSR write，使其不破壞rd，也不推進retire counter
  - CSR存在檢查、抑制read-only CSR寫入side effect、基本反映 `mcounteren` / `scounteren`
  - 為Linux側的probing加入 `senvcfg` / state-enable CSR stub
  - 基本反映 `TVM` / `TW` / `TSR` 和解除 `MPRV`
- Sv39 MMU
  - `satp` mode Bare / Sv39
  - 3段頁表walk
  - 4 KiB / 2 MiB / 1 GiB leaf
  - 基本反映 `SUM` / `MXR` / `MPRV`
  - page fault exception
  - 自動更新PTE `A` / `D` bit
- 類UART 16550風MMIO（`0x10000000`）
  - 來自guest的輸出
  - 從瀏覽器UI進行輸入inject
  - receive interrupt
- 類CLINT風mtime/mtimecmp/msip（`0x02000000`）
  - 面向multi-hart的per-hart MSIP / MTIMECMP routing
- 類PLIC風interrupt controller（`0x0c000000`）
  - priority / pending / enable / threshold
  - claim / complete
  - 每個hart的M/S context
- PMP enforcement
  - TOR / NA4 / NAPOT
  - R/W/X permission
  - 透過locked entry限制M-mode
- 供OpenSBI `fw_dynamic` 使用的boot info
  - 將dynamic info配置在 `0x87dff000`
  - 在 `a2` 中設定dynamic info pointer
  - 可從UI單獨載入S-mode payload / kernel
- virtio-mmio block device（`0x10001000`）
  - modern virtio 1.0 style MMIO register
  - split virtqueue的read/write/flush/get-id最小對應
  - `FEATURES_OK` negotiation與 `VIRTIO_F_VERSION_1` 驗證
  - queue reset、忽略 `DRIVER_OK` 前的notify、基本反映 `NO_INTERRUPT` flag
  - `VIRTIO_RING_F_INDIRECT_DESC` 與indirect descriptor table的處理
  - 依據 `VIRTIO_RING_F_EVENT_IDX` 的used event抑制中斷
  - 可從UI載入disk image
  - 可從UI下載guest修改後的disk image
- virtio-mmio console device（`0x10002000`）
  - device ID 3的最小console
  - queue 0 receive / queue 1 transmit
  - 最小對應 `VIRTIO_CONSOLE_F_SIZE` 、indirect descriptor、event index
  - 將UI輸入inject到UART和virtio-console兩者中
- virtio-mmio net device（`0x10003000`）
  - device ID 1的除錯用最小virtio-net
  - queue 0 receive / queue 1 transmit
  - 最小對應 `VIRTIO_NET_F_MAC` / `VIRTIO_NET_F_STATUS` / indirect descriptor / event index
  - 從UI將Ethernet frame hex進行RX inject
  - 將guest發送的Ethernet frame作為TX log顯示
- virtio-mmio rng device（`0x10004000`）
  - device ID 4的最小entropy source
  - 最小對應split virtqueue、indirect descriptor、event index
  - 可從UI設定deterministic seed
- virtio-mmio input device（`0x10005000`）
  - device ID 18的除錯用最小keyboard/input device
  - 最小對應event queue / status queue、indirect descriptor、event index
  - 可從UI進行key event / raw input event的inject
- virtio-mmio gpu device（`0x10006000`）
  - device ID 16的除錯用2D virtio-gpu foundation
  - 最小對應control / cursor queue、indirect descriptor、event index
  - 針對 `GET_DISPLAY_INFO` / `RESOURCE_CREATE_2D` / `SET_SCANOUT` / `FLUSH` 等的基本回應
  - 用於觀測Linux的virtio-gpu probe和初始modeset command
- initrd / initramfs傳遞
  - default load address: `0x84000000`
  - 反映到自動生成DTB的 `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end`
- bootargs編輯
  - default: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - 供UART / virtio-console / initramfs / verbose debug使用的preset
  - 從UI設定並反映到自動生成DTB中
- 執行追蹤ring buffer
  - 可在UI中查看PC / 指令 / trap / 最後的trap cause/tval
  - 可從UI進行CSR dump和所有hart trace snapshot的text / JSON / CSV export
  - 彙整顯示最後ECALL/SBI參數、SBI BASE/TIME/IPI/RFENCE/HSM/SRST/legacy counters、trap、virtio queue狀態的Diagnostics
  - Diagnostics / device state的JSON export
  - 載入ELF / System.map symbols，支援顯示停止PC周圍的symbol、名稱搜尋、自動解析panic/oops日誌內PC的symbol
  - 用於直接測試無OpenSBI的小型S-mode payload的任意SBI shim
    - BASE / TIME / IPI / RFENCE / HSM / SRST的最小short-circuit
    - 透過HSM `hart_start` 啟動目標hart的S-mode入口的除錯路徑
    - 預設無效。在運行OpenSBI的常規路徑中不使用
  - 可從UI對任意物理記憶體範圍進行dump
  - 可從UI設定PC breakpoint、物理read/write watchpoint、trace filter
  - breakpoint可指定hit count、mode條件、hart條件
  - trace在raw instruction之外顯示簡化的decode mnemonic
  - breakpoint / watchpoint hit時將stop reason記錄到status / diagnostics / trace export
  - 收集MMIO/DRAM access histogram，可在Diagnostics / JSON中確認device probe或queue activity的偏差
  - 將MMIO/DRAM access timeline保存到ring buffer中，可用raw / compact顯示確認probe的時間序列
  - 在MMIO access timeline中附加virtio-mmio / UART / CLINT / PLIC的register decoder名稱，能夠以 `QueueNotify` / `Status` / `LSR` 等為單位進行確認
  - 可選擇啟用CSR access trace，在Diagnostics / trace export中顯示guest的CSR read/write tail及各CSR的read/write summary
  - 可選擇啟用PC hot-spot profile，帶有symbol確認停止前執行次數較多的PC
  - 透過diagnostic snapshot capture / diff，可在UI中確認執行前後的hart/device/CSR/MMIO狀態差異
  - 在compact trace顯示中可折疊連續的同類指令・trap・ECALL日誌
  - 依據各boot preset的smoke runner，自動執行目前已載入firmware/payload指定的hart-step，並獲取結果JSON
  - 在boot phase analyzer中統一顯示OpenSBI / Linux / panic / virtio activity / trap / PC symbol
  - 在boot timeline中將console marker與MMIO probe / status / QueueNotify / PLIC claim整合為時間序列顯示
  - 在device probe analyzer中統計virtio/UART/PLIC/CLINT的read/write、identity register、status negotiation、queue notify
  - 在virtqueue inspector中按device/queue顯示QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify的最近狀態
  - 在descriptor chain visualizer中從avail ring追蹤head descriptor，顯示NEXT / WRITE / INDIRECT descriptor及小型buffer preview
  - 在descriptor chain graph export中可將virtqueue chain保存・視覺化為Graphviz DOT
  - 在guest physical memory scanner中檢測DRAM內類似ELF / FDT / gzip / xz / zstd / squashfs / cpio / ext magic / OpenSBI / Linux version / BusyBox / kernel cmdline的區域
  - 在initcall / driver probe classifier中對Linux console log的initcall、probe、virtio、storage、console、network、graphics相關行進行分類
  - 在initcall timeline中以時間序列分組顯示已分類的initcall / driver probe行
  - 載入帶symbol的ELF的DWARF line table，顯示目前PC附近的file:line、DWARF file summary、trace PC的symbol+line註釋
  - 在panic summary中自動擷取console log內的panic/oops/fault周邊行，用已載入的symbols解析address
  - 透過boot analysis JSON一次性export timeline / device probe / virtqueue / panic summary
  - 在trace replay report中摘要trace的step/trap/ecall/SBI shim件數、hot mnemonic、trap cause
  - 在trace baseline compare中將上次保存的trace與目前trace的PC/指令/trap差異從頭開始比對
  - 可在瀏覽器localStorage中保存/載入trace baseline
  - 除boot regression report/JSON外，可透過Markdown/HTML report export統一保存trace stats、boot events、device probe、virtqueue、memory object、initcall counts
  - 在virtqueue snapshot中同時顯示queue setup與descriptor chain
  - 在virtqueue anomaly detector中檢測ready queue的address缺失、descriptor loop、indirect長度錯誤、DRAM外buffer等
  - 在virtqueue anomaly hints中按檢測結果顯示QueueNum / QueueDesc / QueueReady / descriptor alignment等修復提示
  - 透過integrated diagnostic query，使用相同query橫向搜尋console / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory index
  - 使用diagnostic query presets一次性搜尋panic、virtio negotiation、QueueReady/Notify、satp/mstatus、trap、rootfs相關內容
  - 透過share report MD/JSON/HTML，以自包含形式分享boot regression、virtqueue hints/triage、memory index、query presets、jump hints、query hits
  - 在triage dashboard / stop-cause ranking中按候補順序顯示panic、trap、page/access fault、virtqueue anomaly、device probe停滯
  - 在stop-cause evidence中顯示排名依據、score breakdown、推薦diagnostic query、下一步行動
  - 將triage dashboard baseline保存到localStorage，對比目前dashboard與status/phase/device/anomaly/memory counts
  - 將diagnostic preset baseline保存到localStorage，對比與目前preset hit數的差異
  - 透過redacted share report MD/JSON/HTML輸出隱藏了IP/MAC/email的分享用報告
  - 在redaction options JSON中從UI調整是否替換IP/MAC/email/long hex address
  - 在memory object dump中用hex + ASCII確認memory index/search的hit周邊
  - 在memory range dump中指定任意DRAM address與byte length進行hex + ASCII dump / JSON export
  - 在memory scan snapshot/diff中確認執行前後增減的ELF/FDT/initrd/rootfs片段候補
  - 在memory index中將相鄰的ELF/FDT/initrd/kernel/rootfs signature按範圍彙整並索引化
  - 從UART / virtio-console輸出中擷取類似Linux `dmesg` 的日誌，用已載入的symbols解析panic/oops address
- simple-framebuffer
  - 將 `0x86000000` , 1024x768, `a8r8g8b8` 加入到自動生成DTB的 `/chosen/framebuffer@86000000`
  - 在UI的Canvas繪製framebuffer，可下載RGBA raw dump與PNG
  - 在 `TRANSFER_TO_HOST_2D` / `RESOURCE_FLUSH` 時將virtio-gpu的2D resource backing複製到simple-framebuffer
- DRAM `0x80000000` , 128 MiB
- 自動生成帶virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT的最小virt DTB，或從UI載入DTB
  - 加入 `sifive,plic-1.0.0` / `sifive,clint0` compatible與virtio `dma-coherent`
  - 根據Hart count生成 `cpu@N` 與 `interrupts-extended`

## 使用方法

```bash
make serve
```

在瀏覽器中開啟 `http://localhost:8080` ，選擇OpenSBI的firmware後點擊 `Load firmware` → `Run` 。

若要將virtio-console作為Linux console進行測試，可將bootargs修改為 `console=hvc0 earlycon=sbi root=/dev/vda rw` 等。預設仍按常規使用UART（`ttyS0`）。

解析停止PC時，請在 `Load symbols` 中載入Linux的 `System.map` 或帶symbol的ELF後，使用 `Symbols @ PC` / `Diagnostics` / `Search symbols` 。如果帶symbol的ELF包含DWARF line table，也可以在 `DWARF lines @ PC` 中確認file:line。 `DWARF file summary` 顯示line table中包含的各個檔案的行數。如果firmware/payload是帶symbol的ELF，也會自動匯入symbol table。 `Annotated trace` 會用symbols/DWARF line在trace內的 `pc=` 處進行註釋。可透過 `Download trace` 保存所有hart的trace snapshot。也可選擇JSON/CSV格式。JSON trace中如果存在symbols則會包含symbol/source資訊。在 `Trace filter` 中輸入 `trap` 、 `ecall` 、 `sbi-shim` 、 `pc=` 、 `virtio` 等字串可以過濾trace tail/export、access timeline、compact顯示。 `Compact trace` 會折疊連續的同類指令・trap・ECALL。貼上panic/oops日誌並點擊 `Analyze log symbols` ，將用已載入的symbols解析日誌中類似64-bit PC的位址。

`Trace replay report` 會統計目前的trace， `Trace baseline compare` 貼上已保存的trace與目前trace的PC/指令/trap差異從頭開始比對。 `Save current trace as baseline` / `Load saved baseline` 會在瀏覽器localStorage中保存baseline。 `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` 是彙整了boot timeline、device probe、virtqueue、memory scanner、initcall分類、trace統計的回歸確認用報告。透過 `Capture memory scan` →執行→ `Diff memory scan` ，也能確認guest memory內ELF/FDT/initrd/rootfs候補的增減。 `DWARF source context` 會將目前PC周邊的symbol與DWARF file:line合併顯示。

`Boot phase` 從console log、MMIO histogram、trap、symbol資訊摘要目前的進度。 `Boot timeline` 將console的milestone與MMIO的probe / status / QueueNotify按時間序列排列。 `Device probe` 統計virtio等的register access與negotiation， `Virtqueue inspect` 按device/queue顯示queue setup與notify狀態。 `Descriptor chains` 從queue的avail ring讀取descriptor chain，並顯示indirect descriptor或buffer preview。 `Descriptor DOT` / `Download DOT` 將該chain作為Graphviz DOT輸出。 `Virtqueue anomalies` 檢測queue setup或descriptor chain的不一致， `Anomaly hints` 針對各不一致顯示下一步檢查點。 `Integrated diagnostic query` 用 `virtio QueueReady` 、 `panic` 、 `satp` 、 `0x80200000` 等詞彙橫向搜尋console / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory index。 `Share report MD/JSON/HTML` 是在boot regression report中加入anomaly hints/triage、memory index、memory jump hints、query presets、query hits的分享用bundle。HTML可作為內嵌了JSON的自包含檔案保存。 `Diagnostic query presets` 集中搜尋panic、virtio status、QueueReady/QueueNotify、satp/mstatus、trap、rootfs相關內容。 `Save query` / `Load query` 將diagnostic query保存到browser localStorage。 `Memory scan` 在DRAM內尋找ELF/FDT/initrd/kernel/rootfs片段候補， `Memory index` 將相鄰的signature按範圍彙整。 `Memory search` 用字串或 `0x...` address搜尋memory index， `Memory jumps` 顯示ELF/FDT/Linux/OpenSBI/cmdline/rootfs等有用的跳轉目標候補。 `Initcall classifier` / `Initcall timeline` 對Linux的initcall/driver probe風日誌進行分類並按時間序列化。 `Panic summary` 擷取panic/oops/fault周邊行，如有symbols則解析address。 `Boot analysis JSON` 將這些內容集中保存。 `Dmesg extract` 從UART / virtio-console輸出中僅擷取類似Linux風格的行。 `Decoded MMIO` 附帶register名稱顯示最近的MMIO access。

`Triage dashboard` 將stop-cause ranking、virtqueue anomaly severity、device probe、query bookmarks整合為一螢幕用的文字。 `Stop-cause ranking` 從console/trace/status擷取kernel panic、oops、illegal instruction、page/access fault、virtqueue異常、device probe停滯並帶優先級排序。 `Stop-cause evidence` 顯示排名依據、score breakdown、推薦query、下一步檢查點。透過 `Save triage baseline` →執行→ `Triage diff` 可比對dashboard的status/phase/device/anomaly/memory counts差異。透過 `Save preset baseline` →執行→ `Compare preset baseline` ，可確認panic/virtio/satp/rootfs等preset query hit數相比上次是否有所增減。 `Memory dump hits` 使用diagnostic query或trace filter對memory index hit周邊進行hex/ASCII dump。 `Memory range dump` 指定任意address/length直接對DRAM進行hex/ASCII dump。 `Redacted share MD/JSON/HTML` 在分享前將email / MAC / IPv4替換為 `<email>` / `<mac>` / `<ipv4>` 。在 `Redaction options JSON` 中可切換 `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex` 。

`Smoke preset` 重置選取的boot preset並僅執行指定step。 `Smoke matrix` 依次執行如 `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` 等preset list，並列出各preset的執行step、最後phase、stop-cause候補。

PC breakpoint透過在 `Breakpoints / watchpoints` 的 `PC breakpoint` 填入物理/虛擬PC的hex值添加。 `Run` / `Step 1k` 在breakpoint處停止， `Step` 即使目前PC是breakpoint也只步進1條指令。write watchpoint是針對物理位址範圍的bus write、read watchpoint是檢測bus read的簡易功能。可用於檢查MMIO probe或framebuffer寫入、特定結構體的參照。 `Access timeline` / `Compact access` 將最近的DRAM/MMIO access按時間序列・壓縮顯示。 `PC profile on` 統計hot PC，透過 `Capture snapshot` →執行→ `Diff snapshot` 可確認執行前後的診斷差異。

simple-framebuffer在 `0x86000000` 準備1024x768x32bpp的記憶體，並作為 `simple-framebuffer` 載入自動生成DTB的 `/chosen/framebuffer@86000000` 。若Linux側可使用simplefb，則可透過 `Render framebuffer` 顯示在Canvas上。

virtio-net並非讓瀏覽器連接真實網路，而是packet-level的除錯設備。在 `virtio-net debug` 中輸入Ethernet frame的hex並注入RX，guest發送到TX queue的frame在 `Show TX frames` 中確認。若要在Linux側識別，根據需要在guest側執行 `ip link set dev eth0 up` 等。

virtio-rng是作為guest entropy source展示deterministic PRNG的驗證用設備。為保持可重現性預設seed是固定的，可從UI的 `Set deterministic seed` 進行更改。

virtio-gpu是為觀測Linux的virtio-gpu driver probe與2D resource setup的最小設備。並非真正的GPU acceleration，而是追蹤進入control queue的modeset / scanout / flush系命令，並在Diagnostics中輸出狀態。由於也執行從resource backing memory到simple-framebuffer的複製，可透過 `Render framebuffer` / PNG export確認guest對2D resource進行flush的結果。cursor queue的 `UPDATE_CURSOR` / `MOVE_CURSOR` 也會作為狀態記錄。

`SBI shim on` 用於不使用OpenSBI直接運行S-mode payload的除錯。在通常的 `fw_dynamic.bin` / `fw_payload.bin` 實驗中請保持無效。

若要嘗試Multi-hart，請在載入firmware前設定 `Hart count` 。更改設定會伴隨machine reset，因此前提是更改後重新載入firmware / payload / disk。在 `View hart` 中可切換顯示目標hart的register / CSR / trace。

若使用 `fw_dynamic.bin` ，根據需要在 `Load payload` 中將S-mode payload / kernel載入到 `0x80200000` 附近。模擬器將dynamic info放置在 `0x87dff000` ，並在 `a2` 中設定其位址。

在Linux實驗中可使用以下任意一種:

- `Load disk`: 將rootfs等raw disk image作為virtio-blk傳遞。預設bootargs為 `root=/dev/vda rw` 。
- `Load initrd`: 將initramfs放置在 `0x84000000` ，並在自動生成DTB中反映initrd範圍。如果需要，將bootargs修改為 `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` 等。

使用OpenSBI 1.8.1已發布的RISC-V二進位檔案的範例:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# 將解壓後的 fw_dynamic.bin / fw_payload.bin / fw_jump.bin 等從瀏覽器載入
```

若是本地構建OpenSBI，請準備 `riscv64-unknown-elf-` 等RISC-V toolchain並使用 `PLATFORM=generic` 進行構建。

### 開發用指令

```bash
go test ./...
make wasm
make serve
```

## 注意

該實現分階段具備了調查OpenSBI初始化、S-mode payload遷移、Linux boot所需的功能。面向Linux boot雖加入了PMP、Sv39、virtio-blk、virtio-console、virtio-net、virtio-rng、virtio-input、virtio-gpu、initrd、simple-framebuffer、trap/CSR精度、CSR trace/summary、MMIO histogram/timeline/register decoder、boot phase/timeline analyzer、device probe analyzer、virtqueue inspector/descriptor chain visualizer/DOT export/snapshot/anomaly detector/anomaly hints/anomaly triage、guest memory scanner/index/diff/search/jump hints/dump helper、integrated diagnostic query/query presets/preset baseline comparison、triage dashboard/stop-cause ranking、share report bundle/HTML/redaction、initcall classifier/timeline、DWARF line lookup/source context/trace annotation、panic summary、dmesg extractor、trace replay/compare、boot regression report、PC profile、snapshot diff、boot smoke runner/smoke matrix、triage baseline diff、stop-cause evidence、editable redaction、memory range dump，但主要未實現・簡化的部分為準確的cycle/time模型、AIA/IMSIC、tap/WebSocket bridge等真實網路連接、真正的virgl/DRM/GPU acceleration、所有CSR的嚴格WARL/WPRI動作、使用多個worker的真正平行執行。multi-hart是單一wasm worker內的協作調度。

## 診斷・回歸輔助

強化了smoke matrix與診斷查詢的分享性。

- `Smoke matrix MD/HTML` 將smoke matrix結果保存為Markdown / 自包含HTML。
- 透過 `Save smoke baseline` →執行→ `Compare smoke baseline` 可確認各preset的phase、執行step、top stop-cause的差異。
- `Stop checklist` 根據stop-cause ranking，將下一步需要確認的具體操作項目轉化為檢查清單。
- `CSR/MMIO bookmarks` 從integrated diagnostic query的結果中僅擷取CSR / MMIO / trace的重要命中。
- `Watchpoint hits` 按時間序列顯示read/write watchpoint的hit歷史。透過 `Clear hit timeline` 可僅清除歷史。
- `Artifact manifest` 將目前已載入的firmware / payload / disk / initrd / symbols及生成DTB / dynamic info的range、entry、SHA-256進行列表化。

### regression handoff輔助

- `Manifest diff` / `Manifest diff JSON` 將目前的boot artifact manifest與保存在localStorage的baseline進行比較，顯示bootargs、hart數、load range、entry、ELF判定、SHA-256的差異。
- `Auto break/watch suggestions` 根據stop-cause evidence、最近的trace PC、watchpoint hit timeline，生成下次執行時應設定的PC breakpoint / read watchpoint / write watchpoint候補。
- `Smoke clusters` / `Smoke clusters JSON` 按照phase與top stop-cause對smoke matrix的preset結果進行聚類，將相同失敗類型的preset歸納在一起。
- `Diagnostic bundle JSON` 是彙整了manifest、triage dashboard、stop-cause、breakpoint suggestions、share bundle、watchpoint hits的自包含JSON。
- `Compressed bundle JSON` 是將上述diagnostic bundle轉化為gzip+base64。用於在貼到issue或chat前縮小體積。

### handoff / provenance輔助

- `Decode bundle` 貼上並解壓 `Diagnostic bundle JSON` 或 `Compressed bundle JSON` ，或gzip+base64本體。
- `Bundle compare` / `Bundle compare JSON` 將貼上的過去bundle與目前bundle比較，顯示triage phase、top stop-cause、manifest、artifact hash、smoke cluster、watchpoint hit數、suggestion數的差異。
- `Provenance` / `Provenance JSON` 彙整manifest、trace、console、diagnostic bundle的SHA-256、trace line數、console byte數、top stop-cause。可用作可重現性確認或附加到issue的依據。
- `Handoff MD` 將provenance、top stop-cause、auto break/watch suggestions、stop checklist、baseline diff、artifact manifest彙整為Markdown。
- `Apply auto breaks` 將auto break/watch suggestions的高位候補一次性應用到目前的emulator中。用於再次執行前快速設定停止位置或可疑的MMIO/DRAM範圍的輔助。

### reproduction / signature / headless handoff

- `Repro plan` / `Repro MD` / `Repro JSON`
  - 根據diagnostic bundle、provenance、artifact manifest生成復現步驟。
  - 將firmware / payload / initrd / disk / symbols的role、size、load range、SHA-256作為artifact pin列出。
  - 將smoke preset、bootargs、hart count、next_addr、推薦break/watch條件步驟化。
- `Log signature` / `Log signature JSON`
  - 將trace / console / manifest的SHA-256、trace line count、first/last PC、console first/last line、hot token輕量化為summary。
  - 不貼上raw trace也能比較「是否是同一個日誌」「哪裡發生了變化」。
- `Save log signature` / `Load log signature` / `Compare log signature`
  - 在browser localStorage中保存log signature baseline，並與目前的signature比較。
  - 顯示trace hash、console hash、manifest hash、line count、last PC、last console line的差異。
- `Auto break verify`
  - 顯示應用auto breakpoint/watchpoint suggestion前後的確認用summary。
  - 針對duplicate suggestion或可疑的PC range發出warning。
- `Headless smoke script`
  - 根據目前的artifact manifest、bootargs、hart count、smoke presets、step count生成供CI/handoff使用的shell script skeleton。
  - 作為向執行環境添加Playwright等browser harness的前置階段，用途是固定artifact pin與preset matrix。

#### headless / CI輔助

為了讓repro/signature handoff在CI或issue中易於處理，增加了以下功能。

- `Bundle integrity` / `Integrity JSON` 檢查diagnostic bundle與artifact manifest的整合性，將artifact role、SHA-256、load range、suggestion、smoke result的不一致分類為 `error` / `warn` / `info` 。
- `Repro validation` / `Repro validation JSON` 確認目前的reproduction plan是否與bundle的bootargs、hart count、next_addr、artifact pins、top stop-cause、log signature一致。
- `CI summary` / `CI summary JSON` 彙整bundle integrity、trace/console signature、smoke result、stop-cause，輸出在CI中便於判定pass/warn/fail的summary。
- `Headless runner spec` / `Runner spec JSON` 生成供 `go run ./cmd/rvsmoke ...` 檢查用的preset、steps、artifact pin、推薦command。
- 增加了 `cmd/rvsmoke` 。可在瀏覽器外讀取diagnostic bundle / artifact manifest，以text / JSON / Markdown輸出artifact hash、bundle integrity、CI summary、runner spec。

範例:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

`rvsmoke` 目前負責對bundle/manifest與artifact hash進行復現性檢查及CI summary生成。CPU執行本身仍繼續使用瀏覽器js/wasm側的smoke matrix。

#### rvsmoke CI gate / JUnit / SARIF

`cmd/rvsmoke` 是在CI側檢查由瀏覽器export出的diagnostic bundle / manifest的輔助CLI。由於headless執行的實體化，可輸出baseline bundle比較、CI gate policy、JUnit XML、SARIF、self-contained HTML report。

範例:

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

policy JSON的範例:

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

`-out html` 向標準輸出自包含HTML， `-out junit` 為JUnit XML， `-out sarif` 為SARIF JSON。若同時指定 `-junit` / `-html` / `-sarif` ，除標準輸出的格式外也會保存至各檔案。CI gate將artifact manifest、trace/console signature、baseline diff、virtqueue anomaly、smoke result彙整並正規化為 `pass` / `warn` / `fail` 。

#### rvsmoke policy templates / bundle trend compare

為了方便CI gate的初步引入與多次回歸比較，在 `rvsmoke` 及瀏覽器UI中增加了policy template、action checklist、bundle trend compare。

- `CI policy templates` / `Policy templates JSON` 顯示 `default` 、 `strict` 、 `linux-boot` 、 `artifact-only` 、 `lenient` 等內建policy。
- `Policy template JSON` 將指定的template直接保存為可放入CI的JSON。
- `CI gate` / `CI gate JSON` 對目前的browser狀態應用policy template，顯示pass/warn/fail的gate check。
- `CI checklist` / `CI checklist JSON` 根據gate failure、bundle integrity、artifact diff，將下一步需要確認的項目轉化為checklist。
- `rvsmoke -compare name=bundle.json` 將多個bundle按時間序列排列，將phase、top stop-cause、artifact hash、smoke cluster的變化作為trend report輸出。

Policy template的生成範例:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

多個bundle的比較範例:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

未指定 `-policy` 時， `-policy-template` 作為default policy使用。若指定了 `-policy` ，則優先使用檔案的JSON。

## rvsmoke CI連動

擴展了 `rvsmoke` 的CI/handoff輔助功能。

- 能夠透過 `rvsmoke -print-github-actions linux-boot` 生成GitHub Actions workflow YAML。
- 能夠透過 `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` 將workflow輸出到檔案。
- 能夠透過 `rvsmoke -policy-tree policy-tree.md` 將CI gate / bundle integrity / baseline drift作為原因樹保存。
- 能夠透過 `rvsmoke -history history.txt` 保存多個bundle trend的phase / stop-cause / artifact drift統計。
- 能夠透過 `rvsmoke -repro-zip rvwasm-minimal-repro.zip` ，生成包含README、diagnostic bundle、manifest、runner spec、policy、CI summary、驗證script的最小復現包。

範例:

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

`-repro-zip` 不打包raw firmware/kernel/disk。前提是將bundle內的SHA-256 pin與manifest range一同打包，在分享對象處核對artifact。

### CI復現ZIP檢查 / matrix workflow繼續

在 `rvsmoke` 及browser UI中，增加了檢查最小復現包交接的功能，以及用於GitHub Actions matrix / trend視覺化的輸出。

- 透過 `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` ，無需解壓即可檢查 `-repro-zip` 生成的ZIP。確認必須檔案、unsafe path、 `diagnostic-bundle.json` / `manifest.json` 的一致性、 `ci-policy.json` 、 `scripts/rvsmoke.sh` 。
- 透過 `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` ，可針對各個preset生成GitHub Actions matrix workflow YAML。
- 透過 `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` 可將matrix workflow輸出到檔案。
- 透過 `rvsmoke -trend-csv rvwasm-trend.csv` 與 `-trend-chart-json rvwasm-trend-chart.json` ，可將bundle trend保存到易於在外部圖表化的CSV / JSON。
- 在Browser UI中增加了 `Minimal repro ZIP` 、 `Inspect repro ZIP` 、 `Repro ZIP JSON` 、 `Matrix workflow YAML` 、 `Trend chart JSON` 、 `Trend CSV` 。

範例:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# 保存檢查結果為JSON
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# 將current bundle與previous bundle的trend轉化為CSV/JSON
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` 也可以單獨運行。若與 `-bundle` 同時指定，則在常規的CI summary中也會包含ZIP inspection結果，如果檢查為 `fail` 則CI summary也會視為failure。

### CI matrix聚合 / checksum manifest繼續

強化了 `rvsmoke` 的CI artifact交接。

- 透過 `-repro-checksums rvwasm-repro-checksums.json` ，可根據 `-inspect-repro-zip` 的結果將ZIP內檔案的deterministic checksum manifest進行保存。
- 指定多個 `-matrix-result name=rvsmoke-output.json` ，能夠聚合多個preset / 多個job的 `rvsmoke -out json` 結果。
- 透過 `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` ，可將matrix結果保存為text / JSON / self-contained HTML。
- 透過 `-trend-html rvwasm-trend.html` 可將bundle trend report保存為單體HTML。

範例:

```bash
# 保存最小復現ZIP的內容與checksum manifest
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# 聚合多個matrix job的rvsmoke JSON
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

matrix aggregate彙整了各job的CI status、gate failure/warning數、artifact mismatch、top stop-cause。這是為了在GitHub Actions的matrix job分開時，也能在最後的聚合job中便於查看整體失敗趨勢的輔助功能。

#### CI / release handoff輔助

強化了 `rvsmoke` 的CI artifact管理與release handoff。

- `-artifact-index rvwasm-artifacts.json` 彙整了JUnit / SARIF / HTML / trend / matrix / repro checksum等已生成的CI artifact的path、bytes、SHA-256。
- `-release-manifest rvwasm-release.json` 將diagnostic bundle、log signature、CI gate、matrix aggregate、flake report、artifact index、repro checksum verification彙整為一個handoff manifest。
- `-release-html rvwasm-release.html` 輸出帶有navigation的自包含HTML，能夠跳轉至Summary / Artifacts / Matrix / Checksums / JSON。
- `-verify-repro-checksums baseline-repro-checksums.json` 將目前檢查的minimal repro ZIP的checksum manifest與baseline進行比較，檢測missing / changed / extra。
- `-matrix-flakes` 、 `-matrix-flakes-json` 、 `-matrix-flakes-html` 正規化了如 `uart#1` / `uart#2` 等多次matrix結果，檢測同一preset是否在pass/fail之間波動。

範例:

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

## 發布交接・驗證

為將CI結果交接給其他機器、其他倉庫或程式碼審查負責人，在 `rvsmoke` 中增加了metadata輸出。

### SBOM / provenance擴展

#### SBOM-lite相依性列表

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

該相依性列表旨在採用小型且deterministic的格式。讀取 `go.mod` ，並記錄module path、Go version、直接的 `require` 行、 `replace` 目標、以及CI artifact index中包含的artifact類型。

當從另一個working directory運行 `rvsmoke` 時，請指定 `-go-mod /path/to/go.mod` 。

#### provenance attestation

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

attestation是參考in-toto / SLSA的JSON payload。這本身並非簽章，但因擁有穩定的SHA-256，可用作外部CI tooling的簽章對象。

#### 發布交接ZIP

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

release handoff ZIP中僅包含metadata。

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

不嵌入firmware、kernel、initrd、disk image。大型artifact在manifest側保留SHA-256 pin。

#### 發布交接ZIP的檢查

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

inspector在不解壓ZIP的情況下，檢查必須檔案、危險的path、重複的path、JSON是否可parse、以及release / index / SBOM / attestation的基本整合性。

### 發布驗證

除了建立release handoff ZIP之外，還增加了用於驗證的輸出。

- `-verify-attestation` / `-verify-attestation-text` 確認deterministic provenance attestation hash、release materials、CI artifact subjects是否與已生成的release manifest、SBOM-lite inventory、artifact index相匹配。
- `-sbom-baseline` 、 `-sbom-diff` 、 `-sbom-diff-json` 將目前的SBOM-lite dependency inventory與已保存的baseline進行比較。
- `-compare-release-zip-inspection` 、 `-release-zip-compare` 、 `-release-zip-compare-json` 將目前inspected release handoff ZIP與過去的inspection JSON進行比較。
- `-retention-manifest` / `-retention-text` 生成包含path、kind、bytes、SHA-256、retention days、expiry time、reason的CI artifact retention manifest。
- `-release-verification-html` 輸出整合了release status、attestation verification、SBOM diff、release ZIP comparison、retention資訊且附帶navigation的HTML。

範例:

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

### 發布audit gate

在release verification之上增加了最終的release audit layer。將provenance attestation verification、SBOM-lite diff、release ZIP comparison、artifact retention expiry、matrix flake status、release manifest status彙整為1個score與gate report。

主要flag:

- `-list-release-verify-policies` 列出內建的release audit policy。
- `-print-release-verify-policy strict` 輸出policy JSON template。
- `-release-verify-template default|strict|lenient|archive` 選擇內建policy。
- `-release-verify-policy policy.json` 讀取custom release audit policy。
- `-retention-audit` / `-retention-audit-json` 寫出expiry與minimum-retention的檢查結果。
- `-release-score` / `-release-score-json` 寫出0〜100的release verification score。
- `-release-gate` / `-release-gate-json` 寫出policy gate result。
- `-release-audit` / `-release-audit-json` / `-release-audit-html` 寫出統合的audit report。

範例:

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

strict policy會將非pass的release manifest、失敗的attestation / SBOM / ZIP check、已過期的artifact、低於設定的minimum retention days的artifact視為失敗。default policy適合nightly handoff等日常確認，在允許warning的同時能將明確的verification failure作為CI失敗處理。

#### release audit diff / waiver / TODO交接

`rvsmoke` 的release-audit path支援目前audit與過去audit的比較、對已知issue應用帶期限的waiver、以及生成未waiver作業的checklist。

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

waiver template可透過以下指令建立。

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

waiver用於處理已知且暫時的release-audit finding。各rule擁有ID、任意的kind / name / status / substring matcher、owner、reason、 `expires_at` timestamp。過期的waiver將被報告，但不會用於抑制issue。

#### release decision / evidence bundle

增加了在執行release audit後使用的最終交接輔助功能。

- `-waiver-calendar` 、 `-waiver-calendar-json` 、 `-waiver-calendar-html` 可顯示各waiver的expiry、owner、match count、expired / expiring-soon狀態。
- `-release-changelog` 、 `-release-changelog-json` 將audit diff、waiver state、TODO count、waiver expiry state摘要為便於人類閱讀的changelog。
- `-final-decision` 、 `-final-decision-json` 生成包含blocking item與next action的最終 `go` 、 `go-with-watch` 、 `no-go` decision。
- `-release-evidence-zip` 寫出包含audit、waiver report、TODO list、waiver calendar、changelog、final decision的小型evidence bundle。
- `-inspect-release-evidence-zip` 在不解壓evidence bundle的情況下，檢查必須檔案、危險的path、重複的entry、JSON是否可parse。
- `-dry-run` 僅計算report，不寫出optional output file。
- `-exit-code-mode never` 在通常會因gate failure而失敗的情況下，依然輸出結果。

範例:

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

在CI中檢查evidence bundle的範例:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## 授權

本專案採用BSD 2-Clause License授權。詳情請參閱[LICENSE](../LICENSE)檔案。

SPDX-License-Identifier: BSD-2-Clause
