# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## 概要

Go 1.23.2 `GOOS=js GOARCH=wasm` で動く、RV64IMACエミュレータです。既定はsingle-hartですが、UIから1〜8 hartの協調スケジューリングもできます。OpenSBI 1.8.1の `fw_payload.bin` / `fw_jump.bin` / `fw_dynamic.bin` / ELFをブラウザUIからロードして起動確認できます。

![rvwasm上でOpenSBI fw_payloadが起動している画面](images/fw_payload.png)

rvwasm上でOpenSBI 1.8.1 `fw_payload.bin` が起動し、次段のS-mode payloadへ入っている例です。

## 実装済み

- RV64I基本命令
- M extension
- A extensionのLR/SC/AMO最小実装
- C extensionの一般的な整数命令
- Zicsr / Zifencei相当
- M/S/U privilege modeのCSR/trap/mret/sret最小実装
  - 同期例外の `mepc` / `sepc` をfaulting instruction PCに補正
  - faulting load/CSR writeがrdを壊さず、retire counterも進めないよう補正
  - CSR存在チェック、read-only CSR書き込みside effect抑止、 `mcounteren` / `scounteren` の基本反映
  - Linux側のprobing用に `senvcfg` / state-enable CSR stubを追加
  - `TVM` / `TW` / `TSR` と `MPRV` 解除の基本反映
- Sv39 MMU
  - `satp` mode Bare / Sv39
  - 3段ページテーブルwalk
  - 4 KiB / 2 MiB / 1 GiB leaf
  - `SUM` / `MXR` / `MPRV` の基本反映
  - page fault exception
  - PTE `A` / `D` bitの自動更新
- UART 16550風MMIO（`0x10000000`）
  - guestからの出力
  - ブラウザUIからの入力inject
  - receive interrupt
- CLINT風mtime/mtimecmp/msip（`0x02000000`）
  - multi-hart用のper-hart MSIP / MTIMECMP routing
- PLIC風interrupt controller（`0x0c000000`）
  - priority / pending / enable / threshold
  - claim / complete
  - hartごとのM/S context
- PMP enforcement
  - TOR / NA4 / NAPOT
  - R/W/X permission
  - locked entryによるM-mode制限
- OpenSBI `fw_dynamic` 用boot info
  - dynamic infoを `0x87dff000` に配置
  - `a2` にdynamic info pointerを設定
  - UIからS-mode payload / kernelを別ロード可能
- virtio-mmio block device（`0x10001000`）
  - modern virtio 1.0 style MMIO register
  - split virtqueueのread/write/flush/get-id最小対応
  - `FEATURES_OK` negotiationと `VIRTIO_F_VERSION_1` 検証
  - queue reset、 `DRIVER_OK` 前notifyの無視、 `NO_INTERRUPT` flagの基本反映
  - `VIRTIO_RING_F_INDIRECT_DESC` とindirect descriptor tableの処理
  - `VIRTIO_RING_F_EVENT_IDX` のused eventによる割り込み抑制
  - UIからdisk imageをロード可能
  - guestが書き換えたdisk imageをUIからダウンロード可能
- virtio-mmio console device（`0x10002000`）
  - device ID 3の最小console
  - queue 0 receive / queue 1 transmit
  - `VIRTIO_CONSOLE_F_SIZE` 、indirect descriptor、event indexの最小対応
  - UI入力をUARTとvirtio-consoleの両方へinject
- virtio-mmio net device（`0x10003000`）
  - device ID 1のデバッグ用最小virtio-net
  - queue 0 receive / queue 1 transmit
  - `VIRTIO_NET_F_MAC` / `VIRTIO_NET_F_STATUS` / indirect descriptor / event indexの最小対応
  - UIからEthernet frame hexをRX inject
  - guestから送信されたEthernet frameをTX logとして表示
- virtio-mmio rng device（`0x10004000`）
  - device ID 4の最小entropy source
  - split virtqueue、indirect descriptor、event indexの最小対応
  - deterministic seedをUIから設定可能
- virtio-mmio input device（`0x10005000`）
  - device ID 18のデバッグ用最小keyboard/input device
  - event queue / status queue、indirect descriptor、event indexの最小対応
  - UIからkey event / raw input eventをinject可能
- virtio-mmio gpu device（`0x10006000`）
  - device ID 16のデバッグ用2D virtio-gpu foundation
  - control / cursor queue、indirect descriptor、event indexの最小対応
  - `GET_DISPLAY_INFO` / `RESOURCE_CREATE_2D` / `SET_SCANOUT` / `FLUSH` などの基本応答
  - Linuxのvirtio-gpu probeと初期modeset commandの観測用
- initrd / initramfs渡し
  - default load address: `0x84000000`
  - 自動生成DTBの `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end` に反映
- bootargs編集
  - default: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - UART / virtio-console / initramfs / verbose debug用のpreset
  - UIから設定し、自動生成DTBに反映
