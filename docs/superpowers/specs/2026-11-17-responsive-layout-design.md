# Diseño: Música Nativa adaptable a la ventana

## Intención

Evitar que la interfaz se corte o se desplace cuando Música Nativa se abre o se redimensiona en pantallas Windows con distintas resoluciones y escalas. La pantalla actual es 1600×900; Wails abre una ventana de 1440×900, fija un mínimo de 1050×700 y el CSS repite esos mínimos en `html`, `body` y `#root`.

## Enfoques considerados

- **Reorganizar el layout según el espacio disponible (elegido):** ajustar el tamaño inicial de la ventana al área de trabajo, quitar los mínimos rígidos del documento y reacomodar navegación, contenido y reproductor con breakpoints guiados por el contenido. Conserva el diseño de escritorio y permite ventanas compactas.
- **Ajustar solo márgenes y columnas:** cambio menor, pero conserva el mínimo de 1050 px y no resuelve bien las ventanas estrechas.
- **Reducir toda la interfaz con zoom:** se descarta porque hace el texto y los controles demasiado pequeños y no corrige las relaciones entre columnas.

## Experiencia propuesta

- La ventana inicia dentro del área de trabajo disponible y sigue siendo redimensionable.
- La barra lateral pasa a una variante compacta antes de forzar un ancho mínimo; los nombres de navegación y acciones siguen disponibles.
- Las tarjetas de álbumes, artistas y temas usan columnas flexibles, con un ancho mínimo legible y un máximo que evita estirarlas en pantallas grandes.
- La tabla mantiene visibles los datos principales y contiene su propio desbordamiento cuando no cabe.
- El reproductor inferior compacta metadatos y controles secundarios sin ocultar reproducir/pausar ni la pista actual.
- El contenido central sigue siendo la zona de desplazamiento vertical; la navegación y el reproductor permanecen accesibles.

## Alcance y límites

- Windows 10/11 en monitores de escritorio y portátiles; no se convierte la app en una interfaz móvil.
- Se conserva la identidad visual y las funciones actuales.
- No se usa zoom global ni detección de resolución física; CSS responde al tamaño efectivo de la ventana, que también refleja la escala de Windows.
- Primera comprobación visual propuesta: 1280×720, 1600×900 y 1920×1080; probar también escala de Windows al 125 % y redimensionamiento manual.

## Pendiente de revisión

Tras aprobar este diseño, el plan detallará los breakpoints de la barra lateral, la distribución del reproductor y los límites de tamaño de la ventana antes de editar la interfaz.
