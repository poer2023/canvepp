# Compact native inspector verification · 0.4.8

Verified on 2026-11-06 against [the acceptance contract](native-compact-inspector-acceptance.md).

## Delivered

- Installed the signed 0.4.8 bundle at `/Users/hao/Applications/Canvepp Native.app` and observed its version in Settings.
- Source and Creation use single-line headings. Removed repeated creation counts, draft subtitles, the decorative branch icon and the extra prompt label. History is available through the heading icon.
- Target dimensions start collapsed on selection, showing the ratio and pixel summary. The expanded view retains the ratio grid, Free pixels last, and linked/independent pixel fields. Output count uses one property row.
- Generate, download, preview and delete share one scrolling row. Source actions scroll with source content; history actions are a virtualized list footer. No pinned bottom action bars remain.
- Default single-image creation panel is 320 × 394 DIP, previously 320 × 680. Expanding dimensions adds 158 DIP; smaller windows constrain the panel and scroll its content.

## Evidence

Artifacts are in `artifacts/native-compact-inspector-20261106/`:

- `installed-0.4.8.png`, `installed-ax.txt`: actual installed UI with the existing sunset image and its original next-step draft, target 1920 × 1080 and four outputs.
- `compact-default.png`, `multi-reference.png`: isolated 1440 × 900 acceptance.
- `small-before-scroll.png`, `small-after-scroll.png`: 1000 × 680, five references, expanded dimensions. The action row moves with content and becomes visible after scrolling; canvas zoom remains 31%.
- Live input checks: 16:9 width 1280 links to height 720; Free pixels then permits independent height 777; selecting three outputs updates the generate action. Source retains the original sunset prompt, and returning from history retains the separate rain draft and parameters. These edits were in the acceptance database only; no generation was invoked.
- `go test ./...` passed, including disclosure defaults/reset, draft preservation, action alignment/scrolling, pixel input validation, history and panel docking. `go vet ./...` passed. The native build and strict bundle signature verification passed.

## Data and update handling

`verification.json` confirms 6 projects, 23 asset versions, 14 legacy generation records, 0 native execution records, 11 trash entries and 18 original asset files. Asset bytes and history/trash rows match the fresh pre-install snapshot. All project content fields, prompts, references and edges match; one node position and viewport changed during live interaction and were preserved rather than overwritten. The receipt records that layout difference explicitly; raw project-row hashes are not claimed identical.

The old 0.4.7 process did not exit normally and remained in its closing state. SQLite integrity and the visible draft were checked, full database/asset backups were retained, then the old process was stopped to complete replacement. This was update recovery, not a verified fix to the old shutdown path. Both isolated 0.4.8 acceptance windows exited normally. The old app zip and data snapshots remain available in the artifact directory.

## Dimension semantics and reference

The native integration sends `targetPixels` in Codex request text (`native/codex.go`); it does not pass a structured image-tool `size` argument. UI presets therefore express target intent, not guaranteed native output sizes. The dimension row tooltip says the actual pixels depend on the generated result, and Source continues to show the actual asset dimensions.

OpenAI's current [Image API documentation](https://developers.openai.com/api/docs/guides/image-generation#size-and-quality-options) documents custom dimensions with model-specific constraints. That API contract must not be presented as a guarantee of the Codex native tool used here. No generation backend, subscription routing, pixel bounds or saved parameter format was changed.

The layout follows the grouping and progressive disclosure principles described in [Figma's UI3 design article](https://www.figma.com/blog/behind-our-redesign-ui3/), rather than adding another permanent toolbar.
