# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## Visão Geral

Um emulador RV64IMAC executado em Go 1.23.2 `GOOS=js GOARCH=wasm`. O padrão é um único hart, mas o escalonamento cooperativo de 1 a 8 harts está disponível a partir da interface de usuário (UI). Você pode carregar OpenSBI 1.8.1 `fw_payload.bin`/`fw_jump.bin`/`fw_dynamic.bin`/ELF a partir da UI do navegador para confirmar a inicialização.

[![OpenSBI fw_payload boot on rvwasm](images/fw_payload.png)](https://kitaharata.github.io/rvwasm/)

OpenSBI 1.8.1 `fw_payload.bin` inicializando no rvwasm e entrando no payload de modo S do próximo estágio.

## Recursos Implementados

- Instruções base RV64I
- Extensão M
- Implementação mínima de LR/SC/AMO da extensão A
- Instruções inteiras comuns da extensão C
- Equivalentes de Zicsr/Zifencei
- Implementação mínima do modo de privilégio M/S/U CSR/trap/mret/sret
  - Corrige a exceção síncrona `mepc`/`sepc` para o PC da instrução com falha
  - Corrige a carga/gravação CSR com falha para não corromper rd e interrompe o avanço do contador de aposentadoria (retire counter)
  - Verificação de existência de CSR, supressão de efeitos colaterais de gravação de CSR somente leitura, reflexo básico de `mcounteren`/`scounteren`
  - Stubs de CSR `senvcfg`/habilitação de estado adicionados para sondagens (probes) do Linux
  - Reflexo básico de limpeza de `TVM`/`TW`/`TSR` e `MPRV`
- MMU Sv39
  - Modo `satp` Bare/Sv39
  - Caminhada de tabela de páginas de 3 níveis
  - Folhas de 4 KiB/2 MiB/1 GiB
  - Reflexo básico de `SUM`/`MXR`/`MPRV`
  - Exceção de falha de página (page fault exception)
  - Atualização automática dos bits `A`/`D` de PTE
- MMIO estilo UART 16550 (`0x10000000`)
  - Saída do convidado (guest)
  - Injeção de entrada da UI do navegador
  - Interrupção de recebimento
- mtime/mtimecmp/msip estilo CLINT (`0x02000000`)
  - Roteamento MSIP/MTIMECMP por hart para multi-hart
- Controlador de interrupções estilo PLIC (`0x0c000000`)
  - priority/pending/enable/threshold
  - claim/complete
  - Contexto M/S por hart
- Aplicação de PMP
  - TOR/NA4/NAPOT
  - Permissões R/W/X
  - Restrições de modo M por meio de entradas bloqueadas
- Informações de inicialização do OpenSBI `fw_dynamic`
  - As informações dinâmicas são colocadas em `0x87dff000`
  - O ponteiro de informações dinâmicas é definido como `a2`
  - O payload de modo S / kernel pode ser carregado separadamente a partir da UI
- Dispositivo de bloco virtio-mmio (`0x10001000`)
  - Registradores MMIO modernos estilo virtio 1.0
  - Suporte mínimo para read/write/flush/get-id de virtqueue dividida
  - Negociação de `FEATURES_OK` e verificação de `VIRTIO_F_VERSION_1`
  - Redefinição de fila, ignorando notify antes de `DRIVER_OK`, reflexo básico da flag `NO_INTERRUPT`
  - Manipulação de `VIRTIO_RING_F_INDIRECT_DESC` e tabelas de descritores indiretos
  - Supressão de interrupção via evento usado `VIRTIO_RING_F_EVENT_IDX`
  - Imagens de disco podem ser carregadas a partir da UI
  - Imagens de disco modificadas pelo convidado podem ser baixadas a partir da UI
- Dispositivo de console virtio-mmio (`0x10002000`)
  - Console mínimo com ID de dispositivo 3
  - Fila 0 recepção / Fila 1 transmissão
  - Suporte mínimo para `VIRTIO_CONSOLE_F_SIZE`, descritores indiretos e índices de eventos
  - Injeta entrada da UI para UART e virtio-console
- Dispositivo de rede virtio-mmio (`0x10003000`)
  - virtio-net de depuração mínima com ID de dispositivo 1
  - Fila 0 recepção / Fila 1 transmissão
  - Suporte mínimo para `VIRTIO_NET_F_MAC`/`VIRTIO_NET_F_STATUS` / descritores indiretos / índices de eventos
  - Injeta hexadecimal de quadro Ethernet em RX a partir da UI
  - Exibe quadros Ethernet enviados pelo convidado como logs de TX
- Dispositivo rng virtio-mmio (`0x10004000`)
  - Fonte de entropia mínima com ID de dispositivo 4
  - Suporte mínimo para virtqueues divididas, descritores indiretos e índices de eventos
  - A semente determinística pode ser definida a partir da UI
- Dispositivo de entrada virtio-mmio (`0x10005000`)
  - Dispositivo de teclado/entrada de depuração mínimo com ID de dispositivo 18
  - Suporte mínimo para fila de eventos / fila de status, descritores indiretos e índices de eventos
  - Eventos de teclas / eventos de entrada brutos podem ser injetados a partir da UI
- Dispositivo gpu virtio-mmio (`0x10006000`)
  - Base virtio-gpu 2D mínima para depuração com ID de dispositivo 16
  - Suporte mínimo para filas de controle / cursor, descritores indiretos e índices de eventos
  - Respostas básicas para `GET_DISPLAY_INFO`/`RESOURCE_CREATE_2D`/`SET_SCANOUT`/`FLUSH`, etc.
  - Útil para observar sondagens virtio-gpu do Linux e comandos modeset iniciais
- Passagem de initrd/initramfs
  - Endereço de carregamento padrão: `0x84000000`
  - Refletido em `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end` do DTB gerado automaticamente
- Edição de bootargs
  - Padrão: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - Predefinições para UART / virtio-console / initramfs / depuração detalhada
  - Pode ser definido a partir da UI e refletido no DTB gerado automaticamente
- Buffer circular de rastreamento de execução
  - PC/instruções/traps/última causa de trap/tval podem ser visualizados na UI
  - Exportações em Texto/JSON/CSV de dumps CSR e snapshots de rastreamento de todo o hart estão disponíveis a partir da UI
  - Diagnósticos exibindo os últimos argumentos ECALL/SBI, contadores de SBI BASE/TIME/IPI/RFENCE/HSM/SRST/legados, traps e estados de fila virtio em um relance
  - Exportação JSON de Diagnósticos / estados de dispositivos
  - Carrega símbolos ELF/System.map, exibe símbolos em torno do PC interrompido, pesquisa de nomes e resolução automática de símbolos de PC dentro de logs de panic/oops
  - Shim SBI arbitrário para testar diretamente pequenos payloads de modo S sem OpenSBI
    - Curto-circuito mínimo de BASE/TIME/IPI/RFENCE/HSM/SRST
    - Caminho de depuração para a entrada de modo S do hart alvo via HSM `hart_start`
    - Desativado por padrão. Não usado no caminho normal para executar o OpenSBI
  - Faixas arbitrárias de memória física podem ser despejadas (dump) a partir da UI
  - Pontos de interrupção (breakpoints) de PC, pontos de observação (watchpoints) de leitura/gravação física e filtros de rastreamento podem ser definidos a partir da UI
  - Os pontos de interrupção podem especificar contagens de acertos, condições de modo e condições de hart
  - O rastreamento exibe mnemônicos de decodificação simplificados juntamente com instruções brutas
  - Os acertos de ponto de interrupção/observação registram o motivo da parada nas exportações de status/diagnósticos/rastreamentos
  - Coleta histogramas de acesso a MMIO/DRAM, permitindo verificar vieses nas sondagens de dispositivos e atividades de filas por meio de Diagnósticos/JSON
  - Salva linhas do tempo de acesso a MMIO/DRAM no buffer circular, permitindo verificar a série temporal de sondagens em visualizações brutas/compactas
  - A linha do tempo de acesso a MMIO adiciona nomes de decodificadores de registradores para virtio-mmio/UART/CLINT/PLIC, permitindo a observação em unidades como `QueueNotify`/`Status`/`LSR`
  - Habilita opcionalmente o rastreamento de acesso CSR para exibir caudas de leitura/gravação CSR do convidado e resumos de leitura/gravação por CSR em Diagnósticos / exportações de rastreamento
  - Habilita opcionalmente o perfil de pontos de acesso (hot-spot) do PC para ver PCs executados frequentemente com símbolos antes de parar
  - A captura/diferença de snapshot de diagnóstico permite verificar as diferenças nos estados do hart/dispositivo/CSR/MMIO antes e depois da execução na UI
  - Dobra instruções, traps e logs ECALL idênticos e consecutivos na visualização de rastreamento compacto
  - O executor de testes preliminares (smoke runner) por predefinição de inicialização pode executar automaticamente um número especificado de passos de hart do firmware/payload carregado atualmente e recuperar resultados JSON
  - O analisador de fase de inicialização pode resumir as atividades do OpenSBI / Linux / panic / virtio / traps / símbolos de PC juntos
  - A linha do tempo de inicialização pode exibir marcadores de console e sondagens de MMIO / estados / QueueNotifies / reivindicações de PLIC integradas em uma série temporal
  - O analisador de sondagens de dispositivos pode agregar leituras/gravações, registradores de identidade, negociações de status e notificações de fila de virtio/UART/PLIC/CLINT
  - O inspetor de virtqueue pode exibir os estados mais recentes de QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify por dispositivo/fila
  - O visualizador de cadeias de descritores rastreia os descritores principais a partir do anel disponível e exibe descritores NEXT/WRITE/INDIRECT juntamente com uma pequena visualização do buffer
  - A exportação de gráficos de cadeias de descritores pode salvar e visualizar cadeias de virtqueue como DOTs do Graphviz
  - O verificador (scanner) de memória física do convidado pode detectar áreas na DRAM semelhantes às assinaturas mágicas de ELF/FDT/gzip/xz/zstd/squashfs/cpio/ext / versão OpenSBI/Linux / BusyBox / cmdline do kernel
  - O classificador de initcall / sondagem de driver pode categorizar as linhas de log do console Linux relacionadas a initcalls, sondagens, virtio, armazenamento, consoles, redes e gráficos
  - A linha do tempo initcall pode exibir linhas classificadas de initcall / sondagem de driver em grupos de séries temporais
  - Lê tabelas de linhas DWARF de ELFs com símbolos, exibindo arquivo:linha próximo ao PC atual, resumos de arquivos DWARF e anotações de símbolo+linha para PCs de rastreamento
  - O resumo de pânico extrai automaticamente as linhas ao redor de panic/oops/falha no log do console e resolve os endereços com os símbolos carregados
  - O JSON de análise de inicialização pode exportar coletivamente linhas do tempo / sondagens de dispositivos / virtqueues / resumos de pânico
  - O relatório de repetição de rastreamento pode resumir o número de passos/traps/ecalls/shims SBI, mnemônicos frequentes e causas de trap no rastreamento
  - A comparação de linha de base de rastreamento pode comparar as diferenças de PC/instrução/trap entre um rastreamento salvo anteriormente e o rastreamento atual desde o início
  - A linha de base de rastreamento pode ser salva/carregada no/do localStorage do navegador
  - O relatório/JSON de regressão de inicialização, bem como as exportações de relatórios Markdown/HTML, podem salvar em massa as estatísticas de rastreamento, eventos de inicialização, sondagens de dispositivos, virtqueues, objetos de memória e contagens de initcall
  - O snapshot de virtqueue pode exibir as configurações de fila e as cadeias de descritores simultaneamente
  - O detector de anomalias de virtqueue pode detectar endereços de fila pronta ausentes, loops de descritores, comprimentos indiretos inválidos, buffers fora da DRAM, etc.
  - As dicas de anomalias de virtqueue podem exibir sugestões de reparo, como QueueNum/QueueDesc/QueueReady/alinhamento de descritores para cada resultado de detecção
  - A consulta de diagnóstico integrada pode realizar buscas cruzadas em consoles / rastreamentos / rastreamentos CSR / linhas do tempo MMIO / anomalias de virtqueue / índices de memória usando a mesma consulta
  - As predefinições de consulta de diagnóstico permitem a pesquisa em lote por pânicos, negociações virtio, QueueReady/Notifies, satp/mstatus, traps e rootfs
  - O compartilhamento de relatório MD/JSON/HTML permite compartilhar regressões de inicialização, dicas/triagem de virtqueue, índices de memória, predefinições de consulta, dicas de salto e ocorrências de consulta em um formato autônomo
  - O painel de triagem (triage dashboard) / classificação de causas de parada pode exibir pânicos, traps, falhas de página/acesso, anomalias de virtqueue e sondagens de dispositivos paralisados em ordem de candidatos
  - As evidências da causa da parada exibem a justificativa de classificação, os detalhamentos da pontuação, as consultas de diagnóstico recomendadas e as próximas ações
  - A linha de base do painel de triagem pode ser salva no localStorage para comparar as contagens de status/fase/dispositivo/anomalia/memória com o painel atual
  - A linha de base de predefinição de diagnóstico pode ser salva no localStorage para comparar a diferença com a contagem de acertos da predefinição atual
  - O relatório de compartilhamento ocultado (redacted) em MD/JSON/HTML pode gerar relatórios compartilháveis com IPs/MACs/e-mails ocultados
  - As opções de ocultação JSON permitem alternar a substituição de IPs/MACs/e-mails/endereços hexadecimais longos a partir da UI
  - O despejo (dump) de objetos de memória pode verificar hexadecimal + ASCII em torno de acertos de índice de memória/pesquisa
  - O despejo de faixa de memória pode especificar um endereço DRAM arbitrário e um comprimento de bytes para despejo em hexadecimal + ASCII / exportar JSON
  - A captura/diferença de varredura de memória pode verificar candidatos de fragmentos ELF/FDT/initrd/rootfs que aumentaram/diminuíram antes e depois da execução
  - O índice de memória pode agrupar assinaturas próximas de ELF/FDT/initrd/kernel/rootfs por intervalo para criar um índice
  - Extrai logs estilo `dmesg` do Linux das saídas UART / virtio-console e resolve endereços de panic/oops com os símbolos carregados
- simple-framebuffer
  - Adiciona automaticamente `0x86000000`, 1024x768, `a8r8g8b8` a `/chosen/framebuffer@86000000` no DTB gerado
  - Desenha o framebuffer em um Canvas da UI, e dumps brutos RGBA / PNGs podem ser baixados
  - O recurso de apoio 2D para virtio-gpu pode ser copiado para o simple-framebuffer mediante `TRANSFER_TO_HOST_2D`/`RESOURCE_FLUSH`
- DRAM `0x80000000`, 128 MiB
- Geração automática de DTB virt mínimo com virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT, ou carregamento de DTB a partir da UI
  - Compatibilidade com `sifive,plic-1.0.0`/`sifive,clint0` e `dma-coherent` do virtio adicionada
  - Gera `cpu@N` e `interrupts-extended` de acordo com a contagem de harts

## Uso

```bash
make serve
```

Abra `http://localhost:8080` em seu navegador, selecione o firmware OpenSBI e clique em `Load firmware` → `Run`.

Se você quiser testar o virtio-console como o console do Linux, pode alterar os bootargs para algo como `console=hvc0 earlycon=sbi root=/dev/vda rw`. Por padrão, ele usa UART (`ttyS0`) como de costume.

Para analisar um PC parado, carregue um `System.map` do Linux ou um ELF com símbolos usando `Load symbols` e, em seguida, use `Symbols @ PC` / `Diagnostics` / `Search symbols`. Se o ELF com símbolos contiver tabelas de linhas DWARF, você também pode verificar o arquivo:linha usando `DWARF lines @ PC`. O `DWARF file summary` mostra o número de linhas por arquivo contido na tabela de linhas. Se o firmware/payload for um ELF com símbolos, ele importará automaticamente a tabela de símbolos. O `Annotated trace` anota o `pc=` no rastreamento com símbolos/linhas DWARF. `Download trace` salva um instantâneo do rastreamento para todos os harts. Você também pode selecionar os formatos JSON/CSV. O rastreamento JSON inclui informações de símbolos/código-fonte, se existirem símbolos. Insira strings como `trap`, `ecall`, `sbi-shim`, `pc=` ou `virtio` no `Trace filter` para refinar a cauda/exportação do rastreamento, a linha do tempo de acesso e a visualização compacta. O `Compact trace` dobra instruções, traps e ECALLs idênticos e consecutivos. Se você colar um log de panic/oops e clicar em `Analyze log symbols`, ele resolverá os endereços estilo PC de 64 bits no log usando os símbolos carregados.

`Trace replay report` gera estatísticas para o rastreamento atual, e `Trace baseline compare` cola um rastreamento salvo para comparar as diferenças de PC/instrução/trap com o rastreamento atual desde o início. `Save current trace as baseline` / `Load saved baseline` mantém a linha de base no localStorage do navegador. `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` são relatórios de verificação de regressão que consolidam a linha do tempo de inicialização, as sondagens de dispositivos, as virtqueues, o verificador de memória, a classificação de initcall e as estatísticas de rastreamento. Você também pode verificar o aumento ou a diminuição de candidatos a ELF/FDT/initrd/rootfs na memória do convidado executando `Capture memory scan` → Run → `Diff memory scan`. O `DWARF source context` exibe símbolos e arquivos DWARF:linhas ao redor do PC atual em conjunto.

A `Boot phase` resume o progresso atual dos logs do console, dos histogramas MMIO, dos traps e das informações de símbolos. A `Boot timeline` alinha os marcos do console e as sondagens/estados/QueueNotifies de MMIO em uma série temporal. A `Device probe` agrega os acessos aos registradores e as negociações de virtio e outros, e o `Virtqueue inspect` exibe as configurações da fila e os estados de notificação por dispositivo/fila. `Descriptor chains` lê as cadeias de descritores do anel disponível da fila e exibe descritores indiretos e visualizações prévias do buffer. `Descriptor DOT` / `Download DOT` exporta a mesma cadeia como DOT do Graphviz. `Virtqueue anomalies` detecta inconsistências nas configurações das filas e nas cadeias de descritores, e `Anomaly hints` exibe o próximo ponto de verificação para cada inconsistência. A `Integrated diagnostic query` faz pesquisa cruzada em consoles / rastreamentos / rastreamentos CSR / linhas do tempo MMIO / anomalias de virtqueue / índices de memória usando palavras como `virtio QueueReady`, `panic`, `satp`, `0x80200000`. O `Share report MD/JSON/HTML` é um pacote compartilhável que adiciona dicas/triagem de anomalias, índices de memória, dicas de salto de memória, predefinições de consulta e acertos de consulta ao relatório de regressão de inicialização. O formato HTML pode ser salvo como um arquivo independente com JSON incorporado. As `Diagnostic query presets` agrupam as pesquisas relacionadas a pânicos, estados virtio, QueueReady/QueueNotify, satp/mstatus, traps e rootfs. `Save query` / `Load query` salva as consultas de diagnóstico no localStorage do navegador. A `Memory scan` busca candidatos a fragmentos ELF/FDT/initrd/kernel/rootfs na DRAM, e o `Memory index` agrupa assinaturas próximas por faixa. A `Memory search` pesquisa os índices de memória usando strings ou endereços `0x...`, e os `Memory jumps` exibem candidatos de destino de salto úteis como ELF/FDT/Linux/OpenSBI/cmdline/rootfs. O `Initcall classifier` / `Initcall timeline` classifica e marca o tempo dos logs no estilo initcall/sondagem de driver do Linux. O `Panic summary` extrai as linhas ao redor de pânico/oops/falhas e resolve os endereços, se houver símbolos presentes. O `Boot analysis JSON` salva esses itens em conjunto. O `Dmesg extract` extrai apenas as linhas no estilo Linux das saídas UART / virtio-console. O `Decoded MMIO` exibe os últimos acessos a MMIO com nomes de registradores.

O `Triage dashboard` combina classificações de causas de parada, severidade das anomalias do virtqueue, sondagens de dispositivos e marcadores de consulta em um texto de tela única. O `Stop-cause ranking` prioriza os kernel panics, oops, instruções ilegais, falhas de página/acesso, anormalidades do virtqueue e sondagens de dispositivos paralisadas nos consoles/rastreamentos/estados. As `Stop-cause evidence` exibem a justificativa da classificação, a divisão da pontuação, as consultas recomendadas e os próximos pontos de verificação. Você pode comparar as diferenças nas contagens de status/fase/dispositivo/anomalia/memória no painel executando `Save triage baseline` → Run → `Triage diff`. `Save preset baseline` → Run → `Compare preset baseline` permite verificar se a contagem de acertos de consultas predefinidas como panic/virtio/satp/rootfs aumentou ou diminuiu desde a última vez. `Memory dump hits` faz dump em formato hexadecimal/ASCII em torno de acertos de índices de memória usando consultas de diagnóstico ou filtros de rastreamento. O `Memory range dump` especifica um endereço/comprimento arbitrário para fazer o despejo diretamente em hexadecimal/ASCII da DRAM. O `Redacted share MD/JSON/HTML` substitui e-mails / MACs / IPv4s por `<email>` / `<mac>` / `<ipv4>` antes do compartilhamento. As `Redaction options JSON` alternam `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex`.

O `Smoke preset` redefine a predefinição de inicialização selecionada no momento e executa apenas as etapas especificadas. A `Smoke matrix` executa sequencialmente uma lista de predefinições como `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` e lista os passos de execução, a última fase e os candidatos de causa de parada para cada predefinição.

Os pontos de interrupção (breakpoints) do PC são adicionados inserindo o valor hexadecimal de um PC físico/virtual no campo `PC breakpoint` em `Breakpoints / watchpoints`. `Run` / `Step 1k` para num ponto de interrupção, e `Step` avança exatamente 1 instrução, mesmo que o PC atual seja um ponto de interrupção. Os pontos de observação (watchpoints) de gravação detectam as gravações de barramento em um intervalo de endereços físicos, e os pontos de observação de leitura são um recurso simples para detectar leituras de barramento. Eles são úteis para verificar sondagens MMIO, gravações de framebuffer e referências a estruturas específicas. A `Access timeline` / `Compact access` exibe acessos recentes a DRAM/MMIO em uma série temporal compactada. `PC profile on` agrega PCs quentes, e `Capture snapshot` → Run → `Diff snapshot` permite verificar as diferenças de diagnóstico antes e depois da execução.

O simple-framebuffer prepara uma memória de 1024x768x32bpp em `0x86000000` e a coloca como `simple-framebuffer` em `/chosen/framebuffer@86000000` no DTB gerado automaticamente. Se o simplefb for utilizável no lado do Linux, ele poderá ser exibido no Canvas com `Render framebuffer`.

O virtio-net não se conecta a uma rede real a partir do navegador; é um dispositivo de depuração de nível de pacote. Insira o hexadecimal do frame Ethernet no `virtio-net debug` para injetá-lo no RX, e os quadros enviados pelo convidado para a fila TX podem ser verificados em `Show TX frames`. Para que ele seja reconhecido no lado do Linux, execute comandos como `ip link set dev eth0 up` no lado do convidado conforme necessário.

O virtio-rng é um dispositivo de verificação que apresenta um PRNG determinístico como uma fonte de entropia de convidado. Para manter a reprodutibilidade, a semente padrão é fixa e pode ser alterada via `Set deterministic seed` na UI.

O virtio-gpu é um dispositivo mínimo para observar as sondagens de driver virtio-gpu do Linux e a configuração de recursos 2D. Em vez de uma aceleração de GPU real, ele rastreia os comandos modeset / scanout / flush que chegam na fila de controle e envia os estados para os Diagnósticos. Uma vez que ele também copia a memória de apoio dos recursos para o simple-framebuffer, você pode verificar o resultado da descarga (flush) de um recurso 2D pelo convidado através de `Render framebuffer` / exportação de PNG. `UPDATE_CURSOR` / `MOVE_CURSOR` na fila do cursor também são registrados como estados.

O `SBI shim on` é para depurar payloads de modo S diretamente sem OpenSBI. Mantenha-o desativado para experimentos normais usando `fw_dynamic.bin` / `fw_payload.bin`.

Se você quiser testar o Multi-hart, por favor, defina o `Hart count` antes de carregar o firmware. Dado que a alteração das configurações implica em um reset da máquina, presume-se que você recarregará o firmware / payload / disco depois. `View hart` permite alternar os registradores / CSRs / rastreamentos do hart de destino que está sendo exibido.

Ao usar `fw_dynamic.bin`, carregue o payload de modo S / kernel próximo de `0x80200000` via `Load payload` conforme necessário. O emulador coloca as informações dinâmicas em `0x87dff000` e define o seu endereço como `a2`.

Para experimentos no Linux, você pode usar uma das seguintes opções:

- `Load disk`: Passe uma imagem de disco bruta como rootfs em forma de virtio-blk. Os bootargs padrão são `root=/dev/vda rw`.
- `Load initrd`: Coloque o initramfs em `0x84000000` e reflita o alcance do initrd no DTB gerado. Altere os bootargs para `console=ttyS0 earlycon=sbi root=/dev/ram0 rw` etc., se necessário.

Exemplo usando os binários RISC-V pré-distribuídos do OpenSBI 1.8.1:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# Carregue fw_dynamic.bin / fw_payload.bin / fw_jump.bin dos arquivos extraídos via navegador
```

Se estiver compilando o OpenSBI localmente, prepare uma toolchain RISC-V como `riscv64-unknown-elf-` e compile com `PLATFORM=generic`.

### Comandos de Desenvolvimento

```bash
go test ./...
make wasm
make serve
```

## Nota

Esta implementação inclui gradualmente as funções necessárias para investigar a inicialização do OpenSBI, a transição do payload de modo S e o boot do Linux. Para o boot do Linux, foram adicionados PMP, Sv39, virtio-blk, virtio-console, virtio-net, virtio-rng, virtio-input, virtio-gpu, initrd, simple-framebuffer, precisão de trap/CSR, rastreamento/resumo de CSR, decodificadores de registradores/linha do tempo/histograma MMIO, analisador de fase/linha do tempo de boot, analisador de sondagem de dispositivo, inspetor de virtqueue/visualizador de cadeias de descritores/exportação DOT/snapshot/detector de anomalias/dicas de anomalias/triagem de anomalias, verificador (scanner) de memória de convidado/índice/diferença/busca/dicas de salto/auxiliares de despejo, consulta de diagnóstico integrada/predefinições de consulta/comparação de linha de base de predefinição, painel de triagem/classificação de causa de parada, pacote de relatório de compartilhamento/HTML/ocultação, classificador/linha do tempo de initcall, pesquisa de linha DWARF/contexto de origem/anotação de rastreamento, resumo de pânico, extrator de dmesg, repetição/comparação de rastreamento, relatórios de regressão de inicialização, criação de perfil de PC, diferença de snapshot, executor preliminar (smoke runner)/matriz de smoke, diferença de linha de base de triagem, evidências de causa de parada, ocultação editável e dump de faixa de memória. As principais partes não implementadas ou simplificadas incluem um modelo preciso de ciclo/tempo, AIA/IMSIC, conexão real com rede via pontes tap/WebSocket, aceleração total virgl/DRM/GPU, comportamentos estritos WARL/WPRI para todos os CSRs e uma execução paralela verdadeira utilizando múltiplos workers. Multi-hart é um agendamento cooperativo dentro de um único worker wasm.

## Diagnósticos e Auxiliares de Regressão

Maior facilidade de compartilhamento de matrizes preliminares e consultas de diagnóstico.

- `Smoke matrix MD/HTML` salva os resultados da matriz smoke como Markdown / HTML independente.
- `Save smoke baseline` → Run → `Compare smoke baseline` permite verificar as diferenças na fase, nos passos da execução e nas principais causas de parada para cada predefinição.
- `Stop checklist` cria uma lista de verificação de itens de ação específicos para examinar em seguida, com base na classificação das causas de parada.
- `CSR/MMIO bookmarks` extrai apenas os acertos cruciais de CSR / MMIO / rastreamento dos resultados da consulta de diagnóstico integrada.
- `Watchpoint hits` exibe o histórico de acertos do ponto de observação de leitura/gravação em uma série temporal. `Clear hit timeline` limpa apenas o histórico.
- `Artifact manifest` lista as informações atualmente carregadas do firmware / payload / disco / initrd / símbolos e as faixas, entradas e hashes SHA-256 gerados de DTB / informações dinâmicas.

### Auxiliares de Transferência de Regressão

- O `Manifest diff` / `Manifest diff JSON` compara o manifesto de artefato de inicialização atual com uma linha de base salva no localStorage e exibe diferenças nos bootargs, contagens de hart, intervalos de carregamento, entradas, detecção ELF e hashes SHA-256.
- `Auto break/watch suggestions` gera candidatos para pontos de interrupção (breakpoints) de PC / pontos de observação (watchpoints) de leitura / pontos de observação de gravação a serem definidos na próxima execução com base nas evidências de causa de parada, nos PCs de rastreamento recentes e nas linhas de tempo de acertos do watchpoint.
- `Smoke clusters` / `Smoke clusters JSON` agrupa os resultados das predefinições da matriz smoke por fase e causa principal de parada, agrupando as predefinições com o mesmo tipo de falha.
- `Diagnostic bundle JSON` é um JSON autônomo que agrupa o manifesto, painel de triagem, causas de parada, sugestões de breakpoint, pacote de compartilhamento e acertos de watchpoint.
- `Compressed bundle JSON` é o pacote de diagnóstico acima convertido para gzip+base64. Use-o quando desejar reduzir o tamanho antes de colar em problemas (issues) ou bate-papos.

### Auxiliares de Transferência / Proveniência

- `Decode bundle` extrai um `Diagnostic bundle JSON` ou um `Compressed bundle JSON` colado, ou gzip+base64 bruto.
- `Bundle compare` / `Bundle compare JSON` compara um pacote colado anterior com o pacote atual e exibe as diferenças nas fases de triagem, causas principais de parada, manifestos, hashes de artefatos, clusters smoke, acertos de watchpoint e contagens de sugestões.
- `Provenance` / `Provenance JSON` resume os hashes SHA-256 do manifesto, rastreamento, console e pacote de diagnóstico, as contagens de linha de rastreamento, as contagens de bytes do console e as principais causas de parada. Pode ser usado para verificar a reprodutibilidade ou como prova anexada a problemas (issues).
- `Handoff MD` resume a proveniência, as principais causas de parada, as sugestões automáticas de interrupção/observação, as listas de verificação de paradas, as diferenças de linha de base e os manifestos de artefatos em formato Markdown.
- `Apply auto breaks` aplica de maneira agrupada os principais candidatos de sugestões automáticas de interrupção/observação para o emulador atual. É um utilitário para configurar rapidamente as posições de parada ou intervalos de MMIO/DRAM suspeitos antes de uma nova execução.

### Reprodução / Assinatura / Transferência Headless

- `Repro plan` / `Repro MD` / `Repro JSON`
  - Gera etapas de reprodução a partir de pacotes de diagnóstico, proveniência e manifestos de artefatos.
  - Lista os papéis, tamanhos, faixas de carregamento e os hashes SHA-256 do firmware / payload / initrd / disco / símbolos como fixações de artefatos (artifact pins).
  - Documenta as predefinições de smoke, bootargs, contagens de harts, next_addr e condições de break/watch recomendadas em etapas.
- `Log signature` / `Log signature JSON`
  - Cria um resumo leve a partir dos hashes SHA-256 dos rastreamentos / consoles / manifestos, das contagens de linha de rastreamento, dos primeiros/últimos PCs, das primeiras/últimas linhas do console e dos tokens frequentes.
  - Permite comparar "Esse é o mesmo log?" ou "O que mudou?" sem precisar colar os rastreamentos na íntegra.
- `Save log signature` / `Load log signature` / `Compare log signature`
  - Salva a linha de base da assinatura do log no localStorage do navegador e compara-a com a assinatura atual.
  - Exibe as diferenças nos hashes de rastreamento, hashes do console, hashes do manifesto, contagens de linha, últimos PCs e últimas linhas do console.
- `Auto break verify`
  - Exibe um resumo de confirmação antes de aplicar as sugestões automáticas de breakpoint/watchpoint.
  - Emite avisos sobre sugestões duplicadas ou faixas de PC suspeitas.
- `Headless smoke script`
  - Gera um esqueleto de script de shell para CI/transferência a partir do manifesto de artefato atual, bootargs, contagens de hart, predefinições smoke e contagens de passos.
  - Destina-se a fixar as dependências (pins) de artefatos e as matrizes de predefinições antes de adicionar utilitários de navegador (como Playwright) ao ambiente de execução.

#### Auxiliares Headless / CI

Para facilitar o manuseio da transferência de repro/assinatura no CI ou em problemas, foram adicionados os seguintes recursos.

- `Bundle integrity` / `Integrity JSON` verifica a consistência entre o pacote de diagnóstico e o manifesto do artefato, categorizando as discrepâncias nas funções de artefatos, hashes SHA-256, faixas de carregamento, sugestões e resultados smoke como `error`, `warn` ou `info`.
- `Repro validation` / `Repro validation JSON` verifica se o plano de reprodução atual corresponde aos bootargs do pacote, contagens de hart, next_addr, artifact pins, principais causas de parada e assinaturas de log.
- `CI summary` / `CI summary JSON` consolida a integridade do pacote, assinaturas de rastreamento/console, resultados smoke e causas de parada, gerando um resumo que facilita os julgamentos de pass/warn/fail no CI.
- `Headless runner spec` / `Runner spec JSON` gera predefinições, etapas, artifact pins e comandos recomendados para inspeção através do `go run ./cmd/rvsmoke ...`.
- Adicionado `cmd/rvsmoke`. Ele pode ler pacotes de diagnóstico / manifestos de artefatos fora do navegador e exibir hashes de artefatos, integridade do pacote, resumos de CI e especificações do executor (runner) em texto / JSON / Markdown.

Exemplo:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

O `rvsmoke` realiza atualmente inspeções de reprodutibilidade e a geração de resumos CI para o pacote/manifesto e hashes de artefatos. A própria execução da CPU continuará utilizando a matriz smoke do lado js/wasm do navegador.

#### rvsmoke CI Gate / JUnit / SARIF

O `cmd/rvsmoke` é uma ferramenta CLI (linha de comando) para a inspeção de pacotes de diagnóstico/manifestos exportados em CI. Ao materializar a execução headless, ela pode gerar as comparações de pacotes basais, políticas de liberação CI (CI gate policies), XMLs de JUnit, SARIFs e relatórios HTML autônomos.

Exemplo:

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

Exemplo da política em JSON:

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

`-out html` imprime o HTML autônomo no stdout, `-out junit` faz o mesmo para o JUnit XML, e `-out sarif` para o SARIF JSON. Se o `-junit` / `-html` / `-sarif` forem especificados em simultâneo, eles farão o salvamento em seus respectivos arquivos além do formato stdout. As barreiras (gates) do CI normalizam os manifestos de artefatos, assinaturas de rastreamento/console, diferenças da linha de base, anomalias do virtqueue e os resultados do smoke (testes preliminares) nas categorias `pass`, `warn` ou `fail`.

#### Modelos de Política rvsmoke / Comparação de Tendências de Pacotes

Para facilitar a introdução inicial de barreiras CI (CI gates) e múltiplas comparações de regressão, os modelos de política, as listas de verificação de ações (action checklists) e a comparação de tendências de pacotes foram adicionados ao `rvsmoke` e à interface do navegador.

- O `CI policy templates` / `Policy templates JSON` exibem políticas incorporadas (built-in): `default`, `strict`, `linux-boot`, `artifact-only` e `lenient`.
- O `Policy template JSON` salva o modelo especificado sob o formato JSON, preparado para integração fácil ao CI.
- `CI gate` / `CI gate JSON` aplica um modelo de política para o status atual do navegador e mostra as verificações pass/warn/fail da barreira.
- `CI checklist` / `CI checklist JSON` converte falhas da barreira (gate failures), integridade do pacote e diferenças de artefatos em checklists aplicáveis.
- `rvsmoke -compare name=bundle.json` organiza vários pacotes numa linha cronológica e emite relatórios de tendência revelando as alterações nas fases, as principais razões de parada, hashes de artefatos e os clusters do teste smoke.

Exemplo da geração de modelos de política:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

Exemplo para a comparação de múltiplos pacotes (bundles):

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

O `-policy-template` serve de política padrão para quando `-policy` não é designado. Se `-policy` for especificado, o JSON desse arquivo toma procedência.

## Integração CI rvsmoke

Os auxílios e ferramentas de handoff para CI no `rvsmoke` foram aprofundados.

- `rvsmoke -print-github-actions linux-boot` consegue gerar arquivos YAML para o fluxo de trabalho (workflow) de GitHub Actions.
- `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` exporta o fluxo de trabalho para os arquivos correspondentes.
- `rvsmoke -policy-tree policy-tree.md` consegue salvar as barreiras do CI / a integridade do pacote / desvios das linhas de base como árvores de causas (cause trees).
- `rvsmoke -history history.txt` registra agregações das derivações de fase / motivos de pausa / desvios de artefatos extraídos das tendências de vários pacotes simultaneamente.
- `rvsmoke -repro-zip rvwasm-minimal-repro.zip` elabora arquivos de reprodução mínimos englobando READMEs, os pacotes diagnósticos, manifestos, especificações de executor (runner specs), políticas, resumos do CI e scripts verificadores.

Exemplo:

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

A flag `-repro-zip` não inclui dentro os firmwares originais/kernels/discos. Em vez disso, embute fixações em SHA-256 e os intervalos do manifesto dentro do pacote, baseando-se no pressuposto de que os artefatos sofrerão verificação por quem os receba.

### Inspeção de ZIP de Repro CI / Continuação de Fluxo de Trabalho de Matriz

Ferramentas suplementares foram combinadas ao `rvsmoke` e à Interface do Navegador de forma a inspecionar o processo de transferência envolvendo pacotes repro mínimos (minimal repro packages), gerando produtos finais (outputs) dedicados às matrizes do GitHub Actions / visualizações gráficas de tendências.

- `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` possibilita fiscalizar o ZIP elaborado via `-repro-zip` dispensando a sua extração. Certifica se os arquivos necessários constam do ZIP, bloqueia rotas inseguras (unsafe paths), além de fazer a correspondência com o `diagnostic-bundle.json` / `manifest.json`, bem como com `ci-policy.json` e `scripts/rvsmoke.sh`.
- `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` consegue confeccionar arquivos YAML atrelados aos fluxos matriciais de GitHub Actions com segmentação por presets.
- `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` encaminha os arquivos da matriz para ficheiros.
- `rvsmoke -trend-csv rvwasm-trend.csv` e `-trend-chart-json rvwasm-trend-chart.json` retêm tendências de pacotes no padrão CSV / JSON de modo a favorecer construções de gráficos externos.
- Acrescidos `Minimal repro ZIP`, `Inspect repro ZIP`, `Repro ZIP JSON`, `Matrix workflow YAML`, `Trend chart JSON` e `Trend CSV` na UI do Navegador.

Exemplo:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# Salva os resultados das inspeções sob a forma de JSON
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# Converte as tendências do pacote (bundle) presente bem como do anterior para CSV/JSON
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` tem autonomia para execução independente. Ao ser designada junto com `-bundle`, as análises resultantes da fiscalização do ZIP farão parte do relatório convencional do CI e quaisquer impasses representarão um insucesso englobado no CI.

### Agregação de Matriz CI / Continuação de Manifesto de Checksum

As ferramentas ligadas ao traspasse de artefatos do CI pelo `rvsmoke` receberam aprimoramentos.

- `-repro-checksums rvwasm-repro-checksums.json` armazena manifestos determinísticos que validam a integridade (checksums) alusivos aos arquivos alocados dentro do ZIP usando por base as averiguações provindas do `-inspect-repro-zip`.
- Ao determinar múltiplos `-matrix-result name=rvsmoke-output.json`, consegue-se englobar apurações do `rvsmoke -out json` com diferentes presets / distintos trabalhos operacionais (jobs).
- `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` gravam agregações atreladas à matriz via formulários de texto / JSON / páginas de HTML (self-contained HTML).
- `-trend-html rvwasm-trend.html` salva relatórios com foco na tendência do pacote usando páginas em HTML com estrutura própria (standalone).

Exemplo:

```bash
# Reter conteúdos do ZIP atrelado à reprodução mínima assim como o manifesto referente às checksums
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# Agrupar JSONs referentes ao rvsmoke baseados nas tarefas que abranjam trabalhos envolvendo matrizes
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

Estas matrizes consolidadas compilam panoramas das instâncias CI abrangendo reprovações da barreira/número de advertências proferidas, impasses envolvendo os artefatos juntamente das maiores responsáveis pelas paralisações (stop-causes) catalogadas por trabalho efetuado. Atua a título de utilidade para inspecionar, por via descomplicada, as tendências de insucessos nas consolidações derradeiras ainda com divisões vigentes nos fluxos de GitHub Actions.

#### Auxiliares de Transferência de CI / Lançamento

As gestões centradas em artefatos englobadas no CI assim como os transportes associados aos lançamentos efetuados via `rvsmoke` conquistaram otimizações.

- `-artifact-index rvwasm-artifacts.json` enumera referências sobre os trilhos/pastas correspondentes (paths), os bytes abrangidos aliando as assinaturas baseadas no cômputo via SHA-256 pertinentes aos arquivos elaborados dentro do CI abarcando formatações JUnit / SARIF / HTML / quadros de tendência (trend) / esquemas matriciais e as devidas averiguações com checksum.
- `-release-manifest rvwasm-release.json` conjuga num invólucro singular, focado em handover (repasse), os pacotes de diagnóstico em conjunto às autenticações do registo (log), inspeções referentes às portas do CI, esquemas agregadores embasados em matriz, reportes apontando falhas voláteis (flakes), além da fiscalização em cima das checksums da repro.
- `-release-html rvwasm-release.html` propicia documento do tipo HTML contando com menus propícios aos relatórios atrelados a Summary / Artifacts / Matrix / Checksums e JSON.
- `-verify-repro-checksums baseline-repro-checksums.json` contrapõe as anotações do manifesto embasadas nas checksums referentes ao ZIP repro-minimalista presentemente avaliado utilizando um limiar referencial propiciando a identificação pontual da falta / alteração ou afluência não habitual das inserções de dados.
- `-matrix-flakes`, `-matrix-flakes-json` somados a `-matrix-flakes-html` conferem constância ao fluxo atrelado a sucessivos levantamentos embasados na matriz tal como `uart#1` / `uart#2` propiciando detecção contundente caso os testes pontuem uma instabilidade volátil ao passo em que oscilam frente ao desfecho entre aprovados (pass) ou chumbados (fail).

Exemplo:

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

## Transferência e Verificação de Lançamento

Os desfechos envolvendo compilações com referências em metadados ingressaram de vez no `rvsmoke` viabilizando que o escoamento englobado pelas atestações atinentes à conjuntura do CI adentre às ramificações envolvendo outros terminais/maquinários assim como outros esquemas alocados nos repositórios, tal como o envio até os próprios conferentes (reviewers).

### Extensão SBOM / Proveniência

#### Inventário de Dependências SBOM-lite

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

A listagem atrelada aos quesitos envoltos nas dependências tem como desígnio figurar em proporções restritas garantindo também que haja uma formatação contundente calcada no caráter determinístico. Avalia o arquivo atrelado ao `go.mod` computando as devidas inserções ligadas aos endereçamentos abrangidos no módulo, enumerando edições correspondentes ao Go aliando os informes dispostos sob a alcunha do encargo referenciado via `require`, juntamente aos propósitos abrangidos via `replace` compreendendo as subdivisões categorizadas frente às modalidades atinentes aos artefatos elencados no indexador alocado no CI.

Ao despachar os expedientes inerentes à mecânica atrelada ao `rvsmoke` lançando mão da execução advinda de um diretório em apartado aponte a rota com o argumento explícito `-go-mod /path/to/go.mod`.

#### Atestado de Proveniência

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

O instrumento correspondente ao atestado manifesta-se a título de carga categorizada sob o formato JSON calcando as expetativas com base no esquema abrangido por in-toto / SLSA. O atributo aqui subscrito não pontua atestado em si na conjuntura singular atrelada às assinaturas, muito embora disponha do amparo atinente a uma formulação embasada na robustez contínua via SHA-256 propiciando que o instrumento ganhe adesão e atue sob o caráter atrelado à designação alvo frente aos instrumentais atrelados a mecanismos de assunção ligados à órbita oriunda da estrutura externa referente ao CI.

#### ZIP de Transferência de Lançamento

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

A alocação ligada à embalagem ZIP incumbida do trâmite inerente à passagem do lançamento atrela estritamente aos contornos ditados via metadados sob a sua exclusividade.

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

O instrumento dispensa acoplamentos ou anexações brutas que perfaçam encorporações que mesclem firmwares, núcleos sistêmicos (kernels), instâncias pertinentes ao initrd do mesmo modo atrelado aos espelhamentos ditados no tocante às inserções envolvendo as partições com origem nos discos. As porções estruturais contendo proporções alargadas frente à volumetria dos artefatos retém-se a título de alocação garantida embasada nos esquemas subscritos através dos pins SHA-256 no enquadramento englobado pelo manifesto.

#### Inspeção do ZIP de Transferência de Lançamento

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

A fiscalização inerente à estrutura ligada ao inspetor elabora sondagens pertinentes à constituição englobada dentro do ZIP pautando que não haja a referida necessidade de extração prévia com o fito de constatar a existência alocada às diretrizes indispensáveis ditadas com base nos ficheiros necessários, coibindo direcionamentos propensos à deflagração frente aos perigos iminentes, contendo duplicidades relativas à enunciação ligada às alocações de endereçamento, viabilizando capacidade correspondente à varredura e triagem atinente ao arcabouço originário do JSON acoplando ainda as diretrizes basais que propugnam no que circunscreve à formatação das correspondências atreladas aos despachos dos lançamentos em concordância ao enquadramento imposto nas diretivas englobadas nos indexadores atrelados aos SBOMs juntamente com as legitimações inerentes aos atestados correspondentes.

### Verificação de Lançamento

Como elemento conjugado atinente às conceções alocadas no entorno dos trâmites envolvendo os ZIPs atrelados à dinâmica em que opera as propagações propiciadas na conjuntura subscrita ao handoff correspondente aos lançamentos, os aditivos que deságuam sob o escopo afeto às diligências em torno dos instrumentos que viabilizem devida fiscalização somaram-se.

- `-verify-attestation` / `-verify-attestation-text` atestam validade atinente a congruência envolvendo os parâmetros exatos extraídos advindos na asserção englobada pela integridade baseada num cômputo atrelado sob um escrutínio com formatação referencial focada na certificação propensa à alínea com base nas proveniências juntamente às matérias relativas atreladas à alocação de lançamento no encadeamento embutido nos esquemas que denotem os objetos pertinentes a toda uma formatação correspondente aos artefatos subscritos na CI para conferir uma constatação atrelada aos escopos extraídos a partir da configuração vinculada aos manifestos associados na entrega alocada com as disposições abrangidas ligando aos inventários calcados em SBOM-lite adentrando à matriz atinente à indexação abarcada via artefatos.
- `-sbom-baseline`, `-sbom-diff` em comunhão ao `-sbom-diff-json` perfazem contrastes no enquadramento focado na disposição atual atrelada a toda uma indexação de dependências calcadas no entorno da matriz disposta no formato atrelado com base num inventário que se apresente enquadrado num escopo afeto via SBOM-lite frente a uma métrica subscrita originária de alocações retidas em bases precedentes com armazenamento validado.
- `-compare-release-zip-inspection`, `-release-zip-compare` interligados em comunhão à extensão provinda através do referencial afeto à modalidade baseada em JSON com `-release-zip-compare-json` delineiam verificações e efetuam constatação ligada à discrepância abrangendo a disposição afeta à constatação baseada em toda averiguação a recair no ZIP subscrito afeto às deliberações com base num envio pautado na distribuição do lançamento com formatações JSON alusivas a sondagens pregressas.
- `-retention-manifest` / `-retention-text` engendram um manifesto embasado no panorama com retenção afeto aos artefatos calcados via CI alocando percursos abrangendo endereçamento atrelado a tipologias juntando mensurações calcadas na métrica afeta à dimensão através do esquema medido via bytes associando valências apuradas a partir de SHA-256 e computando ainda mensurações que delimitem dias afeto à duração da alocação de dados associando referencial englobando marcos ditados que estipulem a validade e a cessação destas disposições com motivos correlatos.
- `-release-verification-html` engendra produtos dispostos em linguagem calcada através do HTML promovendo facilitação disposta na formatação de matriz para menus sumariantes que delineiam as situações abrangendo averiguações dispostas e constatadas relativas a escrutínios provindos das certidões propiciadas em conjunção aos ditames provindos dos delineamentos calcados a partir de disparidades apontadas que decorrem dos esquemas advindos via métricas englobadas no referencial atinente a matriz provinda dos exames do SBOM somando averiguações que traçam cotejos propiciados com enfoque subscrito perante a exames baseados nos ZIPs atrelados na conjunção e na conjuntura referenciada nas matrizes embasadas e propensas a referenciar o estado em torno dos dados guardados.

Exemplo:

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

### Portão de Auditoria de Lançamento

Um escalão sobranceiro associado e alocado visando perfazer crivo em caráter final embasado no referencial ditado via esquemas afetos às diretrizes englobadas na matriz ditada perante à dinâmica em que confluem fiscalizações do cariz advindo com a verificação de alocações de lançamentos fora acrescido com destaque em sobreposição face à constatação das liberações afetas aos lançamentos. Reúne num compêndio conciso verificações inerentes a aferimentos do quilate subscrito sob a égide dos informes com atestado afeto na proveniência, oscilações no seio associado ao SBOM-lite, cotejo no entorno da matriz ligada em ZIP alusiva aos despachos englobados no referencial abrangido via lançamento, cessações que atinjam crivos afeto à constatação de perenidade afeta via caducidade referenciada perante à permanência nos prazos associados na guarda dos artefactos e delineamentos englobados na perspetiva propiciada com dados afetos por meio de variações nas instabilidades atinentes à conjuntura subscrita com informes pontuais afetos à referida configuração de instabilidade (flakes) somando averiguações em formato com disposições condensadas associadas aos manifestos que delineiem o crivo correspondente resultando do agrupamento uníssono no tocante com pontuação singular englobando relatório ditado por meio da alocação referente em barreira.

Flags basilares (comandos):

- `-list-release-verify-policies` compila enumerando e descrevendo ordenações com esquemas de caráter focado à dinâmica atinente à verificação associada a preceitos focados perante averiguação do lançamento de tipologia embasada com raiz (built-in).
- `-print-release-verify-policy strict` exporta base estruturante afeta sob a forma com dimensão calcada na linguagem afeta à tipologia em molde com formatação atrelada através do JSON englobando as disposições atinentes perante preceitos pautados perante ordens dispostas na alocação correspondendo à política.
- `-release-verify-template default|strict|lenient|archive` faculta eleição visando pinçar de dentro da constituição englobando o escopo com as estipulações de raiz originárias sob matriz com delineamento (built-in) ligada e associada com enquadramento de política.
- `-release-verify-policy policy.json` aloca absorção visando acionar diretriz amparada em esquema configurável de estipulação e averiguação adveniente com enquadramento perante auditoria calcada no escopo atinente perante lançamento subscrita de maneira exclusiva atrelada com caráter personificável (custom).
- `-retention-audit` / `-retention-audit-json` edita e consigna as constatações aferidas oriundas com raiz embasada sob a premissa de escrutínios atinentes a limiares demarcados sob a cessação (expiry) e alíneas ditando perenidade embasadas no mínimo admissível.
- `-release-score` / `-release-score-json` projeta com escrituração referencial pontuação atinente perante exame afeto com certificações no domínio subscrito no limiar compreendido a partir da escala com arranque balizado via 0 findando na delimitação englobada na matriz com máximo correspondente no 100.
- `-release-gate` / `-release-gate-json` redige desfechos alusivos à matriz e ao desenrolar contido na disposição operante englobando alocações na barreira (gate) amparadas e estipuladas frente a alíneas ligadas nas estipulações com formato calcado e inerente a política.
- `-release-audit` / `-release-audit-json` / `-release-audit-html` perfaz edição na constituição abarcando relatório consolidado abrangente com foco englobando a triagem de auditoria no âmbito integrado.

Exemplo:

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

A diretriz que instaura as imposições em molde de rigidez (strict policy) aloca com perspetiva e enquadramento atrelado visando reputar em desfavor pontuado a título na categoria com falência referentes atinentes perante manifestos que não aufiram conformidade associada propiciando com isso desfecho favorável, refuta a validade em cima das análises afetos na perspetiva propiciada via atestado / SBOM / averiguamento sobre ZIP culminando em desaprovação além da desqualificação na matriz ligada a referencial contendo peças caducas e também condena alocações afetas no referencial de artefactos contendo retenção sob alçada no limiar quantificado em valores situados com déficit em face de quantidade estipulada fixada a termo em cômputo mínimo associando restrição balizada atrelada atinente à durabilidade medida com o quantificador com alocação em dias ditados em referência. A estipulação basilar englobada na configuração assumida de fábrica perfaz alinhamento satisfatório e contundente em adequação e propensão com preceito que aufere a rotina habitual inerente no desenrolar nos repasses e verificações deflagradas de permeio ao trâmite noturno conferindo aval sob a condescendência focada ao deflagrar menções alertando sob a faceta via avisos sem prejuízo com obstar à tramitação ligada com aprovações não obstante deflagrar recusa categórica perante falências evidentes subscritas por meio claro de estipulação e crivo de chancela focado na certificação.

#### Diferença de Auditoria de Lançamento / Isenções (Waiver) / Transferência TODO

A rota designada afeta por meio dos delineamentos que tangem aos percursos delineados com as especificações embasadas na auditoria atinente no escopo atrelado à ótica visando englobar as validações perante envios alusivos face aos delineamentos embasados via `rvsmoke` aufere o amparo focado numa mecânica subscrita ao deflagrar contraste colocando sob a mesma visada a disposição de auditoria no caráter corrente embasada perante ótica em relação de contraste no crivo da sondagem pretérita alocando no intermédio autorizações calcadas englobando escopo temporal em exceções circunscritas aplicáveis focadas na resolução de discrepâncias subscritas no âmbito do conhecimento em comum conjugando ainda a faculdade propensa com foco gerador com base na listagem visando elencar preceitos e diretrizes de trabalhos subscritos alusivos focados na triagem sobre pendências ainda desprovidas da aplicabilidade atrelada na prerrogativa na dispensa por exceção.

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

Tem-se a faculdade de alocação de um exemplar em matriz de arquétipo modelar visando dispensa (waiver template) ativando na consola a instrução ditada sob a constituição a seguir:

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

O instrumento correspondendo à isenção (waivers) angaria vocação e enquadramento prático visando administrar averiguações angariadas atinentes em escopo notório e efémero com constatações perante triagens focadas nas constatações com raiz no balanço via auditorias do lançamento em despacho. Cada disposição englobando as regras alberga identificador pontuando um subscrito sob o cognome ID, atrelamento personificável focado na constituição de tipologia designada de forma arbitrária / rotulação / conjuntura inerente à dinâmica subscrita associada no status / esquemas com disposições comparadoras alusivas de matriz que compõe subcadenas focadas visando atuar como filtros propensos ao cotejo de similitude ligadas como referencial subscrito do agrupamento das strings atinentes a comparadores em formato de subtipos, a fixação do encargo focada na designação propícia na autoria encarregada no âmbito do domínio, sustentáculo em forma de premissa balizada visando firmar em justificação associando ainda rubrica estipulando crivo na barreira de expiração englobada pelo balizador focado ao carimbo inerente na matriz de formatação ligada pela chancela atrelada mediante a aposição temporal ditada na linha com subscrito a constar do ditame em rubrica `expires_at`. Privilégios vencidos e abarcados em caducidade auferem reporte, declinando no desfecho em abster perante o recurso no viés e no âmbito do recurso tencionando obliteração afeta via mascaramento atinente ao abafamento no fito visando submersão ligada atinente à omissão alocada aos impasses evidenciados.

#### Decisão de Lançamento / Pacote de Evidências

Acréscimos relativos no preâmbulo que perfaz cominação ditada face ao amparo focado ao transbordo de feição propícia adstrita de forma a servir ao crivo ditado em fecho ao atrelamento ditado depois do acionamento alusivo a mecânicas atreladas face à constatação das vistorias abarcadas nas instâncias balizadas sob o cunho das auditorias estipulando verificação dos expedientes envoltos em trâmite.

- `-waiver-calendar`, `-waiver-calendar-json` acoplados no formato HTML com `-waiver-calendar-html` perfilam com explicitação englobando disposições focadas à estipulação afeta à barreira imposta em cessação baseada na validade de caducidade, a delegação atinente com enquadramento da alocação de domínios designando matriz na responsabilidade balizada face à detenção na titularidade do privilégio no domínio do registo em titular, quantitativos que encerrem e designem amostragem da paridade baseada nas coincidências apuradas englobando as disposições afetas à caducidade subscrita nas ocorrências juntamente e agregando perante perspetiva alusiva englobando delineamentos balizados atinentes e iminentes face à eminência atrelada a vencimento próximo no cômputo englobado a atuar em reflexo propício para o retrato ditado a cada disposição alusiva na dispensa (waiver).
- `-release-changelog` juntando na conjugação a alínea afluente calcada perante escopo da matriz subscrita por JSON através de `-release-changelog-json` efetuam cômputo congregando delineamento abarcando os contrastes evidenciados na auditoria calcada no cotejo pautado via constatações e contrastes de variações no referencial englobando balanços dispares aferidos a partir da ótica abrangendo os desvios focados com a métrica englobada via desfecho de constatações no domínio abrangido perante auditoria balizada via contrastes atinentes nos exames e deliberações das constatações atinentes, oscilações na vigência e perspetiva disposta atinente nos panoramas relativos com o status do privilégio afetos nos regimes nas isenções dispostas, cômputos a demarcar alíneas focadas nas contabilidades atinentes nas empreitadas carentes englobando incumbências e atribuições pendentes afluentes atinentes no subscrito de afazeres (TODO) com reflexo na conjuntura a dispor perspetiva atrelada nos estágios focados perante a baliza estipulando crivos subscritos no vencimento no limite englobando estipulação afeta na chancela focada no ocaso nas datas das dispensas conjugadas perante crivo subscrito sob a égide e roupagem atinente e propícia perante a formatação e visualização propiciando formatação no diário a englobar os reflexos nos apontamentos focados no rol das alternâncias balizados mediante formatação focada à visada natural propensa perante o limiar tangível perante e voltada ao humano.
- `-final-decision` atuando lado a lado abrangendo perspetiva originária e disposta no molde delineado face a JSON conjugado sob alçada na alocação subscrita via `-final-decision-json` alavancam com prole com feição geradora voltada à matriz com balanço de disposições e ordens ditadas englobando cariz e deliberações com a chancela do enquadramento afeto no limiar com veredito focado na disposição atinente e derradeira em cunho com matriz final calcada via orientações englobando `go`, perante delineamento englobado via ressalvas na chancela do enquadramento e estipulação subscrita em amparo ao `go-with-watch` juntando a estipulação negativa com bloqueios no encerramento propiciado na interdição embasada perante o fito com chancela subscrita via constatação de bloqueio referenciada pelo `no-go` comportando no amago matérias advenientes com aspas focadas com traços em obstáculo gerando embargos aliando o referenciamento na enumeração delineada adveniente e afluente focada a designar a alínea atinente na indicação apontando perspetiva propícia englobando na dinâmica as ações ditadas no cronograma a apontar diretriz e alocação perante o porvir.
- `-release-evidence-zip` subscreve no enquadramento e na formatação escriturada a fluência focada ao preenchimento disposta propiciando que se engendre um envoltório afluente em miniatura que concentre e perfaz compilação na formatação ligada e agrupada numa estrutura diminuta concentrando na modalidade focada com embalagem no crivo da métrica alocada propiciando em anexo os acervos contendo e perfazendo agrupamento englobando com pacote com as provas englobando num escopo único os resultados provenientes atrelados face à extração apurada via mecânica subscrita no limiar focada nos escrutínios atinentes a auditoria, informes atrelados aos expedientes no limiar das diretivas com constatações ditadas a circunscrever ao relatório ditado pelo amparo e no amparo voltado com foco voltado aos enquadramentos focados frente à isenção ditada no escopo atrelado nas deliberações englobando a exceção (waivers), listagens com rol focado abrangendo propósitos atrelados e pendentes de execução englobados a apontar o que se subscreve a estipular incumbência futura, diagramações englobando marcos de visualização afeta em calendário englobando na linha cronológica o enquadramento ditado visando espelhar na calendarização os propósitos englobando exceção propícia abrangendo na conjuntura os limites da vigência nas dispensas, pautas a englobar balanço de alterações advenientes com registos (changelogs) e delineamentos ditados no balanço na perspetiva propícia da disposição propiciando as asserções e delineamentos encabeçando deliberação a firmar estipulação do fecho englobando no cariz atinente do derradeiro veredito.
- `-inspect-release-evidence-zip` aplica estipulação afeta em diligência calcada perante o escopo visando triagem ditada perante inspeção alocada subscrita englobando os envios no encadeamento ditado via embrulho da conjuntura formatada abrangendo fardo atinente aos anexos em pacote perante as comprovações ditadas sem proceder ao desentranhamento (sem extração) no viés propiciando que se angarie premissa com asseveração ditada focada em colher apuro e no limiar do fito a abarcar a pesquisa por ficheiros carentes com imperativo para constar e figurarem de permeio nos ditames de enquadramentos, percursos detentores face à matriz atinente a cariz associado ao perigo em rota perigosa, registros em reincidência afeta mediante a existência atinente focada perante à duplicidade nos apontamentos calcados perante entradas (entries) e a conjuntura a atestar validade propícia calcada no fito ditado na competência afeta à assimilação em sintaxe (parsability) alocada com matriz calcada via escopo provindo em arranjo na conjuntura via formatação ditada e advinda com o pilar no alicerce via enquadramento calcado do viés da linguagem em JSON.
- `-dry-run` encabeça premissa afeta no labor atinente em realizar cômputo englobando apuração atrelada na matemática perante delineamento englobando escopo ditado e voltado a relatórios prescindindo perante a formatação ditada na matriz no desígnio atinente face à constatação ligada a escrituração calcada e vertida na criação dos ficheiros oriundos na matriz de feição abarcando saídas advenientes dotadas e embasadas de forma acessória atuando facultativamente.
- `-exit-code-mode never` exporta premissas calcadas em delineamento englobando desfechos abarcando e propiciando a constatação das resoluções mesmo que ocorram percalços englobados numa contingência e conjuntura estipulando cenários deflagrando trâmites subscritos na matriz com pauta alusiva englobando normalidade calcada em limiares advenientes contendo condenação com revés na porta de filtragem.

Exemplo:

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

Exemplo concernente no escrutínio afeto a averiguação alocada com vistoria englobando embalagem focada a provas atinentes face a expedientes dentro das premissas e trâmites na matriz do ambiente em CI:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## Licença

Este projeto é licenciado sob a BSD 2-Clause License. Consulte o arquivo [LICENSE](../LICENSE) para obter detalhes.

SPDX-License-Identifier: BSD-2-Clause
