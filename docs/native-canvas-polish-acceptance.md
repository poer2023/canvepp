# Native canvas polish · 0.4.6

User scope: creation opens by default; refine image outlines, connections and basic canvas feedback using established creative canvases. Keep MyGo Native UI, the white/gray palette, image-first model, all content and existing gestures.

## References inspected

- [Lovart: Customize Your Canvas](https://www.lovart.ai/docs/edit-your-design/customize-your-canvas): official screenshot shows image as the object, a detached compact contextual toolbar, minimal selection outline; documented Shift selection, right-click actions and Space-pan.
- [FLORA public canvas demonstration](https://flora.ai/): images with minimal framing, horizontal side anchors and smooth restrained connections; tool panels separate from images. Copy spatial hierarchy, not its dark/green palette or separate parameter nodes.
- [Krea Nodes guide](https://www.krea.ai/docs/user-guide/features/nodes): left input/right output, wheel/pinch zoom, Shift multi-select, Space-pan. Live workflow examples require login; no login or generation performed.

## Contract

- Selecting an image opens its editable creation drawer. Source stays above it and is reachable in one click. Existing drafts remain intact; selecting/opening a drawer does not write a draft or turn the immutable original prompt into a new request. Creation history remains reachable.
- Images have no permanent border, header or footer. One thin neutral selection contour only; no doubled outlines or cosmetic resize/connection handles. Contextual icon toolbar sits outside the picture, stays reachable across its gap, and disappears when leaving the image/tools or during a drag.
- Connections keep left/right anchors with horizontal tangents, screen-space line weight and smooth curves. Unrelated paths recede; related paths are legible. Avoid sharp elbows/loops when images are close, reordered or vertically aligned. No lineage changes or edge deletion.
- Preserve wheel/pinch zoom, 400% maximum, placeholders dragging, box selection, Shift selection, multi-drag, keyboard copy/paste/delete/undo, and image transparency. Drawing stays native GPU with bounded curve tessellation and no idle animation loop.
- No unrelated sidebar redesign, model routing change, new generation, content migrations or permanent deletion.

## Acceptance

- Native app at 1440×900 and 1000×680: before/after screenshots, default creation, source toggle, history return, hover toolbar, selection and drag, zoom/fit; inspector must remain reachable.
- Focused regression tests for draft preservation on selection/switch, connection endpoints/direction/curve bounds, toolbar hover corridor and selection gestures. Existing race tests and vet pass.
- Validate on a copied database first. Compare production asset hashes and business rows across installation, permitting only viewport/layout state written by normal app use.
- Save evidence under `artifacts/native-canvas-polish-20261106/`; install the built native app after verification. No claims of measured FPS from CPU paint timings.
