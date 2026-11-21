# Spatial. 交互 / UI 复现规格 —— 交给 Codex 照抄用

> 目标：用 **macOS + AppKit（Swift）** 复刻 Spatial. 的界面与交互动效。
> 本文件所有**数值均为对原 App 的实测结果**（静态逆向类/参数 + lldb 运行时抓 CAAnimation + 官方 60fps 演示逐帧计时）。
> **把本文件 + `MotionKit.swift` 一起丢给 Codex，并说：严格按 `CODEX-REPRO.md` 实现，不要自创动效数值。**
> 唯一未逐方法实测的是「卡片选中放大 `resetScaleWithStiffness:damping:`」的入参（需真实点击触发），已用同一套 CA 实测值覆盖，见 §6.3 备注。

> **技术栈无关说明**：§2 设计 Token、§6 弹簧/时长实测参数、§4 组件解剖、§5 手势状态机都是**语言无关**的，可直接照搬到任意原生技术栈（Go/C++/Rust + 各自窗口框架）。§3 与 `MotionKit.swift` 仅给出 **Swift/AppKit 参考实现与公式**，换栈时用等价实现（如自绘/CADisplayLink 换成目标框架的渲染循环与动画时钟）即可，**但弹簧三参数与颜色/字体/尺寸不要改**。

---

## 0. 给 Codex 的硬性约定（先读）

1. **平台/框架**：macOS 14+，**Swift + AppKit**（`NSView` / `NSViewController` / `NSWindow` / `NSScrollView` / `NSTextView`），**Core Animation**，手势动画用 **`CADisplayLink` 驱动的自绘动画器**（见 `MotionKit.swift` 的 `DisplayLinkAnimator`）。不要用 SwiftUI（原作就是 AppKit 命令式代码）。
2. **两种动画引擎并存**（必须都实现）：
   - **A. CA 弹簧动画**：`CASpringAnimation` 挂在 layer 上，用于绝大多数过渡（淡入淡出、位移、缩放、阴影、边框、圆角）。
   - **B. DisplayLink 手绘动画器**：用于**手势驱动**的高频互动（拖拽惯性、缩放橡皮筋、吸附回弹、画布相机、Hover 视差）。用 `MotionKit.swift` 的弹簧积分器。
3. **数值不许改**：所有时长/刚度/阻尼/mass/颜色/字体/圆角都从本文件 §2、§6 取常量；集中放在 `SpatialTokens`（见 MotionKit）。
4. **目录结构对齐原作**（便于对照）：`Canvas/`、`Canvas/Transitions/`、`Canvas/Resizing/`、`Components/`、`Notes/`、`Popper/`、`Sidebar/`、`Panels/`、`Paywall/`、`Unboxing/`、`Cloud/`、`MCP/`、`Webclip/`。
5. **每个组件交付三样**：视图代码 + 状态机（§5）+ 通过 §7 验收清单。
6. **性能**：卡片数量大，缩放时用 `ZoomRasterGate` 思路（`isPinching` 时降级 layer 栅格化，`layer.shouldRasterize = true` 且 `rasterizationScale = 1`）。

---

## 1. 动效总原则（Motion Language）

- **常规微交互**：`0.3–0.5s`（悬停阴影 0.4、选中边框 0.3、圆角 morph 0.5）。
- **移动/布局**：`0.6–0.8s`。
- **大转场**（卡片抬起→全屏、开屏）：`0.9–1.5s`。
- **按压反馈**：按下 `scale 0.95→0.9`、`duration 0.05`；抬起回弹（网站实测 `whileTap scale 0.9 / 0.05s`）。
- **曲线哲学**：默认「**无回弹的平滑 ease**」（过阻尼弹簧）；只有**拖拽甩出/甩入、圆角 morph**这类才给有限阻尼的回弹（见 §6.2）。
- **空间感**：卡片=纸片（圆角+柔和投影），层叠靠 `zPosition` + 顶部彩色条；画布=相机（缩放/平移带惯性）。
- **一切"出现/消失"**：`opacity` 与 `transform` 一起做；卡片进出至少 50ms 位移+淡入。

---

## 2. Design Tokens（照抄常量）

