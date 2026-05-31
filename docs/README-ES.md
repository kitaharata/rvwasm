# rvwasm

[![GitHub License](https://img.shields.io/github/license/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/kitaharata/rvwasm)](https://github.com/kitaharata/rvwasm/releases)

[English](../README.md) | [Español](README-ES.md) | [Français](README-FR.md) | [Português](README-PT.md) | [Deutsch](README-DE.md) | [Italiano](README-IT.md) | [简体中文](README-ZH-CN.md) | [繁體中文](README-ZH-TW.md) | [日本語](README-JA.md) | [한국어](README-KO.md)

## Resumen

Un emulador de RV64IMAC que se ejecuta en Go 1.23.2 `GOOS=js GOARCH=wasm`. El valor predeterminado es un solo hart, pero la programación cooperativa de 1 a 8 harts está disponible desde la interfaz de usuario (UI). Puede cargar OpenSBI 1.8.1 `fw_payload.bin`/`fw_jump.bin`/`fw_dynamic.bin`/ELF desde la UI del navegador para confirmar el arranque.

![OpenSBI fw_payload boot on rvwasm](images/fw_payload.png)

OpenSBI 1.8.1 `fw_payload.bin` arrancando en rvwasm y entrando en el payload de modo S de la siguiente etapa.

## Características implementadas

- Instrucciones base RV64I
- Extensión M
- Implementación mínima de LR/SC/AMO de la extensión A
- Instrucciones enteras comunes de la extensión C
- Equivalentes de Zicsr/Zifencei
- Implementación mínima del modo de privilegio M/S/U CSR/trap/mret/sret
  - Corrige la excepción síncrona `mepc`/`sepc` al PC de la instrucción que falla
  - Corrige la carga/escritura CSR que falla para no corromper rd y detiene el avance del contador de retiros
  - Comprobación de existencia de CSR, supresión de efectos secundarios de escritura en CSR de solo lectura, reflejo básico de `mcounteren`/`scounteren`
  - Stubs de CSR `senvcfg`/habilitación de estado añadidos para sondeos de Linux
  - Reflejo básico de `TVM`/`TW`/`TSR` y limpieza de `MPRV`
- MMU Sv39
  - Modo `satp` Bare/Sv39
  - Recorrido de tabla de páginas de 3 niveles
  - Hojas de 4 KiB/2 MiB/1 GiB
  - Reflejo básico de `SUM`/`MXR`/`MPRV`
  - Excepción de fallo de página (page fault exception)
  - Actualización automática de los bits `A`/`D` de PTE
- MMIO estilo UART 16550 (`0x10000000`)
  - Salida desde el huésped
  - Inyección de entrada desde la UI del navegador
  - Interrupción de recepción
- mtime/mtimecmp/msip estilo CLINT (`0x02000000`)
  - Enrutamiento MSIP/MTIMECMP por hart para múltiples harts
- Controlador de interrupciones estilo PLIC (`0x0c000000`)
  - priority/pending/enable/threshold
  - claim/complete
  - Contexto M/S por hart
- Cumplimiento de PMP
  - TOR/NA4/NAPOT
  - Permisos R/W/X
  - Restricciones de modo M mediante entradas bloqueadas
- Información de arranque de OpenSBI `fw_dynamic`
  - La información dinámica se ubica en `0x87dff000`
  - El puntero de información dinámica se establece en `a2`
  - El payload de modo S / kernel se puede cargar por separado desde la UI
- Dispositivo de bloque virtio-mmio (`0x10001000`)
  - Registros MMIO modernos estilo virtio 1.0
  - Soporte mínimo para read/write/flush/get-id de virtqueue dividida
  - Negociación de `FEATURES_OK` y verificación de `VIRTIO_F_VERSION_1`
  - Restablecimiento de cola, ignorar notify antes de `DRIVER_OK`, reflejo básico del flag `NO_INTERRUPT`
  - Manejo de `VIRTIO_RING_F_INDIRECT_DESC` y tablas de descriptores indirectos
  - Supresión de interrupciones mediante el evento usado `VIRTIO_RING_F_EVENT_IDX`
  - Las imágenes de disco se pueden cargar desde la UI
  - Las imágenes de disco modificadas por el huésped se pueden descargar desde la UI
- Dispositivo de consola virtio-mmio (`0x10002000`)
  - Consola mínima con ID de dispositivo 3
  - Cola 0 recepción / Cola 1 transmisión
  - Soporte mínimo para `VIRTIO_CONSOLE_F_SIZE`, descriptores indirectos e índices de eventos
  - Inyecta la entrada de la UI tanto a UART como a virtio-console
- Dispositivo de red virtio-mmio (`0x10003000`)
  - virtio-net de depuración mínima con ID de dispositivo 1
  - Cola 0 recepción / Cola 1 transmisión
  - Soporte mínimo para `VIRTIO_NET_F_MAC`/`VIRTIO_NET_F_STATUS` / descriptores indirectos / índices de eventos
  - Inyecta hexadecimal de tramas Ethernet en RX desde la UI
  - Muestra las tramas Ethernet enviadas por el huésped como registros de TX
- Dispositivo rng virtio-mmio (`0x10004000`)
  - Fuente de entropía mínima con ID de dispositivo 4
  - Soporte mínimo para virtqueues divididas, descriptores indirectos e índices de eventos
  - La semilla determinista se puede establecer desde la UI
- Dispositivo de entrada virtio-mmio (`0x10005000`)
  - Dispositivo mínimo de teclado/entrada de depuración con ID de dispositivo 18
  - Soporte mínimo para cola de eventos / cola de estado, descriptores indirectos e índices de eventos
  - Los eventos de teclas / eventos de entrada sin procesar se pueden inyectar desde la UI
- Dispositivo gpu virtio-mmio (`0x10006000`)
  - Base mínima de virtio-gpu 2D para depuración con ID de dispositivo 16
  - Soporte mínimo para colas de control / cursor, descriptores indirectos e índices de eventos
  - Respuestas básicas para `GET_DISPLAY_INFO`/`RESOURCE_CREATE_2D`/`SET_SCANOUT`/`FLUSH`, etc.
  - Útil para observar los sondeos de virtio-gpu de Linux y los comandos iniciales de modeset
- Paso de initrd/initramfs
  - Dirección de carga predeterminada: `0x84000000`
  - Reflejado en `/chosen/linux,initrd-start` / `/chosen/linux,initrd-end` del DTB generado automáticamente
- Edición de bootargs
  - Predeterminado: `console=ttyS0 earlycon=sbi root=/dev/vda rw`
  - Ajustes preestablecidos para UART / virtio-console / initramfs / depuración detallada
  - Se puede configurar desde la UI y reflejar en el DTB generado automáticamente
- Búfer circular de trazas de ejecución
  - PC/instrucciones/traps/última causa de trap/tval se pueden ver en la UI
  - Las exportaciones en Texto/JSON/CSV de volcados CSR y capturas de trazas de harts completos están disponibles desde la UI
  - Diagnósticos que muestran los últimos argumentos ECALL/SBI, contadores de SBI BASE/TIME/IPI/RFENCE/HSM/SRST/heredados, traps y estados de colas virtio de un vistazo
  - Exportación JSON de diagnósticos / estados de dispositivos
  - Carga símbolos ELF/System.map, muestra símbolos alrededor del PC detenido, búsqueda de nombres y resolución automática de símbolos PC dentro de los registros de panic/oops
  - Shim de SBI arbitrario para probar directamente pequeños payloads de modo S sin OpenSBI
    - Cortocircuito mínimo de BASE/TIME/IPI/RFENCE/HSM/SRST
    - Ruta de depuración al punto de entrada del modo S del hart de destino a través de HSM `hart_start`
    - Deshabilitado por defecto. No se usa en la ruta normal para ejecutar OpenSBI
  - Los rangos de memoria física arbitrarios se pueden volcar desde la UI
  - Los puntos de interrupción de PC, puntos de observación de lectura/escritura física y filtros de traza se pueden establecer desde la UI
  - Los puntos de interrupción pueden especificar recuentos de aciertos, condiciones de modo y condiciones de hart
  - La traza muestra mnemotécnicos de decodificación simplificados junto con instrucciones sin procesar
  - Los aciertos de puntos de interrupción/observación registran el motivo de la detención en las exportaciones de estado/diagnósticos/trazas
  - Recopila histogramas de acceso a MMIO/DRAM, permitiendo verificar sesgos en los sondeos de dispositivos y actividades de colas a través de Diagnósticos/JSON
  - Guarda líneas de tiempo de acceso a MMIO/DRAM en el búfer circular, permitiendo verificar la serie temporal de sondeos en vistas sin procesar/compactas
  - La línea de tiempo de acceso a MMIO agrega nombres de decodificadores de registros para virtio-mmio/UART/CLINT/PLIC, permitiendo la observación en unidades como `QueueNotify`/`Status`/`LSR`
  - Opcionalmente habilita la traza de acceso CSR para mostrar las colas de lectura/escritura CSR del huésped y los resúmenes de lectura/escritura por CSR en Diagnósticos / exportaciones de trazas
  - Opcionalmente habilita el perfil de puntos calientes del PC para ver los PC ejecutados frecuentemente con símbolos antes de detenerse
  - La captura/diferencia de instantáneas de diagnóstico permite verificar las diferencias en los estados de hart/dispositivos/CSR/MMIO antes y después de la ejecución en la UI
  - Pliega instrucciones, traps y registros ECALL idénticos y consecutivos en la vista de traza compacta
  - El corredor de humo (smoke runner) por preajuste de arranque puede ejecutar automáticamente un número especificado de pasos de hart del firmware/payload cargado actualmente y recuperar resultados JSON
  - El analizador de fase de arranque puede resumir actividades de OpenSBI / Linux / panic / virtio / traps / símbolos de PC de forma conjunta
  - La línea de tiempo de arranque puede mostrar marcadores de consola y sondeos MMIO / estados / QueueNotifies / reclamos PLIC integrados en una serie temporal
  - El analizador de sondeos de dispositivos puede agregar lecturas/escrituras, registros de identidad, negociaciones de estado y notificaciones de cola de virtio/UART/PLIC/CLINT
  - El inspector de virtqueue puede mostrar los últimos estados de QueueSel/QueueNum/Desc/Driver/Device/QueueReady/QueueNotify por dispositivo/cola
  - El visualizador de cadenas de descriptores rastrea los descriptores iniciales del anillo disponible y muestra descriptores NEXT/WRITE/INDIRECT junto con una pequeña vista previa del búfer
  - La exportación de gráficos de cadenas de descriptores puede guardar y visualizar cadenas de virtqueue como DOTs de Graphviz
  - El escáner de memoria física del huésped puede detectar áreas en la DRAM que se asemejen a firmas mágicas de ELF/FDT/gzip/xz/zstd/squashfs/cpio/ext / versión de OpenSBI/Linux / BusyBox / cmdline del kernel
  - El clasificador de sondeos de controladores / initcall puede categorizar las líneas de registro de la consola de Linux relacionadas con initcalls, sondeos, virtio, almacenamiento, consolas, redes y gráficos
  - La línea de tiempo de initcall puede mostrar las líneas clasificadas de initcall / sondeos de controladores en grupos de series temporales
  - Lee tablas de líneas DWARF de ELFs con símbolos, mostrando archivo:línea cerca del PC actual, resúmenes de archivos DWARF y anotaciones de símbolos+línea para PC de traza
  - El resumen de pánico extrae automáticamente las líneas alrededor de un panic/oops/fallo en el registro de la consola y resuelve las direcciones con los símbolos cargados
  - El JSON de análisis de arranque puede exportar colectivamente líneas de tiempo / sondeos de dispositivos / virtqueues / resúmenes de pánico
  - El informe de repetición de traza puede resumir el número de pasos/traps/ecalls/shims de SBI, mnemotécnicos frecuentes y causas de traps en la traza
  - La comparación de base de traza puede comparar las diferencias de PC/instrucción/trap entre una traza guardada previamente y la traza actual desde el principio
  - La base de traza se puede guardar/cargar en/desde el localStorage del navegador
  - El informe/JSON de regresión de arranque, así como las exportaciones de informes Markdown/HTML, pueden guardar en masa estadísticas de traza, eventos de arranque, sondeos de dispositivos, virtqueues, objetos de memoria y recuentos de initcall
  - La instantánea de virtqueue puede mostrar las configuraciones de colas y las cadenas de descriptores simultáneamente
  - El detector de anomalías de virtqueue puede detectar direcciones de colas listas faltantes, bucles de descriptores, longitudes indirectas inválidas, búferes fuera de DRAM, etc.
  - Las pistas de anomalías de virtqueue pueden mostrar sugerencias de reparación como QueueNum/QueueDesc/QueueReady/alineación de descriptores para cada resultado de detección
  - La consulta de diagnóstico integrada puede buscar de forma cruzada en consolas / trazas / trazas CSR / líneas de tiempo MMIO / anomalías de virtqueue / índices de memoria utilizando la misma consulta
  - Los preajustes de consultas de diagnóstico permiten la búsqueda por lotes de pánicos, negociaciones de virtio, QueueReady/Notifies, satp/mstatus, traps y rootfs
  - Compartir informe MD/JSON/HTML permite compartir regresiones de arranque, pistas/triage de virtqueue, índices de memoria, preajustes de consulta, pistas de salto y resultados de consulta en un formato autónomo
  - El panel de triage / clasificación de causas de detención puede mostrar pánicos, traps, fallos de página/acceso, anomalías de virtqueue y sondeos de dispositivos estancados en orden de candidatos
  - La evidencia de causas de detención muestra la justificación de la clasificación, desgloses de puntuación, consultas de diagnóstico recomendadas y próximos pasos
  - La base del panel de triage se puede guardar en localStorage para comparar recuentos de estado/fase/dispositivo/anomalía/memoria con el panel actual
  - La base de preajustes de diagnóstico se puede guardar en localStorage para comparar la diferencia con el recuento de aciertos del preajuste actual
  - El informe redactado compartido en MD/JSON/HTML puede generar informes compartibles con IPs/MACs/correos electrónicos redactados
  - Las opciones de redacción JSON permiten activar o desactivar el reemplazo de IPs/MACs/correos electrónicos/direcciones hexadecimales largas desde la UI
  - El volcado de objetos de memoria puede verificar hexadecimal + ASCII alrededor de coincidencias de índices de memoria/búsqueda
  - El volcado de rangos de memoria puede especificar una dirección DRAM arbitraria y una longitud de bytes para volcar hexadecimal + ASCII / exportar JSON
  - La captura/diferencia del escaneo de memoria puede verificar candidatos a fragmentos de ELF/FDT/initrd/rootfs que aumentaron/disminuyeron antes y después de la ejecución
  - El índice de memoria puede agrupar firmas cercanas de ELF/FDT/initrd/kernel/rootfs por rango para crear un índice
  - Extrae registros estilo `dmesg` de Linux de las salidas UART/virtio-console y resuelve direcciones de panic/oops con los símbolos cargados
- simple-framebuffer
  - Añade automáticamente `0x86000000`, 1024x768, `a8r8g8b8` a `/chosen/framebuffer@86000000` en el DTB generado
  - Dibuja el framebuffer en un Canvas de la UI, y se pueden descargar volcados RAW RGBA / PNGs
  - El respaldo de recursos 2D para virtio-gpu se puede copiar al simple-framebuffer tras `TRANSFER_TO_HOST_2D`/`RESOURCE_FLUSH`
- DRAM `0x80000000`, 128 MiB
- Generación automática de DTB virt mínimo con virtio-blk / virtio-console / virtio-net / virtio-rng / virtio-input / virtio-gpu / UART / PLIC / CLINT, o cargar DTB desde la UI
  - Compatible con `sifive,plic-1.0.0`/`sifive,clint0` y `dma-coherent` de virtio añadidos
  - Genera `cpu@N` e `interrupts-extended` según el recuento de harts

## Uso

```bash
make serve
```

Abra `http://localhost:8080` en su navegador, seleccione el firmware de OpenSBI, luego haga clic en `Load firmware` → `Run`.

Si desea probar virtio-console como la consola de Linux, puede cambiar los bootargs a algo como `console=hvc0 earlycon=sbi root=/dev/vda rw`. Por defecto, usa UART (`ttyS0`) como es habitual.

Para analizar un PC detenido, cargue un `System.map` de Linux o un ELF con símbolos usando `Load symbols`, luego use `Symbols @ PC` / `Diagnostics` / `Search symbols`. Si el ELF con símbolos contiene tablas de líneas DWARF, también puede verificar archivo:línea usando `DWARF lines @ PC`. `DWARF file summary` muestra el número de líneas por archivo contenido en la tabla de líneas. Si el firmware/payload es un ELF con símbolos, importa automáticamente la tabla de símbolos. `Annotated trace` anota `pc=` en la traza con símbolos/líneas DWARF. `Download trace` guarda una captura de la traza para todos los harts. También puede seleccionar los formatos JSON/CSV. La traza JSON incluye información de símbolos/código fuente si existen símbolos. Ingrese cadenas como `trap`, `ecall`, `sbi-shim`, `pc=`, o `virtio` en `Trace filter` para delimitar la cola/exportación de la traza, la línea de tiempo de acceso y la vista compacta. `Compact trace` pliega instrucciones, traps y ECALLs idénticos y consecutivos. Si pega un registro de panic/oops y hace clic en `Analyze log symbols`, resuelve las direcciones de estilo PC de 64 bits en el registro utilizando los símbolos cargados.

`Trace replay report` genera estadísticas para la traza actual, y `Trace baseline compare` pega una traza guardada para comparar las diferencias de PC/instrucción/trap con la traza actual desde el principio. `Save current trace as baseline` / `Load saved baseline` mantiene la base en el localStorage del navegador. `Boot regression` / `Boot regression JSON` / `Boot regression MD` / `Boot regression HTML` son informes de verificación de regresión que consolidan la línea de tiempo de arranque, los sondeos de dispositivos, las virtqueues, el escáner de memoria, la clasificación de initcall y las estadísticas de traza. También puede verificar el aumento o la disminución de candidatos a ELF/FDT/initrd/rootfs en la memoria del huésped haciendo `Capture memory scan` → Run → `Diff memory scan`. `DWARF source context` muestra símbolos y archivo:línea DWARF alrededor del PC actual en conjunto.

`Boot phase` resume el progreso actual de los registros de la consola, los histogramas MMIO, los traps y la información de símbolos. `Boot timeline` alinea los hitos de la consola y los sondeos MMIO / estados / QueueNotifies en una serie temporal. `Device probe` agrega accesos a registros y negociaciones de virtio y otros, y `Virtqueue inspect` muestra configuraciones de colas y estados de notificación por dispositivo/cola. `Descriptor chains` lee las cadenas de descriptores del anillo disponible de la cola y muestra los descriptores indirectos y vistas previas del búfer. `Descriptor DOT` / `Download DOT` genera la misma cadena como Graphviz DOT. `Virtqueue anomalies` detecta inconsistencias en las configuraciones de colas y cadenas de descriptores, y `Anomaly hints` muestra el siguiente punto de control para cada inconsistencia. `Integrated diagnostic query` busca de forma cruzada en consolas / trazas / trazas CSR / líneas de tiempo MMIO / anomalías de virtqueue / índices de memoria utilizando palabras como `virtio QueueReady`, `panic`, `satp`, `0x80200000`. `Share report MD/JSON/HTML` es un paquete compartible que añade pistas de anomalías/triage, índices de memoria, pistas de saltos de memoria, preajustes de consulta y aciertos de consulta al informe de regresión de arranque. El HTML se puede guardar como un archivo autónomo con JSON incrustado. `Diagnostic query presets` agrupa búsquedas relacionadas con pánicos, estados de virtio, QueueReady/QueueNotify, satp/mstatus, traps y rootfs. `Save query` / `Load query` guarda las consultas de diagnóstico en el localStorage del navegador. `Memory scan` busca candidatos a fragmentos de ELF/FDT/initrd/kernel/rootfs en la DRAM, e `Memory index` agrupa firmas cercanas por rango. `Memory search` busca en índices de memoria utilizando cadenas o direcciones `0x...`, y `Memory jumps` muestra candidatos útiles para el destino del salto, como ELF/FDT/Linux/OpenSBI/cmdline/rootfs. `Initcall classifier` / `Initcall timeline` clasifica y asigna marcas de tiempo a registros estilo initcall/sondeo de controlador de Linux. `Panic summary` extrae líneas alrededor de pánico/oops/fallos y resuelve direcciones si hay símbolos presentes. `Boot analysis JSON` guarda estos en conjunto. `Dmesg extract` extrae solo las líneas de estilo Linux de las salidas UART / virtio-console. `Decoded MMIO` muestra los últimos accesos MMIO con nombres de registros.

`Triage dashboard` combina clasificaciones de causas de detención, gravedad de anomalías de virtqueue, sondeos de dispositivos y marcadores de consulta en un solo texto en pantalla. `Stop-cause ranking` prioriza los kernel panics, oops, instrucciones ilegales, fallos de página/acceso, anormalidades de virtqueue y sondeos de dispositivos estancados de consolas/trazas/estados. `Stop-cause evidence` muestra la justificación de la clasificación, el desglose de la puntuación, las consultas recomendadas y los próximos puntos de control. Puede comparar diferencias en los recuentos de estado/fase/dispositivo/anomalía/memoria en el panel haciendo `Save triage baseline` → Run → `Triage diff`. `Save preset baseline` → Run → `Compare preset baseline` le permite verificar si el recuento de aciertos de consultas preestablecidas como panic/virtio/satp/rootfs ha aumentado o disminuido desde la última vez. `Memory dump hits` vuelca en hexadecimal/ASCII alrededor de aciertos del índice de memoria usando consultas de diagnóstico o filtros de traza. `Memory range dump` especifica una dirección/longitud arbitraria para volcar directamente la DRAM en hexadecimal/ASCII. `Redacted share MD/JSON/HTML` reemplaza correos electrónicos / MACs / IPv4s con `<email>` / `<mac>` / `<ipv4>` antes de compartir. `Redaction options JSON` activa o desactiva `replace_ips` / `replace_macs` / `replace_emails` / `replace_long_hex`.

`Smoke preset` restablece el preajuste de arranque seleccionado actualmente y ejecuta solo los pasos especificados. `Smoke matrix` ejecuta secuencialmente una lista de preajustes como `uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb` y enumera los pasos de ejecución, la última fase y los candidatos a causas de detención para cada preajuste.

Los puntos de interrupción del PC se añaden ingresando el valor hexadecimal de un PC físico/virtual en el campo `PC breakpoint` bajo `Breakpoints / watchpoints`. `Run` / `Step 1k` se detiene en un punto de interrupción, y `Step` avanza exactamente 1 instrucción incluso si el PC actual es un punto de interrupción. Los puntos de observación de escritura detectan escrituras del bus a un rango de direcciones físicas, y los puntos de observación de lectura son una función sencilla para detectar lecturas del bus. Son útiles para verificar sondeos MMIO, escrituras de framebuffer y referencias a estructuras específicas. `Access timeline` / `Compact access` muestra accesos recientes a DRAM/MMIO en una serie temporal comprimida. `PC profile on` agrega PCs frecuentes, y `Capture snapshot` → Run → `Diff snapshot` permite verificar las diferencias de diagnóstico antes y después de la ejecución.

simple-framebuffer prepara una memoria de 1024x768x32bpp en `0x86000000` y la coloca como `simple-framebuffer` en `/chosen/framebuffer@86000000` del DTB generado automáticamente. Si simplefb es utilizable en el lado de Linux, se puede mostrar en el Canvas con `Render framebuffer`.

virtio-net no se conecta a una red real desde el navegador; es un dispositivo de depuración a nivel de paquete. Ingrese el hexadecimal de la trama Ethernet en `virtio-net debug` para inyectarlo en RX, y las tramas enviadas por el huésped a la cola de TX se pueden verificar en `Show TX frames`. Para que sea reconocido en el lado de Linux, ejecute comandos como `ip link set dev eth0 up` en el lado del huésped según sea necesario.

virtio-rng es un dispositivo de verificación que presenta un PRNG determinista como una fuente de entropía del huésped. Para mantener la reproducibilidad, la semilla predeterminada es fija y se puede cambiar a través de `Set deterministic seed` en la UI.

virtio-gpu es un dispositivo mínimo para observar los sondeos del controlador virtio-gpu de Linux y la configuración de recursos 2D. En lugar de una aceleración de GPU real, rastrea los comandos modeset / scanout / flush que llegan en la cola de control y genera los estados a Diagnósticos. Debido a que también copia desde la memoria de respaldo de recursos al simple-framebuffer, puede verificar el resultado del huésped volcando un recurso 2D a través de `Render framebuffer` / exportación PNG. `UPDATE_CURSOR` / `MOVE_CURSOR` en la cola del cursor también se registran como estados.

`SBI shim on` es para depurar payloads de modo S directamente sin OpenSBI. Manténgalo deshabilitado para experimentos normales usando `fw_dynamic.bin` / `fw_payload.bin`.

Si desea probar Multi-hart, configure el `Hart count` antes de cargar el firmware. Dado que cambiar la configuración implica un reinicio de la máquina, asume que recargará el firmware / payload / disco después. `View hart` permite cambiar los registros / CSRs / trazas del hart de destino que se muestra.

Cuando use `fw_dynamic.bin`, cargue el payload de modo S / kernel alrededor de `0x80200000` a través de `Load payload` según sea necesario. El emulador coloca información dinámica en `0x87dff000` y establece su dirección en `a2`.

Para experimentos con Linux, puede utilizar cualquiera de los siguientes:

- `Load disk`: Pase una imagen de disco sin procesar como rootfs como virtio-blk. Los bootargs predeterminados son `root=/dev/vda rw`.
- `Load initrd`: Coloque initramfs en `0x84000000` y refleje el rango initrd en el DTB generado. Cambie los bootargs a `console=ttyS0 earlycon=sbi root=/dev/ram0 rw`, etc., si es necesario.

Ejemplo utilizando binarios RISC-V predistribuidos de OpenSBI 1.8.1:

```bash
curl -LO https://github.com/riscv-software-src/opensbi/releases/download/v1.8.1/opensbi-1.8.1-rv-bin.tar.xz
tar -xf opensbi-1.8.1-rv-bin.tar.xz
# Cargue fw_dynamic.bin / fw_payload.bin / fw_jump.bin desde los archivos extraídos a través del navegador
```

Si compila OpenSBI localmente, prepare una cadena de herramientas RISC-V como `riscv64-unknown-elf-` y compile con `PLATFORM=generic`.

### Comandos de desarrollo

```bash
go test ./...
make wasm
make serve
```

## Nota

Esta implementación incorpora gradualmente las funciones necesarias para investigar la inicialización de OpenSBI, la transición de payload en modo S y el arranque de Linux. Para el arranque de Linux, se han añadido PMP, Sv39, virtio-blk, virtio-console, virtio-net, virtio-rng, virtio-input, virtio-gpu, initrd, simple-framebuffer, precisión de trap/CSR, traza/resumen de CSR, histograma/línea de tiempo MMIO/decodificadores de registros, analizador de fase/línea de tiempo de arranque, analizador de sondeo de dispositivos, inspector de virtqueue/visualizador de cadena de descriptores/exportación DOT/instantánea/detector de anomalías/pistas de anomalías/triage de anomalías, escáner de memoria del huésped/índice/diferencia/búsqueda/pistas de salto/ayudantes de volcado, consulta de diagnóstico integrada/preajustes de consulta/comparación de base de preajustes, panel de triage/clasificación de causas de detención, paquete de informe compartido/HTML/redacción, clasificador/línea de tiempo de initcall, búsqueda de líneas DWARF/contexto fuente/anotación de traza, resumen de pánico, extractor de dmesg, repetición/comparación de trazas, informes de regresión de arranque, perfilado de PC, diferencia de instantáneas, corredor de humo de arranque/matriz de humo, diferencia de base de triage, evidencia de causa de detención, redacción editable y volcado de rango de memoria. Las partes principales no implementadas o simplificadas incluyen un modelo preciso de ciclo/tiempo, AIA/IMSIC, conexión de red real a través de puentes tap/WebSocket, aceleración completa virgl/DRM/GPU, comportamientos estrictos WARL/WPRI para todos los CSR y verdadera ejecución paralela utilizando múltiples workers. Multi-hart es una programación cooperativa dentro de un solo worker wasm.

## Diagnósticos y ayudas de regresión

Mejora de la capacidad de uso compartido para matrices de humo y consultas de diagnóstico.

- `Smoke matrix MD/HTML` guarda los resultados de la matriz de humo como Markdown / HTML autónomo.
- `Save smoke baseline` → Run → `Compare smoke baseline` le permite verificar las diferencias en la fase, los pasos de ejecución y las causas principales de detención para cada preajuste.
- `Stop checklist` crea una lista de verificación de elementos de acción específicos para revisar a continuación en función de la clasificación de causas de detención.
- `CSR/MMIO bookmarks` extrae solo los aciertos cruciales de CSR / MMIO / trazas de los resultados de la consulta de diagnóstico integrada.
- `Watchpoint hits` muestra el historial de aciertos de puntos de observación de lectura/escritura en una serie temporal. `Clear hit timeline` borra solo el historial.
- `Artifact manifest` enumera el firmware / payload / disco / initrd / símbolos cargados actualmente y los rangos de información dinámica / DTB generados, entradas y hashes SHA-256.

### Ayudas de traspaso de regresión

- `Manifest diff` / `Manifest diff JSON` compara el manifiesto actual de artefactos de arranque con una base guardada en localStorage y muestra diferencias en bootargs, recuentos de harts, rangos de carga, entradas, detección de ELF y hashes SHA-256.
- `Auto break/watch suggestions` genera candidatos para puntos de interrupción de PC / puntos de observación de lectura / puntos de observación de escritura para establecer en la próxima ejecución según la evidencia de causas de detención, PC de trazas recientes y líneas de tiempo de aciertos de puntos de observación.
- `Smoke clusters` / `Smoke clusters JSON` agrupa los resultados de los preajustes de la matriz de humo por fase y causa principal de detención, agrupando preajustes con el mismo tipo de fallo.
- `Diagnostic bundle JSON` es un JSON autónomo que agrupa el manifiesto, el panel de triage, las causas de detención, las sugerencias de puntos de interrupción, el paquete compartido y los aciertos de puntos de observación.
- `Compressed bundle JSON` es el paquete de diagnóstico anterior convertido a gzip+base64. Úselo cuando desee reducir el tamaño antes de pegarlo en incidencias o chats.

### Ayudas de traspaso / procedencia

- `Decode bundle` extrae un `Diagnostic bundle JSON` o `Compressed bundle JSON` pegado, o gzip+base64 sin procesar.
- `Bundle compare` / `Bundle compare JSON` compara un paquete pasado pegado con el paquete actual y muestra diferencias en las fases de triage, las causas principales de detención, los manifiestos, los hashes de artefactos, los clústeres de humo, los aciertos de puntos de observación y los recuentos de sugerencias.
- `Provenance` / `Provenance JSON` resume los hashes SHA-256 del manifiesto, la traza, la consola y el paquete de diagnóstico, los recuentos de líneas de traza, los recuentos de bytes de consola y las causas principales de detención. Puede usarse para verificar la reproducibilidad o como evidencia adjunta a incidencias.
- `Handoff MD` resume la procedencia, las causas principales de detención, las sugerencias automáticas de interrupción/observación, las listas de verificación de detención, las diferencias de base y los manifiestos de artefactos en Markdown.
- `Apply auto breaks` aplica en masa los principales candidatos de sugerencias automáticas de interrupción/observación al emulador actual. Una utilidad para configurar rápidamente posiciones de detención o rangos sospechosos de MMIO/DRAM antes de volver a ejecutar.

### Reproducción / Firma / Traspaso Headless

- `Repro plan` / `Repro MD` / `Repro JSON`
  - Genera pasos de reproducción a partir de paquetes de diagnóstico, procedencia y manifiestos de artefactos.
  - Enumera los roles, tamaños, rangos de carga y hashes SHA-256 de firmware / payload / initrd / disco / símbolos como anclajes de artefactos.
  - Documenta los preajustes de humo, bootargs, recuentos de harts, next_addr y condiciones recomendadas de interrupción/observación en pasos.
- `Log signature` / `Log signature JSON`
  - Crea un resumen ligero a partir de los hashes SHA-256 de trazas / consolas / manifiestos, recuentos de líneas de traza, primeros/últimos PC, primeras/últimas líneas de consola y tokens frecuentes.
  - Le permite comparar "¿Es este el mismo registro?" o "¿Qué cambió?" sin pegar trazas sin procesar.
- `Save log signature` / `Load log signature` / `Compare log signature`
  - Guarda la base de la firma del registro en el localStorage del navegador y la compara con la firma actual.
  - Muestra diferencias en hashes de traza, hashes de consola, hashes de manifiesto, recuentos de líneas, últimos PC y últimas líneas de consola.
- `Auto break verify`
  - Muestra un resumen de confirmación antes de aplicar sugerencias automáticas de puntos de interrupción/observación.
  - Emite advertencias para sugerencias duplicadas o rangos de PC sospechosos.
- `Headless smoke script`
  - Genera un esqueleto de script de shell para CI/traspaso a partir del manifiesto de artefactos actual, bootargs, recuentos de harts, preajustes de humo y recuentos de pasos.
  - Destinado a fijar anclajes de artefactos y matrices de preajustes antes de agregar arneses de navegador como Playwright al entorno de ejecución.

#### Ayudas Headless / CI

Para facilitar el manejo del traspaso de reproducción/firma en CI o incidencias, se ha agregado lo siguiente.

- `Bundle integrity` / `Integrity JSON` comprueba la consistencia entre el paquete de diagnóstico y el manifiesto de artefactos, categorizando las discrepancias en roles de artefactos, hashes SHA-256, rangos de carga, sugerencias y resultados de humo como `error`, `warn` o `info`.
- `Repro validation` / `Repro validation JSON` verifica si el plan de reproducción actual coincide con los bootargs, recuentos de harts, next_addr, anclajes de artefactos, causas principales de detención y firmas de registro del paquete.
- `CI summary` / `CI summary JSON` consolida la integridad del paquete, las firmas de traza/consola, los resultados de humo y las causas de detención, generando un resumen que facilita los juicios de pass/warn/fail en CI.
- `Headless runner spec` / `Runner spec JSON` genera preajustes, pasos, anclajes de artefactos y comandos recomendados para la inspección con `go run ./cmd/rvsmoke ...`.
- Añadido `cmd/rvsmoke`. Puede leer paquetes de diagnóstico / manifiestos de artefactos fuera del navegador y generar hashes de artefactos, integridad del paquete, resúmenes de CI y especificaciones del corredor en texto / JSON / Markdown.

Ejemplo:

```bash
go run ./cmd/rvsmoke \
  -bundle rvwasm-diagnostic-bundle.json \
  -trace trace.txt \
  -console console.txt \
  -artifact firmware=fw_dynamic.bin \
  -artifact payload=Image \
  -out md > rvwasm-ci-summary.md
```

`rvsmoke` actualmente realiza inspecciones de reproducibilidad y generaciones de resúmenes de CI para paquetes/manifiestos y hashes de artefactos. La ejecución misma de la CPU continuará utilizando la matriz de humo en el lado js/wasm del navegador.

#### rvsmoke CI Gate / JUnit / SARIF

`cmd/rvsmoke` es una CLI auxiliar para inspeccionar paquetes de diagnóstico / manifiestos exportados en CI. Al materializar la ejecución headless, puede generar comparaciones de paquetes base, políticas de puertas de CI, XML JUnit, SARIF e informes HTML autónomos.

Ejemplo:

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

Ejemplo de JSON de política:

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

`-out html` imprime HTML autónomo en stdout, `-out junit` para XML JUnit y `-out sarif` para JSON SARIF. Si se especifican simultáneamente `-junit` / `-html` / `-sarif`, se guardan en sus respectivos archivos además del formato stdout. Las puertas de CI normalizan manifiestos de artefactos, firmas de traza/consola, diferencias de base, anomalías de virtqueue y resultados de humo en `pass`, `warn` o `fail`.

#### Plantillas de políticas rvsmoke / Comparación de tendencias de paquetes

Para facilitar la introducción inicial de puertas de CI y múltiples comparaciones de regresión, se han agregado a `rvsmoke` y a la UI del navegador plantillas de políticas, listas de verificación de acciones y comparaciones de tendencias de paquetes.

- `CI policy templates` / `Policy templates JSON` muestra las políticas integradas: `default`, `strict`, `linux-boot`, `artifact-only` y `lenient`.
- `Policy template JSON` guarda la plantilla especificada como un JSON listo para colocar en CI.
- `CI gate` / `CI gate JSON` aplica una plantilla de política al estado actual del navegador y muestra las comprobaciones de puerta pass/warn/fail.
- `CI checklist` / `CI checklist JSON` transforma los fallos de puerta, la integridad del paquete y las diferencias de artefactos en listas de verificación procesables.
- `rvsmoke -compare name=bundle.json` alinea múltiples paquetes cronológicamente y genera informes de tendencias que muestran cambios en fases, causas principales de detención, hashes de artefactos y clústeres de humo.

Ejemplo de generación de plantillas de políticas:

```bash
go run ./cmd/rvsmoke -list-policies

go run ./cmd/rvsmoke -print-policy linux-boot > rvwasm-ci-policy.json
```

Ejemplo de comparación de múltiples paquetes:

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -compare nightly=nightly-bundle.json \
  -policy-template linux-boot \
  -out md > rvwasm-trend.md
```

`-policy-template` sirve como política predeterminada cuando no se especifica `-policy`. Si se especifica `-policy`, el JSON del archivo tiene prioridad.

## Integración CI rvsmoke

Ampliación de ayudas de CI/traspaso para `rvsmoke`.

- `rvsmoke -print-github-actions linux-boot` puede generar YAML de flujo de trabajo de GitHub Actions.
- `rvsmoke -github-actions .github/workflows/rvwasm-smoke.yml` puede generar flujos de trabajo en archivos.
- `rvsmoke -policy-tree policy-tree.md` puede guardar puertas de CI / integridad de paquetes / derivas de base como árboles de causas.
- `rvsmoke -history history.txt` puede guardar agregaciones de derivas de fases / causas de detención / artefactos de múltiples tendencias de paquetes.
- `rvsmoke -repro-zip rvwasm-minimal-repro.zip` puede generar paquetes de reproducción mínimos que contengan README, paquetes de diagnóstico, manifiestos, especificaciones del corredor, políticas, resúmenes de CI y scripts de verificación.

Ejemplo:

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

`-repro-zip` no incrusta firmware/kernels/discos sin procesar. Incrusta pines SHA-256 y rangos de manifiesto en el paquete, esperando que el destinatario verifique los artefactos.

### Inspección del ZIP de reproducción de CI / Continuación del flujo de trabajo de matriz

Se agregaron funciones a `rvsmoke` y a la UI del navegador para inspeccionar el traspaso de paquetes de reproducción mínimos, y salidas para matrices de GitHub Actions / visualización de tendencias.

- `rvsmoke -inspect-repro-zip rvwasm-minimal-repro.zip` puede inspeccionar el ZIP generado por `-repro-zip` sin extraerlo. Verifica archivos requeridos, rutas inseguras, coincidencias de `diagnostic-bundle.json` / `manifest.json`, `ci-policy.json` y `scripts/rvsmoke.sh`.
- `rvsmoke -print-github-actions-matrix linux-boot -presets uart-blk,simplefb` puede generar YAML de flujo de trabajo de matriz de GitHub Actions por preajuste.
- `rvsmoke -github-actions-matrix rvwasm-smoke-matrix.yml` puede generar flujos de trabajo de matriz en archivos.
- `rvsmoke -trend-csv rvwasm-trend.csv` y `-trend-chart-json rvwasm-trend-chart.json` pueden guardar tendencias de paquetes en CSV / JSON para graficarlas fácilmente de forma externa.
- Añadidos `Minimal repro ZIP`, `Inspect repro ZIP`, `Repro ZIP JSON`, `Matrix workflow YAML`, `Trend chart JSON` y `Trend CSV` a la UI del navegador.

Ejemplo:

```bash
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip

# Guarda los resultados de la inspección como JSON
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -out json > rvwasm-repro-zip-inspection.json

# Convierte las tendencias del paquete actual y el paquete anterior a CSV/JSON
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -compare previous=previous-bundle.json \
  -policy-template linux-boot \
  -trend-csv rvwasm-trend.csv \
  -trend-chart-json rvwasm-trend-chart.json \
  -github-actions-matrix rvwasm-smoke-matrix.yml \
  -out md > rvwasm-ci-summary.md
```

`-inspect-repro-zip` puede ejecutarse de forma independiente. Si se especifica junto con `-bundle`, incluye los resultados de la inspección del ZIP en el resumen de CI normal y trata los fallos como fallos del resumen de CI.

### Agregación de matriz de CI / Continuación del manifiesto de sumas de comprobación

Mejora de los traspasos de artefactos de CI para `rvsmoke`.

- `-repro-checksums rvwasm-repro-checksums.json` puede guardar manifiestos deterministas de sumas de comprobación para los archivos dentro del ZIP basándose en los resultados de `-inspect-repro-zip`.
- Al especificar varios `-matrix-result name=rvsmoke-output.json`, puede agregar resultados de `rvsmoke -out json` de múltiples preajustes / múltiples trabajos.
- `-matrix-summary` / `-matrix-summary-json` / `-matrix-summary-html` puede guardar resultados de matriz como texto / JSON / HTML autónomo.
- `-trend-html rvwasm-trend.html` puede guardar informes de tendencias de paquetes como HTML independientes.

Ejemplo:

```bash
# Guarda el contenido del ZIP de reproducción mínima y el manifiesto de sumas de comprobación
go run ./cmd/rvsmoke \
  -inspect-repro-zip rvwasm-minimal-repro.zip \
  -repro-checksums rvwasm-repro-checksums.json

# Agrega JSON de rvsmoke de múltiples trabajos de matriz
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

Los agregados de matriz resumen los estados de CI, los recuentos de fallos/advertencias de puertas, los desajustes de artefactos y las causas principales de detención por trabajo. Es una utilidad para ver fácilmente las tendencias generales de fallos en el trabajo de agregación final, incluso cuando los trabajos de matriz de GitHub Actions están divididos.

#### Ayudas para traspaso de CI / Lanzamiento

Mejora de la gestión de artefactos de CI y los traspasos de lanzamiento para `rvsmoke`.

- `-artifact-index rvwasm-artifacts.json` resume las rutas, bytes y hashes SHA-256 de artefactos de CI generados, como JUnit / SARIF / HTML / tendencias / matrices / sumas de comprobación de reproducción.
- `-release-manifest rvwasm-release.json` empaqueta paquetes de diagnóstico, firmas de registros, puertas de CI, agregados de matriz, informes de inestabilidad, índices de artefactos y verificaciones de sumas de comprobación de reproducción en un solo manifiesto de traspaso.
- `-release-html rvwasm-release.html` genera un HTML autónomo con navegación a Summary / Artifacts / Matrix / Checksums / JSON.
- `-verify-repro-checksums baseline-repro-checksums.json` compara el manifiesto de sumas de comprobación del ZIP de reproducción mínima inspeccionado actualmente con una base para detectar entradas faltantes / modificadas / adicionales.
- `-matrix-flakes`, `-matrix-flakes-json` y `-matrix-flakes-html` normalizan múltiples resultados de matriz como `uart#1` / `uart#2` para detectar si el mismo preajuste presenta inestabilidad (flake) entre pass/fail.

Ejemplo:

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

## Traspaso y verificación de lanzamiento

Se han añadido salidas de metadatos a `rvsmoke` para traspasar los resultados de CI a otras máquinas, otros repositorios o revisores.

### Extensión de SBOM / Procedencia

#### Inventario de dependencias SBOM-lite

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -sbom rvwasm-sbom.json \
  -sbom-text rvwasm-sbom.txt
```

Esta lista de dependencias está pensada para tener un formato pequeño y determinista. Lee `go.mod` y registra las rutas de los módulos, versiones de Go, líneas directas de `require`, objetivos de `replace` y tipos de artefactos incluidos en el índice de artefactos de CI.

Al ejecutar `rvsmoke` desde otro directorio de trabajo, especifique `-go-mod /path/to/go.mod`.

#### Certificación de procedencia

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -artifact-index rvwasm-artifacts.json \
  -attestation rvwasm-attestation.json \
  -attestation-text rvwasm-attestation.txt
```

La certificación es un payload JSON inspirado en in-toto / SLSA. No es una firma en sí misma, pero debido a que tiene un SHA-256 estable, puede usarse como objetivo para que herramientas externas de CI la firmen.

#### ZIP de traspaso de lanzamiento

```bash
go run ./cmd/rvsmoke \
  -bundle current-bundle.json \
  -release-manifest rvwasm-release.json \
  -artifact-index rvwasm-artifacts.json \
  -sbom rvwasm-sbom.json \
  -attestation rvwasm-attestation.json \
  -release-zip rvwasm-release-handoff.zip
```

El ZIP de traspaso de lanzamiento incluye solo metadatos.

- `README.md`
- `release-manifest.json`
- `ci-artifact-index.json`
- `dependency-inventory.json`
- `provenance-attestation.json`
- `release.html`

No incrusta firmware, kernels, initrds ni imágenes de disco. Los artefactos grandes se mantienen como pines SHA-256 en el manifiesto.

#### Inspección del ZIP de traspaso de lanzamiento

```bash
go run ./cmd/rvsmoke \
  -inspect-release-zip rvwasm-release-handoff.zip \
  -release-zip-inspect-html rvwasm-release-handoff-inspect.html \
  -out json > rvwasm-release-handoff-inspect.json
```

El inspector revisa el ZIP sin extraerlo en busca de archivos requeridos, rutas peligrosas, rutas duplicadas, capacidad de análisis de JSON y consistencia básica entre lanzamientos / índices / SBOMs / certificaciones.

### Verificación de lanzamiento

Además de crear ZIP de traspaso de lanzamiento, se han agregado salidas orientadas a la verificación.

- `-verify-attestation` / `-verify-attestation-text` confirman si el hash determinista de la certificación de procedencia, los materiales del lanzamiento y los sujetos del artefacto de CI coinciden con el manifiesto de lanzamiento generado, el inventario SBOM-lite y el índice de artefactos.
- `-sbom-baseline`, `-sbom-diff` y `-sbom-diff-json` comparan el inventario de dependencias SBOM-lite actual con una base guardada.
- `-compare-release-zip-inspection`, `-release-zip-compare` y `-release-zip-compare-json` comparan el ZIP de traspaso de lanzamiento inspeccionado actualmente con JSON de inspecciones pasadas.
- `-retention-manifest` / `-retention-text` generan un manifiesto de retención de artefactos de CI que contiene rutas, tipos, bytes, SHA-256, días de retención, tiempos de expiración y motivos.
- `-release-verification-html` genera un HTML con navegación que resume los estados de lanzamiento, verificaciones de certificación, diferencias de SBOM, comparaciones del ZIP de lanzamiento e información de retención.

Ejemplo:

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

### Puerta de auditoría de lanzamiento

Se ha agregado una capa final de auditoría de lanzamiento sobre la verificación de lanzamiento. Resume las verificaciones de certificación de procedencia, diferencias de SBOM-lite, comparaciones de ZIP de lanzamiento, caducidades de retención de artefactos, estados de inestabilidad de la matriz y estados de manifiesto de lanzamiento en una sola puntuación y un informe de puerta.

Indicadores principales:

- `-list-release-verify-policies` enumera las políticas integradas de auditoría de lanzamiento.
- `-print-release-verify-policy strict` genera una plantilla JSON de política.
- `-release-verify-template default|strict|lenient|archive` selecciona una política integrada.
- `-release-verify-policy policy.json` carga una política personalizada de auditoría de lanzamiento.
- `-retention-audit` / `-retention-audit-json` escribe los resultados de la inspección de caducidad y retención mínima.
- `-release-score` / `-release-score-json` escribe una puntuación de verificación de lanzamiento de 0 a 100.
- `-release-gate` / `-release-gate-json` escribe los resultados de la puerta de la política.
- `-release-audit` / `-release-audit-json` / `-release-audit-html` escribe un informe de auditoría integrado.

Ejemplo:

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

La política estricta trata como fallos los manifiestos de lanzamiento que no pasan, las comprobaciones fallidas de certificación / SBOM / ZIP, los artefactos caducados y los artefactos que están por debajo de los días mínimos de retención establecidos. La política predeterminada es adecuada para verificaciones diarias como traspasos nocturnos, permitiendo advertencias pero fallando la CI por fallos claros de verificación.

#### Diferencia de auditoría de lanzamiento / Exenciones / Traspaso TODO

La ruta de auditoría de lanzamiento de `rvsmoke` admite comparar la auditoría actual con una auditoría pasada, aplicar exenciones por tiempo limitado a problemas conocidos y generar listas de verificación para tareas sin exención.

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

Puede crear una plantilla de exención con el siguiente comando:

```bash
go run ./cmd/rvsmoke -print-release-waiver-template > release-waivers.json
```

Las exenciones se utilizan para manejar hallazgos temporales y conocidos de la auditoría de lanzamiento. Cada regla tiene un ID, un tipo arbitrario / nombre / estado / comparadores de subcadenas, un propietario, un motivo y una marca de tiempo `expires_at`. Las exenciones caducadas se reportan pero no se utilizan para suprimir problemas.

#### Decisión de lanzamiento / Paquete de evidencia

Se agregaron ayudas finales de traspaso para usarse después de ejecutar auditorías de lanzamiento.

- `-waiver-calendar`, `-waiver-calendar-json` y `-waiver-calendar-html` muestran la caducidad, el propietario, los recuentos de coincidencias y los estados caducados / por caducar pronto para cada exención.
- `-release-changelog` y `-release-changelog-json` resumen las diferencias de auditoría, los estados de exención, los recuentos de tareas pendientes y los estados de caducidad de las exenciones como registros de cambios legibles por humanos.
- `-final-decision` y `-final-decision-json` generan decisiones finales de `go`, `go-with-watch` y `no-go` que contienen elementos bloqueantes y próximas acciones.
- `-release-evidence-zip` escribe un pequeño paquete de evidencia que contiene auditorías, informes de exenciones, listas de tareas pendientes, calendarios de exenciones, registros de cambios y decisiones finales.
- `-inspect-release-evidence-zip` inspecciona los paquetes de evidencia sin extraerlos en busca de archivos requeridos, rutas peligrosas, entradas duplicadas y capacidad de análisis JSON.
- `-dry-run` calcula los informes sin escribir archivos de salida opcionales.
- `-exit-code-mode never` muestra los resultados incluso en los casos en que normalmente fallaría con un fallo de puerta.

Ejemplo:

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

Ejemplo de inspección de un paquete de evidencia en CI:

```bash
go run ./cmd/rvsmoke \
  -inspect-release-evidence-zip rvwasm-release-evidence.zip \
  -release-evidence-inspect-json rvwasm-release-evidence-inspect.json \
  -out text
```

## Licencia

Este proyecto está licenciado bajo la BSD 2-Clause License. Consulta el archivo [LICENSE](../LICENSE) para más detalles.

SPDX-License-Identifier: BSD-2-Clause
