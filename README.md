# Música Nativa

Reproductor de escritorio para escuchar música guardada en tu PC. Incluye biblioteca local, playlists, favoritos, cola, salida WASAPI y temas visuales personalizables. No necesita una cuenta ni conexión a un servicio de streaming.

## Funciones

- Lee archivos **MP3, WAV y FLAC** de las carpetas que elijas; no copia ni modifica las canciones.
- Lee etiquetas y portadas cuando están disponibles y muestra valores de reemplazo cuando faltan.
- Organiza canciones, álbumes y artistas; permite buscar, crear playlists, marcar favoritos y administrar la cola. Los álbumes suenan en el orden del disco.
- Reproduce en orden o en aleatorio, y puede repetir la cola o una canción. Tiene atajos de teclado (lista en Ajustes).
- Recuerda las carpetas y actualiza la biblioteca al abrir la app.
- El inicio muestra lo que suena o la canción que dejaste a medias, mezclas rápidas (todo en aleatorio, favoritas y una mezcla del día), lo escuchado hace poco, lo que más escuchas, tus artistas y álbumes para redescubrir. El historial de escucha se guarda solo en tu equipo.
- Incluye tres temas y un editor para guardar colores e imagen de fondo.
- Usa salida WASAPI compartida de forma predeterminada. El modo exclusivo es opcional y puede volver a compartido si el dispositivo no lo admite.
- Muestra la letra de la canción, sincronizada letra por letra cuando tiene tiempos. La toma de un `.lrc` junto a la canción, de las etiquetas del archivo o de [LRCLIB](https://lrclib.net) (se puede apagar en Ajustes), y también puedes añadirla a mano. Se abre con el botón de la barra del reproductor o con la tecla L.
- Incluye un ecualizador de 10 bandas con preajustes, que se oye al instante. Se abre desde la barra del reproductor o con la tecla E, y está apagado de forma predeterminada.

## Instalar en Windows

Ejecuta el archivo `Musica-Nativa-Setup-*.exe` de la carpeta `bin`. La instalación es por usuario y no requiere permisos de administrador. Si falta Microsoft Edge WebView2 Runtime, el instalador intentará instalarlo. Luego abre **Música Nativa** desde el menú Inicio y añade las carpetas donde guardas música.

El instalador lo genera el proyecto localmente y no cuenta con firma de editor. Windows puede mostrar una confirmación de seguridad al abrirlo.

La app se abre maximizada y puedes restaurarla y cambiar su tamaño. El tamaño mínimo restaurado es **900 × 560**; la navegación, las cuadrículas y el reproductor se reorganizan según el espacio disponible.

## Actualizaciones dentro de la app

Al abrirse, la app comprueba si hay una versión nueva y, si la hay, marca **Ajustes** con un punto; no descarga nada por su cuenta. En **Ajustes > Actualizaciones**, pulsa **Actualizar**: la app descarga la versión nueva, verifica el SHA-256 publicado junto con el ejecutable, pausa la música y se reinicia ya actualizada. Tu biblioteca y tus preferencias se conservan.

Las versiones se buscan en el repositorio público [`sergioc0702/musica-nativa`](https://github.com/sergioc0702/musica-nativa): el botón solo encuentra las versiones publicadas allí como release.

### Publicar una versión

GitHub Actions compila, prueba y publica cada versión (`.github/workflows/release.yml`):

1. Cambia la versión con `.\scripts\set-version.ps1 -Version x.y.z`; actualiza `version.go`, `build/config.yml` y `scripts/install.ps1`. Las propiedades del `.exe` en Windows toman la versión de `version.go`.
2. Opcional: escribe las notas del release en `docs/releases/vX.Y.Z.md`. Sin ese archivo, GitHub genera la lista de cambios.
3. Haz commit, crea la etiqueta y súbelas:

   ```powershell
   git commit -am "Versión x.y.z"
   git tag vX.Y.Z
   git push origin master vX.Y.Z
   ```

Al recibir la etiqueta, el flujo publica el release `vX.Y.Z` con `musica-nativa-windows-amd64.exe` (lo que descarga el actualizador), `Musica-Nativa-Setup-vX.Y.Z.exe` (para instalaciones nuevas) y `SHA256SUMS`. Cada push a `master` también compila y prueba la app, y guarda esos archivos 14 días como artefacto del flujo.

Para generar los mismos archivos en tu equipo sin publicar nada, ejecuta `.\scripts\prepare-release.ps1 -Version x.y.z`; quedan en `bin/releases/vX.Y.Z`.

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

La librería de audio `malgo` se usa desde `third_party/malgo`, una copia con un parche que evita que la app se bloquee cuando el dispositivo de audio está ocupado; [`third_party/malgo/MUSICA-NATIVA.md`](third_party/malgo/MUSICA-NATIVA.md) explica el cambio y cómo actualizarla.

## Datos locales

La base de datos, las preferencias, los temas y los metadatos de la biblioteca se guardan en `%APPDATA%\MusicaNativa\library.db`. Al desinstalar se conserva esta carpeta para no borrar playlists ni preferencias.

## Tecnologías

Go y Wails 3; React, TypeScript, Tailwind CSS y Motion; malgo/miniaudio para salida WASAPI; SQLite para la biblioteca y las playlists.
