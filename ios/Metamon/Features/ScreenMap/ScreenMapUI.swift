import SwiftUI
import Observation
import MetamonServices

/// 画面マップ(#116)の表示状態と「この画面について話す」の文脈。変身一覧・設定・音声シートの間で共有する。
@MainActor
@Observable
final class ScreenMapUI {
    static let shared = ScreenMapUI()

    var isPresented = false
    /// 開いた時に選んでおく画面(E2E 用)
    var preselect: String?
    /// 音声シートで次に送る edit の対象画面。送ったら消える
    var context: ScreenContext?

    /// 文脈を取り出して消す(送信 1 回分)。
    func takeContext() -> ScreenContext? {
        defer { context = nil }
        return context
    }
}

struct ScreenContext: Equatable {
    var id: String
    var title: String
}

extension AppRuntime {
    /// 画面マップで選んだ画面を文脈にして edit を送る。API の契約は変えず、transcript の先頭に「画面「B」(id: b)で: 」を付ける。
    /// 「元に戻して」等の意図は付ける前の文で判定する(文脈付きでも undo / 切替はそのまま効く)。
    func morph(transcript: String, about screen: ScreenContext?) async {
        let text = transcript.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let screen, !text.isEmpty, VoiceIntent.detect(text) == .morph else {
            await morph(transcript: transcript)
            return
        }
        await generate(ScreenMapCopy.contextPrefix(screen) + text)
    }
}

enum ScreenMapCopy {
    static func contextPrefix(_ s: ScreenContext) -> String { L10n.tr("画面「\(s.title)」(id: \(s.id))で: ") }
    static var title: String { L10n.tr("画面マップ") }
}

extension View {
    /// 画面マップのシートを付ける。`onTalk` は「この画面について話す」(文脈はセット済み・マップは閉じた後に呼ぶ)。
    func screenMapSheet(onTalk: @escaping @MainActor () -> Void) -> some View {
        modifier(ScreenMapSheetHost(onTalk: onTalk))
    }
}

private struct ScreenMapSheetHost: ViewModifier {
    let onTalk: @MainActor () -> Void
    @State private var ui = ScreenMapUI.shared

    func body(content: Content) -> some View {
        content.sheet(isPresented: $ui.isPresented) {
            ScreenMapSheet { screen in
                ui.context = screen
                ui.isPresented = false
                onTalk()
            }
        }
    }
}

/// 変身一覧の「使用中」カードに重ねる入口(サムネ右下)。
struct ScreenMapCardButton: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        if !rt.dsl.isVoiceOnly {
            Button { ScreenMapUI.shared.isPresented = true } label: {
                Label(ScreenMapCopy.title, systemImage: "point.3.connected.trianglepath.dotted")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(.white)
                    .padding(.horizontal, Theme.Spacing.sm + 2)
                    .padding(.vertical, 6)
                    .background(.black.opacity(0.45), in: Capsule())
                    .overlay(Capsule().strokeBorder(.white.opacity(0.3)))
            }
            .buttonStyle(.plain)
            .padding(Theme.Spacing.sm)
            .padding(.bottom, AppCard.height - AppCard.thumbHeight)
            .accessibilityIdentifier("switcher.screenMap")
        }
    }
}

/// 設定シート「アプリ」節の行。
struct ScreenMapSettingsRow: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        if !rt.dsl.isVoiceOnly {
            Button { ScreenMapUI.shared.isPresented = true } label: {
                Label(ScreenMapCopy.title, systemImage: "point.3.connected.trianglepath.dotted")
            }
            .accessibilityIdentifier("settings.screenMap")
        }
    }
}

/// 音声シートの上に出す「画面「B」について話す」チップ。× で文脈を外す。
struct ScreenContextChip: View {
    @State private var ui = ScreenMapUI.shared

    var body: some View {
        if let screen = ui.context {
            HStack(spacing: Theme.Spacing.sm) {
                Image(systemName: "point.3.connected.trianglepath.dotted")
                    .foregroundStyle(Theme.brandGradient)
                Text(L10n.tr("画面「\(screen.title)」について話す"))
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                Button { ui.context = nil } label: {
                    Image(systemName: "xmark.circle.fill").foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("画面の指定を外す")
            }
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm)
            .background(Theme.Colors.surface, in: Capsule())
            .overlay(Capsule().strokeBorder(Theme.brandGradient, lineWidth: 1.5))
            .transition(.scale.combined(with: .opacity))
            .onDisappear { ui.context = nil } // 送らずにシートを閉じたら外す
            .accessibilityIdentifier("voice.screenContext")
        }
    }
}

#if DEBUG
extension ScreenMapUI {
    /// ヘッドレス E2E 用: `METAMON_AUTO_SCREEN_MAP=1`(または `=<screenId>` で選択状態)で起動 2.5 秒後に変身一覧 → 画面マップを開く。
    /// 同梱例は `METAMON_INITIAL_EXAMPLE=navigation` 等と組み合わせる。`METAMON_AUTO_SCREEN_MAP_TALK=1` で続けて「この画面について話す」まで進める。
    func autoOpenIfRequested(_ rt: AppRuntime) async {
        let env = ProcessInfo.processInfo.environment
        guard let spec = env["METAMON_AUTO_SCREEN_MAP"], !spec.isEmpty else { return }
        try? await Task.sleep(for: .seconds(2.5))
        preselect = spec == "1" ? nil : spec
        rt.openSwitcher()
        try? await Task.sleep(for: .seconds(1))
        isPresented = true
        guard env["METAMON_AUTO_SCREEN_MAP_TALK"] == "1", let id = preselect, let s = rt.dsl.screen(id) else { return }
        try? await Task.sleep(for: .seconds(2.5))
        context = ScreenContext(id: s.id, title: s.title.isEmpty ? s.id : s.title)
        isPresented = false
        rt.showAppSwitcher = false
        try? await Task.sleep(for: .milliseconds(700))
        rt.showVoiceSheet = true
    }
}
#endif
