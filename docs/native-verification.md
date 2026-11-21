# Canvepp Native 0.4.0 验收记录

日期：2026-11-06。平台：本机 Apple M4，macOS arm64。

本次按 [原生迁移验收合同](native-acceptance.md) 完成 MyGo + Go 独立应用，并采用纯左右布局。旧 Web 实现、原数据库和图片文件保持独立。正式应用已安装并打开。

## 产物与界面

- 应用：`/Users/hao/Applications/Canvepp Native.app`。
- 可分发压缩包：`native/build/CanveppNative-macos-arm64.zip`，约 5.6 MB；MyGo 构建输出的应用体积约 13.3 MB。
- 左侧 304 px：画布切换、图片参数、历史、回收站、Codex 与备份。右侧整块画布，无可见顶栏和右侧设置栏。
- 顶部 28 px 透明窗口拖拽区。实测系统窗口从 `(240,46)` 移到 `(320,96)`，窗口大小保持 `1440×900`。
- 实际窗口分别验收 `1440×900`、`1100×740`；图片工具在图片外，连线左右接入，透明图片保留棋盘背景。
- 正式包通过 `codesign --verify --deep --strict`。本地 ad-hoc 签名，尚未 Apple 公证。

正式应用截图：

![原生左右布局](../artifacts/native-desktop/native-final.png)

## 原内容保全

只读复制旧库到 `~/Library/Application Support/CanveppNative`。导入后的原始行数据及原图片 SHA-256 核验通过：

| 内容 | 已复制 |
| --- | ---: |
| 原画布 | 5 |
| 图片卡片 | 35 |
| 图片版本 | 23 |
| 生成记录 | 14 |
| 回收站条目 | 2 |
| 唯一图片文件 | 18 |

引用缺失数为 0，原库与原图片未改变。23 个版本共享 18 个实际图片文件，不将版本数量当成不同原图数量。旧生成请求只进入历史，不重新排队。另保留原生端此前创建的空白画布。

证据：[data-preservation.json](../artifacts/native-desktop/data-preservation.json)、[original-manifest.json](../artifacts/native-desktop/original-manifest.json)。

## 验证结果

`go test -race ./...`、`go vet ./...` 均通过。测试覆盖坐标缩放、多选拖动、复制粘贴、删除与恢复、撤销重做、文本输入与画布快捷键隔离、过期保存拒绝、导入事务回滚、跨画布图片引用、迟到生成结果及重启恢复。

真实 GUI 已检查多选和拖动、画布缩放、复制粘贴、删除和回收站、撤销重做、原生文件选择器导入、中文内容粘贴与保存、预览和重开持久化。透明原图已在正式画布显示。窄窗检查通过独立启动 `1100×740` 窗口完成。

Codex 真实只读连接检查返回 `connected: true`、`imageGeneration: true`、`plan: pro`。本轮没有发起真实生图。协议测试使用本地模拟 App Server，验证领取唯一性、提交后断连不重试、查询原线程恢复、重复回执去重及取消后结果保留；不能将这些测试等同于真实生图成功。

证据：[final-tests.txt](../artifacts/native-desktop/final-tests.txt)、[native-import-input-receipt.json](../artifacts/native-desktop/native-import-input-receipt.json)、[窄窗截图](../artifacts/native-desktop/native-narrow.png)。

## 性能实测

真实原生 GPU 窗口中进行连续分组拖动，每组记录 23 个热缓存样本。指标包含 MyGo 帧构建、布局、绘制和 GPU present 提交时间；不等同于显示器实际帧间隔或持续 FPS。

| 画布卡片数 | 中位耗时 | p95 | 最大耗时 |
| --- | ---: | ---: | ---: |
| 100 | 1.3 ms | 1.7 ms | 14.9 ms |
| 500 | 2.3 ms | 2.9 ms | 11.3 ms |

场景复用现有 23 个版本、18 个唯一图片文件；没有验证 500 张不同高分辨率图片同时冷解码。主要修正是将连线路径按屏幕像素生成，避免缩小画布时栅格化巨大的世界坐标路径。100 卡片拖动 p95 从优化前约 24.1 ms 降到 1.7 ms。

模型基准：500 卡片拖动坐标更新约 7.745 μs/op，0 B/op、0 allocs/op；500 卡片命中检测约 4.642 μs/op，0 B/op、0 allocs/op。软件渲染的无窗口基准独立记录，不与 GPU 窗口时间混用。

500 卡片测试进程采样：physical footprint 114.6 MiB、峰值 197.4 MiB，RSS 约 199.3 MiB；这是启用验收功能的测试包在该场景中的采样，不是正式版长期内存上限。正式包启动与原库加载已观察成功，未单独记录冷启动秒数。

证据：[performance-summary.json](../artifacts/native-desktop/performance-summary.json)、[benchmarks.txt](../artifacts/native-desktop/benchmarks.txt)、[500 卡片截图](../artifacts/native-desktop/native-500.png)。

## 尚未实机验证的边界

- Windows、Linux 与远端 `.117` 本轮没有构建或部署。
- 真实生成、完整订阅耗用路径需要用户提交实际任务后验证；不承诺把 ChatGPT 订阅转为通用 Images API 额度。
- Finder 直接拖文件进画布、复制图片到其他原生应用的链路已实现，但未分别进行真实跨应用验收。
- 新导入相机 JPEG 的 EXIF 方向、各输入法组合键和长时间高刷显示表现未全面验收。
- 本地签名包适用于当前本机验收；公开发布仍需签名、公证及目标平台验证。
