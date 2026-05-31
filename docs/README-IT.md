# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## Panoramica

Un emulatore RV64IMAC in esecuzione su Go 1.23.2 `GOOS=js GOARCH=wasm`. L'impostazione predefinita è a singolo hart, ma è disponibile dalla UI del browser la programmazione cooperativa da 1 a 8 hart. È possibile caricare OpenSBI 1.8.1 `fw_payload.bin`/`fw_jump.bin`/`fw_dynamic.bin`/ELF dalla UI per confermare l'avvio.

![OpenSBI fw_payload boot on rvwasm](images/fw_payload.png)

L'avvio di OpenSBI 1.8.1 `fw_payload.bin` su rvwasm e l'ingresso nel payload di modalità S della fase successiva.

## Funzionalità implementate

- Istruzioni di base RV64I
- Estensione M
- Implementazione minima di LR/SC/AMO dell'estensione A
- Istruzioni intere comuni dell'estensione C
- Equivalenti Zicsr/Zifencei
- Implementazione minima della modalità di privilegio M/S/U CSR/trap/mret/sret
  - Corregge l'eccezione sincrona `mepc`/`sepc` sul PC dell'istruzione che causa l'errore
  - Corregge il caricamento/scrittura CSR con errore per non corrompere rd e ferma l'avanzamento del contatore di ritiro (retire counter)
  - Controllo dell'esistenza del CSR, soppressione degli effetti collaterali di scrittura CSR in sola lettura, riflessione di base di `mcounteren`/`scounteren`
  - Aggiunti stub CSR `senvcfg`/abilitazione stato per le sonde (probe) di Linux
  - Riflessione di base di `TVM`/`TW`/`TSR` e pulizia di `MPRV`
- MMU Sv39
  - Modalità `satp` Bare/Sv39
  - Percorso della tabella delle pagine a 3 livelli
  - Foglie da 4 KiB/2 MiB/1 GiB
  - Riflessione di base di `SUM`/`MXR`/`MPRV`
  - Eccezione di page fault (page fault exception)
  - Aggiornamento automatico dei bit `A`/`D` del PTE
- MMIO in stile UART 16550 (`0x10000000`)
  - Output dal guest
  - Iniezione di input dalla UI del browser
  - Interrupt di ricezione
- mtime/mtimecmp/msip in stile CLINT (`0x02000000`)
  - Instradamento MSIP/MTIMECMP per hart per multi-hart
- Controller di interrupt in stile PLIC (`0x0c000000`)
  - priority/pending/enable/threshold
  - claim/complete
  - Contesto M/S per hart
- Applicazione del PMP
  - TOR/NA4/NAPOT
  - Autorizzazioni R/W/X
  - Restrizioni della modalità M tramite voci bloccate
- Informazioni di avvio OpenSBI `fw_dynamic`
  - Le informazioni dinamiche sono collocate in `0x87dff000`
  - Il puntatore alle informazioni dinamiche è impostato su `a2`
  - Il payload di modalità S / kernel può essere caricato separatamente dalla UI
- Dispositivo a blocchi virtio-mmio (`0x10001000`)
  - Moderni registri MMIO in stile virtio 1.0
  - Supporto minimo per read/write/flush/get-id di virtqueue separate
  - Negoziazione `FEATURES_OK` e verifica `VIRTIO_F_VERSION_1`
  - Ripristino della coda, ignorare notify prima di `DRIVER_OK`, riflessione di base del flag `NO_INTERRUPT`
  - Gestione di `VIRTIO_RING_F_INDIRECT_DESC` e tabelle dei descrittori indiretti
  - Soppressione degli interrupt tramite l'evento utilizzato `VIRTIO_RING_F_EVENT_IDX`
  - Le immagini del disco possono essere caricate dalla UI
  - Le immagini del disco modificate dal guest possono essere scaricate dalla UI
- Dispositivo console virtio-mmio (`0x10002000`)
  - Console minima con ID dispositivo 3
  - Coda 0 ricezione / Coda 1 trasmissione
  - Supporto minimo per `VIRTIO_CONSOLE_F_SIZE`, descrittori indiretti e indici di eventi
  - Inietta l'input della UI sia a UART che a virtio-console
- Dispositivo di rete virtio-mmio (`0x10003000`)
  - virtio-net di debug minimo con ID dispositivo 1
  - Coda 0 ricezione / Coda 1 trasmissione
  - Supporto minimo per `VIRTIO_NET_F_MAC`/`VIRTIO_NET_F_STATUS` / descrittori indiretti / indici di eventi
  - Inietta frame Ethernet in formato esadecimale in RX dalla UI
  - Mostra i frame Ethernet inviati dal guest come log TX
- Dispositivo rng virtio-mmio (`0x10004000`)
  - Minima fonte di entropia con ID dispositivo 4
  - Supporto minimo per virtqueue separate, descrittori indiretti e indici di eventi
  - Il seme deterministico può essere impostato dalla UI
- Dispositivo di input virtio-mmio (`0x10005000`)
  - Dispositivo minimo di tastiera/input di debug con ID dispositivo 18
  - Supporto minimo per coda di eventi / coda di stato, descrittori indiretti e indici di eventi
  - Gli eventi chiave / eventi di input non elaborati possono essere iniettati dalla UI
- Dispositivo gpu virtio-mmio (`0x10006000`)
  - Minima base virtio-gpu 2D per il debug con ID dispositivo 16
  - Supporto minimo per code di controllo / cursore, descrittori indiretti e indici di eventi
  - Risposte di base per `GET_DISPLAY_INFO`/`RESOURCE_CREATE_2D`/`SET_SCANOUT`/`FLUSH`, ecc.
  - Utile per osservare i probe virtio-gpu di Linux e i comandi modeset iniziali
- Passaggio di initrd/initramfs
  - Indirizzo di caricamento predefinito: `0x84000000`
  - Riflesso in `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end` del DTB generato automaticamente
- Modifica dei bootargs
  - Predefinito: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - Preimpostazioni per UART / virtio-console / initramfs / debug dettagliato
  - Può essere impostato dalla UI e riflesso nel DTB generato automaticamente