- 実行トレースring buffer
  - PC / 命令 / trap / 最後のtrap cause/tvalをUIで確認可能
  - UIからCSR dumpと全hart trace snapshotのtext / JSON / CSV exportが可能
  - 最後のECALL/SBI引数、SBI BASE/TIME/IPI/RFENCE/HSM/SRST/legacy counters、trap、virtio queue状態をまとめて表示するDiagnostics
  - Diagnostics / device stateのJSON export
  - ELF / System.map symbolsを読み込み、停止PC周辺のsymbol表示、名前検索、panic/oopsログ内PCの自動symbol解決が可能
  - OpenSBIなしの小さなS-mode payloadを直接試すための任意SBI shim
    - BASE / TIME / IPI / RFENCE / HSM / SRSTの最小short-circuit
    - HSM `hart_start` で対象hartのS-modeエントリを起動するデバッグ用経路
    - 既定は無効。OpenSBIを動かす通常経路では使わない
  - UIから任意物理メモリ範囲をdump可能
  - PC breakpoint、物理read/write watchpoint、trace filterをUIから設定可能
  - breakpointはhit count、mode条件、hart条件を指定可能
  - traceはraw instructionに加えて簡易decode mnemonicを表示
  - breakpoint / watchpoint hit時はstop reasonをstatus / diagnostics / trace exportに記録
  - MMIO/DRAM access histogramを収集し、device probeやqueue activityの偏りをDiagnostics / JSONで確認可能
  - MMIO/DRAM access timelineをring bufferに保存し、raw / compact表示でprobeの時系列を確認可能
  - MMIO access timelineにはvirtio-mmio / UART / CLINT / PLICのregister decoder名を付与し、 `QueueNotify` / `Status` / `LSR` などの単位で確認可能
  - CSR access traceを任意で有効化し、guestのCSR read/write tailとCSR別read/write summaryをDiagnostics / trace exportに表示可能
  - PC hot-spot profileを任意で有効化し、停止前に実行回数が多かったPCをsymbol付きで確認可能
  - diagnostic snapshot capture / diffにより、実行前後のhart/device/CSR/MMIO状態差分をUIで確認可能
  - compact trace表示で同種命令・trap・ECALLの連続ログを折りたたみ可能
  - boot presetごとのsmoke runnerで、現在ロード済みfirmware/payloadを一定hart-stepだけ自動実行し、結果JSONを取得可能
  - boot phase analyzerでOpenSBI / Linux / panic / virtio activity / trap / PC symbolをまとめて表示可能
  - boot timelineでconsole markerとMMIO probe / status / QueueNotify / PLIC claimを時系列に統合表示可能
  - device probe analyzerでvirtio/UART/PLIC/CLINTのread/write、identity register、status negotiation、queue notifyを集計可能
  - virtqueue inspectorでQueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotifyの直近状態をdevice/queue別に表示可能
  - descriptor chain visualizerでavail ringからhead descriptorを辿り、NEXT / WRITE / INDIRECT descriptorと小さなbuffer previewを表示可能
  - descriptor chain graph exportでvirtqueue chainをGraphviz DOTとして保存・可視化可能
  - guest physical memory scannerでDRAM内のELF / FDT / gzip / xz / zstd / squashfs / cpio / ext magic / OpenSBI / Linux version / BusyBox / kernel cmdlineらしき領域を検出可能
  - initcall / driver probe classifierでLinux console logのinitcall、probe、virtio、storage、console、network、graphics関連行を分類可能
  - initcall timelineで分類済みのinitcall / driver probe行を時系列グループで表示可能
  - symbol付きELFのDWARF line tableを読み込み、現在PC付近のfile:line、DWARF file summary、trace PCのsymbol+line注釈を表示可能
  - panic summaryでconsole log内のpanic/oops/fault周辺行を自動抽出し、読み込み済みsymbolsでaddressを解決可能
  - boot analysis JSONでtimeline / device probe / virtqueue / panic summaryをまとめてexport可能
  - trace replay reportでtraceのstep/trap/ecall/SBI shim件数、hot mnemonic、trap causeを要約可能
  - trace baseline compareで前回保存したtraceと現在traceのPC/命令/trap差分を先頭から比較可能
  - trace baselineをブラウザlocalStorageに保存/読み込み可能
  - boot regression report/JSONに加えてMarkdown/HTML report exportでtrace stats、boot events、device probe、virtqueue、memory object、initcall countsを一括保存可能
  - virtqueue snapshotでqueue setupとdescriptor chainを同時に表示可能
  - virtqueue anomaly detectorでready queueのaddress欠落、descriptor loop、indirect長不正、DRAM外bufferなどを検出可能
  - virtqueue anomaly hintsで検出結果ごとにQueueNum / QueueDesc / QueueReady / descriptor alignmentなどの修復ヒントを表示可能
  - integrated diagnostic queryでconsole / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory indexを同じqueryで横断検索可能
  - diagnostic query presetsを使ってpanic、virtio negotiation、QueueReady/Notify、satp/mstatus、trap、rootfs関連を一括検索可能
  - share report MD/JSON/HTMLでboot regression、virtqueue hints/triage、memory index、query presets、jump hints、query hitsを自己完結形式で共有可能
  - triage dashboard / stop-cause rankingでpanic、trap、page/access fault、virtqueue anomaly、device probe停滞を候補順に表示可能
  - stop-cause evidenceでランキング根拠、score breakdown、推奨diagnostic query、次アクションを表示可能
  - triage dashboard baselineをlocalStorageに保存し、現在dashboardとstatus/phase/device/anomaly/memory countsを比較可能
  - diagnostic preset baselineをlocalStorageに保存し、現在のpreset hit数との差分を比較可能
  - redacted share report MD/JSON/HTMLでIP/MAC/emailを伏せた共有用レポートを出力可能
  - redaction options JSONでIP/MAC/email/long hex addressの置換有無をUIから調整可能
  - memory object dumpでmemory index/searchのhit周辺をhex + ASCIIで確認可能
  - memory range dumpで任意DRAM addressとbyte lengthを指定してhex + ASCII dump / JSON export可能
  - memory scan snapshot/diffで実行前後に増減したELF/FDT/initrd/rootfs断片候補を確認可能
  - memory indexで近接するELF/FDT/initrd/kernel/rootfs signatureを範囲ごとにまとめて索引化可能
  - UART / virtio-console出力からLinux `dmesg` 風ログを抽出し、読み込み済みsymbolsでpanic/oops addressを解決可能
