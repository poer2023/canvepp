// MotionKit.swift — Spatial 复现动效工具库（配合 CODEX-REPRO.md 使用）
// 目标：macOS 14+ / Swift / AppKit / Core Animation
// 所有数值来源于对 Spatial. 的实测（见 CODEX-REPRO.md §6）。
import AppKit
import QuartzCore

// MARK: - 颜色
extension NSColor {
    convenience init(hex: UInt32, alpha: CGFloat = 1) {
        self.init(srgbRed: CGFloat((hex >> 16) & 0xFF)/255.0,
                  green:   CGFloat((hex >> 8) & 0xFF)/255.0,
                  blue:    CGFloat(hex & 0xFF)/255.0,
                  alpha:   alpha)
    }
}

enum SpatialTokens {
    // 浅色
    static let canvasLight = NSColor(hex: 0xF5F6F6)
    static let canvasLightAlt = NSColor(hex: 0xE8EAEB)
    static let cardLight = NSColor(hex: 0xFFFFFF)
    static let strokeLight = NSColor(hex: 0xDFE3E6)
    static let textLight = NSColor(hex: 0x111417)
    // 深色
    static let canvasDark = NSColor(hex: 0x16181A)
    static let canvasDarkAlt = NSColor(hex: 0x1F2124)
    static let cardDark = NSColor(hex: 0x26282C)
    static let cardDarkAlt = NSColor(hex: 0x2B2E33)
    static let textDark = NSColor(hex: 0xF2F3F5)
    static let textSecondary = NSColor(hex: 0x75777B)
    // 点缀
    static let linkBlue = NSColor(hex: 0x3D6BFF)
    static let linkBlueBright = NSColor(hex: 0x0A84FF)
    static let accentGreen = NSColor(hex: 0x8CF6A1)
    static let accentGreenDeep = NSColor(hex: 0x3DDC84)
    static let accentYellow = NSColor(hex: 0xFFD60A)
    static let accentOrange = NSColor(hex: 0xFF9F0A)
    static let accentRed = NSColor(hex: 0xFF453A)
    static let accentCyan = NSColor(hex: 0x64D2FF)

    static let cardCornerRadius: CGFloat = 16
    static let cardPadding: CGFloat = 16
    static let cardGap: CGFloat = 16
}

// MARK: - 弹簧工厂（与原 App 同款：stiffness=100, mass=duration², damping=DBL_MAX）
enum Spring {
    /// 无回弹平滑过渡（faithful 复刻）
    static func smooth(_ keyPath: String, duration: Double,
                       from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: keyPath)
        a.stiffness = 100
        a.mass      = duration * duration
        a.damping   = .greatestFiniteMagnitude      // 实测原值
        a.initialVelocity = 0
        a.duration  = duration
        a.fromValue = from
        a.toValue   = to
        return a
    }

    /// 若 DBL_MAX 在你机器上观感发死，用临界阻尼等价（damping = 2√(k·m) = 20·duration）
    static func smoothCriticallyDamped(_ keyPath: String, duration: Double,
                                       from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = smooth(keyPath, duration: duration, from: from, to: to)
        a.damping = 20.0 * duration
        return a
    }

    /// 回弹版（拖拽/甩出）：实测 stiffness24/damping1.6/mass0.1/v0=16
    static func bouncy(_ keyPath: String, from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: keyPath)
        a.stiffness = 24; a.damping = 1.6; a.mass = 0.1; a.initialVelocity = 16
        a.duration = a.settlingDuration
        a.fromValue = from; a.toValue = to
        return a
    }

    /// 圆角 morph：实测 stiffness400/damping100/mass1/v0=2（0.5s）
    static func radius(from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: "cornerRadius")
        a.stiffness = 400; a.damping = 100; a.mass = 1; a.initialVelocity = 2
        a.duration = a.settlingDuration
        a.fromValue = from; a.toValue = to
        return a
    }

    /// 选中边框：实测 stiffness100/damping100/mass0.1（0.3s）
    static func border(from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: "borderWidth")
        a.stiffness = 100; a.damping = 100; a.mass = 0.1; a.initialVelocity = 0
        a.duration = a.settlingDuration
        a.fromValue = from; a.toValue = to
        return a
    }

    /// 悬停阴影：实测 stiffness100/damping10/mass0.16（0.4s）
    static func hoverShadow(from: Any? = nil, to: Any? = nil) -> CASpringAnimation {
        let a = CASpringAnimation(keyPath: "shadowOpacity")
        a.stiffness = 100; a.damping = 10; a.mass = 0.16; a.initialVelocity = 0
        a.duration = a.settlingDuration
        a.fromValue = from; a.toValue = to
        return a
    }
}

