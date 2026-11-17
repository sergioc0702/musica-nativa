# Exploración de YouTube en Música Nativa

Fecha: 2026-11-17

## Intención

Conservar Música Nativa como reproductor de archivos del PC y añadir una forma de buscar y ver videos musicales de YouTube dentro de la aplicación. El usuario descartó la conexión con Telegram y la descarga de audio desde internet. Quiere una opción que no exija pagar por consultas, alojar un servidor ni mantener una ventana de video abierta mientras usa el reproductor local para jugar.

## Resultado para el usuario

- La navegación principal ofrece **Explorar YouTube**.
- Al entrar, un botón **Abrir YouTube** crea una segunda ventana de Música Nativa con la página de YouTube. Allí el usuario usa la búsqueda y el reproductor de YouTube tal como aparecen en ese sitio.
- La ventana muestra claramente que el contenido viene de YouTube. Cerrar esa ventana detiene el video y libera sus recursos. Abrirla de nuevo no inicia una segunda copia si ya está abierta.
- La biblioteca, la búsqueda local, los temas, la cola y el motor WASAPI siguen siendo para archivos locales. Al abrir la ventana de YouTube, la app pausa la pista local para evitar dos audios a la vez; el usuario puede reanudarla manualmente.
- Si no hay conexión o YouTube no carga, la ventana informa el problema y la biblioteca local permanece utilizable.

## Alcance y límites

- No se descargan archivos de YouTube ni se extrae su audio.
- Los videos no se incorporan a playlists, favoritos o cola locales. Tampoco pasan por el ecualizador ni el modo exclusivo de Música Nativa.
- No se garantiza la reproducción con la ventana minimizada. El propósito principal de esta sección es buscar y ver el video con la ventana abierta.
- La búsqueda y el video usan la página oficial de YouTube, por lo que la app no necesita una clave de YouTube Data API ni un servicio propio. Requiere internet y puede consumir más memoria mientras la ventana está abierta.
- La calidad y disponibilidad del video dependen de YouTube y de la conexión; la app no altera el flujo multimedia.

## Enfoques considerados

1. **Abrir el navegador predeterminado.** No requiere integración ni claves y mantiene bajo el consumo de la app, pero el usuario eligió ver resultados y video dentro de Música Nativa.
2. **Buscar con YouTube Data API y reproducir con IFrame Player dentro de la ventana principal.** Ofrece una interfaz más propia, pero exige credenciales y cuota de API. Una clave distribuida dentro de un ejecutable público puede extraerse. Además, Wails 3 advierte que los marcos de terceros pueden alcanzar métodos Go vinculados, por lo que no se debe insertar YouTube en la ventana que administra archivos locales.
3. **Ventana aislada con la página oficial de YouTube.** Elegida como propuesta: no requiere clave, servidor ni descarga; permite búsqueda y video dentro de una ventana de la app. Necesita una prueba temprana de reproducción en WebView2 y aislamiento de las funciones locales.

## Diseño técnico

- El proceso principal conserva `LibraryService`, SQLite y el reproductor actual. Añade una acción Go para abrir o enfocar la ventana de exploración. La interfaz React muestra la entrada de navegación y un estado claro cuando la ventana no está abierta.
- La exploración se ejecuta en un proceso secundario del **mismo ejecutable**, con un argumento de inicio propio. Ese proceso crea una ventana WebView2 de Wails que navega a `https://www.youtube.com/` y no registra `LibraryService`, `Store`, `AudioPlayer`, actualizador ni rutas de portadas. Esta separación evita que el sitio tenga acceso a las funciones de biblioteca del proceso principal.
- El proceso principal mantiene el identificador del proceso secundario para enfocarlo si ya existe, observar su salida y cerrarlo al salir de la app o antes de instalar una actualización. No se ejecuta ni consulta YouTube al iniciar Música Nativa.
- La URL inicial es fija. La app no construye direcciones arbitrarias desde texto del usuario ni habilita funciones Go privilegiadas en la ventana de exploración.
- Antes de desarrollar la interfaz definitiva, una prueba corta debe confirmar en Windows 10/11 que la página de YouTube carga, permite buscar y reproduce un video en WebView2. Si la página rechaza ese entorno, se vuelve a revisar el diseño con el usuario; no se sustituye silenciosamente por el navegador externo.

## Validación

- Iniciar la app sin conexión: biblioteca local y controles siguen funcionando; abrir YouTube muestra un error recuperable.
- Abrir, buscar, reproducir, cerrar y reabrir la ventana; no se crean procesos duplicados ni queda video reproduciéndose tras cerrar.
- Abrir YouTube mientras suena una pista local: la pista se pausa y no se reanuda sola.
- Comprobar que el proceso de exploración no puede invocar métodos del servicio local ni leer la base de datos.
- Medir memoria antes, durante y después de usar la ventana de YouTube; la memoria adicional debe liberarse al cerrarla. Revisar diseño con la ventana principal en su mínimo de 900 × 560 y en pantalla maximizada, sin captura de pantalla automatizada a menos que el usuario lo autorice.

## Referencias

- [Wails permite ventanas con URL externa](https://v3.wails.io/features/windows/options/).
- [Wails advierte sobre acceso de marcos de terceros a métodos vinculados](https://github.com/wailsapp/wails/blob/master/v3/release_notes.md).
- [YouTube Data API exige credenciales y tiene cuota](https://developers.google.com/youtube/data/guides/authentication).