### 2.1 颜色（浅/深双主题 + 点缀色）
```swift
extension NSColor {
  // 浅色画布（磨砂米白）
  static let canvasLight      = NSColor(hex: 0xF5F6F6)
  static let canvasLightAlt   = NSColor(hex: 0xE8EAEB)
  static let cardLight        = NSColor(hex: 0xFFFFFF)
  static let strokeLight      = NSColor(hex: 0xDFE3E6)
  static let textLight        = NSColor(hex: 0x111417)
  // 深色画布（磨砂近黑）
  static let canvasDark       = NSColor(hex: 0x16181A)
  static let canvasDarkAlt    = NSColor(hex: 0x1F2124)
  static let cardDark         = NSColor(hex: 0x26282C)
  static let cardDarkAlt      = NSColor(hex: 0x2B2E33)
  static let textDark         = NSColor(hex: 0xF2F3F5)
  static let textSecondary    = NSColor(hex: 0x75777B)
  // 强调/点缀
  static let linkBlue         = NSColor(hex: 0x3D6BFF)
  static let linkBlueBright   = NSColor(hex: 0x0A84FF)
  static let accentGreen      = NSColor(hex: 0x8CF6A1)
  static let accentGreenDeep  = NSColor(hex: 0x3DDC84)
  static let accentYellow     = NSColor(hex: 0xFFD60A)   // 浅色主题的高亮标签 chip
  static let accentOrange     = NSColor(hex: 0xFF9F0A)
  static let accentRed        = NSColor(hex: 0xFF453A)
  static let accentCyan       = NSColor(hex: 0x64D2FF)
}
```
**规则**：浅色主题卡片=白底+柔和阴影；深色主题卡片=深灰底+细边框+极浅阴影。标签 chip：浅色主题用 `accentYellow` 高亮底、深色主题用 `linkBlue`。

### 2.2 字体（原字体名 → 用途）
- **UI 无衬线**：`Unica77LL`（主）/ `Eina03`（Light/Regular/SemiBold/Bold）。→ 标题 `Eina03-SemiBold/Bold`，正文 `Eina03-Regular`。
- **长文阅读衬线**：`Volkhov-Regular/Bold`（"Paragraphs" 长文模式正文用它）。
- 系统兜底：`.systemFont` with `.rounded`? 否，用上述字体，缺失时 `-apple-system`。
- 字号（pt）：卡片标题 15–17 bold；卡片正文 12.5 常规，行高 18；长文标题 34–40 bold、正文 16–17、行高 26；侧栏 13；标签 11。

### 2.3 圆角 / 阴影 / 间距
```
cardCornerRadius      = 16      // 14–18，按卡片尺寸 14/16/18
cardPadding           = 16
cardGap               = 16
shadow(light)         = color #000 12% , radius 24 , offset (0,6)   // 大而软
shadow(dark)          = color #000 35% , radius 10 , offset (0,2) + 1px 细边框 #FFFFFF 6%
borderSelected        = 2.0 pt (selected) / 1.0 pt (hover)
```
**卡片解剖**：无标题栏、无分隔线，纯圆角矩形 + 标题 + 内容；投影必须"软、大扩散、低不透明度"。

---

## 3. 动画工具库（直接抄，`MotionKit.swift`）

### 3.1 核心弹簧工厂（与原 App 同款配方）
原 App 规律：`stiffness = 100` 固定、**`mass = duration²`**、`damping = DBL_MAX`（过阻尼=不回弹）、`v0 = 0`。这样自然角频率 `ω = √(k/m) = 10/duration`。

