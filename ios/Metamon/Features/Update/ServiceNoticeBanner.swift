import SwiftUI
import MetamonServices

/// アプリ内のお知らせのバナー(#333)。1 行の告知・任意の「詳しく」・閉じるボタン。
/// 色は semantic color(ダークモード対応)、VoiceOver では 1 つの要素として読み、操作は個別のボタン。
struct ServiceNoticeBanner: View {
    let notice: ServiceNotice
    let onDismiss: () -> Void
    @Environment(\.locale) private var locale
    @Environment(\.openURL) private var openURL

    private var isWarning: Bool { notice.severity == .warning }
    private var tint: Color { isWarning ? .orange : .accentColor }
    private var text: String { notice.text(languageCode: locale.language.languageCode?.identifier ?? "ja") }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Image(systemName: isWarning ? "exclamationmark.triangle.fill" : "info.circle.fill")
                .foregroundStyle(tint)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 6) {
                Text(verbatim: text)
                    .font(.subheadline)
                    .foregroundStyle(.primary)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityLabel(isWarning ? Text("重要なお知らせ: \(text)") : Text("お知らせ: \(text)"))
                if let url = notice.linkURL {
                    Button("詳しく見る") { openURL(url) }
                        .font(.subheadline.weight(.semibold))
                        .buttonStyle(.borderless)
                        .accessibilityIdentifier("notice.link")
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Button(action: onDismiss) {
                Image(systemName: "xmark")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(.secondary)
                    .frame(minWidth: 44, minHeight: 44) // タップ領域 44pt
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(Text("お知らせを閉じる"))
            .accessibilityIdentifier("notice.dismiss")
            .padding(.vertical, -12) // 見た目の高さは文字に合わせ、タップ領域だけ広げる
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background {
            Rectangle()
                .fill(Color(.secondarySystemBackground))
                .overlay(alignment: .bottom) { Rectangle().fill(tint.opacity(0.6)).frame(height: 1) }
                .ignoresSafeArea(edges: .top)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("notice.banner")
    }
}

#Preview("info / warning") {
    VStack(spacing: 0) {
        ServiceNoticeBanner(notice: ServiceNotice(id: "p1", message: .init(ja: "11月3日 2:00〜3:00 はメンテナンスのため生成できません。", en: "Generation is unavailable during maintenance (Nov 3, 2:00–3:00 JST)."))) {}
        ServiceNoticeBanner(notice: ServiceNotice(id: "p2", severity: .warning, message: .init(ja: "AI の生成を一時的に止めています。作ったアプリと記録はそのまま使えます。"), linkUrl: "https://example.com")) {}
        Spacer()
    }
}