- simple-framebuffer
  - `0x86000000` , 1024x768, `a8r8g8b8` を自動生成DTBの `/chosen/framebuffer@86000000` に追加
  - UIのCanvasにframebufferを描画、RGBA raw dumpとPNGをダウンロード可能
  - virtio-gpuの2D resource backingを `TRANSFER_TO_HOST_2D` / `RESOURCE_FLUSH` 時にsimple-framebufferへコピー可能
- DRAM `0x80000000` , 128 MiB
- virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT付き最小virt DTB自動生成、またはUIからDTBロード
  - `sifive,plic-1.0.0` / `sifive,clint0` compatibleとvirtio `dma-coherent` を追加
  - Hart countに合わせて `cpu@N` と `interrupts-extended` を生成

## 使い方

```bash
make serve
```

ブラウザで `http://localhost:8080` を開き、OpenSBIのfirmwareを選んで `Load firmware` → `Run`。

virtio-consoleをLinux consoleとして試す場合は、bootargsを `console=hvc0 earlycon=sbi root=/dev/vda rw` などに変更できます。既定では従来通りUART（`ttyS0`）を使います。

停止PCの解析には、Linuxの `System.map` またはsymbol付きELFを `Load symbols` で読み込んでから `Symbols @ PC` / `Diagnostics` / `Search symbols` を使います。symbol付きELFにDWARF line tableが含まれる場合は `DWARF lines @ PC` でfile:lineも確認できます。 `DWARF file summary` はline tableに含まれるファイルごとの行数を表示します。firmware/payloadがsymbol付きELFの場合は、自動でもsymbol tableを取り込みます。 `Annotated trace` はtrace内の `pc=` をsymbols/DWARF lineで注釈します。 `Download trace` で全hartのtrace snapshotを保存できます。JSON/CSV形式も選べます。JSON traceにはsymbolsがあればsymbol/source情報も含めます。 `Trace filter` には `trap` 、 `ecall` 、 `sbi-shim` 、 `pc=` 、 `virtio` などの文字列を入れてtrace tail/export、access timeline、compact表示を絞れます。 `Compact trace` は連続する同種命令・trap・ECALLを折りたたみます。panic/oopsログを貼り付けて `Analyze log symbols` を押すと、ログ内の64-bit PC風アドレスを読み込んだsymbolsで解決します。

`Trace replay report` は現在のtraceを統計化し、 `Trace baseline compare` は保存済みtraceを貼り付けて現在traceとの差分をPC/命令/trap単位で比較します。 `Save current trace as baseline` / `Load saved baseline` はブラウザlocalStorageにbaselineを保持します。 `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` はboot timeline、device probe、virtqueue、memory scanner、initcall分類、trace統計をまとめた回帰確認用レポートです。 `Capture memory scan` →実行→ `Diff memory scan` で、guest memory内のELF/FDT/initrd/rootfs候補の増減も確認できます。 `DWARF source context` は現在PC周辺のsymbolとDWARF file:lineをまとめて表示します。

