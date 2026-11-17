# Responsive Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep Música Nativa legible and usable across Windows display sizes and display scaling, without clipping or globally shrinking the interface.

**Architecture:** Let the Wails window use the available work area and permit a compact but usable restored size. Let the WebView adapt its shell with viewport breakpoints and use container queries for cards and settings panels, so components respond to the width actually available to them. Preserve the existing visual identity and keep navigation, playback, and table data available.

**Tech Stack:** Go, Wails 3.1, React, TypeScript, CSS, Windows WebView2.

**Spec:** `docs/superpowers/specs/2026-11-17-responsive-layout-design.md`

## Global Constraints

- Target Windows 10/11 desktop and laptop screens; this is not a mobile redesign.
- Use effective window size and Windows scaling; do not scale the whole interface with CSS zoom or transforms.
- Keep core navigation, current-track details, and play/pause controls accessible at compact widths.
- Preserve the existing visual identity and keep the content column bounded on ultrawide displays.

## Review Focus

- **1280×720 at 100% and 125% Windows scale:** the app opens within the work area; the player remains visible and the main content scrolls independently.
- **900–1100 px window width:** the sidebar switches to an accessible icon rail and the page avoids horizontal overflow outside intentional table scrolling.
- **Short window height below 760 px:** compact vertical spacing does not cover the player or hide page actions.
- **Long track, playlist, and device names:** labels truncate within their own cells without pushing the shell wider.
- **1920×1080 and wider:** grids grow only while cards remain legible; the central content stays bounded rather than stretching across the monitor.

---

### Task 1: Make the Wails window fit common work areas

**Files:**
- Modify: `main.go`

**Interfaces:** Set the normal restore size to `1280×760`, set `StartState` to `application.WindowStateMaximised`, and lower the resize minimum to `900×560`. Keep resizing enabled.

- [x] Update `WebviewWindowOptions` with the restored size, `StartState`, and minimum dimensions from the interface above.
- [x] Run `wails3 task windows:build PRODUCTION=true ARCH=amd64` and confirm the Windows app compiles.

### Task 2: Reflow the desktop shell and its content

**Files:**
- Modify: `frontend/public/style.css`
- Modify: `frontend/src/App.tsx`

**Interfaces:** The page shell keeps a flexible main column. Below `1100px`, navigation becomes a `72px` icon rail. Named container queries reflow card grids and settings panels from their parent width; the home hero artwork yields space below `960px`. The player compacts at narrow widths and at short heights. The main scroll region remains the only page-level vertical scroller.

- [x] Remove the `1050px × 700px` document minimum from `html`, `body`, and `#root`; set shrinkable minimum sizes and retain the fixed player with a scrollable main region.
- [x] Add accessible names and tooltips to navigation items so hiding their visible text in the icon rail does not remove their labels.
- [x] Give `.page-content` a named inline-size container and reflow card grids/editor panels with `@container`; replace fixed card counts with `minmax()`/`auto-fit` tracks where content permits.
- [x] Add viewport breakpoints for the icon rail below `1100px`, compact hero below `960px`, and responsive top bar/player widths.
- [x] Keep the track table's horizontal overflow inside `.track-table-wrap`; do not allow it to widen the app shell.
- [x] Add a short-height compact rule at `760px` for page padding, hero height, and sidebar spacing while preserving player controls.
- [x] Run `npm run build` in `frontend` and confirm TypeScript and Vite complete successfully.

### Task 3: Verify responsive states and document how to launch

**Files:**
- Modify: `README.md`

**Interfaces:** The documentation states that the app opens maximised, can be restored/resized, and adapts from the available WebView size.

- [x] Capture and inspect `1280×720`, `1600×900`, `1920×1080`, and `900×560`; the `1600×900` at `125%` scaling case was represented by its effective `1280×720` CSS viewport (Windows display scaling itself was not changed).
- [x] Confirm the compact sidebar labels remain available to assistive technology, the table alone scrolls horizontally when needed, and no controls are covered by the player.
- [x] Run `wails3 task package PRODUCTION=true ARCH=amd64` to produce the Windows installer after visual fixes.
- [x] Update the README with the resize/maximize behavior and the verified minimum restored size.

## Spec Coverage

- Window sizing and available work area: Task 1.
- Sidebar, cards, table, player, and short-height behavior: Task 2.
- Target resolutions, Windows scaling, accessibility, and launch guidance: Task 3.
