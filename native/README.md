# Canvepp Native 0.6.11

MyGo 0.7.4 + Go 1.27.3 的原生 GPU 桌面应用，macOS 使用 Metal。画布和应用界面原生渲染；Pinterest 官方网页直接嵌在主窗口右侧的系统 WebView；浏览器 OAuth 与原生收藏网格作为次级入口；不运行 React 或 Node 服务。旧 Web 版和原库保持独立。

侧栏默认宽 244 pt。拖动右边缘调宽，点击顶部侧栏图标可折叠或恢复。折叠时隐藏会话列表，可用搜索打开画布，重开应用后沿用上次布局。顶部是固定纵向菜单，中部「画布」标题右侧提供搜索、排序与新建，下方为独立滚动的会话。底部回收站支持恢复与永久删除。设置中的外观可选跟随系统、浅色或深色。控件与鼠标反馈遵循[项目界面规范](../docs/interface-design.md)。画布保留 28 pt 窗口拖动区、图片外悬停工具和左右连线。

## 运行和打包

本机成品在 `~/Applications/Canvepp Native.app`，双击打开。默认数据目录是 `~/Library/Application Support/CanveppNative`。首次数据迁移复制原库和图片，不修改原文件。

开发环境需要 Go；首次构建自动下载项目锁定的 Go 工具链和 MyGo 依赖：

```sh
cd native
go run .
go run . --import-legacy ../.data
go test -race ./...
go test -run '^$' -bench . -benchmem
./scripts/build-macos.sh
./scripts/build-windows.sh
```

脚本先在临时目录构建并签名，避免 iCloud / Finder 元数据污染签名；产物为 `native/build/CanveppNative-macos-arm64.zip`，解压到本地 Applications 目录使用。本地使用 ad-hoc 签名，未做 Apple 公证。

## 使用

- 将鼠标移到灰色窗口顶栏右侧的加号，可分别打开 Pinterest、Eagle、图片历史和操作历史。点击顶栏标签切换页面，拖动分隔条调整右栏宽度。
- 左侧「全部画布」按最近更新展示画布缩略图，点击可继续编辑。「全部图片」按入库时间混排列出不同画布的图片，可预览和定位。
- 底部锁形按钮锁定当前画布的节点位置，图片悬停工具条可单独锁定节点；再次点击解锁。两种锁定都支持继续创作。
- 直接拖入 PNG / JPEG / WebP，或使用系统文件选择器。
- 点击图片打开「来源 / 创作」悬浮抽屉。上方显示不可变的原始提示词、参考图片与尺寸；下方查看后续生成，没有记录和草稿时隐藏。点击创作图标编辑下一次提示词、比例、像素和 1–4 张输出。沿用历史提示词会保护已有草稿。框选多图直接进入创作，最多 8 张共同参考。
- Cmd/Ctrl+C、V 复制粘贴；支持画布内、画布间以及系统图片剪贴板；Delete/Backspace 将选中图片移入回收站，可点提示中的「撤销」。文本输入中的快捷键不会删除画布。
- Cmd/Ctrl+Z 撤销，Cmd/Ctrl+Shift+Z 重做；删除内容进入回收站，原图与版本仍保留。
- 空格或中键拖动平移，触控板滚动/捏合和鼠标滚轮均围绕指针缩放。原图预览支持捏合或 Cmd/Ctrl+滚轮放大，最高 400%。
- 每张生成结果是独立图片节点，保留来源连线。原图不被自动替换。

## Adobe 画布快照

保持 Canvepp 运行，在 Photoshop 文档或 Illustrator 活动画板内按 **⌘ ⌥ ⇧ 9**（macOS）或 **Ctrl + Alt + Shift + 9**（Windows）。首次使用时，macOS 可能会询问是否允许控制 Adobe 应用。快捷键及注册状态可在「设置 → 快捷键」查看。

Canvepp 将可见内容读入一张新画布。图层按原位置、显示尺寸及前后顺序排放，可以逐张移动。选中图片可在来源中看到组路径。Photoshop 剪贴组、有蒙版或效果的组，以及 Illustrator 剪贴组会合成单张透明 PNG。历史中另存整幅合成，便于比对调整图层、混合模式等依赖背景的效果。透明像素和画板边距随快照保留。