```swift
enum Spring {
    /// 「无回弹平滑」—— 复刻原 App 的默认过渡（faithful）
    static func smooth(_ keyPath: String, duration: Double, from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: keyPath)
        a.stiffness = 100
        a.mass      = duration * duration
        a.damping   = .greatestFiniteMagnitude       // ← 原 App 实测就是这个值
        a.initialVelocity = 0
        a.duration  = duration
        if let f = from { a.fromValue = f }
        if let t = to   { a.toValue = t }
        return a
    }
    /// 如果 smooth() 在你机器上观感发死（DBL_MAX 过阻尼），用临界阻尼等价：
    ///   damping = 2*sqrt(stiffness*mass) = 20*duration
    static func smoothCriticallyDamped(_ keyPath: String, duration: Double) -> CASpringAnimation {
        let a = smooth(keyPath, duration: duration)
        a.damping = 20.0 * duration
        return a
    }
    /// 回弹版（拖拽/甩出/圆角 morph）
    static func bouncy(_ keyPath: String, from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: keyPath)
        a.stiffness = 24; a.damping = 1.6; a.mass = 0.1; a.initialVelocity = 16
        a.duration = a.settlingDuration
        if let f = from { a.fromValue = f }
        if let t = to   { a.toValue = t }
        return a
    }
    static func radius(_ keyPath: String = "cornerRadius") -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: keyPath)
        a.stiffness = 400; a.damping = 100; a.mass = 1; a.initialVelocity = 2
        a.duration = a.settlingDuration
        return a
    }
}
```
把动画挂到 layer：`layer.add(Spring.smooth("opacity", duration: 0.4), forKey: "fade")`，并**同时把 model value 设好**（`CATransaction.setDisableActions(true)` 里改 `layer.opacity=to`）。

### 3.2 手势用的弹簧积分器（`DisplayLinkAnimator`）
见 `MotionKit.swift`：一个 `CADisplayLink` 驱动、对任意 `CGFloat`/`CGPoint` 做**半隐式欧拉**弹簧积分，支持 `stiffness / damping / mass / initialVelocity`，到稳态自动停。用于拖拽惯性、橡皮筋、吸附、相机缩放。

```swift
// 半隐式欧拉一步（dt 秒）：
// x'' = (-k*(x-target) - c*v)/m
func step(x: inout CGFloat, v: inout CGFloat, target: CGFloat, k: CGFloat, c: CGFloat, m: CGFloat, dt: CGFloat) {
    let a = (-k*(x-target) - c*v)/m
    v += a*dt
    x += v*dt
}
```
停止条件：`abs(x-target)<0.01 && abs(v)<0.5`。

### 3.3 常用复合动画（封装）
```
fade(layer, to, dur)          -> opacity，Spring.smooth(dur)
move(layer, to, dur)          -> position，Spring.smooth(dur)
scalePop(layer, to, dur=0.5)  -> transform.scale，Spring.smooth(dur)
shadow(layer, to, dur=0.4)    -> shadowOpacity/Radius/Offset，Spring.smooth(0.4)
border(layer, to, dur=0.3)    -> borderWidth，阻尼 100/100/0.1（回弹轻微）
radiusMorph(layer, to)        -> cornerRadius，Spring.radius()（0.5s, v0=2）
```

---

## 4. 组件规格（Anatomy + 动效 + 代码）

### 4.1 窗口 / 画布材质
- **无标题栏观感**圆角窗口，左上保留红黄绿交通灯（`NSWindow` `.titled/.fullSizeContentView` + `titlebarAppearsTransparent=true`，`titleVisibility=.hidden`）。
- 画布面板 = **磨砂玻璃**：`NSVisualEffectView(blendingMode: .behindWindow, material: .underWindowBackground/.hudWindow)` + 圆角 `layer.cornerRadius = 18, masksToBounds`。（macOS 26+ 可用 `NSGlassEffectView`。）
- 内容面板整体轻微内缩（inset 8–12），浮在桌面上。

### 4.2 画布 Canvas（相机）
类对齐：`CanvasView`→`CanvasScrollView`→`CanvasContentView`（放所有 item）+ `CanvasPincher`/`CanvasScroller`。
- **平移**：`NSScrollView` + `CanvasScrollView.mouseDownCanMoveWindow=false`；双指/滚轮滚动。
- **缩放（相机）**：`NSScrollView.allowsMagnification = true`，捏合 → `magnify(with:)` 更新 `magnification`；范围建议 `0.2 … 4.0`。缩放动画若需程序化（`zoomIn:`/`zoomOut:`）用 `SpaceAnimator`：`spring(stiffness, damping, mass, scale)`，配合 `scrollView.animator().magnification`。
- **缩放性能**：`ZoomRasterGate`：`pincher` 开始时 `isPinching=true`，卡片 `container.layer.shouldRasterize=true; rasterizationScale=1`；结束时恢复。阴影随缩放阈值开关：`minMagnificationForShadow/maxMagnificationForShadow`。
- **双击空白**：可新建卡片（原 App 有 CanvasTypePlayground/新建）。
- **画布状态**：保存 `magnification` + `contentOffset`（`Canvas.storedMagnification/storedOffset`）。