- Buffer circolare di traccia dell'esecuzione
  - PC/istruzioni/trap/ultima causa trap/tval possono essere visualizzati nella UI
  - Le esportazioni di testo/JSON/CSV dei dump CSR e degli snapshot delle tracce dell'intero hart sono disponibili dalla UI
  - Diagnostica che mostra gli ultimi argomenti ECALL/SBI, contatori SBI BASE/TIME/IPI/RFENCE/HSM/SRST/legacy, trap e stati della coda virtio a colpo d'occhio
  - Esportazione JSON di Diagnostica / stati dei dispositivi
  - Carica i simboli ELF/System.map, visualizza i simboli attorno al PC bloccato, ricerca dei nomi e risoluzione automatica dei simboli del PC nei log panic/oops
  - Shim SBI arbitrario per testare direttamente piccoli payload di modalità S senza OpenSBI
    - Cortocircuito minimo di BASE/TIME/IPI/RFENCE/HSM/SRST
    - Percorso di debug verso l'ingresso in modalità S dell'hart di destinazione tramite HSM `hart_start`
    - Disabilitato per impostazione predefinita. Non utilizzato nel percorso normale per eseguire OpenSBI
  - Le gamme di memoria fisica arbitrarie possono essere scaricate (dump) dalla UI
  - I punti di interruzione (breakpoint) del PC, i punti di osservazione (watchpoint) di lettura/scrittura fisica e i filtri di traccia possono essere impostati dalla UI
  - I breakpoint possono specificare i conteggi dei colpi (hit), le condizioni della modalità e le condizioni dell'hart
  - La traccia visualizza mnemoniche di decodifica semplificate insieme a istruzioni grezze
  - I colpi di breakpoint/watchpoint registrano il motivo dell'arresto nelle esportazioni di stato/diagnostica/traccia
  - Raccoglie gli istogrammi di accesso MMIO/DRAM, consentendo di verificare le distorsioni nei probe dei dispositivi e nelle attività delle code tramite Diagnostica/JSON
  - Salva le tempistiche di accesso MMIO/DRAM nel buffer circolare, consentendo di verificare la serie temporale dei probe in viste grezze/compatte
  - La tempistica di accesso MMIO aggiunge nomi dei decodificatori di registro per virtio-mmio/UART/CLINT/PLIC, consentendo l'osservazione in unità come `QueueNotify`/`Status`/`LSR`
  - Facoltativamente abilita la traccia di accesso CSR per visualizzare le code di lettura/scrittura CSR del guest e i riepiloghi di lettura/scrittura per CSR in Diagnostica / esportazioni di traccia
  - Facoltativamente abilita il profilo hot-spot del PC per visualizzare i PC eseguiti di frequente con i simboli prima dell'arresto
  - L'acquisizione / il confronto (diff) degli snapshot diagnostici consente di verificare le differenze negli stati dell'hart/dispositivo/CSR/MMIO prima e dopo l'esecuzione sulla UI
  - Piega (fold) istruzioni, trap e log ECALL identici e consecutivi nella vista traccia compatta
  - L'esecutore smoke (smoke runner) per preimpostazione di avvio può eseguire automaticamente un numero specificato di step di hart del firmware/payload attualmente caricato e recuperare i risultati JSON
  - L'analizzatore della fase di avvio può riepilogare le attività di OpenSBI / Linux / panic / virtio / trap / simboli del PC insieme
  - La sequenza temporale di avvio può visualizzare i marker della console e i probe MMIO / gli stati / i QueueNotify / le rivendicazioni PLIC integrati in una serie temporale
  - L'analizzatore dei probe dei dispositivi può aggregare le letture/scritture, i registri di identità, le negoziazioni di stato e le notifiche di coda di virtio/UART/PLIC/CLINT
  - L'ispettore della virtqueue può visualizzare gli stati più recenti di QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify per dispositivo/coda
  - Il visualizzatore della catena dei descrittori traccia i descrittori di testa dall'anello disponibile e visualizza i descrittori NEXT/WRITE/INDIRECT insieme a una piccola anteprima del buffer
  - L'esportazione del grafico della catena dei descrittori può salvare e visualizzare le catene della virtqueue come DOT di Graphviz
  - Lo scanner della memoria fisica del guest può rilevare aree nella DRAM simili alle firme (magic) di ELF/FDT/gzip/xz/zstd/squashfs/cpio/ext / versione OpenSBI/Linux / BusyBox / cmdline del kernel
  - Il classificatore dei probe di initcall / driver può categorizzare le righe di log della console di Linux relative a initcall, probe, virtio, archiviazione, console, reti e grafica
  - La sequenza temporale di initcall può visualizzare le righe classificate di initcall / probe di driver in gruppi di serie temporali
  - Legge le tabelle delle righe DWARF dagli ELF con simboli, visualizzando file:riga vicino al PC corrente, i riepiloghi dei file DWARF e le annotazioni simbolo+riga per i PC di traccia
  - Il riepilogo del panico estrae automaticamente le righe attorno al panic/oops/fault nel log della console e risolve gli indirizzi con i simboli caricati
  - Il JSON di analisi dell'avvio può esportare collettivamente sequenze temporali / probe di dispositivi / virtqueue / riepiloghi dei panici
  - Il report di riproduzione della traccia può riepilogare il numero di step/trap/ecall/shim SBI, le mnemoniche frequenti e le cause di trap nella traccia
  - Il confronto della baseline della traccia può confrontare le differenze tra PC/istruzioni/trap di una traccia salvata in precedenza e la traccia corrente dall'inizio
  - La baseline della traccia può essere salvata/caricata nel/dal localStorage del browser
  - Il report di regressione dell'avvio/JSON, nonché le esportazioni di report in formato Markdown/HTML, possono salvare in blocco statistiche di traccia, eventi di avvio, probe di dispositivi, virtqueue, oggetti in memoria e conteggi di initcall
  - Lo snapshot della virtqueue può visualizzare contemporaneamente le configurazioni della coda e le catene dei descrittori
  - Il rilevatore di anomalie della virtqueue può rilevare indirizzi della coda pronta mancanti, cicli (loop) dei descrittori, lunghezze indirette non valide, buffer fuori dalla DRAM, ecc.
  - I suggerimenti sulle anomalie della virtqueue possono visualizzare suggerimenti di riparazione come QueueNum/QueueDesc/QueueReady/allineamento dei descrittori per ogni risultato di rilevamento
  - L'interrogazione diagnostica integrata può eseguire ricerche incrociate su console / tracce / tracce CSR / sequenze temporali MMIO / anomalie della virtqueue / indici di memoria utilizzando la stessa interrogazione
  - Le preimpostazioni delle interrogazioni diagnostiche consentono la ricerca batch per panici, negoziazioni virtio, QueueReady/Notifies, satp/mstatus, trap e rootfs
  - Il report condiviso MD/JSON/HTML consente di condividere regressioni di avvio, suggerimenti/triage della virtqueue, indici di memoria, preimpostazioni di interrogazione, suggerimenti di salto e hit di interrogazione in un formato autonomo
  - La dashboard di triage / classifica delle cause di arresto può visualizzare panici, trap, page/access fault, anomalie della virtqueue e probe di dispositivi in stallo in ordine di candidato
  - L'evidenza della causa di arresto visualizza le giustificazioni della classifica, i dettagli del punteggio, le query diagnostiche raccomandate e le azioni successive
  - La baseline della dashboard di triage può essere salvata nel localStorage per confrontare i conteggi di stato/fase/dispositivo/anomalia/memoria con la dashboard corrente
  - La baseline preimpostata per la diagnostica può essere salvata nel localStorage per confrontare la differenza con il conteggio degli hit (occorrenze) della preimpostazione corrente
  - Il report condiviso oscurato (redacted) MD/JSON/HTML può generare report condivisibili con IP/MAC/email oscurati
  - Le opzioni di oscuramento JSON consentono di attivare/disattivare la sostituzione di IP/MAC/email/indirizzi esadecimali lunghi dalla UI
  - Il dump degli oggetti in memoria può verificare esadecimale + ASCII attorno alle occorrenze di ricerca/indice in memoria
  - Il dump dell'intervallo di memoria può specificare un indirizzo DRAM arbitrario e una lunghezza in byte per il dump esadecimale + ASCII / l'esportazione JSON
  - L'acquisizione / il confronto (diff) della scansione della memoria può verificare i candidati dei frammenti ELF/FDT/initrd/rootfs che sono aumentati/diminuiti prima e dopo l'esecuzione
  - L'indice di memoria può raggruppare firme ELF/FDT/initrd/kernel/rootfs vicine per intervallo per creare un indice
  - Estrae i log in stile `dmesg` di Linux dagli output UART / virtio-console e risolve gli indirizzi di panic/oops con i simboli caricati
- simple-framebuffer
  - Aggiunge automaticamente `0x86000000`, 1024x768, `a8r8g8b8` a `/chosen/framebuffer@86000000` nel DTB generato
  - Disegna il framebuffer su un Canvas della UI, ed è possibile scaricare dump raw RGBA / PNG
  - Il supporto delle risorse 2D per virtio-gpu può essere copiato nel simple-framebuffer al momento del `TRANSFER_TO_HOST_2D`/`RESOURCE_FLUSH`
- DRAM `0x80000000`, 128 MiB
- Generazione automatica minima del DTB virt con virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT, oppure caricamento del DTB dalla UI
  - Aggiunti la compatibilità con `sifive,plic-1.0.0`/`sifive,clint0` e `dma-coherent` virtio
  - Genera `cpu@N` e `interrupts-extended` in base al conteggio degli hart

## Utilizzo

```bash
make serve
```

Aprire `http://localhost:8080` nel browser, selezionare il firmware OpenSBI, quindi fare clic su `Load firmware` → `Run`.

Se si desidera testare virtio-console come console Linux, è possibile modificare i bootargs in qualcosa come `console=hvc0 earlycon=sbi root=/dev/vda rw`. Per impostazione predefinita, utilizza la UART (`ttyS0`) come di consueto.

Per analizzare un PC interrotto, caricare un `System.map` Linux o un ELF con simboli utilizzando `Load symbols`, quindi utilizzare `Symbols @ PC` / `Diagnostics` / `Search symbols`. Se l'ELF con i simboli contiene tabelle di righe DWARF, è possibile verificare anche file:riga utilizzando `DWARF lines @ PC`. `DWARF file summary` mostra il numero di righe per file contenute nella tabella delle righe. Se il firmware/payload è un ELF con simboli, importa automaticamente la tabella dei simboli. `Annotated trace` annota `pc=` nella traccia con simboli/righe DWARF. `Download trace` salva uno snapshot della traccia per tutti gli hart. È inoltre possibile selezionare i formati JSON/CSV. La traccia JSON include le informazioni sul simbolo/codice sorgente se i simboli esistono. Inserire stringhe come `trap`, `ecall`, `sbi-shim`, `pc=` o `virtio` in `Trace filter` per restringere la coda/esportazione della traccia, la sequenza temporale di accesso e la visualizzazione compatta. `Compact trace` piega le istruzioni, i trap e gli ECALL identici e consecutivi. Incollando un log di panic/oops e facendo clic su `Analyze log symbols`, si risolvono gli indirizzi in stile PC a 64 bit nel log utilizzando i simboli caricati.

`Trace replay report` genera statistiche per la traccia corrente e `Trace baseline compare` incolla una traccia salvata per confrontare le differenze tra PC/istruzioni/trap con la traccia corrente dall'inizio. `Save current trace as baseline` / `Load saved baseline` mantiene la baseline nel localStorage del browser. `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` sono report di controllo delle regressioni che consolidano la sequenza temporale di avvio, i probe dei dispositivi, le virtqueue, lo scanner della memoria, la classificazione di initcall e le statistiche di traccia. È inoltre possibile verificare l'aumento o la diminuzione dei candidati ELF/FDT/initrd/rootfs nella memoria guest eseguendo `Capture memory scan` → Run → `Diff memory scan`. `DWARF source context` visualizza insieme i simboli e il file:riga DWARF attorno al PC corrente.

La `Boot phase` riassume i progressi correnti dai log della console, dagli istogrammi MMIO, dalle trap e dalle informazioni sui simboli. La `Boot timeline` allinea i traguardi della console e i probe/stati/QueueNotify MMIO in una serie temporale. Il `Device probe` aggrega gli accessi ai registri e le negoziazioni di virtio e altri, e il `Virtqueue inspect` visualizza le configurazioni delle code e gli stati di notifica per dispositivo/coda. Le `Descriptor chains` leggono le catene dei descrittori dall'anello disponibile della coda e visualizzano i descrittori indiretti e le anteprime del buffer. Il `Descriptor DOT` / `Download DOT` esporta la stessa catena come DOT di Graphviz. Le `Virtqueue anomalies` rilevano incongruenze nelle configurazioni delle code e nelle catene dei descrittori, e gli `Anomaly hints` visualizzano il punto di controllo successivo per ogni incongruenza. L'`Integrated diagnostic query` effettua ricerche incrociate in console / tracce / tracce CSR / sequenze temporali MMIO / anomalie della virtqueue / indici di memoria utilizzando parole come `virtio QueueReady`, `panic`, `satp`, `0x80200000`. Lo `Share report MD/JSON/HTML` è un bundle condivisibile che aggiunge suggerimenti/triage sulle anomalie, indici di memoria, suggerimenti sui salti in memoria, preimpostazioni di query e query hit al report di regressione dell'avvio. L'HTML può essere salvato come file autonomo con JSON incorporato. I `Diagnostic query presets` raggruppano le ricerche relative a panici, stati virtio, QueueReady/QueueNotify, satp/mstatus, trap e rootfs. `Save query` / `Load query` salva le query diagnostiche nel localStorage del browser. Il `Memory scan` cerca i candidati ai frammenti ELF/FDT/initrd/kernel/rootfs nella DRAM, e il `Memory index` raggruppa le firme vicine per intervallo. La `Memory search` cerca negli indici di memoria utilizzando stringhe o indirizzi `0x...`, e i `Memory jumps` visualizzano utili candidati come destinazione del salto, quali ELF/FDT/Linux/OpenSBI/cmdline/rootfs. L'`Initcall classifier` / `Initcall timeline` classifica e marca temporalmente i log in stile initcall/probe di driver di Linux. Il `Panic summary` estrae le righe attorno al panic/oops/fault e risolve gli indirizzi se sono presenti simboli. Il `Boot analysis JSON` salva questi dati collettivamente. Il `Dmesg extract` estrae solo le righe in stile Linux dagli output UART / virtio-console. Il `Decoded MMIO` visualizza gli ultimi accessi MMIO con i nomi dei registri.

