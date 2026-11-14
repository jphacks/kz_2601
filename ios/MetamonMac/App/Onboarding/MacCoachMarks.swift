import SwiftUI

/// 気づきにくい操作の吹き出し(#3249・iOS #1441 の Mac 版)。決まりと保存先は iOS と同じ(`CoachMarkSchedule`・`CoachMarkStore`)。
/// Mac にあるのはサイドバーのドラッグの並べ替え(iOS の変身一覧の `switcherReorder` に当たる)だけ。マイクのボタン(FAB)は Mac に無いので出さない。
/// 中央にアプリを開いた画面で、サイドバーを指すポップオーバーで出す。
enum MacCoachMarks {
    /// Mac で出すもの
    static let marks: Set<CoachMark> = [.switcherReorder]

    /// いま使える操作。案内・シート・お知らせ・検索の間と、並べ替えられない(作ったアプリが 1 つ以下)時は出さない。
    static func eligible(hasOpenApp: Bool, reorderableCount: Int, isSearching: Bool, tutorialSeen: Bool, busy: Bool) -> Set<CoachMark> {
        guard hasOpenApp, tutorialSeen, !busy, !isSearching, reorderableCount > 1 else { return [] }
        return marks
    }

    static func title(_ mark: CoachMark) -> String {
        switch mark {
        case .switcherReorder: L10n.tr("ドラッグで並べ替え")
        case .fabLongPress, .fabDrag: ""
        }
    }

    static func body(_ mark: CoachMark) -> String {
        switch mark {
        case .switcherReorder: L10n.tr("左の一覧の作ったアプリは、ドラッグで順番を変えられます。右クリックの「上へ移動」「下へ移動」でも変えられます。")
        case .fabLongPress, .fabDrag: ""
        }
    }
}

/// サイドバーに付ける: 出す時だけポップオーバーを開く。閉じる・「わかった」で次から出さない。
struct MacCoachMarkPopover: ViewModifier {
    let eligible: Set<CoachMark>
    var store: CoachMarkStore = .shared
    @State private var shown: CoachMark?

    private var mark: CoachMark? { store.next(eligible: eligible) }

    func body(content: Content) -> some View {
        content
            .popover(isPresented: Binding(get: { shown != nil }, set: { if !$0, let m = shown { finish(m) } }),
                     attachmentAnchor: .rect(.bounds), arrowEdge: .trailing) {
                if let shown { card(shown) }
            }
            .task(id: mark) {
                guard let mark, shown == nil else { return }
                try? await Task.sleep(for: .milliseconds(800)) // 画面が落ち着いてから(シートの閉じ終わりと重ねない)
                guard !Task.isCancelled, self.mark == mark else { return }
                store.markShown(mark)
                shown = mark
                MacVoiceOver.announce(MacCoachMarks.title(mark))
            }
            .onChange(of: eligible) { _, now in
                if let m = shown, !now.contains(m) { shown = nil } // 場面が変わったら閉じる(済みにはしない・次も同じものを出す)
            }
    }

    private func card(_ mark: CoachMark) -> some View {
        VStack(alignment: .leading, spacing: MacTheme.Spacing.group) {
            Text(MacCoachMarks.title(mark)).font(.headline)
            Text(MacCoachMarks.body(mark))
                .fixedSize(horizontal: false, vertical: true)
            HStack {
                Button("もう出さない") {
                    store.optOut()
                    finish(mark)
                }
                Spacer()
                Button("わかった") { finish(mark) }
                    .keyboardShortcut(.defaultAction)
            }
        }
        .padding()
        .frame(width: 280)
        .accessibilityIdentifier("coachMark.\(mark.rawValue)")
    }

    private func finish(_ mark: CoachMark) {
        store.dismiss(mark)
        shown = nil
    }
}

extension View {
    func macCoachMarks(_ eligible: Set<CoachMark>) -> some View { modifier(MacCoachMarkPopover(eligible: eligible)) }
}
