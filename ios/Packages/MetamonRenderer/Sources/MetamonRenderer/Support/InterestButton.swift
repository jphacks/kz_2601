// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、仲間探し(#507)も外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `list.item.interest`(#507): プロフィールの行の右端の「つながりたい」。送った後は「送りました」、つながったら「つながりました」。
struct InterestButton: View {
    let state: InterestState
    let busy: Bool
    let action: () -> Void

    var body: some View {
        switch state {
        case .none, .incoming:
            Button(action: action) {
                Text(Self.title(state)).font(.subheadline.weight(.semibold)).lineLimit(1)
            }
            .buttonStyle(.borderedProminent)
            .buttonBorderShape(.capsule)
            .controlSize(.small)
            .tint(RenderTheme.Colors.brandA)
            .disabled(busy)
            .accessibilityIdentifier("interest.send")
        case .sent, .matched:
            Label(Self.title(state), systemImage: state == .matched ? "checkmark.circle.fill" : "paperplane")
                .font(.subheadline.weight(.medium))
                .labelStyle(.titleAndIcon)
                .foregroundStyle(state == .matched ? AnyShapeStyle(RenderTheme.Colors.brandText) : AnyShapeStyle(.secondary))
                .lineLimit(1)
                .accessibilityIdentifier("interest.state")
        }
    }

    static func title(_ state: InterestState) -> String {
        switch state {
        case .none: L("つながりたい")
        case .incoming: L("承認する")
        case .sent: L("送りました")
        case .matched: L("つながりました")
        }
    }

    /// 行の右端に置く形(VoiceOver は行の操作として読む)。
    static func accessory(state: InterestState, busy: Bool, action: @escaping () -> Void) -> RowAccessory {
        let tappable = state == .none || state == .incoming
        return RowAccessory(view: AnyView(InterestButton(state: state, busy: busy, action: action)),
                            actions: tappable && !busy ? [RowAccessory.Action(label: title(state), perform: action)] : [],
                            value: tappable ? nil : title(state))
    }
}
#endif