### 4.3 卡片基类 `CanvasItemView`（所有卡片的父类，最重要）
**状态机**：`.normal → .hover → .selected → .focused → .dragging → .deleting`。
每张卡片持两层阴影（`shadowLayer1/2`）+ 一层 `fadeLayer` + `highlightBorder` + `container`（内容）。参数与动画：

| 状态/效果 | 动画 | 参数（实测） |
|---|---|---|
| 悬停（hover） | 阴影加深 + 轻微上浮 | `shadowOpacity/Radius/Offset`：`Spring.smooth(0.4)`，`stiffness=100,damping=10,mass=0.16`（轻微阻尼）|
| 悬停高亮 | 边框 0→1pt | `borderWidth`：`stiffness=100,damping=100,mass=0.1`，0.3s |
| 选中（select） | `transform.scale` 1→1.04 + 边框 2pt + 阴影加深 | 见 §6.3（scale 用弹簧，duration≈0.5）|
| 取消选中 | 反向回弹 | 同上反向 |
| 进入聚焦/抬起 | 阴影拉开 + 缩放 | 用 `CanvasTransitionView`（§4.14）|
| 删除 | 淡出 + 缩小 + 杀阴影 | `fadeOutWithIntensity:killShadow:`：opacity→0, scale→0.9, `Spring.smooth(0.3)` |
| 被拖拽 | 抬起阴影 + zPosition 提升 | 拖起时 `shadowOpacity` ↑，`zPosition = 100` |

**方法对齐原名**（Codex 照此命名，便于对照）：
```
addScale(intensity:) / resetScale(stiffness:damping:)
addShadow(intensity:mass:) / resetShadow(mass:) / killShadow(mass:)
highlightForSelection(intensity:borderIntensity:) / unhighlight()
fadeOut(intensity:killShadow:) / resetFade(addBackShadow:)
willAnimate(size:curve:damping:stiffness:mass:)
```

### 4.4 文本卡 / 长文笔记（Notes）
类：`CanvasNotesView`（卡片态）+ `CanvasTransitionNotesView`（打开态）+ `NotesBlockTextView`（TextKit 编辑器）。
- 卡片态：标题粗体 + 正文（截断），纸片投影；重复纸片感可用 `stackedPaper`（背后多一层淡纸）。
- 打开（长文）：见 §4.14 转场；进入编辑后正文用 **Volkhov** 衬线、16–17pt、行高 26、最大行长 ~640pt 居中；标题 Eina03-Bold 34–40。
- 光标：自定义 `CustomCaret`（细竖线宽 2pt，闪烁周期 ~0.5s 用 `CAKeyframeAnimation opacity 1→0`），不要用系统 caret。
- 块编辑：每个 block 是独立 `NotesBlockTextView`；块类型菜单用 `Popper`（§4.11）。

### 4.5 清单 Checklist / Todo
- 每项左侧圆形复选框：未选=空心描边（1.5pt，`strokeLight`），选中=实心填充（`textLight` 或品牌色）+ 对勾。
- 勾选动画：`CASpringAnimation("transform.scale") 0.8→1.0`（`Spring.bouncy` 轻），叠加 `backgroundColor`/`borderWidth` `Spring.smooth(0.25)`；**勾选后文字加删除线并淡化**（`opacity 0.55`）。
- Todo 分区标题：`Today / Tomorrow / One of these days`，小号大写灰字。

### 4.6 便利贴 Sticky
- 彩色小方片（`accentGreen/accentCyan/accentOrange` 低饱和），圆角 10，无阴影或极浅；内容 `StickyTextView`。
- 支持轻微旋转（`Canvas.stickyRotations` 记录每张旋转角，-4°…+4° 随机）。
- 新建/编辑：双击进入编辑，caret 用 `CustomCaret`。

