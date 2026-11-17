# Música Nativa

Reproductor de escritorio para escuchar música guardada en tu PC. Incluye biblioteca local, playlists, favoritos, cola, salida WASAPI y temas visuales personalizables. No necesita una cuenta ni conexión a un servicio de streaming.

## Funciones

- Lee archivos **MP3, WAV y FLAC** de las carpetas que elijas; no copia ni modifica las canciones.
- Lee etiquetas y portadas cuando están disponibles y muestra valores de reemplazo cuando faltan.
- Organiza canciones, álbumes y artistas; permite buscar, crear playlists, marcar favoritos y administrar la cola.
- Recuerda las carpetas y actualiza la biblioteca al abrir la app.
- Incluye tres temas y un editor para guardar colores e imagen de fondo.
- Usa salida WASAPI compartida de forma predeterminada. El modo exclusivo es opcional y puede volver a compartido si el dispositivo no lo admite.
- Incluye un ecualizador de tres bandas opcional, apagado de forma predeterminada.

## Instalar en Windows

Ejecuta el archivo `Musica-Nativa-Setup-*.exe` de la carpeta `bin`. La instalación es por usuario y no requiere permisos de administrador. Si falta Microsoft Edge WebView2 Runtime, el instalador intentará instalarlo. Luego abre **Música Nativa** desde el menú Inicio y añade las carpetas donde guardas música.

El instalador lo genera el proyecto localmente y no cuenta con firma de editor. Windows puede mostrar una confirmación de seguridad al abrirlo.

La app se abre maximizada y puedes restaurarla y cambiar su tamaño. El tamaño mínimo restaurado es **900 × 560**; la navegación, las cuadrículas y el reproductor se reorganizan según el espacio disponible.

## Actualizaciones dentro de la app

En **Ajustes > Actualizaciones**, pulsa **Actualizar**. La app busca y descarga automáticamente la versión nueva; al terminar, pulsa **Reiniciar y actualizar** para aplicarla. La app verifica el SHA-256 publicado junto con el ejecutable; tu biblioteca y tus preferencias se conservan.

Las versiones se buscan en el repositorio público [`sergioc0702/musica-nativa`](https://github.com/sergioc0702/musica-nativa): el botón solo encuentra las versiones publicadas allí como release.

Para preparar los archivos de un release:

1. Pon la misma versión `x.y.z` en `version.go`, `build/config.yml` y `scripts/install.ps1`. Las propiedades del `.exe` en Windows toman la versión de `version.go`.
2. Ejecuta `.\scripts\prepare-release.ps1 -Version x.y.z`.
3. Crea el release público `vX.Y.Z` y adjunta los tres archivos generados en `bin/releases/vX.Y.Z`: `musica-nativa-windows-amd64.exe`, `Musica-Nativa-Setup-vX.Y.Z.exe` y `SHA256SUMS`.

El primer ejecutable sirve para actualizaciones desde la app; el instalador sirve para instalaciones nuevas.

## Desarrollo

Requisitos: Windows 10/11 de 64 bits, Go, Node.js/npm, GCC compatible con MinGW-w64 para CGO, Wails 3 CLI y Microsoft Edge WebView2 Runtime.

Desde la raíz del proyecto:

```powershell
npm --prefix frontend ci
wails3 task dev
```

Compilar y crear el instalador:

```powershell
wails3 task build
wails3 task package
```

El ejecutable de desarrollo queda en `bin/musica-nativa.exe`. El instalador queda en `bin/Musica-Nativa-Setup-*.exe`.

## Datos locales

La base de datos, las preferencias, los temas y los metadatos de la biblioteca se guardan en `%APPDATA%\MusicaNativa\library.db`. Al desinstalar se conserva esta carpeta para no borrar playlists ni preferencias.

## Tecnologías

Go y Wails 3; React, TypeScript, Tailwind CSS y Motion; malgo/miniaudio para salida WASAPI; SQLite para la biblioteca y las playlists.
