import SwiftUI

/// 音声画面の大きなマイク。状態ごとに演出を変える(すべて TimelineView + sin で軽量)。
/// - idle(準備完了): ゆっくり呼吸するグラデーションのリング
/// - idle(準備中):   灰色・リング無し
/// - listening:      音量に反応する波紋(※ SpeechTranscriber に音量 API が無いため擬似レベル。実音量連動は Packages 側の拡張待ち)
/// - transcribing:   波形バー
struct MicButton: View {
    enum Mode: Equatable { case idle(ready: Bool), listening, transcribing }

    let mode: Mode
    var size: CGFloat = 132
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            ZStack {
                halo
                core
                glyph
            }
            .frame(width: size * 1.6, height: size * 1.6)
            .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .disabled(mode == .transcribing || mode == .idle(ready: false))
        .accessibilityLabel(accessibilityLabel)
    }

    // MARK: - Layers

    @ViewBuilder
    private var halo: some View {
        switch mode {
        case .idle(ready: true):
            TimelineView(.animation(minimumInterval: 1.0 / 30)) { tl in
                let t = tl.date.timeIntervalSinceReferenceDate
                let breath = 0.5 + 0.5 * sin(t * (2 * .pi / 3.2)) // 3.2 秒周期
                ZStack {
                    Circle()
                        .stroke(Theme.brandAngular, lineWidth: 3)
                        .frame(width: size * 1.28, height: size * 1.28)
                        .scaleEffect(1 + 0.07 * breath)
                        .opacity(0.55 + 0.35 * breath)
                        .rotationEffect(.degrees(t * 18))
                    Circle()
                        .fill(Theme.brandGradient.opacity(0.10 + 0.12 * breath))
                        .frame(width: size * 1.55, height: size * 1.55)
                        .scaleEffect(1 + 0.06 * breath)
                        .blur(radius: 18)
                }
            }
        case .listening:
            TimelineView(.animation(minimumInterval: 1.0 / 30)) { tl in
                let t = tl.date.timeIntervalSinceReferenceDate
                // 擬似音量: 周期の異なる sin を重ねて「話している」揺らぎを作る
                let level = 0.45 + 0.55 * abs(sin(t * 5.3) * sin(t * 1.9) * cos(t * 0.7))
                ZStack {
                    ForEach(0..<3, id: \.self) { i in
                        let phase = ((t * 0.75) + Double(i) / 3).truncatingRemainder(dividingBy: 1)
                        Circle()
                            .stroke(Theme.Colors.recordA.opacity((1 - phase) * 0.55), lineWidth: 2)
                            .frame(width: size, height: size)
                            .scaleEffect(1 + phase * (0.6 + 0.5 * level))
                    }
                    Circle()
                        .fill(Theme.recordGradient.opacity(0.18))
                        .frame(width: size * 1.3, height: size * 1.3)
                        .scaleEffect(1 + 0.25 * level)
                        .blur(radius: 14)
                }
            }
        case .transcribing:
            Circle()
                .fill(Theme.brandGradient.opacity(0.14))
                .frame(width: size * 1.4, height: size * 1.4)
                .blur(radius: 16)
        case .idle(ready: false):
            EmptyView()
        }
    }

    @ViewBuilder
    private var core: some View {
        Circle()
            .fill(coreStyle)
            .frame(width: size, height: size)
            .shadow(color: coreShadow, radius: 22, y: 10)
            .overlay(Circle().strokeBorder(.white.opacity(0.25), lineWidth: 1))
            .scaleEffect(mode == .listening ? 0.94 : 1)
    }

    private var coreStyle: AnyShapeStyle {
        switch mode {
        case .idle(ready: true): AnyShapeStyle(Theme.brandGradient)
        case .idle(ready: false): AnyShapeStyle(Color.gray.opacity(0.45))
        case .listening: AnyShapeStyle(Theme.recordGradient)
        case .transcribing: AnyShapeStyle(Theme.brandGradient.opacity(0.85))
        }
    }

    private var coreShadow: Color {
        switch mode {
        case .listening: Theme.Colors.recordA.opacity(0.45)
        case .idle(ready: false): .clear
        default: Theme.Colors.brandA.opacity(0.45)
        }
    }

    @ViewBuilder
    private var glyph: some View {
        switch mode {
        case .idle: Image(systemName: "mic.fill").font(.system(size: size * 0.38, weight: .semibold)).foregroundStyle(.white)
        case .listening: Image(systemName: "stop.fill").font(.system(size: size * 0.3, weight: .bold)).foregroundStyle(.white)
        case .transcribing: WaveformBars(barCount: 5, height: size * 0.36).foregroundStyle(.white)
        }
    }

    private var accessibilityLabel: String {
        switch mode {
        case .idle(ready: true): L10n.tr("マイク。タップして話す")
        case .idle(ready: false): L10n.tr("音声モデルを準備中")
        case .listening: L10n.tr("録音中。タップで終了")
        case .transcribing: L10n.tr("文字起こし中")
        }
    }
}

/// 文字起こし中の波形バー。
struct WaveformBars: View {
    var barCount = 5
    var height: CGFloat = 40

    var body: some View {
        TimelineView(.animation(minimumInterval: 1.0 / 30)) { tl in
            let t = tl.date.timeIntervalSinceReferenceDate
            HStack(spacing: height * 0.12) {
                ForEach(0..<barCount, id: \.self) { i in
                    let h = 0.3 + 0.7 * abs(sin(t * 6 + Double(i) * 0.9))
                    Capsule()
                        .frame(width: height * 0.14, height: height * h)
                }
            }
            .frame(height: height)
        }
    }
}

/// 認識テキストを 1 文字ずつ表示する。
struct TypewriterText: View {
    let text: String
    var charactersPerSecond: Double = 28

    @State private var shown = 0

    var body: some View {
        Text(String(text.prefix(shown)))
            .task(id: text) {
                shown = 0
                let interval = 1 / charactersPerSecond
                for i in 1...max(1, text.count) {
                    try? await Task.sleep(for: .seconds(interval))
                    if Task.isCancelled { return }
                    shown = i
                }
            }
    }
}