### 4.7 图片卡 / 视频卡
- 图片：`NSImageView`，圆角裁切，`imageScaling = .scaleProportionallyUpOrDown`，卡片按图片比例高。
- 视频：`AVPlayerLayer` + `AVPlayerLooper`（循环）+ `CustomQueuePlayer`；点播放/暂停，进度条 `CanvasVideoProgressBar`（细条，淡入淡出 0.2s）。
- 悬停显示微小元数据（尺寸/文件名，`textSecondary`，淡入 0.2s）。

### 4.8 Web Clip 卡（杂志式）
类：`CanvasWebClipView` + `WebClipSnapshotter` + 封面层 `CanvasWebClipCoverLayer`。
流程（对应演示）：
1. 顶部一个 URL 输入药丸（`NSTextField` 圆角胶囊，"https://" 前缀）。
2. 逐字输入（typewriter，`typingSpeed≈0.03–0.05s/char`）。
3. 提交后用隐藏 `WKWebView` 加载 → `WKWebView.takeSnapshot` 截图 → 生成封面。
4. 卡片 pop-in：`opacity 0→1 + transform.scale 0.96→1`，`Spring.smooth(0.3)`；内容=顶部视频/截图 + 下方网页截图（竖排"杂志页"）。
- 抓取超时 `timeOutDuration≈15s`，失败显示 `fallBackLayer`。

### 4.9 Stack（叠卡）
类：`Stack` + `StackBuilder` + `CanvasStackDropAnimator` / `CanvasItemsAnimator`。
- **外观**：多张卡在 z 轴层叠，每层向上错位 `±4px`、向右 `±3px`，**顶部露出彩色书脊条**（不同层不同色：绿/蓝/紫/红，从 palette 取），右下角显示 `"N items"` + 标题；可附图标。
- **收拢动画**（散卡 → 叠）：所有卡 `position/transform` → 目标叠位，`Spring.smooth(0.2)`（快），配合 `zPosition` 递增、`scale` 逐层略减；从远到近依次落位（stagger 30–40ms）。
- **展开动画**（叠 → 散开）：反向，`CanvasUnfoldAnimator`：按网格 `spacing/gutter` 展开（`spacing≈16, gutter≈16`），逐张 `Spring.smooth(0.25)` + 小延迟。
- **拖拽成叠**：拖到另一叠上出现 ghost 轮廓（虚线 1.5pt），松手触发 `CanvasStackDropAnimator`。

### 4.10 文件夹 Folder（拆箱）
- 外观：一个"文件夹"底形（`LandscapeFolderShape/PortraitFolderShape`）+ 盖子（`...LidShape`）+ 内容缩略。
- 打开/合上：盖子绕顶边旋转 ±(约 35°) + 内容上浮，`Spring.smooth(0.4)`；`CanvasFolderTransitionView` 负责进入文件夹画布（zoomOutAnchor/zoomOutScale 参数）。

### 4.11 Popper 浮动块菜单（长文选中后弹出）
类：`Popper` + `PopperItemView` + `PopperDivider`。
- 触发：文本选区 → 在选区上方/下方弹出 `NSPanel`（无边框、圆角 12、磨砂底）。
- 出现：`scale 0.92→1 + opacity 0→1`，`Spring.smooth(0.18)`（快，像原生菜单）；锚点跟随选区。
- 菜单项：`Design / Highlight / Heading / Subtitle / Bullet / Number / Task / List`；每项 `PopperItemView`：`fillColor`（默认透明）/ `hoverColor`（浅灰或品牌浅色）/ `hoverBorderColor`；hover 时高亮 `Spring.smooth(0.12)`。
- 键盘上下选择：`selectedItem` 高亮块 `Spring.smooth(0.1)` 在项间滑动（共享位置，类似 layoutId）。
- 点击外部/ESC 关闭：`scale 1→0.96 + fade`，0.12s。

### 4.12 侧栏 Sidebar
类：`CanvasSidebar`/`BookmarksSidebar`/`SearchSidebar`/`ChecklistSidebar` + `SidebarResizer`。
- 从左侧滑出 `position.x` `Spring.smooth(0.25)`，背景变暗 `dimming` opacity 0→0.15（0.2s）。
- 宽度可拖拽（`SidebarResizer`），松手吸附到档位。
- 列表项 hover 底色淡入 0.12s；选中项左侧 3pt 品牌色指示条 + 背景 0x…浅色。

