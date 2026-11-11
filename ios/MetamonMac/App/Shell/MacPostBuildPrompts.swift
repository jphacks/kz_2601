import SwiftUI
import MetamonServices
import MetamonRenderer

/// 生成直後の「個人の情報も公開されます」(#253・iOS の `PublicDataPromptHost`)。要約が閉じてから出す。
/// 時間では消さない。閉じる(Esc)は「公開のままにする」と同じ
struct MacPublicDataBar: View {
    let notice: MacPostBuildNotices.PublicData
    var answer: (MacPostBuildNotices.PublicDataChoice) -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "globe")
                .foregroundStyle(Theme.Colors.warning)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text("個人の情報も公開されます").font(.headline)
                Text(L10n.tr("\(notice.prompt.fieldNames(english: LanguagePreference.shared.code == "en"))は、アプリを開いた人なら誰でも見られます。"))
                    .font(.callout)
                    .fixedSize(horizontal: false, vertical: true)
                HStack(spacing: 8) {
                    Button("この欄を消す", role: .destructive) { answer(.remove) }
                        .help("この欄を消すように直します")
                        .accessibilityIdentifier("publicData.remove")
                    Button("自分だけにする") { answer(.makePrivate) }
                        .help("この記録を自分だけが見られるように直します")
                        .accessibilityIdentifier("publicData.private")
                    Button("公開のままにする") { answer(.keep) }
                        .keyboardShortcut(.cancelAction)
                        .help("何も変えずに閉じます")
                        .accessibilityIdentifier("publicData.keep")
                }
                .controlSize(.small)
                .padding(.top, 2)
            }
        }
        .padding(.horizontal, 16).padding(.vertical, 12)
        .macRaisedSurface(in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .frame(maxWidth: 560)
        .transition(.move(edge: .top).combined(with: .opacity))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("publicData.card")
    }
}

/// 記録の保存期間の確認(#482・iOS の `RetentionPromptHost`)。公開の確認の後に出す。閉じる(Esc)は「あとで」
struct MacRetentionBar: View {
    let prompt: RetentionPrompt
    var confirm: () -> Void
    var stop: () -> Void
    var later: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "clock.arrow.circlepath")
                .foregroundStyle(Theme.brandGradient)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text("古い記録を自動で消しますか?").font(.headline)
                Text(verbatim: prompt.message(english: LanguagePreference.shared.code == "en"))
                    .font(.callout)
                    .fixedSize(horizontal: false, vertical: true)
                HStack(spacing: 8) {
                    Button("自動で消す", role: .destructive, action: confirm)
                        .help("期間を過ぎた記録をこの Mac で自動で消します")
                        .accessibilityIdentifier("retention.confirm")
                    Button("自動で消すのをやめる", action: stop)
                        .help("記録を自動で消さないようにアプリを直します")
                        .accessibilityIdentifier("retention.stop")
                    Button("あとで", action: later)
                        .keyboardShortcut(.cancelAction)
                        .help("今は消しません。アプリの設定から確認できます")
                        .accessibilityIdentifier("retention.later")
                }
                .controlSize(.small)
                .padding(.top, 2)
            }
        }
        .padding(.horizontal, 16).padding(.vertical, 12)
        .macRaisedSurface(in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .frame(maxWidth: 560)
        .transition(.move(edge: .top).combined(with: .opacity))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("retention.card")
    }
}
