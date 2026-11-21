# 图片生成通知选稿

在现有 Baoyu 画布的「生成通知 · 三版」并排展示 A 文字直达、B 图片预览、C 按画布汇总。每个方案使用同一组样例，在 macOS 和 Windows 各展示单组 1 张、单组 4 张、3 组共 8 张。画板内可切换明暗，并点按通知或动作查看结果去向。选稿阶段使用本地样例和系统通知示意。

成功以图片写入资料库为准。一回生成操作算一组，多组样例有 3 组，分属两张画布。「海岸色稿」保存 5 张，「山地色稿」保存 3 张。单组单张定位到图片，单组多张定位到这组结果，多组进入本批记录。张数既出现在文字里，也可在有缩略图的版本里对应查看。

三版保留系统的应用标识、通知文字与激活入口。B 的图片作为一个附件或内嵌图，裁幅只用于通知预览。C 的 macOS 按钮显示在展开状态。通知在系统里的最终位置、主题、展开和可见范围由系统管理。

对照时检查两端六种组合、明暗切换和点击去向。Baoyu 格式检查须通过。原生通知发送、授权与激活的验收在选稿后的接入阶段进行。

[Apple 通知设计](https://developer.apple.com/design/human-interface-guidelines/notifications) · [Apple 通知附件](https://developer.apple.com/documentation/usernotifications/unnotificationattachment) · [Windows 通知内容](https://learn.microsoft.com/en-us/windows/apps/develop/notifications/app-notifications/app-notifications-content) · [大都会博物馆开放馆藏](https://www.metmuseum.org/hubs/open-access)

## 预览状态

三张通知画板均通过 Baoyu 格式检查。全画布检查另报出既有「齐边拼排」画板的属性引号格式。

本机预览沿用 Baoyu 服务 PID 72027，页面返回 HTTP 200。从仓库根目录运行下方第一条命令可重开预览，第二条命令会关闭此画布的预览。 [三版对照](http://127.0.0.1:4720/design/ZzPZoCG3YzdnqTK5YnUc9Q/#page-aa2934610739) · [预览日志](../.artifact-server.log)

```sh
node /Users/hao/project/baoyu-design/chatgpt-plugins/baoyu-canvas/skills/claude-design/scripts/artifact.mjs open designs/canvepp-empty-canvas
node /Users/hao/project/baoyu-design/chatgpt-plugins/baoyu-canvas/skills/claude-design/scripts/artifact.mjs stop designs/canvepp-empty-canvas
```