### 4.13 顶部 / 底部控件
- 左上：`‹` 返回、文档图标、历史(时钟)、`✓` 完成（胶囊按钮组，磨砂底，圆角 10，按钮 hover 0.12s 高亮，按下 scale 0.95）。
- 左下：`✨` 智能（MCP）。
- 右下：视图 glyph、收起 `⌃`、新建 `+`（同样胶囊组）。
- 按钮按压统一：`whileTap scale 0.9, duration 0.05`。

### 4.14 卡片"抬起 → 全屏"转场（`CanvasTransitionView`，招牌）
这是原 App 最核心的过渡（`CanvasTransition*` 一整套）。流程：
1. **present**：卡片从画布位置/尺寸 → 全屏容器位置/尺寸，同时背景加 dimming（0→0.35）、卡片阴影拉开、圆角 morph。
2. 参数：进入用 `curveIn / dampingIn / massIn`，退出用 `curveOut / dampingOut`，可用 `massMultiplier` 缩放整体手感（大卡片更"重"）。`itemScaleWhenPresented`≈1.0，`shadowShapeOpacity` 控制抬起阴影。
3. 用 DisplayLink 驱动 position/size/transform 同步插值（比纯 CA 更可控）；时长手感 ~0.9–1.2s，回弹微小。
4. **关闭手势**（`CanvasTransitionSwiper`）：下滑/捏合关闭，橡皮筋物理：
   - `swipeDamping`、`pinchDamping`（跟手阻尼）；反向 `oppositeSwipeDamping/oppositePinchDamping`；
   - 阈值 `swipeThreshold`（约 120pt 或速度阈值）、`pinchThreshold`；
   - 超过阈值 → 完成关闭（`curveOut/dampingOut`）；未过 → 回弹归位（`animateBackPosition`，`Spring.smooth(0.3)`）；
   - 过程中 `swipeScale` 随下拉缩小（1→0.9）。

---

## 5. 手势 / 交互状态机（逐步）

### 5.1 悬停（Hover）
`mouseEntered` → 阴影 `addShadow(intensity:1)` `Spring.smooth(0.4)`（damping 10）→ `mouseExited` → `resetShadow()`。位移视差：`HoverTranslationAnimator`（`velocityMultiplier≈0.15, minimumTranslation≈-2, maximumTranslation≈2`），跟鼠标微移分层视差。

### 5.2 点选（Select）
`mouseDown` → 记录起点；`mouseUp` 位移 < 4pt 视为点击 → `selected=true` → 卡片 `scale→1.04`（弹簧，§6.3）+ 边框 2pt；点空白 → `CanvasDeselector` 取消选择（反向）。

### 5.3 拖拽（Drag，含惯性）
`mouseDown`→`mouseDragged`：卡片跟手 `position`（`CATransaction disableActions` 直接设）；记录最近 N 帧速度；`mouseUp`：
- 若速度 > 阈值 → `CanvasMovePullOutInertia`/`CanvasMoveAnimator`：用 `Spring.bouncy("transform")`（24/1.6/0.1/v0=16）继续滑行 + 落位；拉出一定距离（`pullThreshold`）可"抽出"到别处/抽出成独立卡。
- 无速度 → `Spring.smooth(0.7)` 落位。
- 吸附：靠近其它卡边/中线出现 `CanvasItemSnapGuide`（细线，淡入 0.1s），落位吸附 `StretchSnapAnimator(omega, restDistance)` 微弹。

### 5.4 缩放卡片（Resize）
拖拽角/边手柄（`CanvasResizeHandles`）：
- 跟手 + **橡皮筋**（`CanvasResizeAnimator.rubberStrength`）：超出最小尺寸时按 `1/(1+excess/rubber)` 衰减。
- `CanvasResizeSnapper`：接近其它卡尺寸时吸附（`minimum/maximumStretchWidth/Height`）。
- 松手：`StretchSnapAnimator(omega≈14, restDistance)` 回弹到整数/吸附尺寸。
- 显示尺寸标签 `CanvasResizeSnapHighlight`（magnification、dimensionLines，0.15s 淡入）。