La `Triage dashboard` combina le classifiche delle cause di arresto, la gravità delle anomalie della virtqueue, i probe dei dispositivi e i segnalibri (bookmark) delle query in un testo a schermata singola. Lo `Stop-cause ranking` assegna priorità a kernel panic, oops, istruzioni illegali, page/access fault, anomalie della virtqueue e probe di dispositivi in stallo da console/tracce/stati. La `Stop-cause evidence` visualizza la logica alla base della classifica, l'analisi del punteggio, le query diagnostiche consigliate e i punti di controllo successivi. È possibile confrontare le differenze nei conteggi di stato/fase/dispositivo/anomalia/memoria nella dashboard eseguendo `Save triage baseline` → Run → `Triage diff`. `Save preset baseline` → Run → `Compare preset baseline` consente di verificare se il conteggio degli hit di query preimpostate come panic/virtio/satp/rootfs è aumentato o diminuito rispetto all'ultima volta. Gli `Memory dump hits` scaricano in formato esadecimale/ASCII l'area circostante agli hit dell'indice di memoria utilizzando query diagnostiche o filtri di traccia. Il `Memory range dump` specifica un indirizzo/lunghezza arbitrario per eseguire il dump esadecimale/ASCII direttamente dalla DRAM. Il `Redacted share MD/JSON/HTML` sostituisce email / MAC / IPv4 con `<email>` / `<mac>` / `<ipv4>` prima della condivisione. Le `Redaction options JSON` attivano o disattivano `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex`.

Lo `Smoke preset` reimposta l'impostazione di avvio attualmente selezionata ed esegue solo i passaggi specificati. La `Smoke matrix` esegue in sequenza un elenco di preimpostazioni come `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` ed elenca i passaggi di esecuzione, l'ultima fase e i candidati alla causa di arresto per ciascuna preimpostazione.

I punti di interruzione del PC (breakpoint) vengono aggiunti immettendo il valore esadecimale di un PC fisico/virtuale nel campo `PC breakpoint` alla voce `Breakpoints / watchpoints`. `Run` / `Step 1k` si ferma su un breakpoint e `Step` avanza esattamente di 1 istruzione anche se il PC corrente è un breakpoint. I punti di osservazione in scrittura (write watchpoint) rilevano le scritture sul bus verso un intervallo di indirizzi fisici, e i punti di osservazione in lettura (read watchpoint) sono una semplice funzione per rilevare le letture sul bus. Sono utili per verificare probe MMIO, scritture framebuffer e riferimenti a strutture specifiche. L'`Access timeline` / `Compact access` visualizza i recenti accessi alla DRAM/MMIO in una serie temporale compressa. Il `PC profile on` aggrega i PC più frequenti (hot PC), e `Capture snapshot` → Run → `Diff snapshot` consente di controllare le differenze diagnostiche prima e dopo l'esecuzione.

Il simple-framebuffer prepara una memoria a 1024x768x32bpp all'indirizzo `0x86000000` e la inserisce come `simple-framebuffer` in `/chosen/framebuffer@86000000` del DTB generato automaticamente. Se simplefb è utilizzabile sul lato Linux, può essere visualizzato sul Canvas con `Render framebuffer`.

virtio-net non si connette a una vera rete dal browser; è un dispositivo di debug a livello di pacchetto. Immettere un frame Ethernet in formato esadecimale in `virtio-net debug` per iniettarlo in RX, e i frame inviati dal guest alla coda TX possono essere verificati in `Show TX frames`. Per farlo riconoscere dal lato Linux, eseguire comandi come `ip link set dev eth0 up` sul lato guest, se necessario.

virtio-rng è un dispositivo di verifica che presenta un PRNG deterministico come fonte di entropia del guest. Per mantenere la riproducibilità, il seme predefinito è fisso e può essere modificato tramite `Set deterministic seed` nella UI.

virtio-gpu è un dispositivo minimo per l'osservazione dei probe del driver virtio-gpu di Linux e per la configurazione delle risorse 2D. Invece della vera accelerazione GPU, tiene traccia dei comandi modeset / scanout / flush in arrivo nella coda di controllo e ne invia gli stati alla Diagnostica. Poiché esegue anche la copia dalla memoria di backup delle risorse al simple-framebuffer, è possibile verificare il risultato dello svuotamento (flush) di una risorsa 2D da parte del guest tramite `Render framebuffer` / esportazione PNG. Anche `UPDATE_CURSOR` / `MOVE_CURSOR` nella coda del cursore vengono registrati come stati.

L'`SBI shim on` serve per eseguire il debug diretto dei payload di modalità S senza OpenSBI. Mantenerlo disabilitato per i normali esperimenti che utilizzano `fw_dynamic.bin` / `fw_payload.bin`.

Se si desidera testare Multi-hart, impostare l'`Hart count` prima di caricare il firmware. Poiché la modifica delle impostazioni comporta il ripristino (reset) della macchina, si presuppone che successivamente si ricaricherà il firmware / payload / disco. La vista `View hart` consente di alternare i registri / CSR / tracce dell'hart di destinazione visualizzato.

Quando si utilizza `fw_dynamic.bin`, caricare il payload di modalità S / kernel intorno a `0x80200000` tramite `Load payload`, se necessario. L'emulatore inserisce le informazioni dinamiche all'indirizzo `0x87dff000` e ne imposta l'indirizzo su `a2`.

Per gli esperimenti con Linux, è possibile utilizzare una delle seguenti opzioni:

- `Load disk`: Passa un'immagine disco raw come rootfs nei panni di virtio-blk. I bootargs predefiniti sono `root=/dev/vda rw`.
- `Load initrd`: Posiziona initramfs all'indirizzo `0x84000000` e riflette l'intervallo initrd nel DTB generato. Modificare i bootargs in `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` ecc., se necessario.

