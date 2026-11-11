import SwiftUI
import MetamonServices

/// 会話のシートの失敗を出す状態カード(#968)。分け方と文言は iOS の `ErrorPresentation` / `ErrorCopy` をそのまま使う
/// (上限・Pro が要る・一時停止・オフライン・混雑・失敗)。見た目は Mac に合わせ、ボタンは「もう一度」と「閉じる」。
/// 持てるアプリの数の上限(#630)は「置き換える」で置き換えシート(`MacAppLimitSheet`)を出し、1 つ空いたら同じ依頼でやり直す。
/// サインイン・Pro にする・今すぐ送るは Mac にまだ無い(#930・課金は後の Issue)ので出さず、本文の案内だけにする。
struct MacErrorCard: View {
    let presentation: ErrorPresentation
    var retry: () -> Void
    var dismiss: () -> Void
    /// 置き換えに使えるアプリ(会話のシートを開いた `MacRootView` が渡す)。無ければ「置き換える」を出さない
    @Environment(\.macAppReplace) private var replaceContext
    @State private var replacing: AppLimit?

    /// 置き換えシートで解ける上限(カードが持つ上限)。シートを出せない時は nil
    private var appLimit: AppLimit? {
        guard presentation.primary == .replaceApp, replaceContext != nil, case .appLimit(let limit) = presentation.kind else { return nil }
        return limit
    }

    /// Mac で押せる主ボタン。もう一度で直るものだけ
    static func showsRetry(_ presentation: ErrorPresentation) -> Bool {
        switch presentation.primary {
        case .retry?: true
        case .sendNow?, .signIn?, .signInApple?, .upgradePro?, .replaceApp?, nil: false // 置き換えは別のボタン(下の appLimit)
        }
    }

    var body: some View {
        HStack(alignment: .top, spacing: MacTheme.Spacing.group) {
            Image(systemName: presentation.icon)
                .font(.title3.weight(.semibold))
                .foregroundStyle(presentation.isWarning ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(Theme.brandGradient))
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 6) {
                Text(presentation.title).font(.headline)
                Text(presentation.body)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                HStack(spacing: MacTheme.Spacing.group) {
                    if Self.showsRetry(presentation) {
                        Button(ErrorCopy.retry, action: retry)
                            .buttonStyle(.borderedProminent)
                    }
                    if let appLimit {
                        Button(AppLimitCopy.replaceButton) { replacing = appLimit }
                            .buttonStyle(.borderedProminent)
                            .accessibilityIdentifier("appLimit.replace")
                    }
                    Button(ErrorCopy.dismiss, action: dismiss)
                }
                .padding(.top, 2)
            }
            Spacer(minLength: 0)
            CraneMascot(pose: .panic, size: 44, sway: 3)
                .accessibilityHidden(true)
        }
        .padding(MacTheme.Spacing.group)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).strokeBorder(.separator))
        .accessibilityElement(children: .contain)
        .accessibilityLabel(presentation.announcement)
        .task(id: presentation.id) {
            MacVoiceOver.announce(presentation.announcement)
            replacing = appLimit // iOS と同じく、断られたらすぐ置き換えシートを出す(閉じてもカードの「置き換える」で開き直せる)
        }
        .sheet(item: Binding(get: { replacing.map(LimitItem.init) }, set: { if $0 == nil { replacing = nil } })) { item in
            if let replaceContext {
                MacAppLimitSheet(limit: item.limit, context: replaceContext, onFreed: retry)
            }
        }
    }
}

/// `.sheet(item:)` に渡すための包み(上限 1 件 = シート 1 枚)
private struct LimitItem: Identifiable {
    let limit: AppLimit
    var id: String { "\(limit.held)/\(limit.limit)" }
}
