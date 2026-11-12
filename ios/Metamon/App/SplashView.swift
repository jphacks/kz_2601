import SwiftUI
import UIKit

/// 起動のスプラッシュを出すかどうか(コールド起動の 1 回だけ)。見た目の正典は web/pv/index.html#splash。
enum SplashGate {
    enum Mode: Equatable {
        case hidden
        /// 赤い糸が縫う → アイコン → 文字(約 2.5 秒)
        case animated
        /// 視差効果を減らす: 動かさず、短くフェードするだけ
        case reduced
    }

    /// UI テスト・スクショ撮りで飛ばす起動引数(`-metamonSkipSplash`)
    static let skipArgument = "-metamonSkipSplash"

    /// このプロセスで既に出したか(背景から戻った時・シーンの作り直しでは出さない)
    private(set) static var didShowThisProcess = false

    /// 起動時に 1 回呼ぶ。出すと決めたら「出した」と記録する。
    static func decide(
        arguments: [String] = ProcessInfo.processInfo.arguments,
        environment: [String: String] = ProcessInfo.processInfo.environment,
        reduceMotion: Bool = UIAccessibility.isReduceMotionEnabled,
        voiceOver: Bool = UIAccessibility.isVoiceOverRunning
    ) -> Mode {
        guard !didShowThisProcess else { return .hidden }
        let mode = mode(arguments: arguments, environment: environment, reduceMotion: reduceMotion, voiceOver: voiceOver)
        if mode != .hidden { didShowThisProcess = true }
        return mode
    }

    /// 記録を残さない判定(テスト用にも使う)。
    static func mode(arguments: [String], environment: [String: String], reduceMotion: Bool, voiceOver: Bool = false) -> Mode {
        if arguments.contains(skipArgument) { return .hidden }
        // VoiceOver: 最初のダブルタップが下の画面に届くよう出さない(読み上げも下の画面だけにする)
        if voiceOver { return .hidden }
        switch environment["METAMON_SPLASH"] {
        case "0": return .hidden
        case "1": return reduceMotion ? .reduced : .animated // モックでも見たい時(スクショ撮り)
        default: break
        }
        // UI テスト(全部モック)と単体テストのホスト起動は待たせない
        if environment["METAMON_MOCK"] == "1" || environment["XCTestConfigurationFilePath"] != nil { return .hidden }
        return reduceMotion ? .reduced : .animated
    }

    #if DEBUG
    static func resetForTesting() { didShowThisProcess = false }
    #endif
}

/// 起動のスプラッシュ。ルートの上に重ね、下では起動処理がそのまま進む。タップで飛ばせる。
struct SplashView: View {
    let mode: SplashGate.Mode
    let onFinish: () -> Void

    enum Palette {
        static let background = Color("LaunchBackground")
        static let thread = Color(red: 0.84, green: 0.22, blue: 0.24)       // アイコンの赤い糸
        static let cream = Color(red: 0xF6 / 255, green: 0xF2 / 255, blue: 0xEA / 255)
        static let tagline = Color(red: 0xB7 / 255, green: 0xC6 / 255, blue: 0xD1 / 255)
    }

    /// 秒。web/pv の #splash と同じ並び(少し詰めて 2.5 秒に収める)
    enum Timing {
        static let stitch = 0.95
        static let icon = 0.30
        static let word = 1.05
        static let tag = 1.50
        static let finish = 2.20
        static let reducedFinish = 0.90
        /// 開始を待ちきれなかった時(メインスレッドが空かない): 完成形を止めて見せる長さ
        static let stillFinish = 0.50
    }

    @Environment(\.accessibilityReduceMotion) private var envReduceMotion
    @Environment(\.accessibilityVoiceOverEnabled) private var voiceOverOn
    @ScaledMetric(relativeTo: .largeTitle) private var wordSize: CGFloat = 40
    @ScaledMetric(relativeTo: .subheadline) private var tagSize: CGFloat = 15

    @State private var ambience = false
    @State private var stitch: CGFloat = 0
    @State private var iconShown = false
    @State private var wordShown = false
    @State private var tagShown = false
    @State private var finished = false

    private var reduced: Bool { mode == .reduced || envReduceMotion }

