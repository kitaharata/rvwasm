# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## Übersicht

Ein RV64IMAC-Emulator, der auf Go 1.23.2 `GOOS=js GOARCH=wasm` läuft. Der Standardwert ist ein einzelner Hart, aber kooperatives Scheduling von 1 bis 8 Harts ist über die Benutzeroberfläche (UI) verfügbar. Sie können OpenSBI 1.8.1 `fw_payload.bin`/`fw_jump.bin`/`fw_dynamic.bin`/ELF über die Browser-UI laden, um das Booten zu bestätigen.

[![OpenSBI fw_payload boot on rvwasm](images/fw_payload.png)](https://kitaharata.github.io/rvwasm/)

OpenSBI 1.8.1 `fw_payload.bin` bootet auf rvwasm und betritt die S-Modus-Payload der nächsten Stufe.

## Implementierte Funktionen

- RV64I Basis-Instruktionen
- M-Erweiterung
- Minimale Implementierung von A-Erweiterung LR/SC/AMO
- Allgemeine Ganzzahl-Instruktionen der C-Erweiterung
- Zicsr/Zifencei-Äquivalente
- Minimale Implementierung von M/S/U-Privilegienmodus CSR/trap/mret/sret
  - Korrigiert synchrone Ausnahme `mepc`/`sepc` auf den fehlerhaften Instruktions-PC
  - Korrigiert fehlerhaftes Laden/CSR-Schreiben, um rd nicht zu beschädigen, und stoppt das Voranschreiten des Retire-Zählers
  - CSR-Existenzprüfung, Unterdrückung von Schreib-Seiteneffekten bei Read-Only-CSRs, grundlegende Reflexion von `mcounteren`/`scounteren`
  - Hinzugefügte `senvcfg`/State-Enable CSR-Stubs für Linux-Probing
  - Grundlegende Reflexion von `TVM`/`TW`/`TSR` und `MPRV`-Löschung
- Sv39 MMU
  - `satp`-Modus Bare/Sv39
  - 3-stufiger Page-Table-Walk
  - 4 KiB/2 MiB/1 GiB Leaves
  - Grundlegende Reflexion von `SUM`/`MXR`/`MPRV`
  - Seitenfehler-Ausnahme (page fault exception)
  - Automatische Aktualisierung der PTE `A`/`D`-Bits
- UART 16550-Stil MMIO (`0x10000000`)
  - Ausgabe vom Gast
  - Eingabeinjektion von der Browser-UI
  - Empfangs-Interrupt
- CLINT-Stil mtime/mtimecmp/msip (`0x02000000`)
  - MSIP/MTIMECMP-Routing pro Hart für Multi-Hart
- PLIC-Stil Interrupt-Controller (`0x0c000000`)
  - priority/pending/enable/threshold
  - claim/complete
  - M/S-Kontext pro Hart
- PMP-Durchsetzung
  - TOR/NA4/NAPOT
  - R/W/X-Berechtigungen
  - M-Modus-Einschränkungen durch gesperrte Einträge
- OpenSBI `fw_dynamic` Boot-Infos
  - Dynamische Infos werden bei `0x87dff000` platziert
  - Dynamischer Info-Zeiger wird auf `a2` gesetzt
  - S-Modus-Payload / Kernel kann separat von der UI geladen werden
- virtio-mmio Blockgerät (`0x10001000`)
  - Moderne MMIO-Register im virtio 1.0-Stil
  - Minimale Unterstützung für geteilte virtqueue read/write/flush/get-id
  - `FEATURES_OK`-Aushandlung und `VIRTIO_F_VERSION_1`-Verifizierung
  - Queue-Reset, Ignorieren von Notify vor `DRIVER_OK`, grundlegende Reflexion des `NO_INTERRUPT`-Flags
  - Behandlung von `VIRTIO_RING_F_INDIRECT_DESC` und indirekten Deskriptortabellen
  - Interrupt-Unterdrückung durch verwendetes Ereignis `VIRTIO_RING_F_EVENT_IDX`
  - Disk-Images können von der UI geladen werden
  - Vom Gast geänderte Disk-Images können von der UI heruntergeladen werden
- virtio-mmio Konsolengerät (`0x10002000`)
  - Minimale Konsole mit Geräte-ID 3
  - Queue 0 Empfang / Queue 1 Senden
  - Minimale Unterstützung für `VIRTIO_CONSOLE_F_SIZE`, indirekte Deskriptoren und Ereignisindizes
  - Injiziert UI-Eingaben sowohl an UART als auch an virtio-console
- virtio-mmio Netzwerkgerät (`0x10003000`)
  - Minimales Debug-virtio-net mit Geräte-ID 1
  - Queue 0 Empfang / Queue 1 Senden
  - Minimale Unterstützung für `VIRTIO_NET_F_MAC`/`VIRTIO_NET_F_STATUS` / indirekte Deskriptoren / Ereignisindizes
  - Injiziert Ethernet-Frame-Hex in RX von der UI
  - Zeigt vom Gast gesendete Ethernet-Frames als TX-Logs an
- virtio-mmio rng-Gerät (`0x10004000`)
  - Minimale Entropiequelle mit Geräte-ID 4
  - Minimale Unterstützung für geteilte virtqueues, indirekte Deskriptoren und Ereignisindizes
  - Deterministischer Seed kann von der UI aus festgelegt werden
- virtio-mmio Eingabegerät (`0x10005000`)
  - Minimales Debug-Tastatur-/Eingabegerät mit Geräte-ID 18
  - Minimale Unterstützung für Ereignis-Queue / Status-Queue, indirekte Deskriptoren und Ereignisindizes
  - Tastenereignisse / rohe Eingabeereignisse können von der UI injiziert werden
- virtio-mmio gpu-Gerät (`0x10006000`)
  - Minimale 2D-virtio-gpu-Grundlage für das Debugging mit Geräte-ID 16
  - Minimale Unterstützung für Kontroll-/Cursor-Queues, indirekte Deskriptoren und Ereignisindizes
  - Grundlegende Antworten für `GET_DISPLAY_INFO`/`RESOURCE_CREATE_2D`/`SET_SCANOUT`/`FLUSH` usw.
  - Nützlich zur Beobachtung von Linux virtio-gpu-Probes und anfänglichen Modeset-Befehlen
- initrd/initramfs-Übergabe
  - Standard-Ladeadresse: `0x84000000`
  - Gespiegelt in `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end` des automatisch generierten DTB
- Bearbeitung der bootargs
  - Standard: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - Voreinstellungen für UART / virtio-console / initramfs / ausführliches Debugging
  - Kann über die UI festgelegt und im automatisch generierten DTB gespiegelt werden
- Ringpuffer für Ausführungs-Traces
  - PC/Instruktionen/Traps/letzte Trap-Ursache/tval können in der UI angezeigt werden
  - Text-/JSON-/CSV-Exporte von CSR-Dumps und Trace-Snapshots des gesamten Harts sind über die UI verfügbar
  - Diagnosen, die auf einen Blick die letzten ECALL/SBI-Argumente, SBI BASE/TIME/IPI/RFENCE/HSM/SRST/Legacy-Zähler, Traps und virtio-Queue-Zustände anzeigen
  - JSON-Export von Diagnosen / Gerätezuständen
  - Lädt ELF/System.map-Symbole, zeigt Symbole um den angehaltenen PC an, Namenssuche und automatische PC-Symbolauflösung innerhalb von Panic/Oops-Logs
  - Beliebiger SBI-Shim zum direkten Testen kleiner S-Modus-Payloads ohne OpenSBI
    - Minimaler Kurzschluss von BASE/TIME/IPI/RFENCE/HSM/SRST
    - Debug-Pfad zum S-Modus-Eintrag des Ziel-Harts über HSM `hart_start`
    - Standardmäßig deaktiviert. Wird im normalen Pfad zum Ausführen von OpenSBI nicht verwendet
  - Beliebige physikalische Speicherbereiche können von der UI aus gedumpt werden
  - PC-Breakpoints, physische Lese-/Schreib-Watchpoints und Trace-Filter können von der UI aus festgelegt werden
  - Breakpoints können Trefferzähler, Modusbedingungen und Hart-Bedingungen angeben
  - Trace zeigt vereinfachte Dekodierungs-Mnemotechniken zusammen mit rohen Instruktionen an
  - Breakpoint-/Watchpoint-Treffer zeichnen den Stoppgrund in Status-/Diagnose-/Trace-Exporten auf
  - Sammelt MMIO/DRAM-Zugriffs-Histogramme, mit denen Sie Verzerrungen in Geräte-Probes und Queue-Aktivitäten über Diagnosen/JSON überprüfen können
  - Speichert MMIO/DRAM-Zugriffszeitachsen im Ringpuffer, sodass Sie die Zeitreihe der Probes in rohen/kompakten Ansichten überprüfen können
  - Die MMIO-Zugriffszeitachse fügt Register-Decoder-Namen für virtio-mmio/UART/CLINT/PLIC hinzu und ermöglicht die Beobachtung in Einheiten wie `QueueNotify`/`Status`/`LSR`
  - Aktiviert optional den CSR-Zugriffs-Trace, um die Lese-/Schreib-Tails des Gast-CSR und Lese-/Schreib-Zusammenfassungen pro CSR in Diagnosen / Trace-Exporten anzuzeigen
  - Aktiviert optional das PC-Hot-Spot-Profil, um häufig ausgeführte PCs mit Symbolen vor dem Anhalten anzuzeigen
  - Die Erfassung / der Vergleich (Diff) von Diagnose-Snapshots ermöglicht es Ihnen, die Unterschiede in den Hart-/Geräte-/CSR-/MMIO-Zuständen vor und nach der Ausführung auf der UI zu überprüfen
  - Faltet aufeinanderfolgende identische Instruktionen, Traps und ECALL-Logs in der kompakten Trace-Ansicht
  - Der Smoke-Runner pro Boot-Voreinstellung kann automatisch eine angegebene Anzahl von Hart-Schritten der aktuell geladenen Firmware/Payload ausführen und JSON-Ergebnisse abrufen
  - Der Boot-Phasen-Analysator kann Aktivitäten von OpenSBI / Linux / Panic / virtio / Traps / PC-Symbolen zusammenfassen
  - Die Boot-Zeitachse kann Konsolenmarkierungen und MMIO-Probes / Zustände / QueueNotifies / PLIC-Claims integriert in eine Zeitreihe anzeigen
  - Der Geräte-Probe-Analysator kann virtio/UART/PLIC/CLINT-Lese-/Schreibvorgänge, Identitätsregister, Statusverhandlungen und Queue-Notifies aggregieren
  - Der Virtqueue-Inspektor kann die neuesten Zustände von QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify nach Gerät/Queue anzeigen
  - Der Deskriptor-Ketten-Visualisierer verfolgt Kopf-Deskriptoren aus dem Avail-Ring und zeigt NEXT/WRITE/INDIRECT-Deskriptoren zusammen mit einer kleinen Puffer-Vorschau an
  - Der Export von Deskriptor-Ketten-Graphen kann virtqueue-Ketten als Graphviz-DOTs speichern und visualisieren
  - Der physische Speicher-Scanner des Gastes kann Bereiche im DRAM erkennen, die ELF/FDT/gzip/xz/zstd/squashfs/cpio/ext-Magics / OpenSBI-/Linux-Version / BusyBox / Kernel-Cmdline ähneln
  - Der Initcall-/Treiber-Probe-Klassifikator kann Linux-Konsolen-Protokollzeilen im Zusammenhang mit Initcalls, Probes, virtio, Speicher, Konsolen, Netzwerken und Grafiken kategorisieren
  - Die Initcall-Zeitachse kann klassifizierte Initcall-/Treiber-Probe-Zeilen in Zeitreihengruppen anzeigen
  - Liest DWARF-Zeilentabellen aus ELFs mit Symbolen und zeigt Datei:Zeile in der Nähe des aktuellen PCs, DWARF-Dateizusammenfassungen und Symbol+Zeilen-Anmerkungen für Trace-PCs an
  - Die Panic-Zusammenfassung extrahiert automatisch Zeilen rund um Panic/Oops/Fehler im Konsolenprotokoll und löst Adressen mit geladenen Symbolen auf
  - Das Boot-Analyse-JSON kann Zeitachsen / Geräte-Probes / virtqueues / Panic-Zusammenfassungen kollektiv exportieren
  - Der Trace-Replay-Bericht kann die Anzahl der Schritte/Traps/Ecalls/SBI-Shims, heiße Mnemotechniken und Trap-Ursachen im Trace zusammenfassen
  - Der Trace-Baseline-Vergleich kann die PC-/Instruktions-/Trap-Unterschiede zwischen einem zuvor gespeicherten Trace und dem aktuellen Trace von Anfang an vergleichen
  - Die Trace-Baseline kann im/aus dem localStorage des Browsers gespeichert/geladen werden
  - Der Boot-Regressionsbericht/JSON sowie die Exporte von Markdown-/HTML-Berichten können Trace-Statistiken, Boot-Ereignisse, Geräte-Probes, virtqueues, Speicherobjekte und Initcall-Zählungen in großen Mengen speichern
  - Der Virtqueue-Snapshot kann Queue-Setups und Deskriptor-Ketten gleichzeitig anzeigen
  - Der Virtqueue-Anomaliedetektor kann fehlende Ready-Queue-Adressen, Deskriptor-Schleifen, ungültige indirekte Längen, Puffer außerhalb des DRAM usw. erkennen
  - Virtqueue-Anomalie-Hinweise können Reparaturhinweise wie QueueNum/QueueDesc/QueueReady/Deskriptor-Ausrichtung für jedes Erkennungsergebnis anzeigen
  - Die integrierte Diagnoseabfrage kann Konsolen / Traces / CSR-Traces / MMIO-Zeitachsen / virtqueue-Anomalien / Speicherindizes mit derselben Abfrage durchsuchen
  - Diagnoseabfrage-Voreinstellungen ermöglichen die Stapelsuche nach Panics, virtio-Verhandlungen, QueueReady/Notifies, satp/mstatus, Traps und rootfs
  - Der Share-Bericht MD/JSON/HTML ermöglicht das Teilen von Boot-Regressionen, virtqueue-Hinweisen/Triage, Speicherindizes, Abfrage-Voreinstellungen, Sprunghinweisen und Abfragetreffern in einem in sich geschlossenen Format
  - Das Triage-Dashboard / Stopp-Ursachen-Ranking kann Panics, Traps, Seiten-/Zugriffsfehler, virtqueue-Anomalien und blockierte Geräte-Probes in der Reihenfolge der Kandidaten anzeigen
  - Die Evidenz der Stopp-Ursache zeigt Ranking-Begründungen, Punkteaufschlüsselungen, empfohlene Diagnoseabfragen und nächste Aktionen an
  - Die Triage-Dashboard-Baseline kann im localStorage gespeichert werden, um Status-/Phasen-/Geräte-/Anomalie-/Speicherzählungen mit dem aktuellen Dashboard zu vergleichen
  - Die Diagnose-Voreinstellungs-Baseline kann im localStorage gespeichert werden, um den Unterschied zur Trefferzahl der aktuellen Voreinstellung zu vergleichen
  - Der redigierte Share-Bericht MD/JSON/HTML kann teilbare Berichte mit redigierten IPs/MACs/E-Mails ausgeben
  - Die Redaktionsoptionen JSON ermöglichen das Umschalten des Ersetzens von IPs/MACs/E-Mails/langen Hex-Adressen über die UI
  - Der Speicherobjekt-Dump kann Hex + ASCII um Speicherindex-/Suchtreffer herum verifizieren
  - Der Speicherbereich-Dump kann eine beliebige DRAM-Adresse und Bytelänge für einen Hex + ASCII-Dump / JSON-Export angeben
  - Die Erfassung / der Vergleich (Diff) von Speicherscans kann ELF-/FDT-/initrd-/rootfs-Fragmentkandidaten überprüfen, die vor und nach der Ausführung zu-/abgenommen haben
  - Der Speicherindex kann nahegelegene ELF-/FDT-/initrd-/Kernel-/rootfs-Signaturen nach Bereich gruppieren, um einen Index zu erstellen
  - Extrahiert Linux-Protokolle im `dmesg`-Stil aus UART-/virtio-console-Ausgaben und löst Panic-/Oops-Adressen mit geladenen Symbolen auf
- simple-framebuffer
  - Fügt automatisch `0x86000000`, 1024x768, `a8r8g8b8` zu `/chosen/framebuffer@86000000` im generierten DTB hinzu
  - Zeichnet den Framebuffer auf einen UI-Canvas, und rohe RGBA-Dumps / PNGs können heruntergeladen werden
  - Der 2D-Ressourcen-Hintergrund für virtio-gpu kann bei `TRANSFER_TO_HOST_2D`/`RESOURCE_FLUSH` in den simple-framebuffer kopiert werden
- DRAM `0x80000000`, 128 MiB
- Automatische Generierung eines minimalen virt DTB mit virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT oder Laden eines DTB über die UI
  - Hinzugefügt `sifive,plic-1.0.0`/`sifive,clint0` kompatibel und virtio `dma-coherent`
  - Generiert `cpu@N` und `interrupts-extended` entsprechend der Hart-Anzahl

## Verwendung

```bash
make serve
```

Öffnen Sie `http://localhost:8080` in Ihrem Browser, wählen Sie die OpenSBI-Firmware aus und klicken Sie dann auf `Load firmware` → `Run`.

Wenn Sie virtio-console als Linux-Konsole testen möchten, können Sie die bootargs auf so etwas wie `console=hvc0 earlycon=sbi root=/dev/vda rw` ändern. Standardmäßig wird UART (`ttyS0`) wie gewohnt verwendet.

Um einen angehaltenen PC zu analysieren, laden Sie eine Linux `System.map` oder eine ELF mit Symbolen über `Load symbols` und verwenden Sie dann `Symbols @ PC` / `Diagnostics` / `Search symbols`. Wenn die ELF mit Symbolen DWARF-Zeilentabellen enthält, können Sie Datei:Zeile auch mit `DWARF lines @ PC` überprüfen. `DWARF file summary` zeigt die Anzahl der Zeilen pro Datei an, die in der Zeilentabelle enthalten sind. Wenn die Firmware/Payload eine ELF mit Symbolen ist, wird die Symboltabelle automatisch importiert. `Annotated trace` annotiert `pc=` im Trace mit Symbolen/DWARF-Zeilen. `Download trace` speichert einen Trace-Snapshot für alle Harts. Sie können auch JSON-/CSV-Formate auswählen. Der JSON-Trace enthält Symbol-/Quellinformationen, falls Symbole vorhanden sind. Geben Sie Zeichenfolgen wie `trap`, `ecall`, `sbi-shim`, `pc=` oder `virtio` in den `Trace filter` ein, um das Trace-Tail/den Export, die Zugriffszeitachse und die kompakte Ansicht einzugrenzen. `Compact trace` faltet aufeinanderfolgende identische Instruktionen, Traps und ECALLs. Wenn Sie ein Panic-/Oops-Protokoll einfügen und auf `Analyze log symbols` klicken, werden 64-Bit-PC-ähnliche Adressen im Protokoll unter Verwendung der geladenen Symbole aufgelöst.

`Trace replay report` generiert Statistiken für den aktuellen Trace, und `Trace baseline compare` fügt einen gespeicherten Trace ein, um PC-/Instruktions-/Trap-Unterschiede zum aktuellen Trace von Anfang an zu vergleichen. `Save current trace as baseline` / `Load saved baseline` speichert die Baseline im localStorage des Browsers. `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` sind Regressionsprüfberichte, die die Boot-Zeitachse, Geräte-Probes, virtqueues, den Speicher-Scanner, die Initcall-Klassifizierung und die Trace-Statistiken konsolidieren. Sie können auch die Zunahme oder Abnahme von ELF-/FDT-/initrd-/rootfs-Kandidaten im Gastspeicher überprüfen, indem Sie `Capture memory scan` → Run → `Diff memory scan` ausführen. `DWARF source context` zeigt Symbole und DWARF-Datei:Zeile um den aktuellen PC herum gemeinsam an.

`Boot phase` fasst den aktuellen Fortschritt aus Konsolenprotokollen, MMIO-Histogrammen, Traps und Symbolinformationen zusammen. `Boot timeline` richtet Konsolenmeilensteine und MMIO-Probes / Status / QueueNotifies in einer Zeitreihe aus. `Device probe` aggregiert Registerzugriffe und Verhandlungen von virtio und anderen, und `Virtqueue inspect` zeigt Queue-Setups und Benachrichtigungsstatus nach Gerät/Queue an. `Descriptor chains` liest Deskriptor-Ketten aus dem Avail-Ring der Queue und zeigt indirekte Deskriptoren und Puffer-Vorschauen an. `Descriptor DOT` / `Download DOT` gibt dieselbe Kette als Graphviz-DOT aus. `Virtqueue anomalies` erkennt Inkonsistenzen bei Queue-Setups und Deskriptor-Ketten, und `Anomaly hints` zeigt den nächsten Prüfpunkt für jede Inkonsistenz an. Die `Integrated diagnostic query` durchsucht Konsolen / Traces / CSR-Traces / MMIO-Zeitachsen / virtqueue-Anomalien / Speicherindizes unter Verwendung von Wörtern wie `virtio QueueReady`, `panic`, `satp`, `0x80200000`. `Share report MD/JSON/HTML` ist ein gemeinsam nutzbares Bundle, das Anomalie-Hinweise/Triage, Speicherindizes, Speicher-Sprunghinweise, Abfrage-Voreinstellungen und Abfragetreffer zum Boot-Regressionsbericht hinzufügt. HTML kann als in sich geschlossene Datei mit eingebettetem JSON gespeichert werden. `Diagnostic query presets` bündelt Suchen im Zusammenhang mit Panics, virtio-Status, QueueReady/QueueNotify, satp/mstatus, Traps und rootfs. `Save query` / `Load query` speichert Diagnoseabfragen im localStorage des Browsers. `Memory scan` sucht nach ELF-/FDT-/initrd-/Kernel-/rootfs-Fragmentkandidaten im DRAM, und der `Memory index` gruppiert nahegelegene Signaturen nach Bereich. `Memory search` durchsucht Speicherindizes anhand von Zeichenfolgen oder `0x...`-Adressen, und `Memory jumps` zeigt nützliche Sprungzielkandidaten wie ELF/FDT/Linux/OpenSBI/Cmdline/rootfs an. `Initcall classifier` / `Initcall timeline` klassifiziert und mit Zeitstempeln versehene Linux-Initcall-/Treiber-Probe-Protokolle. `Panic summary` extrahiert Zeilen um Panic/Oops/Fehler herum und löst Adressen auf, wenn Symbole vorhanden sind. `Boot analysis JSON` speichert diese zusammen. `Dmesg extract` extrahiert nur Linux-ähnliche Zeilen aus UART-/virtio-console-Ausgaben. `Decoded MMIO` zeigt die neuesten MMIO-Zugriffe mit Registernamen an.

Das `Triage dashboard` kombiniert Stopp-Ursachen-Rankings, virtqueue-Anomalie-Schweregrade, Geräte-Probes und Abfrage-Lesezeichen in einem einzigen Bildschirmtext. `Stop-cause ranking` priorisiert Kernel-Panics, Oops, illegale Instruktionen, Seiten-/Zugriffsfehler, virtqueue-Anomalien und blockierte Geräte-Probes aus Konsolen/Traces/Status. Die `Stop-cause evidence` zeigt die Begründung für das Ranking, die Punkteaufschlüsselung, empfohlene Abfragen und die nächsten Prüfpunkte an. Sie können Unterschiede bei Status-/Phasen-/Geräte-/Anomalie-/Speicherzählungen im Dashboard vergleichen, indem Sie `Save triage baseline` → Run → `Triage diff` ausführen. `Save preset baseline` → Run → `Compare preset baseline` ermöglicht es Ihnen zu überprüfen, ob die Trefferzahl von voreingestellten Abfragen wie panic/virtio/satp/rootfs seit dem letzten Mal zu- oder abgenommen hat. `Memory dump hits` erstellt Hex-/ASCII-Dumps um Speicherindex-Treffer herum unter Verwendung von Diagnoseabfragen oder Trace-Filtern. `Memory range dump` gibt eine beliebige Adresse/Länge an, um das DRAM direkt hexadezimal/in ASCII zu dumpen. `Redacted share MD/JSON/HTML` ersetzt E-Mails / MACs / IPv4s durch `<email>` / `<mac>` / `<ipv4>` vor dem Teilen. `Redaction options JSON` schaltet `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex` um.

`Smoke preset` setzt das aktuell ausgewählte Boot-Preset zurück und führt nur die angegebenen Schritte aus. Die `Smoke matrix` führt sequenziell eine Preset-Liste wie `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` aus und listet die Ausführungsschritte, die letzte Phase und die Stopp-Ursachen-Kandidaten für jedes Preset auf.

PC-Breakpoints werden hinzugefügt, indem der Hex-Wert eines physischen/virtuellen PCs in das Feld `PC breakpoint` unter `Breakpoints / watchpoints` eingegeben wird. `Run` / `Step 1k` stoppt an einem Breakpoint, und `Step` geht genau 1 Instruktion weiter, selbst wenn der aktuelle PC ein Breakpoint ist. Schreib-Watchpoints erkennen Bus-Schreibvorgänge in einem physischen Adressbereich, und Lese-Watchpoints sind eine einfache Funktion zum Erkennen von Bus-Lesevorgängen. Sie sind nützlich, um MMIO-Probes, Framebuffer-Schreibvorgänge und Referenzen auf spezifische Strukturen zu überprüfen. `Access timeline` / `Compact access` zeigt aktuelle DRAM-/MMIO-Zugriffe in einer komprimierten Zeitreihe an. `PC profile on` aggregiert heiße PCs, und `Capture snapshot` → Run → `Diff snapshot` ermöglicht die Überprüfung von Diagnoseunterschieden vor und nach der Ausführung.

simple-framebuffer bereitet einen 1024x768x32bpp-Speicher bei `0x86000000` vor und platziert ihn als `simple-framebuffer` in `/chosen/framebuffer@86000000` des automatisch generierten DTB. Wenn simplefb auf der Linux-Seite verwendbar ist, kann es mit `Render framebuffer` auf dem Canvas angezeigt werden.

virtio-net stellt vom Browser aus keine Verbindung zu einem echten Netzwerk her; es ist ein Debug-Gerät auf Paketebene. Geben Sie Ethernet-Frame-Hex in `virtio-net debug` ein, um es in RX einzuspeisen, und die vom Gast an die TX-Queue gesendeten Frames können in `Show TX frames` überprüft werden. Damit es auf der Linux-Seite erkannt wird, führen Sie bei Bedarf Befehle wie `ip link set dev eth0 up` auf der Gast-Seite aus.

virtio-rng ist ein Verifizierungsgerät, das einen deterministischen PRNG als Gast-Entropiequelle präsentiert. Um die Reproduzierbarkeit aufrechtzuerhalten, ist der Standard-Seed festgelegt und kann über `Set deterministic seed` in der UI geändert werden.

virtio-gpu ist ein minimales Gerät zur Beobachtung der Linux-virtio-gpu-Treiber-Probes und des 2D-Ressourcen-Setups. Anstelle einer echten GPU-Beschleunigung verfolgt es die Modeset-/Scanout-/Flush-Befehle, die in der Control-Queue ankommen, und gibt die Zustände an die Diagnosen aus. Da es auch vom Ressourcen-Hintergrundspeicher in den simple-framebuffer kopiert, können Sie das Ergebnis der Leerung (Flush) einer 2D-Ressource durch den Gast über `Render framebuffer` / PNG-Export überprüfen. `UPDATE_CURSOR` / `MOVE_CURSOR` in der Cursor-Queue werden ebenfalls als Zustände aufgezeichnet.

`SBI shim on` dient zum direkten Debuggen von S-Modus-Payloads ohne OpenSBI. Lassen Sie es für normale Experimente mit `fw_dynamic.bin` / `fw_payload.bin` deaktiviert.

Wenn Sie Multi-hart testen möchten, stellen Sie bitte den `Hart count` ein, bevor Sie die Firmware laden. Da das Ändern der Einstellungen einen Maschinen-Reset mit sich bringt, wird davon ausgegangen, dass Sie die Firmware / Payload / Disk danach neu laden. `View hart` ermöglicht das Umschalten der Register / CSRs / Traces des angezeigten Ziel-Harts.

Wenn Sie `fw_dynamic.bin` verwenden, laden Sie die S-Modus-Payload / den Kernel bei Bedarf um `0x80200000` herum über `Load payload`. Der Emulator platziert dynamische Infos bei `0x87dff000` und setzt seine Adresse auf `a2`.

Für Linux-Experimente können Sie eines der folgenden verwenden:

- `Load disk`: Übergeben Sie ein rohes Disk-Image wie rootfs als virtio-blk. Die Standard-bootargs sind `root=/dev/vda rw`.
- `Load initrd`: Platzieren Sie initramfs bei `0x84000000` und spiegeln Sie den initrd-Bereich im generierten DTB wider. Ändern Sie die bootargs bei Bedarf auf `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` usw.

Beispiel unter Verwendung von vorverteilten RISC-V-Binärdateien von OpenSBI 1.8.1:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# Laden Sie fw_dynamic.bin / fw_payload.bin / fw_jump.bin aus den extrahierten Dateien über den Browser
```

Wenn Sie OpenSBI lokal erstellen, bereiten Sie eine RISC-V-Toolchain wie `riscv64-unknown-elf-` vor und erstellen Sie sie mit `PLATFORM=generic`.

### Entwicklungsbefehle

```bash
go test ./...
make wasm
make serve
```

## Hinweis

Diese Implementierung enthält schrittweise die Funktionen, die zur Untersuchung der OpenSBI-Initialisierung, des Übergangs der S-Modus-Payload und des Linux-Boots erforderlich sind. Für den Linux-Boot wurden PMP, Sv39, virtio-blk, virtio-console, virtio-net, virtio-rng, virtio-input, virtio-gpu, initrd, simple-framebuffer, Trap-/CSR-Genauigkeit, CSR-Trace/Zusammenfassung, MMIO-Histogramm/Zeitachse/Register-Decoder, Boot-Phasen-/Zeitachsen-Analysator, Geräte-Probe-Analysator, virtqueue-Inspektor/Deskriptor-Ketten-Visualisierer/DOT-Export/Snapshot/Anomaliedetektor/Anomalie-Hinweise/Anomalie-Triage, Gast-Speicher-Scanner/Index/Diff/Suche/Sprunghinweise/Dump-Helfer, integrierte Diagnoseabfrage/Abfrage-Voreinstellungen/Vergleich der Voreinstellungs-Baseline, Triage-Dashboard/Stopp-Ursachen-Ranking, Share-Report-Bundle/HTML/Redaktion, Initcall-Klassifikator/Zeitachse, DWARF-Zeilensuche/Quellkontext/Trace-Anmerkung, Panic-Zusammenfassung, dmesg-Extraktor, Trace-Replay/Vergleich, Boot-Regressionsberichte, PC-Profiling, Snapshot-Diff, Boot-Smoke-Runner/Smoke-Matrix, Triage-Baseline-Diff, Stopp-Ursachen-Evidenz, bearbeitbare Redaktion und Speicherbereich-Dump hinzugefügt. Die wichtigsten nicht implementierten oder vereinfachten Teile umfassen ein genaues Zyklus-/Zeitmodell, AIA/IMSIC, echte Netzwerkverbindung über tap/WebSocket-Brücken, vollständige virgl-/DRM-/GPU-Beschleunigung, striktes WARL-/WPRI-Verhalten für alle CSRs und echte parallele Ausführung mit mehreren Workern. Multi-hart ist ein kooperatives Scheduling innerhalb eines einzigen wasm-Workers.

## Diagnosen und Regressionshilfen

Verbesserte Teilbarkeit für Smoke-Matrizen und Diagnoseabfragen.

- `Smoke matrix MD/HTML` speichert Smoke-Matrix-Ergebnisse als Markdown / in sich geschlossenes HTML.
- `Save smoke baseline` → Run → `Compare smoke baseline` ermöglicht es Ihnen, die Unterschiede in Bezug auf Phase, Ausführungsschritte und die wichtigsten Stopp-Ursachen für jedes Preset zu überprüfen.
- Die `Stop checklist` erstellt eine Checkliste mit bestimmten Aktionspunkten, die basierend auf dem Ranking der Stopp-Ursachen als Nächstes überprüft werden müssen.
- `CSR/MMIO bookmarks` extrahiert nur die entscheidenden CSR- / MMIO- / Trace-Treffer aus den Ergebnissen der integrierten Diagnoseabfrage.
- `Watchpoint hits` zeigt den Verlauf der Lese-/Schreib-Watchpoint-Treffer in einer Zeitreihe an. `Clear hit timeline` löscht nur den Verlauf.
- Das `Artifact manifest` listet die aktuell geladene Firmware / Payload / Disk / initrd / Symbole sowie die generierten DTB- / dynamischen Info-Bereiche, Einträge und SHA-256-Hashes auf.

### Hilfen für die Regressionsübergabe

- `Manifest diff` / `Manifest diff JSON` vergleicht das aktuelle Boot-Artefakt-Manifest mit einer im localStorage gespeicherten Baseline und zeigt Unterschiede bei bootargs, Hart-Anzahlen, Ladebereichen, Einträgen, ELF-Erkennung und SHA-256-Hashes an.
- `Auto break/watch suggestions` generiert Kandidaten für PC-Breakpoints / Lese-Watchpoints / Schreib-Watchpoints, die für den nächsten Lauf basierend auf der Stopp-Ursachen-Evidenz, aktuellen Trace-PCs und Watchpoint-Treffer-Zeitachsen festgelegt werden sollen.
- `Smoke clusters` / `Smoke clusters JSON` bündelt die Smoke-Matrix-Preset-Ergebnisse nach Phase und oberster Stopp-Ursache und gruppiert Presets mit demselben Fehlertyp.
- Das `Diagnostic bundle JSON` ist ein in sich geschlossenes JSON, das das Manifest, das Triage-Dashboard, Stopp-Ursachen, Breakpoint-Vorschläge, das Share-Bundle und Watchpoint-Treffer gruppiert.
- Das `Compressed bundle JSON` ist das obige Diagnose-Bundle, konvertiert in gzip+base64. Verwenden Sie dies, wenn Sie die Größe reduzieren möchten, bevor Sie es in Issues oder Chats einfügen.

### Hilfen für Übergabe / Herkunft (Provenance)

- `Decode bundle` extrahiert ein eingefügtes `Diagnostic bundle JSON` oder `Compressed bundle JSON` oder rohes gzip+base64.
- `Bundle compare` / `Bundle compare JSON` vergleicht ein eingefügtes vergangenes Bundle mit dem aktuellen Bundle und zeigt Unterschiede bei Triage-Phasen, Top-Stopp-Ursachen, Manifesten, Artefakt-Hashes, Smoke-Clustern, Watchpoint-Treffern und Vorschlagszählungen an.
- `Provenance` / `Provenance JSON` fasst die SHA-256-Hashes des Manifests, des Traces, der Konsole und des Diagnose-Bundles, Trace-Zeilenzählungen, Konsolen-Byte-Zählungen und Top-Stopp-Ursachen zusammen. Kann zur Überprüfung der Reproduzierbarkeit oder als Beweismittel im Anhang von Issues verwendet werden.
- `Handoff MD` fasst Herkunft, Top-Stopp-Ursachen, Auto-Break-/Watch-Vorschläge, Stopp-Checklisten, Baseline-Diffs und Artefakt-Manifeste in Markdown zusammen.
- `Apply auto breaks` wendet die Top-Kandidaten von Auto-Break-/Watch-Vorschlägen in großen Mengen auf den aktuellen Emulator an. Ein Dienstprogramm zum schnellen Einrichten von Stopppositionen oder verdächtigen MMIO-/DRAM-Bereichen vor dem erneuten Ausführen.

### Reproduktion / Signatur / Headless-Übergabe

- `Repro plan` / `Repro MD` / `Repro JSON`
  - Generiert Reproduktionsschritte aus Diagnose-Bundles, Herkunft und Artefakt-Manifesten.
  - Listet die Rollen, Größen, Ladebereiche und SHA-256-Hashes von Firmware / Payload / initrd / Disk / Symbolen als Artefakt-Pins auf.
  - Dokumentiert Smoke-Presets, bootargs, Hart-Anzahlen, next_addr und empfohlene Break-/Watch-Bedingungen in Schritten.
- `Log signature` / `Log signature JSON`
  - Erstellt eine leichtgewichtige Zusammenfassung aus den SHA-256-Hashes von Traces / Konsolen / Manifesten, Trace-Zeilenzählungen, ersten/letzten PCs, ersten/letzten Konsolenzeilen und häufigen Token.
  - Ermöglicht es Ihnen zu vergleichen: „Ist dies dasselbe Protokoll?“ oder „Was hat sich geändert?“, ohne rohe Traces einzufügen.
- `Save log signature` / `Load log signature` / `Compare log signature`
  - Speichert die Protokollsignatur-Baseline im localStorage des Browsers und vergleicht sie mit der aktuellen Signatur.
  - Zeigt Unterschiede bei Trace-Hashes, Konsolen-Hashes, Manifest-Hashes, Zeilenzählungen, letzten PCs und letzten Konsolenzeilen an.
- `Auto break verify`
  - Zeigt eine Bestätigungszusammenfassung an, bevor automatische Breakpoint-/Watchpoint-Vorschläge angewendet werden.
  - Gibt Warnungen für doppelte Vorschläge oder verdächtige PC-Bereiche aus.
- `Headless smoke script`
  - Generiert ein Shell-Skript-Skelett für CI/Übergabe aus dem aktuellen Artefakt-Manifest, bootargs, Hart-Anzahlen, Smoke-Presets und Schrittzahlen.
  - Soll Artefakt-Pins und Preset-Matrizen fixieren, bevor Browser-Harnesse wie Playwright zur Ausführungsumgebung hinzugefügt werden.

#### Headless / CI-Hilfen

Um das Handling von Repro-/Signatur-Übergaben in CI oder Issues zu erleichtern, wurde Folgendes hinzugefügt.

- `Bundle integrity` / `Integrity JSON` überprüft die Konsistenz zwischen dem Diagnose-Bundle und dem Artefakt-Manifest und kategorisiert Diskrepanzen bei Artefakt-Rollen, SHA-256-Hashes, Ladebereichen, Vorschlägen und Smoke-Ergebnissen als `error`, `warn` oder `info`.
- `Repro validation` / `Repro validation JSON` verifiziert, ob der aktuelle Reproduktionsplan mit den bootargs des Bundles, den Hart-Anzahlen, next_addr, den Artefakt-Pins, den Top-Stopp-Ursachen und den Protokollsignaturen übereinstimmt.
- `CI summary` / `CI summary JSON` konsolidiert die Bundle-Integrität, Trace-/Konsolen-Signaturen, Smoke-Ergebnisse und Stopp-Ursachen und gibt eine Zusammenfassung aus, die Pass-/Warn-/Fail-Beurteilungen in CI erleichtert.
- `Headless runner spec` / `Runner spec JSON` generiert Presets, Schritte, Artefakt-Pins und empfohlene Befehle zur Inspektion mit `go run ./cmd/rvsmoke ...`.
- `cmd/rvsmoke` hinzugefügt. Es kann Diagnose-Bundles / Artefakt-Manifeste außerhalb des Browsers lesen und Artefakt-Hashes, Bundle-Integrität, CI-Zusammenfassungen und Runner-Spezifikationen in Text / JSON / Markdown ausgeben.

Beispiel:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

`rvsmoke` führt derzeit Reproduzierbarkeitsprüfungen und CI-Zusammenfassungsgenerierungen für Bundle-/Manifest- und Artefakt-Hashes durch. Die CPU-Ausführung selbst wird weiterhin die Smoke-Matrix auf der Browser-JS-/WASM-Seite verwenden.

#### rvsmoke CI Gate / JUnit / SARIF

`cmd/rvsmoke` ist eine Hilfs-CLI zur Inspektion exportierter Diagnose-Bundles / Manifeste in CI. Durch die Materialisierung der Headless-Ausführung kann es Baseline-Bundle-Vergleiche, CI-Gate-Richtlinien, JUnit-XMLs, SARIFs und in sich geschlossene HTML-Berichte ausgeben.

Beispiel:

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

Beispiel einer Richtlinien-JSON:

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

`-out html` druckt in sich geschlossenes HTML nach stdout, `-out junit` für JUnit XML und `-out sarif` für SARIF JSON. Wenn `-junit` / `-html` / `-sarif` gleichzeitig angegeben werden, speichern sie diese zusätzlich zum stdout-Format in ihren jeweiligen Dateien. CI-Gates normalisieren Artefakt-Manifeste, Trace-/Konsolen-Signaturen, Baseline-Diffs, virtqueue-Anomalien und Smoke-Ergebnisse in `pass`, `warn` oder `fail`.

#### rvsmoke Richtlinienvorlagen / Vergleich des Bundle-Trends

Um die anfängliche Einführung des CI-Gates und Vergleiche mehrerer Regressionen zu erleichtern, wurden Richtlinienvorlagen, Aktionschecklisten und Vergleiche von Bundle-Trends zu `rvsmoke` und der Browser-UI hinzugefügt.

- `CI policy templates` / `Policy templates JSON` zeigen integrierte Richtlinien an: `default`, `strict`, `linux-boot`, `artifact-only` und `lenient`.
- `Policy template JSON` speichert die angegebene Vorlage als ein JSON, das direkt in CI abgelegt werden kann.
- `CI gate` / `CI gate JSON` wendet eine Richtlinienvorlage auf den aktuellen Browserstatus an und zeigt Pass-/Warn-/Fail-Gate-Prüfungen an.
- `CI checklist` / `CI checklist JSON` wandelt Gate-Ausfälle, Bundle-Integrität und Artefakt-Diffs in umsetzbare Checklisten um.
- `rvsmoke -compare name=bundle.json` richtet mehrere Bundles chronologisch aus und gibt Trendberichte aus, die Änderungen bei Phasen, Top-Stopp-Ursachen, Artefakt-Hashes und Smoke-Clustern zeigen.

Beispiel für die Generierung einer Richtlinienvorlage:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

Beispiel für den Vergleich mehrerer Bundles:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

`-policy-template` dient als Standardrichtlinie, wenn `-policy` nicht angegeben ist. Wenn `-policy` angegeben ist, hat die JSON der Datei Vorrang.

## rvsmoke CI-Integration

Erweiterte CI-/Übergabehilfen für `rvsmoke`.

- `rvsmoke -print-github-actions linux-boot` kann GitHub Actions Workflow-YAMLs generieren.
- `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` kann Workflows in Dateien ausgeben.
- `rvsmoke -policy-tree policy-tree.md` kann CI-Gates / Bundle-Integrität / Baseline-Drifts als Ursachenbäume speichern.
- `rvsmoke -history history.txt` kann Phasen- / Stopp-Ursachen- / Artefakt-Drift-Aggregationen mehrerer Bundle-Trends speichern.
- `rvsmoke -repro-zip rvwasm-minimal-repro.zip` kann minimale Reproduktionspakete generieren, die READMEs, Diagnose-Bundles, Manifeste, Runner-Spezifikationen, Richtlinien, CI-Zusammenfassungen und Verifizierungsskripte enthalten.

Beispiel:

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

`-repro-zip` bettet keine rohe Firmware/Kernel/Disks ein. Es bettet SHA-256-Pins und Manifest-Bereiche in das Bundle ein, wobei erwartet wird, dass Artefakte vom Empfänger verifiziert werden.

### Inspektion des CI-Repro-ZIPs / Fortsetzung des Matrix-Workflows

Es wurden Funktionen zu `rvsmoke` und zur Browser-UI hinzugefügt, um die Übergabe minimaler Reproduktionspakete zu inspizieren, sowie Ausgaben für GitHub Actions-Matrizen / Trendvisualisierung.

- `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` kann das von `-repro-zip` generierte ZIP inspizieren, ohne es zu extrahieren. Überprüft erforderliche Dateien, unsichere Pfade, `diagnostic-bundle.json` / `manifest.json`-Übereinstimmungen, `ci-policy.json` und `scripts/rvsmoke.sh`.
- `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` kann GitHub Actions-Matrix-Workflow-YAMLs pro Preset generieren.
- `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` kann Matrix-Workflows in Dateien ausgeben.
- `rvsmoke -trend-csv rvwasm-trend.csv` und `-trend-chart-json rvwasm-trend-chart.json` können Bundle-Trends als CSV / JSON für einfaches externes Diagramm-Zeichnen speichern.
- `Minimal repro ZIP`, `Inspect repro ZIP`, `Repro ZIP JSON`, `Matrix workflow YAML`, `Trend chart JSON` und `Trend CSV` zur Browser-UI hinzugefügt.

Beispiel:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# Inspektionsergebnisse als JSON speichern
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# Trends des aktuellen Bundles und des vorherigen Bundles in CSV/JSON konvertieren
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` kann eigenständig ausgeführt werden. Wenn es zusammen mit `-bundle` angegeben wird, schließt es die ZIP-Inspektionsergebnisse in die normale CI-Zusammenfassung ein und behandelt Fehlschläge als Fehlschläge der CI-Zusammenfassung.

### CI-Matrix-Aggregation / Fortsetzung des Checksum-Manifests

Erweiterte CI-Artefakt-Übergaben für `rvsmoke`.

- `-repro-checksums rvwasm-repro-checksums.json` kann deterministische Checksum-Manifeste für Dateien innerhalb des ZIPs speichern, basierend auf `-inspect-repro-zip`-Ergebnissen.
- Durch Angabe mehrerer `-matrix-result name=rvsmoke-output.json` können Sie `rvsmoke -out json`-Ergebnisse aus mehreren Presets / mehreren Jobs aggregieren.
- `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` kann Matrix-Ergebnisse als Text / JSON / in sich geschlossenes HTML speichern.
- `-trend-html rvwasm-trend.html` kann Bundle-Trendberichte als eigenständige HTMLs speichern.

Beispiel:

```bash
# Inhalt des minimalen Reproduktions-ZIPs und des Checksum-Manifests speichern
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# rvsmoke JSONs aus mehreren Matrix-Jobs aggregieren
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

Matrix-Aggregate fassen CI-Status, Gate-Fehler-/Warnungszählungen, Artefakt-Diskrepanzen und Top-Stopp-Ursachen pro Job zusammen. Es ist ein Dienstprogramm, um allgemeine Fehlertrends im endgültigen Aggregations-Job leicht zu erkennen, selbst wenn GitHub Actions-Matrix-Jobs aufgeteilt sind.

#### Hilfen für CI- / Release-Übergaben

Verbesserte Verwaltung von CI-Artefakten und Release-Übergaben für `rvsmoke`.

- `-artifact-index rvwasm-artifacts.json` fasst die Pfade, Bytes und SHA-256-Hashes der generierten CI-Artefakte wie JUnit / SARIF / HTML / Trend / Matrix / Repro-Checksums zusammen.
- `-release-manifest rvwasm-release.json` bündelt Diagnose-Bundles, Protokollsignaturen, CI-Gates, Matrix-Aggregate, Flake-Berichte, Artefaktindizes und Repro-Checksum-Verifizierungen in einem Übergabemanifest.
- `-release-html rvwasm-release.html` gibt ein in sich geschlossenes HTML mit Navigation zu Summary / Artifacts / Matrix / Checksums / JSON aus.
- `-verify-repro-checksums baseline-repro-checksums.json` vergleicht das Checksum-Manifest des aktuell inspizierten minimalen Repro-ZIPs mit einer Baseline, um fehlende / geänderte / zusätzliche Einträge zu erkennen.
- `-matrix-flakes`, `-matrix-flakes-json` und `-matrix-flakes-html` normalisieren mehrere Matrix-Ergebnisse wie `uart#1` / `uart#2`, um zu erkennen, ob dasselbe Preset zwischen Pass/Fail schwankt (flake).

Beispiel:

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

## Release-Übergabe und Verifizierung

Es wurden Metadatenausgaben zu `rvsmoke` hinzugefügt, um CI-Ergebnisse an andere Maschinen, andere Repositories oder Reviewer zu übergeben.

### SBOM / Herkunftserweiterung (Provenance Extension)

#### SBOM-lite Abhängigkeitsinventar

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

Diese Abhängigkeitsliste soll in einem kleinen, deterministischen Format vorliegen. Sie liest `go.mod` und erfasst Modulpfade, Go-Versionen, direkte `require`-Zeilen, `replace`-Ziele und Artefakttypen, die im CI-Artefaktindex enthalten sind.

Wenn Sie `rvsmoke` aus einem anderen Arbeitsverzeichnis ausführen, geben Sie `-go-mod /path/to/go.mod` an.

#### Herkunftsbescheinigung (Provenance Attestation)

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

Die Bescheinigung ist ein JSON-Payload, das von in-toto / SLSA inspiriert ist. Es ist an sich keine Signatur, aber da es einen stabilen SHA-256 aufweist, kann es als Ziel für externe CI-Tools zum Signieren verwendet werden.

#### Release-Übergabe-ZIP

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

Das Release-Übergabe-ZIP enthält nur Metadaten.

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

Es bettet keine Firmware, Kernel, initrds oder Disk-Images ein. Große Artefakte werden als SHA-256-Pins im Manifest aufbewahrt.

#### Inspektion des Release-Übergabe-ZIPs

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

Der Inspektor überprüft das ZIP, ohne es zu extrahieren, auf erforderliche Dateien, gefährliche Pfade, doppelte Pfade, JSON-Parsierbarkeit und grundlegende Konsistenz zwischen Releases / Indizes / SBOMs / Bescheinigungen.

### Release-Verifizierung

Zusätzlich zur Erstellung von Release-Übergabe-ZIPs wurden verifizierungsorientierte Ausgaben hinzugefügt.

- `-verify-attestation` / `-verify-attestation-text` bestätigen, ob der deterministische Herkunftsbescheinigungs-Hash, die Release-Materialien und die CI-Artefaktsubjekte mit dem generierten Release-Manifest, dem SBOM-lite-Inventar und dem Artefaktindex übereinstimmen.
- `-sbom-baseline`, `-sbom-diff` und `-sbom-diff-json` vergleichen das aktuelle SBOM-lite-Abhängigkeitsinventar mit einer gespeicherten Baseline.
- `-compare-release-zip-inspection`, `-release-zip-compare` und `-release-zip-compare-json` vergleichen das aktuell inspizierte Release-Übergabe-ZIP mit vergangenen Inspektions-JSONs.
- `-retention-manifest` / `-retention-text` generieren ein CI-Artefakt-Aufbewahrungsmanifest, das Pfade, Arten, Bytes, SHA-256, Aufbewahrungstage, Ablaufzeiten und Gründe enthält.
- `-release-verification-html` gibt HTML mit Navigation aus, das den Release-Status, Bescheinigungsverifizierungen, SBOM-Diffs, Release-ZIP-Vergleiche und Aufbewahrungsinformationen zusammenfasst.

Beispiel:

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

### Release-Audit-Gate

Eine abschließende Release-Audit-Schicht wurde über die Release-Verifizierung gelegt. Sie fasst Verifizierungen der Herkunftsbescheinigung, SBOM-lite-Diffs, Release-ZIP-Vergleiche, Abläufe der Artefaktaufbewahrung, Matrix-Flake-Status und Release-Manifest-Status in einer einzigen Bewertung und einem Gate-Bericht zusammen.

Wichtigste Flags:

- `-list-release-verify-policies` listet integrierte Release-Audit-Richtlinien auf.
- `-print-release-verify-policy strict` gibt eine Richtlinien-JSON-Vorlage aus.
- `-release-verify-template default|strict|lenient|archive` wählt eine integrierte Richtlinie aus.
- `-release-verify-policy policy.json` lädt eine benutzerdefinierte Release-Audit-Richtlinie.
- `-retention-audit` / `-retention-audit-json` schreibt Inspektionsergebnisse zu Ablauf und Mindestaufbewahrung aus.
- `-release-score` / `-release-score-json` schreibt eine Release-Verifizierungsbewertung von 0 bis 100 aus.
- `-release-gate` / `-release-gate-json` schreibt Richtlinien-Gate-Ergebnisse aus.
- `-release-audit` / `-release-audit-json` / `-release-audit-html` schreibt einen integrierten Auditbericht aus.

Beispiel:

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

Die strenge Richtlinie behandelt nicht bestehende Release-Manifeste, fehlgeschlagene Attestierungs- / SBOM- / ZIP-Prüfungen, abgelaufene Artefakte und Artefakte, die unter den festgelegten Mindestaufbewahrungstagen liegen, als Fehler. Die Standardrichtlinie eignet sich für tägliche Überprüfungen wie nächtliche Übergaben, lässt Warnungen zu, lässt CI jedoch bei eindeutigen Verifizierungsfehlern fehlschlagen.

#### Release-Audit-Diff / Ausnahmeregelungen (Waivers) / TODO-Übergabe

Der `rvsmoke`-Release-Audit-Pfad unterstützt den Vergleich des aktuellen Audits mit einem vergangenen Audit, die Anwendung zeitlich begrenzter Ausnahmeregelungen auf bekannte Probleme und die Generierung von Checklisten für Aufgaben ohne Ausnahmeregelung.

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

Sie können eine Ausnahmeregelungsvorlage mit dem folgenden Befehl erstellen:

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

Ausnahmeregelungen werden verwendet, um bekannte, temporäre Release-Audit-Ergebnisse zu behandeln. Jede Regel hat eine ID, beliebige Art- / Namens- / Status- / Teilstring-Matcher, einen Eigentümer, einen Grund und einen `expires_at`-Zeitstempel. Abgelaufene Ausnahmeregelungen werden gemeldet, aber nicht verwendet, um Probleme zu unterdrücken.

#### Release-Entscheidung / Evidenz-Bundle

Es wurden abschließende Übergabehilfen hinzugefügt, die nach der Ausführung von Release-Audits verwendet werden sollen.

- `-waiver-calendar`, `-waiver-calendar-json` und `-waiver-calendar-html` zeigen für jede Ausnahmeregelung den Ablauf, den Eigentümer, die Übereinstimmungszählungen und die abgelaufenen / bald ablaufenden Zustände an.
- `-release-changelog` und `-release-changelog-json` fassen Audit-Diffs, Ausnahmeregelungszustände, TODO-Zählungen und Ablaufordner von Ausnahmeregelungen als für Menschen lesbare Änderungsprotokolle zusammen.
- `-final-decision` und `-final-decision-json` generieren endgültige `go`-, `go-with-watch`- und `no-go`-Entscheidungen, die blockierende Elemente und nächste Aktionen enthalten.
- `-release-evidence-zip` schreibt ein kleines Evidenz-Bundle aus, das Audits, Ausnahmeregelungsberichte, TODO-Listen, Ausnahmeregelungskalender, Änderungsprotokolle und endgültige Entscheidungen enthält.
- `-inspect-release-evidence-zip` inspiziert Evidenz-Bundles, ohne sie zu extrahieren, auf erforderliche Dateien, gefährliche Pfade, doppelte Einträge und JSON-Parsierbarkeit.
- `-dry-run` berechnet Berichte, ohne optionale Ausgabedateien zu schreiben.
- `-exit-code-mode never` gibt Ergebnisse auch in Fällen aus, in denen es normalerweise mit einem Gate-Fehler fehlschlagen würde.

Beispiel:

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

Beispiel für die Inspektion eines Evidenz-Bundles in CI:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## Lizenz

Dieses Projekt ist unter der BSD 2-Clause License lizenziert. Details finden Sie in der Datei [LICENSE](../LICENSE).

SPDX-License-Identifier: BSD-2-Clause
