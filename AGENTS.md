<!-- BEGIN:nextjs-agent-rules -->

## This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->

## Canvepp 界面设计规则

当前桌面产品使用 `native/` 中的 Go + MyGo。界面采用全局扁平风格。常用操作优先使用图标，提供可访问名称和悬停、键盘焦点气泡。操作说明按需展示。保留用户内容、字段、状态、错误与不可逆操作确认。控件使用纯色和必要的细边线，禁止装饰性渐变、阴影、按压位移与光环。侧栏、菜单和工具栏的磨砂由「通用」中的外观开关控制。修改任何界面前阅读[当前界面规范](docs/interface-design.md)。其中历史配方用于溯源。画布几何与直接操控遵守[画布还原合同](docs/native-canvas-restore.md)。

独立控件默认无外框；菜单与会话名称使用较小字号和中灰色。侧栏可调宽、可折叠，图标态通过气泡显示名称。外观、图标、手型光标、快速气泡及侧栏须遵守[导航、外观与指针](docs/interface-design.md#导航外观与指针)。

同级页面统一标题和工具位置。详情返回时保留浏览位置，保存错误保留到数据实际写入成功。长标题、加载占位、失败重试与两种主题的验收见[学习手册的桌面端应用](docs/interface-design.md#学习手册的桌面端应用)。
