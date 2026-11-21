# 主窗口素材页验证记录 · 0.6.2

日期：2026-11-07。对应 [验收合同](native-embedded-materials-acceptance.md)。

## 已实现与验证

- Pinterest、Eagle、Pinterest 连接设置和大图预览均在主窗口右侧原生内容区展示，不再创建独立素材窗口；左侧目录保留。
- 返回箭头与 Esc 按层级退出预览、连接设置、素材页；左侧画布 / 历史 / 回收站及设置入口正常切换。修正了子页移除焦点控件后 Esc 不响应的问题，快捷键由主窗口处理。
- 素材页保留连接、筛选和已加载内容；切回画布保留选择、视角和图内容。异步导入仍写入发起时捕获的画布。
- 本机浏览器授权回调激活主窗口并打开 Pinterest 页。使用隔离假 OAuth 服务验证，两个尺寸的回执均记录一次授权交换与两次 API 读取；没有使用真实账号或凭据。
- 1440×900 和 1000×680 原生窗口均检查连接配置、图板和图片列表，控件可操作。大图预览直接占用右侧内容区。
- 隔离数据副本中完成 Pinterest 图片预览与导入，“Codex 原生 · 实际生图验收”从 4 个节点变为 5 个；返回画布可见导入图片。生产画布仍为 4 个节点。
- Eagle 未连接状态与搜索入口在主内容区正常展示；本轮未连接真实 Eagle 实例。

## 自动检查

- `go test -race ./...`：通过，30.306 秒。
- `go vet ./...`：通过。
- 新增 `TestMaterialsInMainContentKeepsCanvasState`，覆盖两种窗口尺寸、布局边界、页面路由、Esc、输入框退格保护和画布状态保留。
- MyGo 发布构建和严格签名校验通过；已安装并启动 `/Users/hao/Applications/Canvepp Native.app`，实际设置页显示 0.6.2。
- 安装二进制 SHA-256：`973ae5233b127b700a46241e147363c87a0b977b7ba85a11062f11a89ee8755c`。

## 数据与安装

更新前已备份 SQLite、原始素材和旧版应用。更新后只读核对全部业务表与 18 个原始文件，内容哈希完全一致；SQLite 完整性检查为 `ok`。保留 6 张画布、23 条素材记录、14 条运行记录、11 条回收站记录和 2 条操作事件。完整回执见 [final-integrity.json](../artifacts/native-embedded-materials-20261107/final-integrity.json)。

已在用户此前授权范围内刷新当前安装包的输入监控注册，设置页确认“全局监听已开启”。本轮没有再次用实际跨应用三击空格验证触发，不将监听就绪等同于该交互验收。

## 截图与边界

- [已安装版本的右侧连接设置](../artifacts/native-embedded-materials-20261107/installed-content.png)
- [隔离素材列表](../artifacts/native-embedded-materials-20261107/qa-grid.png)
- [右侧大图预览](../artifacts/native-embedded-materials-20261107/qa-preview.png)
- [隔离导入后的画布](../artifacts/native-embedded-materials-20261107/qa-imported.png)
- [1000×680 连接设置](../artifacts/native-embedded-materials-20261107/qa-config-1000x680.png)
- [1000×680 素材列表](../artifacts/native-embedded-materials-20261107/qa-grid-1000x680.png)
- [Eagle 未连接状态](../artifacts/native-embedded-materials-20261107/qa-eagle.png)

真实 Pinterest 授权、收藏及私密图板的覆盖仍待用户在本机填写开发者凭据后验证。测试使用独立数据目录，假 OAuth 入口只存在于 `pinterestqa` 构建；生产安装包使用官方连接流程。本轮未触发生图、修改真实收藏或访问用户密钥。
