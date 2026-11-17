# Diseño: actualizaciones dentro de Música Nativa

## Objetivo

Permitir que la persona busque y aplique una actualización desde Música Nativa, sin descargar y abrir manualmente un nuevo instalador.

## Enfoques considerados

- **Volver a ejecutar el instalador:** sencillo, pero no cumple la experiencia de un botón dentro de la app.
- **Actualizador de Wails con GitHub Releases:** elegido. Reutiliza el proveedor GitHub, la verificación por SHA-256 y el reemplazo/reinicio asistidos por Wails.
- **Actualizador propio:** descartado porque duplicaría la descarga, verificación y sustitución del ejecutable que ya resuelve Wails.

## Experiencia

En Ajustes aparecerá una sección «Actualizaciones» con la versión activa y un botón «Actualizar». Al pulsarlo, la app buscará y descargará automáticamente la versión nueva; el estado y el progreso se mostrarán en esa misma pantalla en español. Al terminar, aparecerá «Reiniciar y actualizar». La app no buscará automáticamente al arrancar.

## Publicación

La fuente será el repositorio público `sergioc0702/musica-nativa`. Cada release incluirá el ejecutable autónomo `musica-nativa-windows-amd64.exe` y `SHA256SUMS`. El instalador por usuario seguirá siendo el medio de primera instalación; el actualizador reemplaza solamente el ejecutable instalado y conserva la biblioteca local. La interfaz usa `WindowNone` de Wails y consume sus eventos de actualización desde React, para mantener la experiencia dentro de Ajustes.

El repositorio aún no existe. Hasta que se cree y contenga al menos un release, el botón podrá informar el error del canal; no se publicará código ni binarios desde esta implementación local.

## Límites

- Windows x64 en esta primera etapa.
- Se requiere reiniciar la app para completar el reemplazo del ejecutable.
- El checksum protege contra descargas corruptas, pero no sustituye la firma Authenticode ni una firma criptográfica de release.
- El canal de actualización es manual: la persona decide cuándo buscar y reiniciar.
