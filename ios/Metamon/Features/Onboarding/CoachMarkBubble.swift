import SwiftUI

/// 吹き出しの文言(#1441)。`voiceOver` は VoiceOver の利用者向け(長押し・ドラッグの代わりに操作の一覧を使う)。
enum CoachMarkCopy {
    static func title(_ mark: CoachMark) -> String {
        switch mark {
        case .fabLongPress: L10n.pick(L10n.tr("長押しで変身一覧へ"), review: L10n.tr("長押しで記録帳の一覧へ"))
        case .fabDrag: L10n.tr("ボタンは動かせます")
        case .switcherReorder: L10n.tr("長押しで並べ替え")
        }
    }

    static func body(_ mark: CoachMark) -> String {
        switch mark {
        case .fabLongPress:
            L10n.pick(L10n.tr("このボタンを長押しすると、作ったアプリの一覧に戻れます。"),
                      review: L10n.tr("このボタンを長押しすると、記録帳の一覧に戻れます。"))
        case .fabDrag:
            L10n.tr("指で押さえたまま動かすと、左右の辺に移せます。端に寄せると隠れて、取っ手だけ残ります。")
        case .switcherReorder:
            L10n.tr("カードを長押ししたまま動かし、別のカードの上で離すと順番を変えられます。")
        }
    }

    static func voiceOver(_ mark: CoachMark) -> String {
        switch mark {
        case .fabLongPress:
            L10n.pick(L10n.tr("このボタンの操作の一覧から「変身一覧」を選ぶと、作ったアプリの一覧に戻れます。"),
                      review: L10n.tr("このボタンの操作の一覧から「記録帳の一覧」を選ぶと、記録帳の一覧に戻れます。"))
        case .fabDrag:
            L10n.tr("このボタンの操作の一覧の「反対側へ移動」「端に隠す」で、置き場所を変えられます。")
        case .switcherReorder:
            L10n.tr("カードの「…」のメニューの「左へ移動」「右へ移動」で順番を変えられます。")
        }
    }

    static var gotIt: String { L10n.tr("わかった") }
    static var neverAgain: String { L10n.tr("もう出さない") }
}

/// 部品を指す吹き出し(#1441)。`arrowX` = 矢印の先の横位置(吹き出しの左端から)。矢印は下向き(部品の上に出す)。
struct CoachMarkBubble: View {
    let mark: CoachMark
    var arrowAlignment: HorizontalAlignment = .leading
    var arrowInset: CGFloat = Theme.Spacing.lg
    let onEvent: (CoachMarkEvent) -> Void
    @Environment(\.accessibilityVoiceOverEnabled) private var voiceOver

    var body: some View {
        VStack(alignment: arrowAlignment, spacing: 0) {
            card
            Triangle()
                .fill(Theme.Colors.surfaceElevated)
                .frame(width: 18, height: 9)
                .padding(arrowAlignment == .leading ? .leading : .trailing, max(0, arrowInset - 9))
                .accessibilityHidden(true)
        }
        .shadow(color: .black.opacity(0.18), radius: 12, y: 4)
        .onAppear {
            onEvent(.shown)
            AccessibilityNotification.Announcement(CoachMarkCopy.title(mark) + "\n" + CoachMarkCopy.voiceOver(mark)).post()
        }
    }

    private var card: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            Label {
                Text(CoachMarkCopy.title(mark)).font(.subheadline.weight(.bold))
            } icon: {
                Image(systemName: "hand.tap.fill").foregroundStyle(Theme.brandGradient)
            }
            Text(voiceOver ? CoachMarkCopy.voiceOver(mark) : CoachMarkCopy.body(mark))
                .font(.footnote)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack {
                Button(CoachMarkCopy.neverAgain) { withAnimation { onEvent(.neverAgain) } }
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("coachMark.never")
                Spacer(minLength: Theme.Spacing.md)
                Button(CoachMarkCopy.gotIt) { withAnimation { onEvent(.dismissed) } }
                    .buttonStyle(.borderedProminent)
                    .tint(Theme.Colors.brandA)
                    .accessibilityIdentifier("coachMark.gotIt")
            }
            .font(.footnote.weight(.semibold))
        }
        .padding(Theme.Spacing.md)
        .frame(maxWidth: 300, alignment: .leading)
        .background(Theme.Colors.surfaceElevated, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous).strokeBorder(Theme.Colors.hairline))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("coachMark.\(mark.rawValue)")
    }

    private struct Triangle: Shape {
        func path(in rect: CGRect) -> Path {
            var p = Path()
            p.move(to: CGPoint(x: rect.minX, y: rect.minY))
            p.addLine(to: CGPoint(x: rect.maxX, y: rect.minY))
            p.addLine(to: CGPoint(x: rect.midX, y: rect.maxY))
            p.closeSubpath()
            return p
        }
    }
}