`Boot phase` はconsole log、MMIO histogram、trap、symbol情報から現在の進行状況を要約します。 `Boot timeline` はconsoleのmilestoneとMMIOのprobe / status / QueueNotifyを時系列に並べます。 `Device probe` はvirtioなどのregister accessとnegotiationを集計し、 `Virtqueue inspect` はqueue setupとnotify状態をdevice/queue別に表示します。 `Descriptor chains` はqueueのavail ringからdescriptor chainを読み、indirect descriptorやbuffer previewを表示します。 `Descriptor DOT` / `Download DOT` は同じchainをGraphviz DOTとして出力します。 `Virtqueue anomalies` はqueue setupやdescriptor chainの不整合を検出し、 `Anomaly hints` はそれぞれの不整合に対する次の確認ポイントを表示します。 `Integrated diagnostic query` は `virtio QueueReady` 、 `panic` 、 `satp` 、 `0x80200000` などの語でconsole / trace / CSR trace / MMIO timeline / virtqueue anomalies / memory indexを横断検索します。 `Share report MD/JSON/HTML` はboot regression reportにanomaly hints/triage、memory index、memory jump hints、query presets、query hitsを加えた共有用bundleです。HTMLはJSONを埋め込んだ自己完結ファイルとして保存できます。 `Diagnostic query presets` はpanic、virtio status、QueueReady/QueueNotify、satp/mstatus、trap、rootfs関連をまとめて検索します。 `Save query` / `Load query` はdiagnostic queryをbrowser localStorageに保存します。 `Memory scan` はDRAM内のELF/FDT/initrd/kernel/rootfs断片候補を探し、 `Memory index` は近接するsignatureを範囲ごとにまとめます。 `Memory search` はmemory indexを文字列または `0x...` addressで検索し、 `Memory jumps` はELF/FDT/Linux/OpenSBI/cmdline/rootfsなどの有用なジャンプ先候補を表示します。 `Initcall classifier` / `Initcall timeline` はLinuxのinitcall/driver probe風ログを分類・時系列化します。 `Panic summary` はpanic/oops/fault周辺行を抽出し、symbolsがあればaddressを解決します。 `Boot analysis JSON` はこれらをまとめて保存します。 `Dmesg extract` はUART / virtio-consoleに出たログからLinux風の行だけを抜き出します。 `Decoded MMIO` は直近のMMIO accessをregister名付きで表示します。

`Triage dashboard` はstop-cause ranking、virtqueue anomaly severity、device probe、query bookmarksを一画面用テキストにまとめます。 `Stop-cause ranking` はconsole/trace/statusからkernel panic、oops、illegal instruction、page/access fault、virtqueue異常、device probe停滞を優先度付きで並べます。 `Stop-cause evidence` はランキングの根拠、score breakdown、推奨query、次の確認ポイントを表示します。 `Save triage baseline` →実行→ `Triage diff` でdashboardのstatus/phase/device/anomaly/memory countsの差分を比較できます。 `Save preset baseline` →実行→ `Compare preset baseline` で、panic/virtio/satp/rootfsなどのpreset query hit数が前回から増減したか確認できます。 `Memory dump hits` はdiagnostic queryまたはtrace filterを使ってmemory index hit周辺をhex/ASCII dumpします。 `Memory range dump` は任意address/lengthを指定してDRAMを直接hex/ASCII dumpします。 `Redacted share MD/JSON/HTML` は共有前にemail / MAC / IPv4を `<email>` / `<mac>` / `<ipv4>` へ置き換えます。 `Redaction options JSON` では `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex` を切り替えられます。

`Smoke preset` は選択中のboot presetをresetして指定stepだけ実行します。 `Smoke matrix` は `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` などのpreset listを順に実行し、各presetの実行step、最後のphase、stop-cause候補を一覧化します。

PC breakpointは `Breakpoints / watchpoints` の `PC breakpoint` に物理/仮想PCのhex値を入れて追加します。 `Run` / `Step 1k` はbreakpointで停止し、 `Step` は現在PCがbreakpointでも1命令だけ進めます。write watchpointは物理アドレス範囲に対するbus write、read watchpointはbus readを検出する簡易機能です。MMIO probeやframebuffer書き込み、特定構造体の参照確認に使えます。 `Access timeline` / `Compact access` は直近のDRAM/MMIO accessを時系列・圧縮表示します。 `PC profile on` はhot PCを集計し、 `Capture snapshot` →実行→ `Diff snapshot` で実行前後の診断差分を確認できます。

simple-framebufferは `0x86000000` に1024x768x32bppのメモリを用意し、自動生成DTBの `/chosen/framebuffer@86000000` に `simple-framebuffer` として載せます。Linux側でsimplefbが使える場合は `Render framebuffer` でCanvasに表示できます。

virtio-netはブラウザから実ネットワークへ接続するものではなく、packet-levelのデバッグデバイスです。 `virtio-net debug` にEthernet frameのhexを入れてRXに注入し、guestがTX queueへ出したframeは `Show TX frames` で確認します。Linux側で認識させる場合は必要に応じて `ip link set dev eth0 up` などをguest側で実行します。

virtio-rngはdeterministic PRNGをguest entropy sourceとして見せる検証用デバイスです。再現性を保つため既定seedは固定で、UIの `Set deterministic seed` から変更できます。

virtio-gpuはLinuxのvirtio-gpu driver probeと2D resource setupを観測するための最小デバイスです。実GPU accelerationではなく、control queueに来るmodeset / scanout / flush系コマンドを追跡し、Diagnosticsに状態を出します。resource backing memoryからsimple-framebufferへのコピーも行うため、guestが2D resourceをflushした結果を `Render framebuffer` / PNG exportで確認できます。cursor queueの `UPDATE_CURSOR` / `MOVE_CURSOR` も状態として記録します。

`SBI shim on` はOpenSBIを使わずにS-mode payloadを直接走らせるデバッグ用です。通常の `fw_dynamic.bin` / `fw_payload.bin` 実験では無効のままにしてください。

