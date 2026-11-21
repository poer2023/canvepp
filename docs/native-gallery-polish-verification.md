# Canvepp Native 0.4.4 画廊视觉精简

2026-11-06，依据 [验收合同](native-gallery-polish-acceptance.md)。已安装并打开 `/Users/hao/Applications/Canvepp Native.app`。

图片去掉卡片外框、白色背景、内层底色和外包裹内边距，按原比例显示，透明棋盘只存在于图片区域。下方保留一行淡色名称，操作改为无文字图标；画布名称与完整文件名通过提示查看。

左侧新建、按时间分组、按画布分组、排序与设置均以图标承载。搜索输入、目录名称和导航名称保留文字。分组图标移入搜索同一行，减少左侧垂直占用。排序选项仍在弹出菜单中清楚展示。

实机发现并修复了 MyGo 连续调用 Tooltip 会附加两条提示的问题；图标提示现在只注册一次。截图及辅助功能树确认仅出现一条“回到来源画布”，没有文字重影。

验证覆盖：

- 1440×900 和 1000×680 原生窗口，无外卡片边框或底色，无图像拉伸。
- 图片点击打开原图；图标返回来源画布；左栏分组图标切换、排序菜单操作生效。
- 独立数据副本中，点击恢复图标后图片回到原画布。
- 既有 UI 测试补充图标悬停说明与点击导航、分组行为；`go test -race ./...`、`go vet ./...` 通过。
- macOS arm64 构建、安装签名校验通过。仅更新本机原生应用，无生图、登录或远程部署。

正式数据逐行比较、原图 SHA-256 比较均一致：6 张画布、23 条图片记录、18 个原始图片文件、3 条回收站记录完整保留。原 0.4.3 应用和生产数据已备份；操作性验收使用独立副本。

证据：

- [正式窗口](../artifacts/native-gallery-polish-20261106/gallery-installed.png)
- [窄窗口](../artifacts/native-gallery-polish-20261106/gallery-1000x680.png)
- [悬停说明](../artifacts/native-gallery-polish-20261106/tooltip.png)
- [画布导航](../artifacts/native-gallery-polish-20261106/canvas-installed.png)
- [数据校验](../artifacts/native-gallery-polish-20261106/preservation.json)
- [版本安装记录](../artifacts/native-gallery-polish-20261106/install-receipt.json)
- [测试日志](../artifacts/native-gallery-polish-20261106/tests.log)、[静态检查](../artifacts/native-gallery-polish-20261106/vet.log)、[构建日志](../artifacts/native-gallery-polish-20261106/build.log)

本轮为视觉与操作呈现调整，没有扩大到生成能力或 GPU 性能验收。