    var body: some View {
        GeometryReader { geo in
            let side = min(min(geo.size.width, geo.size.height) * 0.36, 168)
            ZStack {
                backdrop(in: geo.size).opacity(ambience ? 1 : 0)
                VStack(spacing: 0) {
                    mark(side: side)
                    Text(verbatim: "Tailor It")
                        .font(.system(size: wordSize, weight: .heavy))
                        .tracking(-0.5)
                        .foregroundStyle(Palette.cream)
                        .minimumScaleFactor(0.6)
                        .lineLimit(1)
                        .padding(.top, side * 0.30)
                        .opacity(wordShown ? 1 : 0)
                        .offset(y: wordShown || reduced ? 0 : 14)
                    Text("ひと声で、あなた仕立て。")
                        .font(.system(size: tagSize, weight: .bold))
                        .tracking(1)
                        .foregroundStyle(Palette.tagline)
                        .multilineTextAlignment(.center)
                        .padding(.top, 10)
                        .opacity(tagShown ? 1 : 0)
                }
                .padding(.horizontal, Theme.Spacing.lg)
                .dynamicTypeSize(...DynamicTypeSize.accessibility2)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(Palette.background) // 最初のコマ = 起動画面(UILaunchScreen の LaunchBackground)
        .ignoresSafeArea()
        .contentShape(Rectangle())
        .onTapGesture { finish() }
        .allowsHitTesting(!voiceOverOn) // 途中で VoiceOver が付いたら触れるのを下の画面に通し、すぐ閉じる
        .onChange(of: voiceOverOn) { _, on in if on { finish() } }
        // 要素にはせず、下の画面へのフォーカスを妨げない(VoiceOver の時はそもそも出さない)
        .accessibilityHidden(true)
        .task { await run() }
    }

    private func mark(side: CGFloat) -> some View {
        let seam = side * 1.18
        return ZStack {
            // 縫い目(赤い糸)。web 版と同じく角丸の枠を一周する
            RoundedRectangle(cornerRadius: seam * 0.237, style: .continuous)
                .trim(from: 0, to: stitch)
                .stroke(Palette.thread, style: StrokeStyle(lineWidth: max(2, side * 0.018), lineCap: .round, dash: [side * 0.05, side * 0.04]))
                .frame(width: seam, height: seam)
                .rotationEffect(.degrees(-90))
                .shadow(color: Palette.thread.opacity(0.45), radius: 4)
            Image("SplashIcon")
                .resizable()
                .interpolation(.high)
                .scaledToFill()
                .frame(width: side, height: side)
                .clipShape(RoundedRectangle(cornerRadius: side * 0.225, style: .continuous))
                .shadow(color: .black.opacity(0.28), radius: 20, y: 10)
                .scaleEffect(iconShown || reduced ? 1 : 0.86)
                .opacity(iconShown ? 1 : 0)
        }
        .frame(width: seam, height: seam)
    }

    /// 奥行き: 中央の淡い光 + 下へ沈むグラデ + 綾織りの細い斜線(生地の手触り)
    private func backdrop(in size: CGSize) -> some View {
        ZStack {
            LinearGradient(colors: [.white.opacity(0.06), .black.opacity(0.22)], startPoint: .top, endPoint: .bottom)
            RadialGradient(colors: [Color(red: 0.36, green: 0.55, blue: 0.66).opacity(0.38), .clear],
                           center: .init(x: 0.5, y: 0.42), startRadius: 0, endRadius: max(size.width, size.height) * 0.55)
            Canvas { ctx, sz in
                var path = Path()
                let step: CGFloat = 7
                var x: CGFloat = -sz.height
                while x < sz.width {
                    path.move(to: CGPoint(x: x, y: sz.height))
                    path.addLine(to: CGPoint(x: x + sz.height, y: 0))
                    x += step
                }
                ctx.stroke(path, with: .color(.white.opacity(0.025)), lineWidth: 1)
            }
        }
        .allowsHitTesting(false)
    }

    private func run() async {
        if reduced {
            withAnimation(.easeOut(duration: 0.25)) {
                ambience = true; stitch = 1; iconShown = true; wordShown = true; tagShown = true
            }
            await wait(Timing.reducedFinish)
            finish()
            return
        }
        // 最初のコマが描かれ、メインスレッドが空いてから縫い始める(起動処理で詰まったまま始めると縫う所が飛ぶ)
        let start = await SplashStartWaiter.wait()
        if start.decision == .fallback {
            ambience = true; stitch = 1; iconShown = true; wordShown = true; tagShown = true
            await wait(Timing.stillFinish)
            finish()
            return
        }
        // 開始を待った分だけ縮める(待つ + アニメ + フェードを 3 秒に収める)
        let k = SplashStartGate.pace(waited: start.waited, animation: Timing.finish)
        withAnimation(.easeOut(duration: 0.6 * k)) { ambience = true }
        withAnimation(.easeInOut(duration: Timing.stitch * k)) { stitch = 1 }
        await wait(Timing.icon * k)
        withAnimation(.spring(duration: 0.7 * k, bounce: 0.28)) { iconShown = true }
        await wait((Timing.word - Timing.icon) * k)
        withAnimation(.spring(duration: 0.6 * k, bounce: 0.15)) { wordShown = true }
        await wait((Timing.tag - Timing.word) * k)
        withAnimation(.easeOut(duration: 0.45 * k)) { tagShown = true }
        await wait((Timing.finish - Timing.tag) * k)
        finish()
    }

    private func wait(_ seconds: Double) async {
        try? await Task.sleep(for: .seconds(seconds))
    }

    private func finish() {
        guard !finished else { return }
        finished = true
        onFinish()
    }
}

/// ルートに重ねる。終わったらクロスフェードで外す。
struct SplashOverlay: ViewModifier {
    @State var mode: SplashGate.Mode

    func body(content: Content) -> some View {
        content.overlay {
            if mode != .hidden {
                SplashView(mode: mode) {
                    withAnimation(.easeInOut(duration: mode == .reduced ? 0.2 : SplashStartGate.fadeOut)) { mode = .hidden }
                    SplashPresence.shared.end() // 待たせていたシート・リンク・吹き出しを出す
                }
                .transition(.opacity)
                .onDisappear { SplashPresence.shared.end() }
                .zIndex(1)
            }
        }
    }
}

#Preview("Splash") {
    SplashView(mode: .animated) {}
}
