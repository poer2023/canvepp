# Pinterest 浏览器授权与原生素材库 · 0.6.1 验证

日期：2026-11-07。合同：[native-pinterest-oauth-acceptance.md](native-pinterest-oauth-acceptance.md)。

## 交付

- 已安装 `/Users/hao/Applications/Canvepp Native.app`，设置页实测版本 0.6.1。
- Pinterest 登录改由系统默认浏览器打开官方授权页；应用侧接收一次性回调、兑换令牌、自动前置素材窗口。
- 收藏使用原生图板目录和分页图片网格，支持筛选、原图预览、来源链接和一键导入。每页最多 48 张缩略图、两路后台解码，正常发布版本不包含验收服务。
- 本机配置入口已打开，等待用户自行填写 App ID / App Secret；默认回调地址为 `http://localhost:47843/oauth/pinterest/callback`。凭据保存在 macOS 钥匙串，不放入画布备份。

## 已验证

| 范围 | 结果 |
| --- | --- |
| 自动测试 | `go test -race ./...` 通过，28.444 秒；`go vet ./...` 通过 |
| 授权边界 | 随机 state、错误回调、重放、拒绝、取消、单次兑换、串行刷新、令牌轮换、断开后不恢复旧令牌通过测试 |
| 数据读取 | 只读 API、游标分页、非法图片域名过滤、多图 Pin 的静态图片保留通过测试 |
| 本机端到端 | 明确标注的隔离假 OAuth 服务，应用打开默认 Chrome，允许后自动回到原生素材库；拒绝后可重新连接 |
| 原生浏览 | 图板及图片分页、空图板、筛选、图片预览、断开连接均经实际窗口操作观察 |
| 导入与恢复 | 隔离画布导入后出现独立节点；来源及 Pin URL 保留；撤销移除、重做恢复；重开后仍存在 |
| 原始图片字节 | 导入文件 SHA-256 与本机测试原图一致，见 `qa-import-integrity.json` |
| 窗口尺寸 | 1040×740 和 700×460 实机检查；小窗口图板、网格与表单可滚动，保存入口可达 |
| 发布 | 0.6.1 ARM64 构建及严格代码签名校验通过；已安装二进制哈希与验收构建一致 |
| 输入监控 | 更新后通过系统设置刷新既有授权，重启后应用显示「全局监听已开启」；未将该状态等同于本轮真实三击空格输入验收 |

截图与结构化回执位于 `artifacts/native-pinterest-oauth-20261107/`：

- `browser-fixture.png`：明确标注的本机假授权页。
- `native-grid-final.png`、`grid-700x460.png`：原生素材网格的大、小窗口。
- `preview.png`、`imported-node.png`：预览与画布导入。
- `denied.png`、`disconnected.png`：拒绝与断开连接。
- `installed-settings.png`、`installed-config.png`：正式安装版及配置入口。
- `go-test.log`、`build.log`、`fixture-*-receipt.json`、`final-integrity.json`：测试、构建、隔离授权及安装回执。

## 原有内容保护

旧版本正常退出后再备份并更新。与关闭前的备份相比，仅一个画布保存了当前 `graph.viewport`，节点、连线、提示词和图片内容一致。

正式安装、启动及恢复输入监控后，全部 8 张业务表与关闭后的备份逐行哈希一致，18 个原始素材文件 SHA-256 全部一致；SQLite `integrity_check` 为 `ok`。保留 6 张画布、23 条素材记录、14 条生成记录、11 条回收站记录及 2 条操作日志。

数据备份：`artifacts/native-pinterest-oauth-20261107/pre-update-closed-data/`。旧应用保存在同目录上级的 `CanveppNative-0.6.0-rollback.zip`，归档内容逐文件比对后移除了安装目录内的重复旧应用，避免系统选错应用版本。

## 尚未进行的真实账户验证

用户已有开发者凭据，选择之后在本机填写。本轮没有输入真实密钥、登录 Pinterest 账户或读取真实私人收藏；假服务验收证明应用流程，不能证明用户应用的审批、权限范围、实际收藏覆盖或真实账户刷新策略。填写后仍需完成一次官方授权并核对自己的图板和导入结果。

正式版本使用 Pinterest 官方 API 返回的最大图片版本，不保证该版本等同于上传者原始文件；视频不作为静态图片导入。公开分发的共用应用密钥及云端授权服务不在本次实现范围内。