Multi-hartを試す場合は、firmwareをロードする前に `Hart count` を設定してください。設定変更はmachine resetを伴うため、変更後にfirmware / payload / diskを再ロードする前提です。 `View hart` で表示対象hartのregister / CSR / traceを切り替えられます。

`fw_dynamic.bin` を使う場合は、必要に応じて `Load payload` でS-mode payload / kernelを `0x80200000` 付近へ読み込みます。エミュレータはdynamic infoを `0x87dff000` に置き、 `a2` にそのアドレスを設定します。

Linux実験では次のどちらかを使えます。

- `Load disk`: rootfsなどのraw disk imageをvirtio-blkとして渡す。既定bootargsは `root=/dev/vda rw` 。
- `Load initrd`: initramfsを `0x84000000` に置き、自動生成DTBにinitrd範囲を反映する。必要ならbootargsを `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` などに変更する。

OpenSBI 1.8.1の配布済みRISC-Vバイナリを使う例:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# 展開後の fw_dynamic.bin / fw_payload.bin / fw_jump.bin などをブラウザでロード
```

OpenSBIをローカルでビルドする場合は、 `riscv64-unknown-elf-` などのRISC-V toolchainを用意して `PLATFORM=generic` でビルドしてください。

### 開発用コマンド

```bash
go test ./...
make wasm
make serve
```

## 注意

この実装はOpenSBIの初期化、S-mode payload移行、Linux boot調査に必要な機能を段階的に備えています。Linux bootに向けてPMP、Sv39、virtio-blk、virtio-console、virtio-net、virtio-rng、virtio-input、virtio-gpu、initrd、simple-framebuffer、trap/CSR精度、CSR trace/summary、MMIO histogram/timeline/register decoder、boot phase/timeline analyzer、device probe analyzer、virtqueue inspector/descriptor chain visualizer/DOT export/snapshot/anomaly detector/anomaly hints/anomaly triage、guest memory scanner/index/diff/search/jump hints/dump helper、integrated diagnostic query/query presets/preset baseline comparison、triage dashboard/stop-cause ranking、share report bundle/HTML/redaction、initcall classifier/timeline、DWARF line lookup/source context/trace annotation、panic summary、dmesg extractor、trace replay/compare、boot regression report、PC profile、snapshot diff、boot smoke runner/smoke matrix、triage baseline diff、stop-cause evidence、editable redaction、memory range dumpは追加しましたが、主な未実装・簡略化は正確なcycle/timeモデル、AIA/IMSIC、tap/WebSocket bridgeなどの実ネットワーク接続、本格的なvirgl/DRM/GPU acceleration、すべてのCSRの厳密なWARL/WPRI動作、複数workerを使った真の並列実行です。multi-hartは単一wasm worker内の協調スケジューリングです。

## 診断・回帰補助

smoke matrixと診断クエリの共有性を強化しています。

- `Smoke matrix MD/HTML` はsmoke matrix結果をMarkdown / 自己完結HTMLとして保存します。
- `Save smoke baseline` →実行→ `Compare smoke baseline` でpresetごとのphase、実行step、top stop-causeの差分を確認できます。
- `Stop checklist` はstop-cause rankingから、次に確認すべき具体的な作業項目をチェックリスト化します。
- `CSR/MMIO bookmarks` はintegrated diagnostic queryの結果からCSR / MMIO / traceの重要ヒットだけを抜き出します。
- `Watchpoint hits` はread/write watchpointのhit履歴を時系列で表示します。 `Clear hit timeline` で履歴だけを消去できます。
- `Artifact manifest` は現在ロード済みのfirmware / payload / disk / initrd / symbolsと、生成DTB / dynamic infoのrange、entry、SHA-256を一覧化します。

### regression handoff補助

- `Manifest diff` / `Manifest diff JSON` は現在のboot artifact manifestとlocalStorageに保存したbaselineを比較し、bootargs、hart数、load range、entry、ELF判定、SHA-256の差分を表示します。
- `Auto break/watch suggestions` はstop-cause evidence、直近trace PC、watchpoint hit timelineから、次回実行時に置くべきPC breakpoint / read watchpoint / write watchpoint候補を生成します。
- `Smoke clusters` / `Smoke clusters JSON` はsmoke matrixのpreset結果をphaseとtop stop-causeでクラスタリングし、同じ失敗型のpresetをまとめます。
- `Diagnostic bundle JSON` はmanifest、triage dashboard、stop-cause、breakpoint suggestions、share bundle、watchpoint hitsをまとめた自己完結JSONです。
- `Compressed bundle JSON` は上記diagnostic bundleをgzip+base64にしたものです。issueやchatに貼る前にサイズを小さくしたい場合に使います。

### handoff / provenance補助

- `Decode bundle` は `Diagnostic bundle JSON` か `Compressed bundle JSON` 、またはgzip+base64本体を貼り付けて展開します。
- `Bundle compare` / `Bundle compare JSON` は貼り付けた過去bundleと現在bundleを比較し、triage phase、top stop-cause、manifest、artifact hash、smoke cluster、watchpoint hit数、suggestion数の差分を表示します。
- `Provenance` / `Provenance JSON` はmanifest、trace、console、diagnostic bundleのSHA-256、trace line数、console byte数、top stop-causeをまとめます。再現性確認やissue添付の根拠として使えます。
- `Handoff MD` はprovenance、top stop-cause、auto break/watch suggestions、stop checklist、baseline diff、artifact manifestをMarkdownにまとめます。
- `Apply auto breaks` はauto break/watch suggestionsの上位候補を現在のemulatorに一括適用します。再実行前に停止位置や疑わしいMMIO/DRAM範囲を素早く設定するための補助です。

### reproduction / signature / headless handoff

- `Repro plan` / `Repro MD` / `Repro JSON`
  - diagnostic bundle、provenance、artifact manifestから再現手順を生成します。
  - firmware / payload / initrd / disk / symbolsのrole、size、load range、SHA-256をartifact pinとして列挙します。
  - smoke preset、bootargs、hart count、next_addr、推奨break/watch条件を手順化します。
- `Log signature` / `Log signature JSON`
  - trace / console / manifestのSHA-256、trace line count、first/last PC、console first/last line、hot tokenを軽量summary化します。
  - raw traceを貼らずに「同じログか」「どこが変わったか」を比較できます。
- `Save log signature` / `Load log signature` / `Compare log signature`
  - browser localStorageにlog signature baselineを保存し、現在のsignatureと比較します。
  - trace hash、console hash、manifest hash、line count、last PC、last console lineの差分を表示します。
- `Auto break verify`
  - auto breakpoint/watchpoint suggestionを適用する前後の確認用summaryを表示します。
  - duplicate suggestionや怪しいPC rangeのwarningを出します。
- `Headless smoke script`
  - 現在のartifact manifest、bootargs、hart count、smoke presets、step countからCI/handoff用のshell script skeletonを生成します。
  - 実行環境にPlaywrightなどのbrowser harnessを足す前段として、artifact pinとpreset matrixを固定する用途です。

#### headless / CI補助

repro/signature handoffをCIやissueで扱いやすくするため、以下を追加しています。

- `Bundle integrity` / `Integrity JSON` はdiagnostic bundleとartifact manifestの整合性を検査し、artifact role、SHA-256、load range、suggestion、smoke resultの不整合を `error` / `warn` / `info` で分類します。
- `Repro validation` / `Repro validation JSON` は現在のreproduction planがbundleのbootargs、hart count、next_addr、artifact pins、top stop-cause、log signatureと一致しているかを確認します。
- `CI summary` / `CI summary JSON` はbundle integrity、trace/console signature、smoke result、stop-causeをまとめ、CIでpass/warn/fail判定しやすいsummaryを出します。
- `Headless runner spec` / `Runner spec JSON` は `go run ./cmd/rvsmoke ...` で検査するためのpreset、steps、artifact pin、推奨commandを生成します。
- `cmd/rvsmoke` を追加しました。ブラウザ外でdiagnostic bundle / artifact manifestを読み込み、artifact hash、bundle integrity、CI summary、runner specをtext / JSON / Markdownで出力できます。

例:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

`rvsmoke` は現在、bundle/manifestとartifact hashの再現性検査・CI summary生成を行います。CPU実行そのものは引き続きブラウザjs/wasm側のsmoke matrixを使います。

#### rvsmoke CI gate / JUnit / SARIF

`cmd/rvsmoke` はブラウザでexportしたdiagnostic bundle / manifestをCI側で検査する補助CLIです。headless実行の実体化により、baseline bundle比較、CI gate policy、JUnit XML、SARIF、self-contained HTML reportを出力できます。

例:

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

policy JSONの例:

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

`-out html` は標準出力に自己完結HTML、 `-out junit` はJUnit XML、 `-out sarif` はSARIF JSONを出します。 `-junit` / `-html` / `-sarif` を同時に指定すると、標準出力の形式とは別に各ファイルも保存します。CI gateはartifact manifest、trace/console signature、baseline diff、virtqueue anomaly、smoke resultをまとめて `pass` / `warn` / `fail` に正規化します。

#### rvsmoke policy templates / bundle trend compare

CI gateの初期導入と複数回のregression比較を楽にするため、 `rvsmoke` とブラウザUIにpolicy template、action checklist、bundle trend compareを追加しました。

- `CI policy templates` / `Policy templates JSON` は `default` 、 `strict` 、 `linux-boot` 、 `artifact-only` 、 `lenient` の組み込みpolicyを表示します。
- `Policy template JSON` は指定したtemplateをそのままCIに置けるJSONとして保存します。
- `CI gate` / `CI gate JSON` は現在のbrowser状態に対してpolicy templateを適用し、pass/warn/failのgate checkを表示します。
- `CI checklist` / `CI checklist JSON` はgate failure、bundle integrity、artifact diffから次に確認すべき項目をchecklist化します。
- `rvsmoke -compare name=bundle.json` は複数bundleを時系列に並べ、phase、top stop-cause、artifact hash、smoke clusterの変化をtrend reportとして出力します。

Policy templateの生成例:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

複数bundleの比較例:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

`-policy-template` は `-policy` を指定しない場合のdefault policyとして使われます。 `-policy` を指定した場合はファイルのJSONが優先されます。

## rvsmoke CI連携

`rvsmoke` のCI/handoff補助を拡張しています。

- `rvsmoke -print-github-actions linux-boot` でGitHub Actions workflow YAMLを生成できます。
- `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` でworkflowをファイル出力できます。
- `rvsmoke -policy-tree policy-tree.md` でCI gate / bundle integrity / baseline driftを原因ツリーとして保存できます。
- `rvsmoke -history history.txt` で複数bundle trendのphase / stop-cause / artifact drift集計を保存できます。
- `rvsmoke -repro-zip rvwasm-minimal-repro.zip` で、README、diagnostic bundle、manifest、runner spec、policy、CI summary、検証scriptを含む最小再現パッケージを生成できます。

例:

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

`-repro-zip` はraw firmware/kernel/diskを同梱しません。bundle内のSHA-256 pinとmanifest rangeを同梱し、共有先でartifactを照合する前提です。

### CI再現ZIP検査 / matrix workflow継続

`rvsmoke` とbrowser UIに、最小再現パッケージの受け渡しを検査する機能と、GitHub Actions matrix / trend可視化用の出力を追加しています。

- `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` で、 `-repro-zip` が生成したZIPを展開せずに検査できます。必須ファイル、unsafe path、 `diagnostic-bundle.json` / `manifest.json` の一致、 `ci-policy.json` 、 `scripts/rvsmoke.sh` を確認します。
- `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` で、presetごとのGitHub Actions matrix workflow YAMLを生成できます。
- `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` でmatrix workflowをファイル出力できます。
- `rvsmoke -trend-csv rvwasm-trend.csv` と `-trend-chart-json rvwasm-trend-chart.json` で、bundle trendを外部グラフ化しやすいCSV / JSONに保存できます。
- Browser UIに `Minimal repro ZIP` 、 `Inspect repro ZIP` 、 `Repro ZIP JSON` 、 `Matrix workflow YAML` 、 `Trend chart JSON` 、 `Trend CSV` を追加しました。

例:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# JSONで検査結果を保存
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# current bundleとprevious bundleのtrendをCSV/JSON化
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` は単体でも実行できます。 `-bundle` と同時に指定した場合は通常のCI summaryにZIP inspection結果も含め、検査が `fail` の場合はCI summaryもfailure扱いにします。