// MARK: - CALayer 便捷动画（自动同步 model value，避免回跳）
extension CALayer {
    func setModel(_ apply: () -> Void) {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        apply()
        CATransaction.commit()
    }

    func fade(to opacity: Float, duration: Double) {
        setModel { self.opacity = opacity }
        add(Spring.smooth("opacity", duration: duration), forKey: "fade")
    }

    func move(to point: CGPoint, duration: Double) {
        setModel { self.position = point }
        add(Spring.smooth("position", duration: duration), forKey: "move")
    }

    func popScale(to scale: CGFloat, duration: Double = 0.5) {
        setModel { self.transform = CATransform3DMakeScale(scale, scale, 1) }
        add(Spring.smooth("transform.scale", duration: duration), forKey: "pop")
    }

    func shadow(toOpacity o: Float, radius: CGFloat, offset: CGSize, duration: Double = 0.4) {
        setModel {
            self.shadowOpacity = o
            self.shadowRadius = radius
            self.shadowOffset = offset
        }
        add(Spring.hoverShadow(), forKey: "shadowOpacity")
        add(Spring.smooth("shadowRadius", duration: duration), forKey: "shadowRadius")
    }

    func morphRadius(to r: CGFloat) {
        setModel { self.cornerRadius = r }
        add(Spring.radius(), forKey: "cornerRadius")
    }

    func selectBorder(_ on: Bool, width: CGFloat = 2) {
        setModel { self.borderWidth = on ? width : 0 }
        add(Spring.border(), forKey: "borderWidth")
    }
}

// MARK: - 自定义闪烁光标（长文/便签）
extension CALayer {
    func startCaretBlink() {
        let blink = CAKeyframeAnimation(keyPath: "opacity")
        blink.values = [1.0, 1.0, 0.0, 0.0]
        blink.keyTimes = [0, 0.5, 0.5, 1.0]
        blink.duration = 1.0
        blink.repeatCount = .infinity
        add(blink, forKey: "caretBlink")
    }
}

// MARK: - DisplayLink 弹簧动画器（手势用：拖拽惯性 / 橡皮筋 / 吸附 / 相机）
/// 对单个 CGFloat 通道做半隐式欧拉弹簧积分；到稳态自动停。
final class DisplayLinkAnimator {
    private var link: CADisplayLink?
    private var lastTime: CFTimeInterval = 0

    // 状态
    var value: CGFloat
    var velocity: CGFloat = 0
    var target: CGFloat

    // 参数（可按 §6 传：拖拽 bouncy k=24 c=1.6 m=0.1 v0=16；吸附 k=100 c=? m=?）
    var stiffness: CGFloat
    var damping: CGFloat
    var mass: CGFloat

    var onUpdate: ((CGFloat) -> Void)?
    var onComplete: (() -> Void)?

    /// v0 为初速度（点/秒）
    init(value: CGFloat, target: CGFloat, stiffness: CGFloat, damping: CGFloat,
         mass: CGFloat, initialVelocity: CGFloat = 0) {
        self.value = value; self.target = target
        self.stiffness = stiffness; self.damping = damping; self.mass = mass
        self.velocity = initialVelocity
    }

    func start(in view: NSView) {
        stop()
        let l = view.displayLink(target: self, selector: #selector(tick(_:)))
        l.add(to: .main, forMode: .common)
        link = l
        lastTime = 0
    }

    func stop() {
        link?.invalidate(); link = nil
    }

    @objc private func tick(_ sender: CADisplayLink) {
        let now = sender.timestamp
        var dt: CGFloat = lastTime == 0 ? 1.0/60.0 : CGFloat(now - lastTime)
        lastTime = now
        dt = min(max(dt, 1.0/240.0), 1.0/30.0)   // 夹紧，防跳帧爆炸

        // 半隐式欧拉
        let a = (-stiffness * (value - target) - damping * velocity) / mass
        velocity += a * dt
        value += velocity * dt
        onUpdate?(value)

        if abs(value - target) < 0.01 && abs(velocity) < 0.5 {
            value = target; onUpdate?(value); stop(); onComplete?()
        }
    }
}

// MARK: - 卡片手势参数预设（集中管理，Codex 直接引用）
enum GestureSpring {
    // 拖拽甩出惯性（实测）
    static let dragStiffness: CGFloat = 24
    static let dragDamping:   CGFloat = 1.6
    static let dragMass:      CGFloat = 0.1
    static let dragInitialVelocity: CGFloat = 16    // 乘以捕获速度方向

    // 吸附回弹
    static let snapStiffness: CGFloat = 100
    static let snapDamping:   CGFloat = 20
    static let snapMass:      CGFloat = 1

    // 画布相机（SpaceAnimator）
    static let spaceStiffness: CGFloat = 100
    static let spaceDamping:   CGFloat = 20
    static let spaceMass:      CGFloat = 1
}
