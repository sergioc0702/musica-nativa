# malgo con un parche de Música Nativa

Copia de [`github.com/gen2brain/malgo`](https://github.com/gen2brain/malgo) **v0.11.28** (dominio público, ver `LICENSE`), usada mediante `replace` en el `go.mod` del proyecto.

## Qué cambia

Solo `context.go`: los callbacks de log de miniaudio usan su propio candado (`logMutex`) en lugar de `contextMutex`.

## Por qué

`Context.Devices` toma `contextMutex` y llama a `ma_context_get_devices`. Si miniaudio escribe un mensaje de log durante esa llamada (pasa, por ejemplo, cuando el dispositivo de audio está ocupado), el callback `goLogCallback` se ejecuta en la misma goroutine e intentaba tomar `contextMutex` otra vez. Los mutex de Go no son reentrantes, así que esa goroutine se bloqueaba para siempre. En Música Nativa eso dejaba la app sin cargar la biblioteca y sin poder cerrarse.

`context_log_test.go` reproduce el caso y protege el arreglo.

## Al actualizar malgo

Copia la versión nueva sobre esta carpeta, vuelve a aplicar el cambio de `context.go` si upstream no lo ha corregido, y ejecuta `go test -run TestLogDuringDeviceEnumerationDoesNotDeadlock .` aquí. Si upstream ya lo corrigió, elimina esta carpeta y la línea `replace` del `go.mod`.
