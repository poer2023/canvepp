# Canvepp 设置设计稿验收

「助手输入条」画板随系统切换明暗。点 **Play** 可查看 ASCII 字符绘制的整面曲线和图标。轮廓持续弯折，键入时短暂回弹。点顶栏画布名称可切换目标。正文从两侧均可点击输入，顶栏和底栏的留白可拖动整个输入条。

在 Baoyu 的全屏预览里，从正文左边缘直接输入了文字，切到「海报构图」后草稿和示例附件仍在。顶栏拖动改变了窗口位置，关闭再打开后恢复草稿、目标和正文焦点。

## A 版修订与助手输入条

A 的主题行改用三张可点击的工作区缩略图。点击「登录」可预览账号样式，「添加密钥」可预览保存后的管理菜单。在 A 版的 **Play** 模式中点「打开助手」，可切到新的助手输入条画板。原生窗口验收与明暗截图见 [A 版与全局助手输入条](../../../docs/native-settings-dialog-acceptance.md#a-版与全局助手输入条)。

## 上一轮三版对照

- 以现有首页侧栏、设置窗和 [Canvepp 界面规范](../../../docs/interface-design.md) 为参照。交付三张可编辑的 Baoyu 设置画板，画板的显示尺寸为 850 × 704，最小布局为 640 × 430。
- 保留独立窗口的标题区、通用／数据／快捷键分类、原有设置项与行内状态。采用白色或深灰内容面、实色按钮、线性图标及必要细线。主题和额度选择显示对应图标，当前值在展开时带勾。
- A 用同一列排列设置，操作在右侧对齐。B 让通用的模型额度、数据的应用版本和快捷键的画布操作占据右栏，窄窗折成单栏。C 在通用下提供三个可直达的分组，点击通用可返回总览。
- 点击播放后可切页、切主题、试开关和切换两种额度的来源。登录、系统权限、备份及凭据区域使用演示状态。画板独立于 Canvepp 正式应用。
- 用 Baoyu Canvas 检查项目格式。在实际编辑器确认三张画板、分类切换、主题和额度控件；核对常用与最小尺寸的正文滚动和控件可达性。

## 验证结果

Baoyu Canvas 的格式检查识别出 3 张画板，问题数为 0。三版均在 850 × 704 切换浅色和深色，在 640 × 430 滚动至密钥操作区。A 验证了键盘展开主题、分类切换和生图／摘要额度独立修改。B 验证了两栏在窄窗折回单列。C 验证了选择分组和返回通用总览。

- A [浅色](../exports/A-light.png) · [深色](../exports/A-dark.png)
- B [浅色](../exports/B-light.png) · [深色](../exports/B-dark.png) · [640 × 430](../exports/B-640x430.png)
- C [浅色](../exports/C-light.png) · [深色](../exports/C-dark.png) · [额度分组](../exports/C-funding.png)

## 本机预览

在画板名称旁点击 **Play**，可在画布内或全窗操作。浅色总览见[三版对照](../exports/comparison.png)。[打开 Baoyu 画布](http://127.0.0.1:4720/design/i2FZ5S4RegMPVgJR5SKOaj/)

预览由本机的 Baoyu 编辑器进程（PID 72027）提供。日志见[编辑器日志](../.artifact-server.log)。

在 `/Users/hao/Documents/ChatGPT/canvepp` 重开预览。

```sh
node "/Users/hao/project/baoyu-design/chatgpt-plugins/baoyu-canvas/skills/claude-design/scripts/artifact.mjs" open designs/canvepp-settings
```

用完后可关闭该画布的本地服务。

```sh
node "/Users/hao/project/baoyu-design/chatgpt-plugins/baoyu-canvas/skills/claude-design/scripts/artifact.mjs" stop designs/canvepp-settings
```