### CI matrix集約 / checksum manifest継続

`rvsmoke` のCI artifact受け渡しを強化しています。

- `-repro-checksums rvwasm-repro-checksums.json` で、 `-inspect-repro-zip` の結果からZIP内ファイルのdeterministic checksum manifestを保存できます。
- `-matrix-result name=rvsmoke-output.json` を複数指定して、複数preset / 複数jobの `rvsmoke -out json` 結果を集約できます。
- `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` でmatrix結果をtext / JSON / self-contained HTMLとして保存できます。
- `-trend-html rvwasm-trend.html` でbundle trend reportを単体HTMLとして保存できます。

例:

```bash
# 最小再現ZIPの内容とchecksum manifestを保存
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# 複数matrix jobのrvsmoke JSONを集約
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

matrix aggregateはjobごとのCI status、gate failure/warning数、artifact mismatch、top stop-causeをまとめます。GitHub Actionsのmatrix jobが分かれている場合でも、最後の集約jobで全体の失敗傾向を見やすくするための補助です。

#### CI / release handoff補助

`rvsmoke` のCI artifact管理とrelease handoffを強化しています。

- `-artifact-index rvwasm-artifacts.json` はJUnit / SARIF / HTML / trend / matrix / repro checksumなど、生成したCI artifactのpath、bytes、SHA-256をまとめます。
- `-release-manifest rvwasm-release.json` はdiagnostic bundle、log signature、CI gate、matrix aggregate、flake report、artifact index、repro checksum verificationを1つのhandoff manifestにまとめます。
- `-release-html rvwasm-release.html` はSummary / Artifacts / Matrix / Checksums / JSONへ移動できるnavigation付きの自己完結HTMLを出力します。
- `-verify-repro-checksums baseline-repro-checksums.json` は現在検査したminimal repro ZIPのchecksum manifestをbaselineと比較し、missing / changed / extraを検出します。
- `-matrix-flakes` 、 `-matrix-flakes-json` 、 `-matrix-flakes-html` は `uart#1` / `uart#2` のような複数回matrix結果を正規化して、同じpresetがpass/failで揺れているかを検出します。