Esempio di utilizzo dei binari RISC-V predistribuiti per OpenSBI 1.8.1:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# Carica fw_dynamic.bin / fw_payload.bin / fw_jump.bin dai file estratti tramite il browser
```

Se si compila OpenSBI localmente, preparare una toolchain RISC-V come `riscv64-unknown-elf-` e compilare con `PLATFORM=generic`.

### Comandi per lo sviluppo

```bash
go test ./...
make wasm
make serve
```

## Nota

Questa implementazione presenta in modo incrementale le funzioni necessarie per indagare sull'inizializzazione di OpenSBI, sulla transizione del payload di modalità S e sull'avvio di Linux. Per l'avvio di Linux sono stati aggiunti: PMP, Sv39, virtio-blk, virtio-console, virtio-net, virtio-rng, virtio-input, virtio-gpu, initrd, simple-framebuffer, precisione di trap/CSR, traccia/riepilogo CSR, decodificatori di registri/sequenza temporale/istogramma MMIO, analizzatore della sequenza temporale/fase di avvio, analizzatore di probe dei dispositivi, ispettore della virtqueue/visualizzatore della catena dei descrittori/esportazione DOT/snapshot/rilevatore di anomalie/suggerimenti sulle anomalie/triage delle anomalie, scanner della memoria guest/indice/diff/ricerca/suggerimenti per i salti/helper di dump, query diagnostica integrata/preimpostazioni della query/confronto della baseline preimpostata, dashboard di triage/classifica delle cause di arresto, bundle/HTML/oscuramento del report di condivisione, classificatore/sequenza temporale di initcall, ricerca delle righe DWARF/contesto del codice sorgente/annotazione delle tracce, riepilogo dei panici, estrattore dmesg, riproduzione/confronto delle tracce, report di regressione dell'avvio, profiling del PC, diff degli snapshot, esecutore smoke/matrice smoke di avvio, diff della baseline di triage, prove della causa di arresto, oscuramento modificabile e dump degli intervalli di memoria. Le parti principali non implementate o semplificate includono: un modello accurato di cicli/tempo, AIA/IMSIC, connessione di rete reale tramite bridge tap/WebSocket, accelerazione virgl/DRM/GPU completa, comportamenti rigorosi WARL/WPRI per tutti i CSR e un'esecuzione parallela reale utilizzando più worker. L'opzione Multi-hart è una pianificazione (scheduling) cooperativa all'interno di un singolo worker wasm.

## Diagnostica e strumenti per le regressioni

Condivisione migliorata per matrici smoke e query diagnostiche.

- Il formato `Smoke matrix MD/HTML` salva i risultati della matrice smoke come Markdown / HTML autonomo.
- Il comando `Save smoke baseline` → Run → `Compare smoke baseline` consente di verificare le differenze nella fase, nei passaggi di esecuzione e nelle cause principali di arresto per ciascuna preimpostazione.
- La `Stop checklist` crea una lista di controllo con le azioni specifiche da eseguire in base alla classifica delle cause di arresto.
- L'opzione `CSR/MMIO bookmarks` estrae solo le occorrenze cruciali del CSR / MMIO / traccia dai risultati della query diagnostica integrata.
- La visualizzazione `Watchpoint hits` mostra la cronologia delle occorrenze dei punti di osservazione in lettura/scrittura in una serie temporale. Il comando `Clear hit timeline` cancella solo la cronologia.
- L'`Artifact manifest` elenca i file firmware / payload / disco / initrd / simboli attualmente caricati e i range dinamici di DTB / informazioni, le voci e gli hash SHA-256 generati.

### Strumenti per la transizione delle regressioni (Regression Handoff Aids)

- I comandi `Manifest diff` / `Manifest diff JSON` confrontano il manifesto dell'artefatto di avvio corrente con una baseline salvata nel localStorage e mostrano le differenze nei bootargs, nel conteggio degli hart, negli intervalli di caricamento, nelle voci, nel rilevamento ELF e negli hash SHA-256.
- La funzione `Auto break/watch suggestions` genera i candidati per i punti di interruzione PC / punti di osservazione in lettura / punti di osservazione in scrittura da impostare per l'esecuzione successiva in base alle evidenze delle cause di arresto, ai recenti PC tracciati e alla cronologia delle occorrenze del punto di osservazione.
- Le opzioni `Smoke clusters` / `Smoke clusters JSON` raggruppano i risultati preimpostati della matrice smoke per fase e per causa di arresto principale, riunendo le preimpostazioni con lo stesso tipo di errore.
- L'opzione `Diagnostic bundle JSON` è un file JSON autonomo che raggruppa il manifesto, la dashboard di triage, le cause di arresto, i suggerimenti di interruzione, il bundle condiviso e le occorrenze del punto di osservazione.
- Il `Compressed bundle JSON` corrisponde al bundle diagnostico sopra convertito in formato gzip+base64. Da utilizzare se si desidera ridurre le dimensioni prima di incollarlo in segnalazioni (issue) o chat.

### Strumenti per transizione / provenienza (Handoff / Provenance Aids)

- `Decode bundle` estrae un `Diagnostic bundle JSON` o un `Compressed bundle JSON` incollato, o direttamente un gzip+base64 raw.
- `Bundle compare` / `Bundle compare JSON` confronta un bundle precedente incollato con il bundle corrente e mostra le differenze nelle fasi di triage, nelle cause di arresto principali, nei manifesti, negli hash degli artefatti, negli smoke cluster, nelle occorrenze dei punti di osservazione e nel conteggio dei suggerimenti.
- `Provenance` / `Provenance JSON` riassume gli hash SHA-256 del manifesto, della traccia, della console e del bundle diagnostico, nonché il numero delle righe della traccia, i byte della console e le cause di arresto principali. Può essere utilizzato per verificare la riproducibilità o come evidenza allegata alle segnalazioni (issue).
- `Handoff MD` raggruppa la provenienza, le cause di arresto principali, i suggerimenti di interruzione/osservazione automatica, le liste di controllo, le differenze della baseline e i manifesti degli artefatti in formato Markdown.
- L'opzione `Apply auto breaks` applica in batch i candidati principali dei suggerimenti di interruzione/osservazione automatica all'emulatore in uso. È uno strumento utile per configurare rapidamente le posizioni di arresto o per isolare gli intervalli MMIO/DRAM sospetti prima di una nuova esecuzione.

### Riproduzione / Firma / Transizione Headless

- `Repro plan` / `Repro MD` / `Repro JSON`
  - Genera i passaggi di riproduzione dai bundle diagnostici, dalla provenienza e dai manifesti degli artefatti.
  - Elenca ruoli, dimensioni, intervalli di caricamento e hash SHA-256 del firmware / payload / initrd / disco / simboli come pin dell'artefatto.
  - Documenta in passaggi successivi i preset smoke, i bootargs, i conteggi degli hart, il next_addr e le condizioni raccomandate per interruzione/osservazione.
- `Log signature` / `Log signature JSON`
  - Crea un riepilogo leggero dagli hash SHA-256 delle tracce / console / manifesti, del conteggio delle righe tracciate, dei PC primari/finali, delle prime/ultime righe della console e dei token (hot token).
  - Consente di confrontare "È lo stesso log?" o "Cosa è cambiato?" senza dover incollare file di traccia raw.
- `Save log signature` / `Load log signature` / `Compare log signature`
  - Salva la baseline della firma del log nel localStorage del browser e la confronta con la firma corrente.
  - Mostra le differenze per quanto riguarda gli hash delle tracce, della console, dei manifesti, il numero delle righe, i PC finali e le ultime righe della console.
- `Auto break verify`
  - Mostra un riepilogo di conferma prima di applicare i suggerimenti automatici di breakpoint/watchpoint.
  - Genera avvisi se vi sono suggerimenti duplicati o range PC sospetti.
- `Headless smoke script`
  - Genera uno schema (skeleton) in script della shell per la CI o transizione partendo dal manifesto dell'artefatto attuale, i bootargs, i conteggi degli hart, i preset smoke e le esecuzioni in vari passaggi (steps).
  - Destinato a vincolare i pin degli artefatti e le matrici preset prima dell'integrazione di sistemi harness come Playwright all'interno dell'ambiente di calcolo.

#### Strumenti Headless / CI

Per semplificare l'utilizzo del passaggio delle firme e della riproduzione nelle CI o nelle segnalazioni, è stato aggiunto quanto segue.

- `Bundle integrity` / `Integrity JSON` verifica la coerenza fra il bundle diagnostico e il manifesto dell'artefatto, inserendo eventuali difformità nei ruoli degli artefatti, gli hash SHA-256, i range in caricamento, i suggerimenti e l'esito dei test smoke per le categorie `error`, `warn` o `info`.
- `Repro validation` / `Repro validation JSON` convalida che il piano di riproduzione in uso combaci con bootargs, next_addr, numero degli hart, i pin dell'artefatto, le cause d'arresto primarie o con le varie firme di log all'interno del bundle.
- `CI summary` / `CI summary JSON` riunisce in uno le analisi di integrità del bundle, le firme per tracce/console, esiti per smoke e cause d'arresto, fornendo un riepilogo atto ad agevolare i test (pass/warn/fail) passati in revisione dalla CI.
- `Headless runner spec` / `Runner spec JSON` genera preset, passaggi, pin per gli artefatti nonché alcuni tra i comandi d'ispezione maggiormente indicati per `go run ./cmd/rvsmoke ...`.
- È stato introdotto il comando `cmd/rvsmoke`. Ciò permette la lettura sui bundle diagnostici / manifesti dell'artefatto sganciandosi dal browser emettendo testo / JSON o file Markdown circa hash di un artefatto, stabilità sul bundle, resoconti d'insieme della CI oltre ai dettagli d'avvio specifici dei runner.

Esempio:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

Il programma `rvsmoke` al momento opera indagini tese ad accertare la solidità per le riproduzioni generandone i rispettivi riepiloghi CI applicati ad hash sull'artefatto/bundle. Nel merito delle esecuzioni dipendenti dalla CPU verrà ancora impiegata un'apposita matrice (smoke matrix) tramite l'architettura di base in js/wasm a livello di browser.

#### Porte CI di rvsmoke / JUnit / SARIF

Lo strumento `cmd/rvsmoke` fa le veci di una CLI complementare addetta ad ispezionare le esportazioni nei formati previsti dai bundle/manifesti della CI. Sostituendosi con operatività headless, questa rassegna rende disponibili gli outcome riferibili ai raffronti per le baseline dei bundle, criteri d'inclusione associati ai Gate della CI, output derivati sotto forma di JUnit XML, in estensione SARIF ed esiti autonomi tramite file in architettura HTML.

Esempio:

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

Esempio riguardante la policy come dicitura JSON:

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

La dicitura `-out html` veicolerà tramite stampa il documento HTML via stdout, per contro `-out junit` opererà per JUnit XML, ed infine `-out sarif` in favore del SARIF JSON. Nell'ipotesi in cui vengano invocati contemporaneamente `-junit` / `-html` / `-sarif` i sistemi produrranno un salvataggio dedicato tramite il file peculiare d'assegnazione ad affiancare il formato per lo standard di emissione (stdout). Nel meccanismo interno dei gate per CI si assisterà a standardizzazioni di esito `pass`, `warn` oppure `fail` su ambiti tra cui si collocano: firme applicabili alle tracce/console, anomalie ricollegabili per la virtqueue, difformità nel quadro generato su baseline dei manifesti per artefatti, così via in riferimento ai risultati dello smoke matrix.

#### Template di Policy per rvsmoke / Confronto Trend del Bundle

Allo scopo di facilitare l'avvio delle impostazioni per le CI gate originarie in parallelo alle rassegne e verifiche regressive molteplici, è stato esteso l'impiego per determinati canovacci o moduli (template), le direttive e note programmatiche (action checklists) o valutazioni delle variazioni sui trend verso `rvsmoke` accodandole alle impostazioni originarie via interfaccia.

- `CI policy templates` / `Policy templates JSON` rende esplicito i template previsti (built-in) preimpostati, ossia: `default`, `strict`, `linux-boot`, `artifact-only` in ultima accezione `lenient`.
- Il formato `Policy template JSON` assicurerà in via conservativa un prototipo di file d'impiego a JSON di pratico conferimento nei meccanismi della CI.
- Le istanze `CI gate` / `CI gate JSON` operano come interfacce atte a tramutare la policy corrente ed assestandosi in combinato assieme ad un test d'ingresso tramite le condizioni (pass/warn/fail).
- Il parametro `CI checklist` / `CI checklist JSON` rideterminerà i dinieghi d'accesso sulle gate o anomalie sui livelli originari in un'elencazione d'azioni concrete esplicabili.
- Facendo uso di `rvsmoke -compare name=bundle.json` verranno catalogati, disposti secondo ordine cronologico i bundle generanti un panorama descrittivo circa i trend ed evoluzioni manifestabili nei passaggi legati alle disconnessioni di maggiore gravità, le matrici afferenti ai smoke o ancora agli identificativi associabili ai cluster d'artefatti in hash.

Esempio riguardante la generazione di template per policy:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

Esempio per il raffronto sui diversi bundle in coabitazione:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

L'impiego di `-policy-template` detterà la normazione per i percorsi ordinari allorché omettendo un dettame esplicito verso `-policy`. Nel caso venisse invocato `-policy`, si riconoscerà al referenziamento esplicitato per JSON sul file d'inclusione piena prevalenza e facoltà primaria.

## Integrazione CI di rvsmoke

Perfezionamenti estesi applicabili ai passaggi in handoff / rassegne CI con `rvsmoke`.

- `rvsmoke -print-github-actions linux-boot` potrà tradurre e generare in output dei Workflow dedicati in struttura GitHub Actions in codice YAML.
- L'utilizzo afferente a `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` trasferirà operatività o cicli attuativi con l'intento di riprodurli su supporti via file.
- L'estensione data da `rvsmoke -policy-tree policy-tree.md` garantirà lo scarico e preservazione legati alla salvaguardia a titolo d'alberi per derivazione cause sia di CI gate / per stabilità nei bundle ed un egual grado nei scostamenti riferibili alla baseline.
- Assicurandosi i servigi dettati via `rvsmoke -history history.txt` si immagazzineranno e riproporranno dati e andamenti circa cause di arresto e scostamenti riferibili ad esiti raggruppati afferenti agli artefatti da visuali molteplici in termini di trend sui bundle.
- Sfruttando `rvsmoke -repro-zip rvwasm-minimal-repro.zip` andranno ad attivarsi schemi tesi ad assemblare configurazioni di ridotta caratura ricollegabili alle specifiche d'esecuzione contenenti di massima un'alberatura tra cui vi sono presenti README, file in conformazione di policy o runner specs e via discorrendo come le verifiche applicabili verso gli script di CI o d'integrazione di bundle per le operazioni in via d'analisi.

Esempio:

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

L'intento alla base in uso a `-repro-zip` non è assolutizzare l'accorpamento interno a beneficio delle forme base inerenti ai raw per disk/kernel/firmware. Verranno accorporati, d'opposto posteggio, pin in base originaria da algoritmi in SHA-256 e partizioni per manifesto facenti la presunzione fondante riguardo l'accertamento d'ogni singola parte o artefatto per vie di delega al destinatario presunto nell'operazione.

### Ispezione del Repro ZIP per la CI / Continuazione del Workflow a Matrice

Sono state disposte opzioni e diramazioni associate per le peculiarità ed impieghi ad uso `rvsmoke` oltre che da interfaccia su terminale (browser UI) tese in ultima misura alla facilitazione per riproposizioni o indagini con impieghi minimali su pacchetti ad uso derivativo in GitHub Actions matrix unitamente ad una rassegna visiva d'impiego legata al mutamento nei trend.

- In via di utilizzo con `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` varrà a farsi facoltà ispettiva in favore del prodotto creato da estensione in dicitura su `-repro-zip` aggirando procedimenti volti ad estrazione primaria per le rassegne. Opererà constatazione ed allineamento sui file obbligatori ed esecutivi preposti nei criteri stabiliti (es. dicitura per policy in riferimento da `ci-policy.json` od allineamento dettato dal percorso via `scripts/rvsmoke.sh`) assieme ai divieti sui rami (unsafe path) con riscontro sui dettami in forma di bundle o `manifest.json`.
- Applicato l'utilizzo di `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` comporrà i format e documenti dedicati al comparto YAML con afferenza da Workflow su base ad azioni multiple come predisposto via GitHub per preset impiegato.
- L'utilizzo tramite `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` instillerà i dati inerenti da matrici d'esecutività a titolo da scriversi direttamente via file d'allocazione preposti.
- Impiegando `rvsmoke -trend-csv rvwasm-trend.csv` a corredo di `-trend-chart-json rvwasm-trend-chart.json` preserverà memorie e mutamenti attinenti nell'evolvere per la via dettata verso l'assestamento e archiviazione nel compendio CSV o JSON promuovendone per la strada a ritroso la rappresentazione visiva (grafici) esterna semplificata.
- Aggregati i supporti al terminale integrando funzioni dedicate quali `Minimal repro ZIP`, `Inspect repro ZIP`, `Repro ZIP JSON`, `Matrix workflow YAML`, l'inserimento per `Trend chart JSON` in conformazione unita alla documentazione `Trend CSV`.

Esempio:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# Esecuzione del salvataggio risultati riferibili ai test in veste di JSON
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# Transposizione legata al mutamento sul bundle attuale ed intercorrente su CSV/JSON a raffronto d'uno pregresso
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

L'istruzione generata tramite `-inspect-repro-zip` può detenere indipendenza a livello attuativo d'esecuzione svincolata dal resto. Affiancando codesta a ridosso a `-bundle` incamererà le note in riepilogo generate alle verifiche compiute via ZIP annettendole nel riassunto globale ed addebitando a carico ed afferenza propria tutti gli esiti ascrivibili ed accomunabili per gli stadi legati al rigetto d'istruzione in ambito CI.

### Aggregazione della Matrice CI / Continuazione del Manifesto di Checksum

Correzioni ed evoluzioni su trasferimenti d'appoggio tesi ad assecondare gli ambiti in CI assunti ad hoc da artefatto via `rvsmoke`.

- Generando comandi via `-repro-checksums rvwasm-repro-checksums.json` s'inserirà documentazione deterministica che funga d'asseverante riguardo i responsi di conformità emersi in test (checksum manifest) ascrivibili a corredo della verifica emersa entro archivi con desinenza in file ZIP che traggano il referto partendo da riscontro legato all'investigazione ricollegabile per `-inspect-repro-zip`.
- Mediante designazioni a matrice su opzioni accodabili (es. ricorrendo ad esplicitare molti `-matrix-result name=rvsmoke-output.json`) consentirà di coagulare responsi pregressi e deduzioni ricollegabili ad appositi rami emersi entro esplicitazioni JSON di `rvsmoke -out json` che derivino da task d'esecuzioni od anche su diramazioni legate a test prefissati per istruzione (preset) o job attuati in parallelo.
- Opzioni che ricorrono all'impiego per `-matrix-summary` ed in connubio per il frangente referenziato con l'aiuto d'appendici quali `-matrix-summary-json` / `-matrix-summary-html` tramuteranno ed attueranno d'inclusione piena test e report d'indagine in salvataggi nei paradigmi via HTML autonomo in sé (self-contained HTML) o come formati dedotti in text ed in appendice formato e costrutto per i JSON per l'appunto.
- I richiami ed i legami tesi tramite `-trend-html rvwasm-trend.html` fungeranno da referenziatori atti nel preservare note o constatazioni e risvolti assunti tramite report che si dipanino nell'indirizzamento teso allo scopo del posizionamento finale come singole pagine HTML a struttura univoca e sganciata da supporti vincolati od accessori autonomi (standalone).

Esempio:

```bash
# Impostazioni finalizzate ad arginare conservazione riguardante ambiti minimi o documentazioni di checksum
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# Indirizzamenti al JSON volti verso l'aggregazione di file ed operatività multiple intercorse
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