普通 Photoshop 图层按文档像素导出。智能对象和 Illustrator 置入图片按可读取的原尺寸提高采样，Illustrator 矢量默认按 300 ppi 输出。每张 PNG 遵循当前图片库的 4000 万像素上限。源文档保留在 Adobe 中，Canvepp 保存 PNG 与排布数据。桌面脚本宿主说明见 [Photoshop](https://helpx.adobe.com/photoshop/using/scripting.html) 和 [Illustrator](https://helpx.adobe.com/illustrator/using/automation-scripts.html)。

## ChatGPT 登录与 API 额度

助手附件支持图片、UTF-8 文本、PDF、DOCX、RTF 和 ODT。Windows 在隔离的 WebAssembly 运行环境读取 PDF 文字，旧版二进制 Word 文档可另存为 DOCX 后添加。macOS 使用系统文档解析器。附件文字上限为 128 KiB，PDF 上限为 200 页。扫描 PDF 可将相关页作为图片添加。[PDFium WebAssembly](https://github.com/klippa-app/go-pdfium)

Windows x64 和 macOS Apple 芯片安装包随附已校验的官方 Codex CLI。使用者可以直接在「设置 → 通用 → ChatGPT 连接」打开系统浏览器授权。若本机回调端口已被占用或保留，设置中会显示官方设备码，可复制到 OpenAI 授权页。Codex 在当前系统用户下管理登录态。开发构建也可以使用已安装的 CLI。

在「模型额度」中配置 OpenAI API Key，应用将密钥保存在本机系统凭据库。每项任务可以独立选择 ChatGPT 订阅或 OpenAI API，重启后沿用选择。选 API 时，生图与参考图编辑使用 `gpt-image-2.5-sunburst`，操作历史摘要使用 `gpt-6-luna`。选订阅时使用 Codex 内置生图或同名摘要模型。实际权限、限额与计费由所选账户和项目决定。任务提交后固定来源，失败不会切换额度。API 图片结果未确认时不会自动重发。

构建安装包需要 Python 3.11 或更新版本。首次构建会下载已锁定版本与 SHA-512 的官方 Codex 平台包。

```sh
go run . --check-account
```

结果卡片各自保存独立的不可变请求、线程和轮次。数据库原子领取避免重复提交；连接丢失、应用退出或回执不明确后，只查询原会话，不自动重新生图。取消后已返回的图片仍保留。重开应用会补回已保存但尚未关联的图片。旧版任务只导入历史，不重新排队。

## 性能与保护

可见区域裁剪、后台两路图片解码、按显示尺寸加载缩略图、64 MiB CPU 图片缓存预算；预览原图按需加载。历史画廊网格虚拟化，只加载可见缩略图，拖动只更新坐标，松手才形成撤销记录并后台保存。连线按屏幕像素细分并由 GPU 绘制，避免连续缩放时分配巨大路径蒙版；多选拖动复用选区身份，不逐帧复制卡片内容。

SQLite WAL + FULL synchronous，保存有版本保护。完整备份包含数据库、原图、历史、回收站和本地助手附件，并逐文件校验 SHA-256。图片保留原始字节与透明通道。「设置 → 数据」可以合并新画布，也可以将备份恢复到新的资料库。合并时跳过已有画布。恢复时保留备份时的画布 ID，新资料库可独立打开和继续编辑。系统凭据库、Codex 会话正文和缩略图缓存各自由原服务管理。

常规画布最多容纳 500 张图片，普通导入单图上限为 20 MiB。Adobe 画布可容纳原快照的全部图层，单张高分辨率 PNG 可超过 20 MiB。两种导入均使用 4000 万像素上限。

基础验收：`../docs/native-verification.md`。0.4.1 交互修复及实测：`../docs/native-interaction-verification.md`；0.4.2 悬浮界面合同：`../docs/native-floating-acceptance.md`，验收：`../docs/native-floating-verification.md`。

0.4.3 导航与画廊：[验收合同](../docs/native-gallery-acceptance.md)、[实机结果与数据保护记录](../docs/native-gallery-verification.md)。

0.4.4 视觉精简：无边框图片、图标操作和单层悬停提示。[验收记录](../docs/native-gallery-polish-verification.md)。

0.4.5 图片抽屉：来源与创作上下排列，以 200 ms 滑动切换；后续记录按图片版本查询，独立于画布连线，列表与缩略图按需加载。[验收合同](../docs/native-image-drawer-acceptance.md)、[实机验收](../docs/native-image-drawer-verification.md)。

0.4.6 画布视觉调整：默认创作，保留已有草稿；图片无常驻边框，单层选中轮廓；图片外悬停工具，左右侧柔和走线及相关路径强调。[验收合同](../docs/native-canvas-polish-acceptance.md)。

0.4.7 拖动与参数整理：图片裁剪改为屏幕坐标，避免移动时重复生成缩放遮罩；拖动期间延后切换详情。移除左下角图片设置按钮和标题竖线，比例使用九宫格，像素使用双输入，张数使用一排选择。[验收合同](../docs/native-drag-settings-acceptance.md)、[测量与验证记录](../docs/native-drag-settings-verification.md)。

诊断时可传 `--receipt /绝对路径/receipt.json`，在关闭时保存绘制耗时与真实拖动的输入/绘制间隔。该模式不注入输入、不改变帧调度；默认不开启拖动采样。这些时长不包含 GPU 呈现和远程传输，不能视作输入到屏幕的完整延迟。

0.4.8 精简图片设置：来源与创作使用单行标题，目标尺寸默认折叠，展开后保留比例宫格与像素输入；张数为一行选择。生成与下载、预览、删除在同一行，随内容滚动。目标像素只是发给 Codex 的生成意图，实际尺寸以回图为准，不等同于 Images API 的 size 参数。[验收合同](../docs/native-compact-inspector-acceptance.md)、[实机验证](../docs/native-compact-inspector-verification.md)。


0.5.0 工作区工具：所有侧栏搜索使用无硬边框的浅色输入；设置按通用、数据、快捷键分组。大图预览贴合内容区，透明背景与容器一致；画布上的透明图仅在悬停或选中时显示棋盘格。

- 右下角「操作历史」显示图片创建、导入、粘贴、生成、版本切换、连线和删除／恢复等关键变化。单击记录定位图片，多图记录可选目标。仍在回收站的图片会定位到对应条目。移动和输入仍可撤销，历史列表与摘要只读取关键操作，每次最多加载 500 条。
- 日志右上角星形按钮通过当前 Codex 订阅的 `gpt-6-luna` 总结最近最多 100 条，保留原始记录和摘要覆盖范围；同一批操作复用摘要。模型不可用会显示错误，不静默切换模型。
- 右下角对话气泡按钮或 `Cmd+Shift+Space` 打开助手输入条。Enter 发送、Esc 收起，回复记录在每张画布关联的 Codex 会话中，输入条上的跳转图标可打开完整会话。助手只提供文字建议，不声称修改画布或自动生图。
- 应用运行时，连按三次空格可全局唤起助手；需要 macOS「隐私与安全性 → 输入监控」授权，可在设置中关闭。只识别空格、其他按键/鼠标的中断以及时间，不保存键盘文字，不拦截原应用的按键。未授权仍可使用应用内按钮和快捷键。
- 回收站永久删除需要再次确认，不能撤销；仍被其他图片、版本来源或任务引用的素材继续保留。旧保存快照和撤销不会把已永久删除的卡片恢复出来。

[0.5.0 验收合同](../docs/native-workspace-tools-acceptance.md)、[验证记录](../docs/native-workspace-tools-verification.md)。

0.6.0 助手与素材库：

- 助手有可见关闭按钮及 Esc，无占位符。流动边框只在窗口可见时刷新；输入自动折行增高，Enter 发送、Shift+Enter 换行。关闭保留未发送草稿和附件。
- 回形针按钮或拖入可添加最多 8 个附件，每个 20 MB。图片直接交给 Codex；PDF、Word、RTF、ODT 和 UTF-8 文本先在本地提取正文后发送。文档最多 128 KB 正文，PDF 最多 200 页；不包含文档中的图片或版式。加密、纯扫描、损坏或不支持的文件明确报错，不会将文件名冒充实际内容。原始副本保留在应用数据目录，原文件移动后不影响已发送会话。
- 左下角素材库图标或设置 → Pinterest 与 Eagle 打开素材窗口。Pinterest 从 0.6.1 起使用系统浏览器授权和原生图板网格，见下方连接步骤。
- Eagle 使用本机 41595 端口的只读接口，支持搜索、分页与导入。启动 Eagle 并打开素材库后点连接；不改动 Eagle 原库。导入的图片复制到 Canvepp，保留来源，支持画布撤销与恢复。
- 导入目标在点击时确定，异步导入期间切换画布不会放错项目。原图导入后不依赖 Pinterest 或 Eagle 持续在线。

[0.6.0 验收合同](../docs/native-assistant-materials-acceptance.md)、[验证记录](../docs/native-assistant-materials-verification.md)。


## 0.6.1 Pinterest 浏览器授权

1. 打开左下角素材库 → Pinterest → 齿轮「连接设置」。在本机填写你获批应用的 App ID 和 App Secret。已有 Secret 留空可保留；两者和登录令牌仅存入 macOS 钥匙串，不进入画布备份。
2. 在 Pinterest 开发者后台登记完全相同的回调地址，默认 `http://localhost:47843/oauth/pinterest/callback`。可修改为其他本机端口；连接时端口须空闲。私密图板需勾选对应的只读权限，并由你的应用获得该权限。
3. 保存后点「连接 Pinterest」。默认浏览器打开官方授权页，完成后自动前置 Canvepp 主窗口、在右侧素材页读取图板；浏览器的回调页也有返回入口。拒绝、取消、超时和失效均可重新连接。
4. 左侧图板目录可筛选并加载更多，右侧是原生图片网格。点击图片看大图，点加号导入点击时的画布，保留原始字节、Pin 来源和撤销记录。断开连接只清除本机登录令牌，画布内容不受影响。若要撤销 Pinterest 端的授权，可在 Pinterest 的应用授权管理中处理。

API 和图片各自分页。每次最多解码 48 张缩略图，两路后台解码，网格虚拟化；多图 Pin 的静态图片会逐张保留并分页，视频不会被误当作图片。导入采用官方 API 返回的最大图片版本，不猜测原图地址。画廊不显示推荐流、广告或登录网页。

此版本支持个人在本机配置自己的开发者应用。公开分发时不应把共用 App Secret 编入安装包，应另行部署授权服务。官方依据：[授权流程](https://developers.pinterest.com/docs/getting-started/set-up-authentication-and-authorization/)、[官方 API 定义](https://github.com/pinterest/api-description/blob/main/v5/openapi.yaml)。

验收用 `pinterestqa` 构建标签启用本机假 OAuth 服务，正常构建不包含替换官方服务的入口；验收版拒绝使用生产默认数据目录。真实账号授权与收藏覆盖范围，须在填写真实凭据后验收。

[0.6.1 验收合同](../docs/native-pinterest-oauth-acceptance.md)、[验证记录](../docs/native-pinterest-oauth-verification.md)。

## 0.6.2 主窗口素材页

左侧固定菜单的素材库入口直接切换右侧内容区，不再创建独立窗口。Pinterest、Eagle、连接设置及 Pinterest 大图预览都在主窗口内显示；左侧目录保持可用，素材页右上角可返回画布。Esc 先退出大图或连接设置，再返回画布。切换页面保留画布视角、选择与内容，也保留素材连接、筛选及已加载页面。浏览器授权完成时返回这一主窗口。

[0.6.2 验收合同](../docs/native-embedded-materials-acceptance.md)、[验证记录](../docs/native-embedded-materials-verification.md)。


## 0.6.3 画布材质、空间动效与图标搜索

- 原生磨砂窗口背景，画布圆角 18、内缩 10；画布材质跟随系统浅/深外观。图片圆角 16，悬停阴影、选中 1.04 / 2pt、抬起层级、删除淡出；保留透明图的空闲透明背景。
- 指针按住时直接移动，不做追赶缓动。最近至少三次位置样本才判断甩出，使用 24/1.6/0.1/v0=16；最终落点立即保存为单次移动，显示层完成弹簧，撤销和异步回图互不覆盖。
- 图片角/边手柄按原比例调整画布显示尺寸，支持最小尺寸橡皮筋、其他卡片尺寸吸附及尺寸标签。原图字节、原始尺寸与生成参数保持不变。
- 滚轮/捏合围绕指针缩放 20%–400%；空格/中键平移。缩放按钮使用相机弹簧，连续缩放期间复用缩略图并隐藏阴影；双击空白新建图片。减少动态效果立即落位，静止时停止刷新。
- 颜色、字体名、半径、阴影与弹簧统一在 `spatial_tokens.go`。通用过渡记录 DBL_MAX，数值求解采用参考 §3.1 允许的临界阻尼；其余参数原值。MyGo 原生显示时钟负责动画，稳定解析求解避免高阻尼 Euler 发散；无需 Swift。
- 字体优先使用已安装字体、`CANVEPP_FONT_DIR` 或本机 Spatial.app 已有的 Unica77LL / Eina03 / Volkhov。缺失使用系统字体，不将第三方商业字体重新打包。中文继续使用系统字形回退。
- 画布、历史、Eagle 与授权图板的搜索默认只显示放大镜，点击展开，空值失焦收起，Esc 保留非空筛选并显示圆点；Pinterest 自身网页的搜索属于第三方页面。
- 素材库默认直接显示 Pinterest 官方网页，不要求先填写 App ID。图片悬停加号、右键或工具栏可导入，目标画布在点击时确定。登录图标进入素材库的登录流程。手机 Pinterest 可使用 Google 账号，在同一 Wi-Fi 下扫码并确认，即可登录正在显示二维码的素材库。网页版登录保存在本机的 Pinterest 网页分区。点击网页表单中的 Google 按钮会打开系统浏览器中的 Pinterest。继续选择 Google 后，网页与授权弹窗共用这个浏览器的会话。配置了 Pinterest 图板 API 的设备也可在浏览器授权后直接回到原生收藏。

[首轮验收合同](../docs/native-spatial-acceptance.md)、[网页和搜索合同](../docs/native-pinterest-web-search-acceptance.md)、[实机验证与数据完整性](../docs/native-spatial-verification.md)。未包含笔记、叠卡等无关组件。

0.6.4 将图片画布还原为 0.6.2 的绘制、布局和输入行为。图片以原有画布尺寸显示，选中后从边角拖动也只移动位置。画布缩放为 5%–400%。0.6.3 保存的单图尺寸元数据继续随画布保存。[还原与验收记录](../docs/native-canvas-restore.md)
