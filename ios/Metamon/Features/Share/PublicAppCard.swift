// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

// 「探す」(#50)のカード・並び順・文言。iOS の `DiscoverSheet` と Mac の `MacDiscoverView`(#2069)で共有する。

/// 並び順。値はサーバーの `sort`(`popular` = インストール数の多い順・`new` = 新しい順)
enum DiscoverSort: String, CaseIterable, Identifiable {
    case popular
    case new

    var id: String { rawValue }

    var label: LocalizedStringKey {
        switch self {
        case .popular: "人気"
        case .new: "新着"
        }
    }
}

enum DiscoverCopy {
    /// 結果が 0 件の時の一言(検索語があればそれを添える)
    static func emptyMessage(query: String) -> String {
        query.isEmpty ? L10n.tr("まだ公開されたアプリがありません") : L10n.tr("「\(query)」に合うアプリは見つかりませんでした")
    }

    /// サーバーにつながっていない(モック・plist 無し)
    static var unavailable: String { L10n.tr("「探す」はサーバー接続時のみ使えます") }
}

/// 検索結果カード: 名前・説明・作者・使っている部品のアイコン・インストール数。
struct PublicAppCard: View {
    let item: PublicAppListing
    /// 入れている途中(Mac はシートを開いたまま待つ・#2069)。ボタンを押せなくして輪を出す
    var busy = false
    let install: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            HStack(alignment: .top, spacing: Theme.Spacing.md) {
                Text(String(item.name.prefix(1)))
                    .font(.system(.title2, design: .rounded).weight(.bold))
                    .foregroundStyle(.white)
                    .frame(width: 48, height: 48)
                    .background(Theme.brandGradient, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                VStack(alignment: .leading, spacing: 2) {
                    Text(item.name).font(.headline).lineLimit(1)
                    Text(byline).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                }
                Spacer(minLength: 0)
                Button(action: install) {
                    Group {
                        if busy {
                            ProgressView().controlSize(.small).tint(.white)
                        } else {
                            Text(item.mine ? L10n.tr("コピー") : L10n.tr("入れる"))
                        }
                    }
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.white)
                    .padding(.horizontal, Theme.Spacing.md)
                    .frame(height: 34)
                    .background(Theme.brandGradient, in: Capsule())
                }
                .buttonStyle(.plain)
                .disabled(busy)
                .accessibilityLabel("\(item.name)をインストール")
            }
            if item.hidden == true { // #321: 運営が非公開にした自分の掲載
                Label(item.hiddenReason.map { L10n.tr("運営により非公開: \($0)") } ?? L10n.tr("運営により非公開"),
                      systemImage: "eye.slash")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.orange)
            }
            if !item.description.isEmpty {
                Text(item.description)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack(spacing: Theme.Spacing.sm) {
                ForEach(item.components.prefix(6), id: \.self) { type in
                    Image(systemName: ComponentIcon.symbol(for: type))
                        .font(.caption)
                        .foregroundStyle(Theme.Colors.brandA)
                        .frame(width: 26, height: 26)
                        .background(Theme.Colors.brandA.opacity(0.12), in: Circle())
                        .accessibilityLabel(type)
                }
                Spacer(minLength: 0)
                ForEach(item.tags.prefix(2), id: \.self) { tag in
                    Text("#\(tag)").font(.caption2.weight(.medium)).foregroundStyle(.secondary)
                }
            }
        }
        .metamonCard()
    }

    private var byline: String {
        let author = item.authorName.isEmpty ? L10n.tr("匿名") : item.authorName
        return L10n.tr("\(author) ・ \(item.installs) 人が利用") + (item.mine ? L10n.tr(" ・ あなたの公開") : "")
    }
}

/// DSL の component type → SF Symbol(カードの部品アイコン)。
enum ComponentIcon {
    static func symbol(for type: String) -> String {
        switch type {
        case "stat": "number"
        case "chart": "chart.bar.fill"
        case "list", "table": "list.bullet"
        case "button", "shareButton", "openUrlButton": "hand.tap.fill"
        case "textField": "character.cursor.ibeam"
        case "numberField": "textformat.123"
        case "picker", "segmented": "filemenu.and.selection"
        case "datePicker", "calendar": "calendar"
        case "toggle": "switch.2"
        case "photoField", "camera": "camera.fill"
        case "image": "photo"
        case "drawingField": "pencil.tip"
        case "videoField", "videoPlayer": "video.fill"
        case "map", "mapView", "locationButton", "locationPicker", "placeSearch": "map.fill"
        case "timer", "timerDisplay": "timer"
        case "webView": "safari"
        case "qrScanner": "qrcode.viewfinder"
        case "qrCode": "qrcode"
        case "progress", "gauge": "gauge.with.dots.needle.50percent"
        case "text": "text.alignleft"
        default: "square.grid.2x2"
        }
    }
}
#endif
