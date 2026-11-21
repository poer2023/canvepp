# Native canvas polish · 0.4.6 verification

Installed and opened `/Users/hao/Applications/Canvepp Native.app` (`app.canvepp.native`, version 0.4.6). MyGo native renderer remains in use; no WebView or generation backend changes.

## Observed result

- Existing images open Creation by default. The installed app retained the user's transparent-background draft. In the isolated copy, the sunset image still had its original sunset prompt in Source and its separate rain draft in Creation; switching views preserved the 1920×1080 draft parameters.
- Images have no permanent frame. A single thin outline marks selection. The borderless icon toolbar floats outside the image; real pointer movement into the inspector dismisses it. AX-only button activation does not move the physical pointer, so hover acceptance used coordinate input.
- Curves use horizontal sides, quieter unrelated lines and stronger related paths. Model tests cover left-to-right, reversed, vertically stacked, near-touching cards, endpoint direction, culling, no crossing through either image, and zero route allocations.
- Real GUI checks: four-card marquee selection; single and multi-card drag/undo; source/history/creation switching; double-click preview; narrow-window parameter scrolling; zoom buttons; wheel zoom to 400%, decrease to 320%, Fit. No image generation was performed.
- 1440×900 and 1000×680 passed. At very high zoom, a toolbar is hidden if neither the top nor bottom outside edge fits, preserving the visible image pixels; inspector/context-menu actions stay available.

## Validation and performance

- `go test -race ./...`: passed (16.969 s); `go vet ./...`: passed; signed native build and strict signature verification passed.
- The unchanged-selection and 500-card drag benchmarks still allocate zero per update. The native view software harness was 21.41 ms / 37.08 ms for 100 / 500 cards versus 26.92 ms / 48.87 ms in the saved 0.4.5 source (15 iterations, informational rather than a sustained FPS measurement).
- Actual native UI CPU drawing-command construction p95: 0.207 ms at the desktop test view; 0.705 ms during narrow-window zooming. These figures exclude presentation/GPU time and are not FPS measurements.

## Content preservation

- Exact business-row comparison across installation: identical projects, assets, runs, trash and native runs. All 18 original-file SHA-256 hashes identical.
- Retained: 6 canvases, 23 asset versions, 14 legacy generation records, 3 trash records. No new generation records.
- Closing 0.4.5 flushed the user's existing unsaved viewport for one canvas; nodes and edges were unchanged. The closed database was captured as the exact pre-install baseline.
- GUI interaction tests used a database copy with deliberately arranged card positions. That copy was never restored to production. Production card positions, content and drafts were not edited by acceptance.

## Evidence

- [Contract](native-canvas-polish-acceptance.md)
- [Installed application](../artifacts/native-canvas-polish-20261106/installed-creation.png)
- [Hover tools and connections, isolated copy](../artifacts/native-canvas-polish-20261106/creation-hover-1440.png)
- [Multiple selection](../artifacts/native-canvas-polish-20261106/multiselect-1440.png)
- [Source](../artifacts/native-canvas-polish-20261106/source-1440.png) and [history](../artifacts/native-canvas-polish-20261106/history-1440.png)
- [1000×680](../artifacts/native-canvas-polish-20261106/creation-1000.png) and [scrolled parameters](../artifacts/native-canvas-polish-20261106/parameters-1000.png)
- [Preservation receipt](../artifacts/native-canvas-polish-20261106/preservation.json), [installation receipt](../artifacts/native-canvas-polish-20261106/install-receipt.json), [tests](../artifacts/native-canvas-polish-20261106/tests.log)

References were public Lovart documentation, FLORA's public canvas demonstration, and Krea's documented input/navigation conventions. Lovart/Krea live authenticated workflows were not tested. No full-product or external generation acceptance is claimed.
