# Native compact image inspector · 0.4.8

## B 版参数清单 · 2026-11-19

- 以 [Baoyu 的 B 版](../designs/canvepp-empty-canvas/project/ImageProperties.dc.html)替换原生图片面板的创作表单。单张空图显示「创作图片」。提示词下方依次是比例、目标像素和张数，宽高直接可编辑，八种固定比例与自由像素放在浮层。打开比例浮层时面板尺寸和吸附位置保持稳定。
- 浅色使用白色外壳和浅灰输入面，深色使用既有的深灰面和反色主按钮。底部左侧是回收站，右侧用文字显示生成张数。来源、创作记录、原图工具、草稿、参考图和回收站撤销沿用现有路径。窄面板允许目标像素换行，短视口可滚到完整的操作行。
- 在隔离 macOS 窗口和原生测试中检查浅色和深色、最小与常用尺寸、固定比例联动、独立宽高、草稿及操作保留。生成验证覆盖按钮状态，使用隔离资料库。

在 Mac 的独立验收版确认新版面板和空草稿的禁用按钮。渲染与交互测试覆盖明暗主题、比例浮层、固定比例双向联动、自由像素、保存草稿、来源和创作记录。把两侧栏拉宽后，226 pt 面板仍能显示两个四位数像素字段，底部吸附状态下可选比例。`go test -race ./...`、`go vet ./...`、macOS 签名与 Windows 交叉构建通过。[浅色](../artifacts/native-inspector-b-20261119/inspector-light.png) · [深色](../artifacts/native-inspector-b-20261119/inspector-dark.png) · [比例浮层](../artifacts/native-inspector-b-20261119/ratio-popup.png)

## 历史验收 · 0.4.8

## Scope

Use the user's screenshot and the installed 0.4.7 native app as the baseline. Follow Figma UI3's grouped property rows and progressive disclosure (https://www.figma.com/blog/behind-our-redesign-ui3/). Change the Go/MyGo floating inspector only. Preserve images, editable drafts, original prompts, immutable history, references and trash; do not generate images or change account access during acceptance.

## Layout and behavior

- Single-line Source/Creation headings; remove duplicate creation counts, draft subtitles and decorative branch icons. Keep source/history navigation accessible.
- Target dimensions collapse by default on image selection. The collapsed row shows ratio and target pixels. Expand to the existing regular ratio grid, Free pixels last, linked width/height inputs. No dropdown. Collapsing and navigation must not alter saved inputs.
- These values are target intent sent in Codex request text, not guaranteed native output dimensions. Explain via the dimension row tooltip; retain actual image dimensions in Source.
- Output count is one compact property row. Generate and image actions share a single row inside the scrolling content; no pinned bottom action bars, divider bars, or duplicate delete button.
- Keep readable 12–13 pt property text, consistent alignment and compact panel height. Source and Creation retain their drawer switching behavior. Small windows scroll without obscuring controls or zooming the canvas.

## Verification

- Snapshot source and production data; use a separate data copy for live editing checks.
- Test disclosure default/reset, fixed/free pixels, draft preservation across disclosure and source/history switches, multi-reference mode, count, scrolling action geometry, and panel docking after height changes. Run native tests and vet, build/sign.
- Inspect real native windows at 1440×900 and 1000×680; capture screenshots. Install the verified build locally and compare business rows and asset hashes against a fresh baseline. No remote installation in this scope.