### 5.5 画布缩放/平移
见 §4.2。捏合跟手；停止后惯性（`CanvasScroller`，`velocityMultiplier`）。

### 5.6 勾选 / 输入 / 光标
见 §4.5、§4.4。typewriter `typingSpeed 0.03–0.05s`。

---

## 6. 实测参数速查表

### 6.1 各交互时长（来自官方 60fps 演示逐帧，ms）
| 交互 | 时长 |
|---|---|
| 卡片新建滑入 + 吸附 | 166–417 |
| 勾选填充 | ~117 |
| 叠卡收/展 | 150–200 / 次 |
| 主题色/内容擦除 | 84–183 |
| Web clip 内容 pop-in | ~316 |
| 浮动菜单出现 | 83–266 |
| 画布相机加速段 | 83–283 |

### 6.2 CA 弹簧实测（lldb，40 组代表值）
| keyPath | 场景 | stiffness | damping | mass | v0 | 时长感 |
|---|---|---|---|---|---|---|
| transform | 拖拽/甩出惯性 | **24** | **1.6** | **0.1** | **16** | ~0.92s 回弹 |
| cornerRadius | 圆角 morph | **400** | **100** | **1** | **2** | 0.5s |
| borderWidth | 选中边框 | 100 | 100 | 0.1 | 0 | 0.3s |
| shadowOpacity | 悬停阴影 | 100 | 10 | 0.16 | 0 | 0.4s |
| opacity / position / transform / transform.scale / bounds.size.height / backgroundColor / cornerRadius | 通用过渡 | 100 | DBL_MAX | **duration²** | 0 | 0.4–1.5s |

> 通用过渡 mass↔duration：`0.4→0.16 · 0.5→0.24 · 0.6→0.32 · 0.7→0.48 · 0.8→0.64 · 1.0→1.0 · 1.2→1.6 · 1.5→2.56`（≈ duration²）。

### 6.3 卡片选中放大（补）
`resetScaleWithStiffness:damping:` 需真实点击触发，未逐方法实测；**用它同族规则即可**：`transform.scale`，`Spring.smooth(duration: 0.5)`（即 stiffness100/mass0.25/damping DB或临界 20*0.5=10）；若想要一点"Q弹"用 `Spring.bouncy` 的缩小版（stiffness 300 / damping 20 / mass 1，不设 v0）。

---

## 7. 验收清单（Codex 自测）
- [ ] 窗口磨砂圆角、交通灯透明标题栏；浅/深主题切换正确。
- [ ] 卡片：白/深底、圆角16、软阴影；hover 阴影 0.4s 变化；选中 1→1.04 + 2pt 边框。
- [ ] 拖拽有跟手 + 松手惯性（24/1.6/0.1/v0=16）+ 吸附细线。
- [ ] 缩放有橡皮筋 + 尺寸吸附 + 尺寸标签。
- [ ] 画布捏合缩放（0.2–4.0）+ 缩放时栅格化降级；双击空白新建。
- [ ] 勾选：圆形→实心 + 对勾弹簧 + 文字删除线淡化。
- [ ] 叠卡：彩色书脊 + "N items"；收/展 150–200ms、错位层叠。
- [ ] Web clip：输入→抓取→杂志页 pop-in 0.3s。
- [ ] 长文：Volkhov 衬线、自定义 caret 闪烁、选中弹 Popper 块菜单（0.18s）。
- [ ] 卡片点开全屏转场 0.9–1.2s + 下滑/捏合关闭橡皮筋。
- [ ] 所有按钮按压 scale 0.9 / 0.05s。
- [ ] 60fps（CADisplayLink 驱动，无卡顿）。

## 8. 数据来源 / 可信度
- **实测**：弹簧参数（lldb 抓 CAAnimation）、类/结构（otool -ov）、字体与颜色（资源+截图取样）、各交互时长（逐帧）。
- **推断**（工程常识补全）：具体圆角 14/16/18 的分配、部分手势阈值（swipeThreshold 等）为经验值，可按 §7 观感微调，但**不要动 §6 的弹簧三参数**。
