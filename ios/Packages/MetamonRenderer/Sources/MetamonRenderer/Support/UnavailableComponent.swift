import SwiftUI
import MetamonDSL

/// レジストリに無い部品の場所に出す控えめな枠(#536)。リリース版でもグリッドに穴を開けず、理由だけ短く出す。
/// 部品名は出さない(審査用の版で外した部品の名前を見せない)。DEBUG だけ type を小さく添える。
struct UnavailableComponent: View {
    enum Reason: Equatable {
        /// この版が知らない type(新しい版の DSL)
        case needsUpdate
        /// type は知っているが、このビルド・端末では描けない(審査用の版で外した部品など)
        case unavailableHere

        /// `ComponentType` に無い type は新しい版の部品。有るのにレジストリに無いものはこの版で外した部品。
        init(_ component: Component) {
            self = component.type == .unknown ? .needsUpdate : .unavailableHere
        }

        var message: String {
            switch self {
            case .needsUpdate: L("アプリを更新すると表示できます")
            case .unavailableHere: L("この端末では使えません")
            }
        }

        var symbol: String {
            switch self {
            case .needsUpdate: "arrow.down.circle"
            case .unavailableHere: "nosign"
            }
        }
    }

    let reason: Reason
    let rawType: String

    init(component: Component) {
        reason = Reason(component)
        rawType = component.rawType
    }

    var body: some View {
        VStack(spacing: RenderTheme.Spacing.xs) {
            Label {
                Text(reason.message)
                    .font(.caption)
                    .lineLimit(2)
                    .minimumScaleFactor(0.8)
                    .multilineTextAlignment(.center)
            } icon: {
                Image(systemName: reason.symbol)
                    .imageScale(.small)
            }
            .foregroundStyle(.secondary)
            #if DEBUG
            Text(rawType)
                .font(.caption2.monospaced())
                .foregroundStyle(.tertiary)
            #endif
        }
        .padding(RenderTheme.Spacing.sm)
        .frame(maxWidth: .infinity, minHeight: 44, maxHeight: .infinity)
        .overlay(
            RoundedRectangle(cornerRadius: RenderTheme.Radius.inner)
                .strokeBorder(RenderTheme.Colors.hairline, lineWidth: 1)
        )
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(reason.message)
        .accessibilityIdentifier("component.unavailable")
    }
}
