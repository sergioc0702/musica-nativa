# YouTube Explorer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Música Nativa search and watch YouTube in an isolated, on-demand app window while keeping local playback and files separate.

**Architecture:** The main Wails process keeps the library and audio services. A second process launched from the same `.exe` opens the official YouTube site in its own WebView2 window and registers no library services. The main process owns the child lifecycle and pauses local audio when exploration opens.

**Tech Stack:** Go 1.26, Wails 3.1.0, Windows WebView2, React, TypeScript, existing CSS.

**Spec:** `docs/superpowers/specs/2026-11-17-youtube-explore-design.md`

## Global Constraints

- Windows 10/11 x64; keep the one-executable distribution and existing update flow.
- No YouTube download, audio extraction, Telegram, paid API, Google key, or new server.
- Do not load third-party content in the main Wails window with bound library methods.
- Do not start YouTube or its child process on app startup.
- Do not alter users' music files or local library semantics.
- Do not take automated screenshots or control the user's screen without first telling them; they raised this concern earlier.
- Preserve unrelated working-tree files, including `third_party/`.

## Review Focus

1. Repeated clicks on **Abrir YouTube** must focus one child window, not launch duplicates; Task 2 tests this with a fake child.
2. A child process that exits on its own must clear manager state so the next click relaunches it; Task 2 tests exit and relaunch.
3. A failed launch or offline website must leave local library and playback usable; Task 2 tests failed launch without pausing, Task 3 verifies the UI error/retry path.
4. Installing an update while YouTube is open must close the child before replacing/restarting the executable; Task 2 adds an updater test.
5. YouTube content must have no access to SQLite, artwork, updater, or Go library bindings; Task 1 tests child options and reviews the early mode branch before any store is opened.

---

### Task 1: Isolated YouTube child and feasibility check

**Files:**
- Create: `youtube_child.go`, `youtube_child_test.go`
- Modify: `main.go`

**Interfaces:**
- Produces: `isYouTubeChild(args []string) bool`; `youtubeChildOptions() application.Options`; `youtubeWindowOptions() application.WebviewWindowOptions`; `runYouTubeChild() error`; constant `youtubeChildArg = "--youtube-explorer"`.
- `main` calls `isYouTubeChild(os.Args[1:])` before `os.UserConfigDir`, `OpenStore`, `NewAudioPlayer`, `configureUpdates`, or service registration.

- [ ] **Step 1: Write failing tests** for exact child-argument recognition, a pure `youtubeChildOptions()` builder whose `application.Options.Services` is empty and has no artwork middleware or local assets, and a `youtubeWindowOptions()` builder with fixed URL `https://www.youtube.com/` and title `Música Nativa · YouTube`.
- [ ] **Step 2: Run `go test ./... -run 'TestYouTubeChild' -count=1`** and confirm the new tests fail for missing interfaces.
- [ ] **Step 3: Implement the child branch.** `runYouTubeChild` constructs a fresh Wails app with no services and a WebView window that loads the fixed YouTube URL; it does not touch the normal store. Keep the child branch in `main` before all local-app initialization.
- [ ] **Step 4: Run the focused tests and `go test ./...`**; both pass.
- [ ] **Step 5: Build one development executable and manually test the child mode on Windows** without screenshots: launch it with `--youtube-explorer`, search for a public music video, play/pause it, close the window, and confirm the child exits. If WebView2 cannot load or play YouTube, stop this design and report the result before changing the approach.
- [ ] **Step 6: Commit only Task 1 files.**

### Task 2: Parent lifecycle, audio, and updates

**Files:**
- Create: `youtube_manager.go`, `youtube_manager_test.go`, `youtube_focus_windows.go`
- Modify: `libraryservice.go`, `updates.go`, `updates_test.go`

**Interfaces:**
- Produces: `youtubeManager.Open() error`, `youtubeManager.Close() error`, and `LibraryService.OpenYouTubeExplorer() error` for Wails bindings.
- `youtubeManager` launches `os.Executable()` with only `youtubeChildArg`, tracks one process, observes its exit, and focuses the existing window by child PID on Windows. Its process start/focus/stop operations are injectable for deterministic tests.

- [ ] **Step 1: Write failing manager tests** for first launch, repeated launch/focus, spontaneous exit followed by relaunch, launch failure, and close idempotence. Use a fake process; make no actual YouTube or Windows UI calls in tests.
- [ ] **Step 2: Run `go test ./... -run 'TestYouTubeManager' -count=1`** and confirm expected failures.
- [ ] **Step 3: Implement the manager and Windows focus function.** Protect process state with a mutex; never create a second child while one is running. `Close` requests window close, then kills only that child after a short timeout if needed. Do not start it during app startup.
- [ ] **Step 4: Add service tests** proving a successful `OpenYouTubeExplorer` pauses local playback and emits the changed state, while a failed launch leaves playback untouched. Implement the exported service method, initialize the manager in `NewLibraryService`, and close it in `ServiceShutdown`.
- [ ] **Step 5: Add an updater test** proving `UpdateNow` closes the child before `DownloadAndInstall` or `Restart`. Implement this ordering in `updates.go`.
- [ ] **Step 6: Run `go test ./...` and `go vet ./...`**; both pass, then commit only Task 2 files.

### Task 3: Entry point, copy, and release verification

**Files:**
- Modify: `frontend/src/App.tsx`, `frontend/public/style.css`, `frontend/bindings/musica-nativa/libraryservice.ts`, `README.md`, `../PRODUCT.md`

**Interfaces:**
- Consumes: `LibraryService.OpenYouTubeExplorer(): Promise<void>` generated from Task 2.
- Produces: a navigation entry and lightweight **Explorar YouTube** page with **Abrir YouTube** button, loading/error state, and text explaining that local files and YouTube are separate.

- [ ] **Step 1: Run `modern-web-guidance` searches** for the relevant navigation/button and status-message patterns, retrieve applicable guides, and read Impeccable's craft floor before touching UI files.
- [ ] **Step 2: Generate Wails bindings** with `wails3 generate bindings -clean=true -ts`; confirm `OpenYouTubeExplorer` exists in the generated service file.
- [ ] **Step 3: Add the navigation and page.** Reuse existing design tokens and responsive layout, call the binding on click, show a retryable error if opening fails, and leave the main library search box local-only. Do not insert an iframe into the main window.
- [ ] **Step 4: Update `README.md` and `../PRODUCT.md`** to describe optional YouTube exploration, no downloads, no key/server cost, the second window, and the local-first gaming path.
- [ ] **Step 5: Run `npm --prefix frontend run build`, `go test ./...`, `go vet ./...`, and `wails3 task build`**; all pass. Run the built executable and manually verify local playback, opening/focusing/closing the YouTube window, and offline failure/retry. Verify the extra process disappears after closing the window and compare memory before/during/after with process metrics, without screenshots. The updater ordering is verified by Task 2's test.
- [ ] **Step 6: Commit only Task 3 files inside the repo; report the `../PRODUCT.md` edit separately.**

## Self-review

The three tasks cover child feasibility and isolation, parent lifecycle and update behavior, and user-facing entry/documentation. Each task has a separate verification gate. A failed WebView2 feasibility check in Task 1 blocks later tasks and returns to the user instead of silently falling back to the external browser.