例:

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

## リリース引き継ぎ・検証

CI結果を別のマシン、別リポジトリ、レビュー担当者へ引き継ぐためのmetadata出力を `rvsmoke` に追加しています。

### SBOM / provenance拡張

#### SBOM-lite依存関係一覧

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

この依存関係一覧は、小さくdeterministicな形式を意図しています。 `go.mod` を読み取り、module path、Go version、直接の `require` 行、 `replace` 先、CI artifact indexに含まれるartifact種別を記録します。

`rvsmoke` を別のworking directoryから実行する場合は、 `-go-mod /path/to/go.mod` を指定してください。

#### provenance attestation

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

attestationはin-toto / SLSAを参考にしたJSON payloadです。これ自体は署名ではありませんが、安定したSHA-256を持つため、外部CI toolingで署名する対象として使えます。

#### リリース引き継ぎZIP

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

release handoff ZIPにはmetadataだけを含めます。

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

firmware、kernel、initrd、disk imageは埋め込みません。大きなartifactはmanifest側にSHA-256 pinを保持します。

#### リリース引き継ぎZIPの検査

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

inspectorはZIPを展開せずに、必須ファイル、危険なpath、重複path、JSON parse可否、release / index / SBOM / attestationの基本的な整合性を検査します。

### リリース検証

