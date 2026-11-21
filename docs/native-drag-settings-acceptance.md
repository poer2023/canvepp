# Native drag and image settings · 0.4.7

## Scope and references

The user's two attached screenshots and the installed 0.4.6 native window are the reference. Change the Go/MyGo application only: canvas dragging and the floating image settings. Keep all existing images, drafts, source information, generation records, connections, and recoverability.

## Information and visual contract

- Remove the sidebar's bottom sliders button. Keep the Settings button and reopen image settings by selecting an image or its creation action.
- Remove the vertical marker beside both Source and Creation titles.
- Keep ratios as a regular, equally sized grid; no dropdown. Put Free pixels last. Keep editable width and height below it, without increment/decrement buttons.
- Use one tidy row for 1–4 output images. Remove repeated slanted rectangle glyphs, redundant count labels, and duplicate reference explanations.
- Retain neutral white/light grey, consistent spacing, restrained active states, existing icon actions, and the draggable floating inspector.

## Interaction contract

- Fixed ratios link width and height; Free pixels allows independent values. Valid dimensions stay within 64–8192 pixels, including linked dimensions. Typing a partial or invalid number must not corrupt saved parameters.
- Selection, copying, dragging, undo/redo, image settings, and source/creation switching retain content. The canvas remains anchored to the pointer during zoom and drag.
- Diagnose complete native frames (build/layout/paint/presentation), not just drawing-command construction. Compare the same isolated canvas before and after. No production edits or image generation for acceptance.

## Verification

- Fresh SQLite backup and asset hashes before changes; isolated copy for destructive or movement checks.
- Native computer-use checks at 1440×900 and 1000×680: parameters, multi-reference selection, panel reopening, image and panel drag, undo/redo, zoom.
- Targeted regression tests, race suite, vet, signed native build. Measure live frame timings and capture screenshots. Report measurement limits rather than inferring actual FPS from CPU-only timings.
- Install the tested build locally, verify the actual installed UI, and compare business rows and original file hashes against a fresh pre-install baseline.
