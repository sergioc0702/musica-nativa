# Actualizaciones integradas — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Añadir un botón único que compruebe y descargue releases públicos de Música Nativa; reiniciar aplica la actualización.

**Architecture:** El backend configura `app.Updater` en modo headless con el proveedor GitHub de Wails y expone comprobación, descarga y reinicio al frontend. Ajustes muestra el estado y progreso dentro de la app, mientras el actualizador gestiona verificación por SHA-256 y reemplazo seguro.

**Tech Stack:** Go, Wails 3.1.0 updater, React, TypeScript, GitHub Releases.

**Spec:** `docs/superpowers/specs/2026-11-16-in-app-updates-design.md`

## Global Constraints

- La distribución inicial sigue siendo Windows 10/11 x64.
- El botón es manual; no hay comprobación automática al abrir la app.
- Los releases usan `sergioc0702/musica-nativa` y el ejecutable `musica-nativa-windows-amd64.exe`.
- La biblioteca y las preferencias permanecen en `%APPDATA%\MusicaNativa`.
- No publicar código o binarios durante el trabajo local.

## Review Focus

- Repositorio o release ausente: el botón muestra un error legible y no bloquea el reproductor.
- Versión actual igual o mayor: el proveedor reporta que la app está al día.
- Archivo descargado no coincide con SHA256SUMS: Wails rechaza la actualización.
- Descarga lista: el usuario debe reiniciar para que el helper sustituya el ejecutable.
- Release sin `windows/amd64`: la búsqueda debe fallar con un mensaje legible, sin descargar el instalador de primera instalación.

---

### Task 1: Configurar actualizador y servicio Go

**Files:** `main.go`, `libraryservice.go`, `version.go`

**Interfaces:** Exportar `LibraryService.CheckForUpdates() (UpdateCheckResult, error)`, `DownloadUpdate() error` y `RestartUpdate() error`; establecer versión actual `1.0.0` y proveedor `sergioc0702/musica-nativa` con `ChecksumAsset: "SHA256SUMS"`.

- [x] Añadir proveedor y `app.Updater.Init` con `WindowNone` y versión estable.
- [x] Exponer comprobación, descarga y reinicio, además de `AppState.Version`.
- [x] Verificar compilación Go y generación de bindings.

### Task 2: Añadir botón de actualización a Ajustes

**Files:** `frontend/src/App.tsx`, `frontend/public/style.css`, bindings generados por Wails.

**Interfaces:** La sección «Actualizaciones» inicia comprobación y descarga con un botón, escucha los eventos `wails:updater:*` y muestra un botón de reinicio cuando el paquete está listo; la fase activa bloquea llamadas duplicadas.

- [x] Mostrar versión, disponibilidad, notas, progreso y botón de reinicio en «Actualizaciones».
- [x] Escuchar `no-update`, `update-available`, `download-progress`, `verifying`, `installing`, `update-ready` y `error`.
- [x] Verificar build de producción del frontend.

### Task 3: Preparar artefactos de release

**Files:** `scripts/prepare-release.ps1`, `README.md`.

**Interfaces:** El script recibe la versión y produce el ejecutable de autoactualización, el instalador de primera instalación y `SHA256SUMS` bajo `bin/releases/<versión>`.

- [x] Ejecutar el empaquetador existente y copiar los dos artefactos con nombres estables.
- [x] Calcular SHA-256 en formato compatible con el proveedor GitHub de Wails.
- [x] Documentar que el asset autónomo y `SHA256SUMS` deben adjuntarse al release público con el tag `v<versión>`.
- [x] Verificar build completo e inspeccionar los nombres y hashes de salida; no subir nada a GitHub.