release handoff ZIPの作成に加えて、検証向けの出力を追加しています。

- `-verify-attestation` / `-verify-attestation-text` は、deterministic provenance attestation hash、release materials、CI artifact subjectsが、生成済みrelease manifest、SBOM-lite inventory、artifact indexと一致するか確認します。
- `-sbom-baseline` 、 `-sbom-diff` 、 `-sbom-diff-json` は、現在のSBOM-lite dependency inventoryを保存済みbaselineと比較します。
- `-compare-release-zip-inspection` 、 `-release-zip-compare` 、 `-release-zip-compare-json` は、現在のinspected release handoff ZIPを過去のinspection JSONと比較します。
- `-retention-manifest` / `-retention-text` は、path、kind、bytes、SHA-256、retention days、expiry time、reasonを含むCI artifact retention manifestを生成します。
- `-release-verification-html` は、release status、attestation verification、SBOM diff、release ZIP comparison、retention情報をまとめたnavigation付きHTMLを出力します。

例:

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

### リリースaudit gate

release verificationの上に最終release audit layerを追加しています。provenance attestation verification、SBOM-lite diff、release ZIP comparison、artifact retention expiry、matrix flake status、release manifest statusを1つのscoreとgate reportにまとめます。

主なflag:

- `-list-release-verify-policies` は組み込みrelease audit policyを一覧表示します。
- `-print-release-verify-policy strict` はpolicy JSON templateを出力します。
- `-release-verify-template default|strict|lenient|archive` は組み込みpolicyを選択します。
- `-release-verify-policy policy.json` はcustom release audit policyを読み込みます。
- `-retention-audit` / `-retention-audit-json` はexpiryとminimum-retentionの検査結果を書き出します。
- `-release-score` / `-release-score-json` は0〜100のrelease verification scoreを書き出します。
- `-release-gate` / `-release-gate-json` はpolicy gate resultを書き出します。
- `-release-audit` / `-release-audit-json` / `-release-audit-html` は統合audit reportを書き出します。

例:

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

strict policyは、passではないrelease manifest、失敗したattestation / SBOM / ZIP check、期限切れartifact、設定されたminimum retention daysを下回るartifactを失敗扱いにします。default policyはnightly handoffなどの日常的な確認に向いており、warningは許容しつつ、明確なverification failureはCI失敗にできます。

#### release audit diff / waiver / TODO引き継ぎ

`rvsmoke` のrelease-audit pathは、現在のauditと過去のauditの比較、既知issueへの期限付きwaiver適用、未waiver作業のchecklist生成に対応しています。

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

waiver templateは次のコマンドで作成できます。

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

waiverは、既知の一時的なrelease-audit findingを扱うためのものです。各ruleはID、任意のkind / name / status / substring matcher、owner、reason、 `expires_at` timestampを持ちます。期限切れwaiverは報告されますが、issueの抑制には使われません。

#### release decision / evidence bundle

release audit実行後に使う最終引き継ぎ補助を追加しています。

- `-waiver-calendar` 、 `-waiver-calendar-json` 、 `-waiver-calendar-html` は、各waiverのexpiry、owner、match count、expired / expiring-soon状態を表示します。
- `-release-changelog` 、 `-release-changelog-json` は、audit diff、waiver state、TODO count、waiver expiry stateを人間が読みやすいchangelogとして要約します。
- `-final-decision` 、 `-final-decision-json` は、blocking itemとnext actionを含む最終 `go` 、 `go-with-watch` 、 `no-go` decisionを生成します。
- `-release-evidence-zip` は、audit、waiver report、TODO list、waiver calendar、changelog、final decisionを含む小さなevidence bundleを書き出します。
- `-inspect-release-evidence-zip` は、evidence bundleを展開せずに、必須ファイル、危険なpath、重複entry、JSON parse可否を検査します。
- `-dry-run` はoptional output fileを書き出さずにreportを計算します。
- `-exit-code-mode never` は、通常ならgate failureで失敗する場合でも結果を出力します。

例:

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

CIでevidence bundleを検査する例:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## ライセンス

このプロジェクトはBSD 2-Clause Licenseの下でライセンスされています。詳細は[LICENSE](../LICENSE)ファイルを参照してください。

SPDX-License-Identifier: BSD-2-Clause