Le operazioni d'aggiunzione ed accorpamento con base a rassegna matriciale concentrano al loro fondamento gli statuti o parametri riscontrabili all'ambito CI, avvisi di blocco ed in via affine di conteggio per gli avvertimenti emersi per Gate non conformi ai parametri imposti (es. gate failure/warning), disaccordi od anomalie (mismatch) nel paradigma per gli artefatti come d'intercorrenza delle stasi con blocco d'entità maggiore e referenziati di lavoro per job ed incarichi assunti da eseguire. Funge a mo' d'ausilio nel momento si intenda constatare ed evidenziare la deviazione sui responsi tendenziali su difformità (failure trend) qualora vengano ripartiti o scomposti come spesso in voga d'impiego sui lavori eseguiti per GitHub Actions per compiti frammentati a matrice.

#### Strumenti per la Transizione / Rilascio CI

Accrescimenti o predisposizioni dedotte d'uso applicate a strumenti di CI atti all'ausilio delle gestione legata da una parte in riferimento al ciclo vitale sugli artefatti e come paritetico in egual modo alla sponda legata al transito dei rilasci associabili verso ed a favore ed in seno a logiche preposte su base a referenze connesse e veicolate su terminale in `rvsmoke`.

- Parametrizzando comandi affini in virtù a richiami a logica e stringhe che poggino per tramite d'indirizzi a `-artifact-index rvwasm-artifacts.json` decanterà su sintesi ridotta a schemi le entità in esame estese o ridimensionabili ascrivendole a percorso o tragitti in dislocamento d'origine, volume ed ampiezza dedotta e determinata (bytes) fino a chiudere referenze ad appoggio ed al fido dei controlli ad asseverazione generati o adibiti secondo i consueti JUnit/SARIF intercorsi ai vari transiti di verifica su riproposizioni o file di calcolo esulanti (es. hash da elaborati SHA-256 applicati su calcoli via repro per i file).
- Includendo per `-release-manifest rvwasm-release.json` concentrerà una pluralità su forme, referenze fra tracce estese a bundle diagnostici, accorpamenti tesi in agglomerati ad indici via artefatto, validazioni incamerate a titolo referenziale per le gate impiegate via CI o constatazioni da instabilità non definitive ma d'indagine al volo come in riferimento o a dipanamento delle referenze ascrivibili per riproduzioni e conteggi via diff o repro in compendio finale unitario o deducibile in log o manifesto transizionale compatto in transito d'handoff.
- Mediante `-release-html rvwasm-release.html` sfornerà di per sé file estesi con richiami adoperati per navigazione d'esamina tesi a sondare o analizzare i rimandi in esame o accorpabili con pertinenza con i costrutti per riassunto (Summary), su file dedotti o generanti l'artefatto medesimo od ancora i deducibili ad inchieste da checksum sino ad incanalarsi alle estensioni del file o derivazioni a matrice tesi o compendiosi per interrelazioni con deduzioni in costrutti a base per i JSON.
- Adoperando `-verify-repro-checksums baseline-repro-checksums.json` opporrà referenze su basi deducibili o ad intersezioni fra test ad accertamento condotti e portati per il tramite a fondamento dell'effettiva costituzione intercorsa (baseline) con test d'indagine in dislocamenti appositi atti ed assoggettati a rilevare le deviazioni di rito od insorgenze atipiche ad imputazioni d'insorgenza additiva ed al limite opposto d'imputazione legata in base carente con sottrazione di voci (es. accertamenti con mancanze su imputazione referenziabile, difformità non pattuite/modifiche arbitrarie, deduzioni per adducibilità ulteriore di record in appoggio non conformi ai canoni del bundle via ZIP minimale d'asset intercorso attualmente nell'oggetto preso per l'ispezione od in revisione).
- Facendo seguito d'appendici quali `-matrix-flakes`, con associati d'attestazione in JSON con `...-json` e d'HTML con estensione referenziata su `...-html` si prenderà in rassegna multipli test o risultanze ad induzione a matrice parificata che detengano diciture ad esempio e conformazione derivata del tenore assimilabile per i costrutti o matrici d'asseverazione per `uart#1` oppure parimenti interposti con valenza omologa in assunzione del dettame intercorso con `uart#2` ricalcando con ciò d'indurre verifiche che denotino o smascherino propensioni ad oscillazioni incerte od andamenti anomali derivati tra esiti positivi o reiezioni d'esito sfavorevole ad accertamento ad intermittenza volubile nell'andamento in alternanza pass/fail che ricalchino insidie da esame ad esiti non saldi riferiti alle accezioni definite per "flake".

Esempio:

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

## Transizione e Verifica del Rilascio

Output associati a metadati con ricollegamenti pertinenti alle facoltà espresse per deleghe derivate o trasposizioni assoggettabili a corredo ai flussi informativi per vie di derivazione con l'adozione e compendio per l'utilizzo annesso con associazione in rinvio ed estensione per il costrutto integrato adibito ed adiacente a logica al sistema `rvsmoke` d'indirizzare l'invio derivante dai responsi ed accertamenti operati in veste d'asseverazione ed ispezione compiuti e condotti tramite test per le dinamiche relative ai comparti o reparti assunti d'integrazione o referenze continue per e nella CI per riversarle ed indirizzarle in affido od a disposizione all'uso e revisione ricollegabile per stazioni alternative dislocate ad esecuzione parallela o disposte separatamente per vie e macchine d'impiego per computi slegati o differenti ma in rassegna o a riuso da appigli esterni.

### Estensione della Provenienza / SBOM

#### Inventario delle Dipendenze in SBOM-lite

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

Gli intenti previsti si focalizzano nell'elenco d'annesso al determinismo dettato e derivante da logica ed assiomi assunti che determinano per l'appunto e denotano referenze estrapolate a dimensioni di caratura modesta e compatta in deduzioni d'appoggio ad architetture dipendenti su logica per dipendenze ad appoggio condotte tramite formati piccoli tesi a letture dettate ed assumibili partendo ad estrapolazioni ad inchiesta ed investigazione su registrazioni d'associazioni e correlazioni che si evincono per la via ed attraverso le specifiche di modulazione contenute nel richiamo di parametrazione `go.mod`. Da quest'ultimo ricaverà note e postille annesse per percorsi dei moduli incamerati, esegesi legate in dipendenza sulle versioni afferenti o varianti per le deduzioni e configurazioni basate su linguaggi Go, referenze ed attribuzioni legate od ingenerate per associazioni dirette ricollegabili ad impartizioni ad attuazione su direttive impartite a dipendenza condotte e dislocate per chiamate con afferenza su comando per il costrutto relativo alle righe d'attribuzione che intercorrono via comando in assunzione con associazione via rimando su `require` (ed in affiancamento per gli analoghi o sostituzioni a destinazione preposta su `replace`) ingenerando ad ultimo le derivazioni che annoverano tra esse gli elenchi legati alle attribuzioni categorizzate in tipologia ascrivibile a referenze ed incameramenti con indicizzazione riferibile ed addotta o derivata tra indici assunti in associazione su rassegne preposte a caratura o artefatto per la CI.

Se si operassero direttive tese all'esecutività da indurre con l'inclusione associativa per `rvsmoke` da impartizione e dipendenza di derivazione originata partendo a dislocazione operativa disposta in ambienti od allocazioni alternative in sede differita a distaccamento da posizionamento primario con afferenza ad area lavorativa non contigua si dovrà impartire la segnalazione indicativa d'apporto indicata e veicolata mediante rinvio all'argomento preposto con esplicitazione in estensione deducibile e veicolata al riferimento o attributo d'inserzione indicando la nota ascrivibile con la sintassi per stringa con `-go-mod /path/to/go.mod`.

#### Attestazione della Provenienza

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

La referenza tesa in accertamento come prova documentale non funge e non espleta una peculiarità assimilabile o sovrapponibile a validazione autografata con estensione a sigillo ed autenticazione riferibile in prima battuta a firme (non opererà né figurerà di per sé a mo' o come rimpiazzo d'autenticazione derivata per vie dirette od a certificazione siglata digitalmente come firma a tutto tondo per sé medesima o di suo avviso in senso prettamente autoriale od annesso) ma operando nella costanza in referenze dislocate come estensioni ricalcabili o payload ad inquadramento di strutturazioni deducibili su ispirazioni da assunzioni ed analogie tese su fondamenti a ricalco delle prerogative ed architetture d'ideazione mutuata con riferimento basale per SLSA od in associazione a prerogative di strutturazioni conformi per analogie condotte e disposte nell'ambito per architettura del progetto associabile a in-toto con diciture derivanti e prodotte da emanazioni basate su formattazioni in referenze per file o deduzioni con afferenza di architetture del JSON si assicurerà ad indurre peculiarità in adozione o disposizioni sfruttabili come ancoraggi stabili a preposto per rintracciabilità ed asseverazioni di consistenza fisse da cui dipartire in validazioni successive ed in dipendenza ricollegabile per rinvii ed ancoraggi in appigli tesi ad agevolare per certificazioni condotte ad integrazione derivabile ed in aggiunta o supporti esterni che ricorrano ed addivengano ad operare su hash di derivazione per firme preposte applicate per vie od attrezzature dedicate ed annesse d'apporto o per vie esterne d'accertamento od ingenerate per vie accessorie e veicolate dalla o nell'ambito in esecuzione in corredo ai sistemi associati ai terminali in riferimento da o con sistemi d'interazione esterna nella CI a dislocazione per l'interfaccia.

#### ZIP per la Transizione del Rilascio

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

Gli insiemi d'apposizione integrabili con accorpamento in trasposizione ed affido dedotti per trasmissioni e comunicazioni in accorpamento dislocate o traslati a passaggi o a transizioni veicolabili ad o con raggruppamento per le rassegne d'incameramento ascrivibili o d'annessione derivanti in file ZIP tesi od addotti a favorire i rilasci si concentreranno ad immagazzinare e disporre per un impiego preposto includendovi o confinando in aggregazione unicamente formati per referenze su attribuzioni circoscritte a specifiche in dotazione ai soli o propri metadati.

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

Questa allocazione o predisposizione respinge formati a natura o struttura originante o ricalcante file base o configurazioni in afferenza di esecutivi (come i kernel), d'ingombro per dispositivi ad architettura estesa per masse su o da allocazioni per disco od allocazioni che annoverino incameramenti bruti non filtrati da astrazioni a logiche come firmware od initrd, che vedono preclusa l'eventualità d'inserzione a o con asseverazioni ad inglobamenti ed innesti o fusioni interne al bundle come inserzioni per dipendenze ad apposizioni di archivi non filtrabili al pari degli innesti ed integrazioni per archivi con ampiezze od in referenze a volumi dilatati e grezzi assoggettati o limitati a prefigurare, figurare ed esistere e permanere nei documenti in rassegna ai o con manifesti in funzione ed a titolo d'imputazione o d'impronta ad ancoraggio con afferenza ridotta e ristretta in attribuzioni basate a ricalco con pin e referenze fisse limitatamente da hash elaborati ad e su algoritmi d'accertamento per SHA-256 in vece della componente non decantata od integra.

#### Ispezione dello ZIP per la Transizione del Rilascio

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

Si porranno ad essere le condizioni e disposizioni da ingenerarsi per controlli o valutazioni indotti e condotti a cura od incarico derivante per il meccanismo e sistema od applicativo adibito ad inchiesta ed asseverazione delle integrità (ispettore) esentandolo e preservandolo dal compito d'ingenerare processi estrattivi o aperti prima del crivello ed accertamento sulle condizioni assunte con asseverazioni d'integrità ad archivio teso in preclusioni per insidie latenti o celate in diramazioni o radici per cammini ad indicizzazione sospette (percorsi ritenuti ad alto ed elevato margine di pericolo od omonimie od accavallamenti doppi per allocazioni), in coabitazione alle capacità d'interpretabilità, assunzione, assorbimento e d'interpretazione sintattica su e con validazione formale adeguate all'elaborazione d'incorporazione dei dettami afferenti alle diciture derivanti e redatte sotto o per formattazioni incardinate sui dettami ed assiomi del JSON, inglobando a corredo la pertinenza sulle fondamenta, sulla tenuta strutturale o sulla coerenza e solidità basale d'accertamento tra rassegne ed indici o su asserzioni d'attribuzioni derivanti dai o per e da certificati d'attestazione in rassegne in e con SBOMs interposti per rilascio in dote ai vari raggruppamenti.

### Verifica del Rilascio

Oltre alle consuete e note elaborazioni e costruzioni da ZIP atte al rilascio in vece a corredo d'ausili per e su trasferimenti, si addizionano ed intercorrono a suffragio del rilascio preposizioni addotte a deduzione derivante da emissioni o dati elaborati miranti o indirizzati su rassegne per controlli preposti e vagli.

- Operando a dicitura per `-verify-attestation` / `-verify-attestation-text` accerterà od originerà valutazioni con costatazione al fine della validità o tenuta con verifica a riprova che i raffronti a o con la coincidenza in esame e sotto rassegna od in perizia dettata e scaturita dai rimandi ai codici (hash) in asseverazioni per asseveranti in determinismi d'accertamento su provenienza (attestazioni di provenienza deterministica), con i soggetti (o attributi preposti) ad o con afferenza al CI e adeguatezze o corredi d'apposizione ai file d'inserimento o in supporto all'estrapolazioni dei test e dell'emissione associabile all'elencazione d'estrazioni generate a manifesto in o per il rilascio od a manifesti associati al vaglio d'indici su inventari ascrivibili all'adibita rassegna ad appoggio con SBOM-lite.
- Immettendo per o in ricorso e supporto con `-sbom-baseline`, `-sbom-diff` e `-sbom-diff-json` metteranno e presteranno adito ad operare in comparazione i corredi od afferenze d'informazioni in detenzione o stoccaggio attuali ed in essere o figurazione riferibile al momento all'esame corrente d'ispezione ed inventario all'anagrafe delle dipendenze d'inclusione associata a o in SBOM-lite opposti od appaiati a referenze conservate ed estrapolate su test storici pregressi od a basamenti di riferimenti incamerati (baseline) precedentemente archiviati e ritenuti in stoccaggio da referenza.
- Esercitando `-compare-release-zip-inspection`, `-release-zip-compare` e parimenti nell'assunto correlato per `-release-zip-compare-json` avvierà in indagine o contrasto tra documentazione o rassegna inerente allo stato dei plichi od archivi per e nei formati ZIP al vaglio dell'attenzione e revisione d'indagine al momento ed instanti in atto con rinvii d'appoggio documentale ad estrapolazione e conservazione ricollegabile per ed in e su formati referenziati d'indagine in deduzione ad annotazioni d'attestazione pregresse in e per resoconti estratti a rassegna JSON d'uso o di verifica superata nelle o su o in test condotti su ispezioni passate ed in rassegna o a consulto d'archivio in analisi retrospettive e consultive d'indagini condotte o terminate.
- Assegnando in e con attribuzione d'inclusione associabile o tesa ad apporto d'avvio od esecuzione via `-retention-manifest` / `-retention-text` generano o deducono ed esplicitano a produzione manifesti su ed ad ed in attribuzioni tese e volte al trattenimento o conservazione d'oggetti ascrivibili a corredo della e nella o in test o passaggi con attribuzioni ricollegabili alla CI i cui parametri od elenchi contengono preposizioni che riassumano direttrici di rotte d'allocazione (paths), afferenze di conformazioni o derivazioni ed a e per tipologia (kinds), pesi d'allocazione (bytes), rinvii e deduzioni od estrazioni computate via algoritmo d'indagine ed attestazione ricollegabile ad asseverazioni in SHA-256, computi che quantificano durate temporali ad e per l'immagazzinamento al deposito temporale di validità con o in diciture per e su attribuzione riferita per giorni di durata al contenimento (retention days), scadenze d'orologio a decorso massimo e motivi giustificativi (reasons) dell'operato preposto.
- Allineando per `-release-verification-html` si avrà in emissione un report autoportante a struttura con impaginazione a e su o con documento HTML d'appoggio, arricchito od includendovi per l'occasione ed in veste accessoria o correlata la preposizione d'architettura utile e dedita all'assolvimento di necessità attinenti ad orientamento o direzioni di menù in e su pannello d'ispezione d'avanzamento d'utilità correlata alla rassegna per gli avanzamenti e le esposizioni in e su compendio od a raggruppamento per le e con asseverazioni ad esito della e su od in revisioni d'accertamento d'integrità ad esiti o discordanze in SBOM, revisioni a crivello o contrasti da raffronto su plico od archivio per rilascio prefigurato ad esito in esecuzione ZIP e dettagli legati ad o per le tempistiche ed informative sulle trattenute d'archiviazione (retention information).

Esempio:

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

### Gate di Controllo del Rilascio

All'apice delle operazioni relative all'accertamento dei rilasci (release verification) è stato posto un ultimo, ma stringente strato d'ispezione (release audit layer). Questa configurazione riassume in una sintesi integrata (gate report) ed esprime con un unico valore quantitativo l'idoneità derivante da verifiche puntuali ad esempio concernenti: attestazioni sulla provenienza, eventuali difformità individuate dai resoconti di SBOM-lite, discrepanze ravvisate incrociando i file ZIP afferenti ai rilasci, scadenze pendenti connesse ai margini previsti dalla conservazione (retention), situazioni di precaria ripetibilità accertate all'interno dei framework in ambito matrix (flake status) od infine ai rilievi posti nei manifesti conclusivi emessi per i rilasci.

Principali modificatori (flag):

- Il parametro `-list-release-verify-policies` enumera ed inquadra i dettami integrati di partenza previsti all'origine circa le disposizioni con pertinenza legata agli accertamenti rivolti verso ispezioni sui processi in fase terminale d'approvazione e rilascio (release audit policies).
- Mediante `-print-release-verify-policy strict` si esporta la base testuale di partenza predisposta in modello JSON, indicando un parametro od orientamento restrittivo ai controlli per la direttiva ed impianto della predetta policy.
- Selezionando un criterio tramite `-release-verify-template default|strict|lenient|archive` s'indurrà il programma ad applicare direttamente una delle politiche ed impostazioni normative strutturate e previste a monte del sistema come configurazioni d'ordinaria dotazione (built-in policy).
- Con il parametro `-release-verify-policy policy.json` è concessa la facoltà e possibilità di far gravare sui controlli e verifiche legati ai rilasci un impianto documentale personale ad ispezione modellato da o su premesse d'accertamento create in proprio (custom policy).
- Impiegando `-retention-audit` / `-retention-audit-json` i resoconti conclusivi genereranno una trascrizione in chiaro attinente alle disamine ispettive correlate sia ai tempi legati ad un esaurimento nella disponibilità o decadenza, quanto a scadenze in materia ai pregressi in trattenimento minimo.
- La dicitura in `-release-score` / `-release-score-json` conferisce visibilità trascrivendo ed imputando all'analisi sui rilasci in corso di rassegna una valutazione numerica complessiva d'accertamento racchiusa nel punteggio tra valori delimitati da 0 a 100.
- Introducendo `-release-gate` / `-release-gate-json` produrrà stampe a terminale riferite all'emissione od agli esiti e conclusioni estrapolate d'indagine in materia d'analisi sulle barriere ed alle soglie prestabilite nella direttiva di controllo prescritta in policy.
- Assegnando infine `-release-audit` / `-release-audit-json` / `-release-audit-html` emetterà a corredo la stesura in rapporto riassuntivo (audit report) capace d'agglomerare ogni esito o risultanza all'interno d'un prospetto unificato comprensivo ed articolato.

Esempio:

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

Le norme stringenti che intervengono applicando dettami conformi ad impostazioni su severità (`strict policy`) declinano negativamente e di riflesso valutano a stregua e pari d'un totale fallimento gli addebiti riscontrabili su manifesti esaminati in rilasci la cui verifica ad esito in approvazione fosse fallita (non-passing release manifest), come eguali si annoverano fallimenti ed impedimenti occorsi ed incappati in accertamenti concernenti stadi attestativi per documenti afferenti via ZIP od analogo o se intercorressero rassegne relative ad SBOM, così come non tralasciano in esenzione decadenze ravvisate tra e su artefatti i cui termini abbiano oltrepassato e deluso le tutele predisposte sui margini di permanenza a tenuta minima, od in ultima istanza ad imputazione diretta ad artefatti a prescrizione con termini e periodi esauriti a tutti gli effetti d'uso (expired artifacts). Di norma le direttive con caratura ordinaria (`default policy`) trovano l'habitat confacente prestando la giusta misura nell'applicazione d'impiego ove i vagli intercorrono all'esercizio con metodicità a cadenza periodica giornaliera come d'abitudine o routine assodata nelle trasposizioni e chiusure espletate a consuntivi o consegne con passaggi per rilasci a conclusione del ciclo notturno, le medesime consentono ammissibilità elargita sotto forma ed accezione di avviso o notifica precauzionale d'avvertimento nei gradi minori (warning), nondimeno operano la recisione dell'iter a fronte ed a costatazione d'evidenza qualora ed al presentarsi od alla riprova in via preclusiva ad eventualità legate ad interruzioni da chiari indizi comprovati sulle negligenze d'esito a vagli od indagini fallite palesemente o su mancati riscontri ad esami in CI o controlli d'integrità.

#### Differenze di Controllo del Rilascio / Deroghe / Passaggio delle Tendenze TODO

I percorsi afferenti nell'assetto di rassegna al controllo preposti in verifica con `rvsmoke` (release-audit path) elargiscono il supporto a favor dell'operatività comparativa mettendo a contrasto ed analizzando le risultanze accertate negli stadi correnti dei passaggi ispettivi al lato delle referenze acquisite e ritenute in rassegne e bilanci estratti od esaminati nei pregressi d'analisi preesistenti od in scorsi rilasci analizzati con lo stesso canovaccio (past audit), le attribuzioni in dote s'addizionano alla concessione d'adempimenti ad assunzione temporanea a valenza definita tesa a sollevare limitazioni transitorie applicate con dispensa temporale e transitoria in circoscritta deroga ed eccezionale abdicazione ed ispezione per e contro ostacoli in rilascio da cause ricollegabili ad appurata consistenza ma indotti e tollerati (known issues), con ciò parimenti a quest'ultimo annesso l'attività ingenera redazione inerente ad elenco riepilogativo atto alla catalogazione dell'operatività preposta ma sguarnita dai privilegi afferenti ad assunzioni concesse con le poc'anzi assodate dispensa ad hoc.

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

Le deleghe, predisposizioni e facoltà riferite a deroghe possono ricalcare per la costituzione referenziale d'impianto documentale partendo ed usufruendo della struttura generabile in via originaria ad un primo richiamo, a far vece a ciò la direttiva ed imputazione da terminale operabile tramite riga esplicativa di comando formulabile ed attuabile al ricalco seguente:

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

L'impiego adoperato per e nelle deroghe risponde ed ottempera alla necessità pratica indirizzata nel rintracciare e tollerare ed amministrare a scopi gestionali o transitori referenze o addebiti emersi e messi a referto se e laddove d'insorgenza e ripercussioni acclarate per natura ma la cui genesi ne denota persistenza volubile e limitata all'interno delle fasi analitiche e d'ispezione propense ai rilasci. I dettami in singola occorrenza portano o possiedono con sé indicatori idonei ed identificativi nominali propri od associabili a stringhe, designazioni ad etichettature ricollegabili a definizioni per genere di classificazione affine a raggruppamento fittizio / denominazione definita a discrezione / affiliazione dello status preposto in afferenza per correlazioni d'eventi / attributi in associazioni con parità parziale testuale o di match applicabili tra stringhe interne, designando il titolare depositario avente qualifica (owner), suffragando le contingenze e perimetrando via causa giustificativa le concessioni (reason), non prescindendo dall'estipulazione di terminazione o fine mandato ricollegabile per data ed hora limite incastonata da apposito marcatore in `expires_at`. Privilegi decorsi al margine oltre le tolleranze incappano ad inserimento e menzioni in esito da accertamenti tra indagini di riscontro, a questo associati o conseguenti tuttavia non potranno apportare appigli utili al lenimento del problema d'intralcio preposto (non utilizzati a fini soppressivi verso cause o impedimenti che ne ostacolano procedure di rilascio ordinario).

#### Decisione sul Rilascio / Pacchetto di Evidenze

Il novero e rassegna d'ausili aggiunti preposti d'aggiunta o completamento atti ai compiti finalizzati d'operatività nel trapasso in fase utile a terminale successiva in ordine cronologico d'operazione condotta e compiuta a consuntivo della rassegna degli esami ed addebiti riferibili o ricollegabili da controllo a revisioni e validazione in fase ai controlli in e di verifiche sui rilasci (release audit).

- Formati dedotti tra cui figurano `-waiver-calendar`, in associazione con `-waiver-calendar-json` a complemento dell'uso per `-waiver-calendar-html` pongono in esposizione a prospetto panoramico cadenze e termini a decadere di prescrizione per l'esaurirsi (expiry), l'afferenza del legittimo proprietario all'estensione del privilegio, occorrenze conteggiate che riscontrino coincidenze su affinità tra test esaminati nel novero a tolleranze predefinite in uso, riassunti degli attributi attinenti all'esaustione pregressa od al limitare ultimo (expired / expiring-soon states) ed applicabili con disamina esaminatrice riferiti o dipartenti verso ogni delega d'immunità ed in ed ad ognuna singola dispensa di tolleranza adoperata (waiver).
- Impostando in dicitura per `-release-changelog` ed apportando in combinato a supporto e richiamo per l'omologa preposizione su `-release-changelog-json` perverranno in rassegna condensata riassuntiva valutazioni dedotte circa disallineamenti ravvisati durante le comparazioni analitiche ispettive d'accertamento od emersi a contrasto in esami su controlli in divergenza, stasi od in alterazione nei presidi e contingenze correlate alle peculiarità di dispensa ad immunizzazione provvisoria (waiver states), stime e rilevamenti per quantificazione riferibili in e per giacenze su operazioni od impieghi e lavori irrisolti ricollegabili ai pending di transizione o sospeso preposto al rimando da eseguirsi e non ultimato a carico in esecuzione attinente su note o appigli d'attribuzione derivata e demandabile su annotazione da fare ad astenuto nel TODO list, come per converso e parimenti su riepiloghi attinenti per i riscontri dei terminati e limitati attributi decaduti delle deroghe pregresse al momento opportuno ed occorso come referenza visiva a formato compatibile ai resoconti d'agevolazione di comprensibilità tesa alla lettura diretta al supporto del fattore intellegibile al lato umano esente da ostica esposizione.
- Assumendo in uso le indicazioni impartite per l'occasione ed in veste accessoria o correlata tra `-final-decision` che concorre ed associata per confluire nella forma associata e derivata per `-final-decision-json` opereranno un confluire od accorpamento e genesi su prospettive d'esito conclusivo o verdetti emanati in pronunciazione referenziale a sentenze categorizzate ad e per un definitivo avvallo positivo con `go`, ovvero prefiggendosi avvallo con prescrizioni e vaglio a controllo su rilascio con clausole ad esito differito con appoggio da tutela ispettiva ed attenzione od avvertimento ad induzione via appello referenziabile con dicitura cautelativa d'avviso al vaglio su e con `go-with-watch`, ed ad eccezione in contrapposizione del medesimo in vece in o verso diniego d'approvazione od opposizione recisa d'avanzamento dettata ed imposta per rigetto assoluto con diniego d'azione a procedere ed iter ostacolato su rimandi tesi al respingimento senza appello via e con `no-go` appuntando o prescrivendo in esse cause attinenti e referenze o motivi od elenchi riconducibili od annessi alle preclusioni intercorse al momento ed ad azioni preposte di sblocco in agenda all'occorrenza.
- L'utilizzo esplicato in comando ad `-release-evidence-zip` instillerà o riporrà od agirà redigendo in asseverazione all'inclusione d'affidamento in appoggio ed invio a documentazione a corredo (evidence bundle) estrapolazioni per riprova ad un fine con o in stesure condensabili di ridotto formato comprendenti verifiche ispettive e referenze in revisione condotte ad inchiesta ed asseverazione con o ad audits preposti, fascicoli dedotti con risultanze in dote delle dispense temporanee concesse ed emerse od abrogate, resoconti di promemoria a giacenza d'onere in stesura per appigli o liste ad impegni differiti per l'occorrenza (TODO lists), referenze e prospettive cronologiche legate ai diari d'uso in ed a supporto in vigore nei periodi ed allocazione del calendario ad impiego delle tolleranze (waiver calendars), elenchi di rassegna dei dettagli delle differenze appurate intercorrenti e di variabilità nei cambiamenti documentati per rassegna progressiva a storicità pregressa di cronistoria od evoluzione nei rilasci (changelogs) ed annotazioni che referenziano esiti ultimi determinanti d'autorizzazione emanata e deliberata via verdetti od esclusioni irrevocabili.
- Agendo d'ispezione ed invio d'accertamento od inquisizione per sondare via e con `-inspect-release-evidence-zip` comporterà od espleterà a finalità con rassegna l'incameramento e lettura ai plichi d'apposizione per referti in dotazione a prove fornite per e negli involucri esaminati senza compierne o dar adito d'aprirli o di scorporarli preventivamente dalle trattenute via disimballo tese allo sgroviglio esplorativo (sinonimizzando opererà scansione di superficie al pacchetto probatorio esimendosi dall'ingenerare manovre espansive con decodifiche su extraction d'uso per non inficiare) valutando per via documentale incroci d'adempienza all'obbligo del rintraccio di e per archivi vincolanti ed esigibili per dovere a prassi in o a completamento del tutto conformemente al necessario (required files), ravvisando rotte sospette preposte o prefiguranti periglio (dangerous paths), omonimie od innesti multipli per i doppi depositati o accavallati a registrazione (duplicate entries), saggiandone le qualità ad analisi ed interpretabilità di lettura alle formattazioni derivanti dal linguaggio JSON.
- Assegnando in richiamo operativo `-dry-run` simulerà o percorrerà in esecuzione conteggi di processo che elaborino in dote o d'ispezione a compimento ed in vece all'occorrenza e fine le deduzioni che approdino in calcoli od elenchi estrapolati a fine rendiconto prescindendoli ed occultandoli dalla trascrizione a documento esentando dalle riscritture i prospetti fisici associabili al termine della stringa od omissibili con file aggiuntivi.
- Attivando l'esecuzione con l'associazione d'impulsi in ` -exit-code-mode never` espliciterà l'evidenza palesando al momento o palesando per l'occasione referenze ricollegabili od imputabili agl'incamerati ed a o degli in rassegna emersi nei riscontri palesati negli esiti preposti ad output prescindendo dalla presenza e coabitazione ad insidie insormontabili che ad induzione ordinaria od usuale indurrebbero crolli od annullamenti (failure) causati per blocchi al diniego del varco a sbarramento d'ispezione (gate failure) superandoli o bypassando d'ufficio e palesando egualmente d'intenti.

Esempio:

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

Esempio in e per o con un fine riferito ed all'impiego per e di un accertamento referenziale ispettivo addebitabile o da o in e con d'espletarsi su ed a un pacchetto di prove in e per la ed in rassegna entro ambiente e procedure prefiguranti d'afferenza al CI:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## Licenza

Questo progetto è concesso in licenza secondo la BSD 2-Clause License. Consulta il file [LICENSE](../LICENSE) per i dettagli.

SPDX-License-Identifier: BSD-2-Clause
