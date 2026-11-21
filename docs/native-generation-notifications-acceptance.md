# 图片生成完成通知 · B 版

1. 从生图结果写入资料库的回执触发。一次生成操作算一组，同时在处理的组完成后合成一条通知。张数以实际成功保存的结果计，部分成功提示剩余未完成数量。旧成功回执与重复回调不会重复提醒。
2. 单组一张显示缩略图，单组多张展示最多三张拼图，多组优先各取一张。macOS 使用系统图片附件，Windows 使用系统通知的内嵌图。点单张通知定位图片，点多张通知打开对应结果列表。
3. 用隔离资料库和本地样图验收单组 1 张、单组 4 张、3 组共 8 张及通知点击链路。回归测试记录发布条件、去重、已保存路由与代表图选择。

[B 版画板](http://127.0.0.1:4720/design/ZzPZoCG3YzdnqTK5YnUc9Q/#page-aa2934610739) · [macOS 图片附件](https://developer.apple.com/documentation/usernotifications/unnotificationattachment) · [Windows 通知图片](https://learn.microsoft.com/en-us/windows/apps/develop/notifications/app-notifications/app-notifications-content)

## 验收结果

- macOS 通知中心显示了单组 1 张、单组 4 张、3 组 8 张，每条均有图片附件。点击分别定位画布中的图片、打开 4 张本组结果、打开 8 张本批结果。
- Windows 11 从系统通知历史回读到三条 Toast，每条均有内嵌图片。以通知中的协议地址验证系统回跳，单张定位到图片，多组打开 8 张结果。关闭应用后再次激活四张通知，应用启动并显示 4 张结果。
- 两端使用隔离资料库和本地样图，经过实际的结果写入与完成回调。通知逻辑测试覆盖成功回执、部分失败、回调去重、代表图选择和结果页筛选。Go 竞态测试及两端的 `go vet` 通过。
