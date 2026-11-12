#if METAMON_STG
import SwiftUI
import MetamonDSL

/// 開発用メニュー: 日付をずらす(#2082)。連続記録・毎日リセット・今日の集計の確認用。
/// ずらすのは式の評価の「今日」(`DSLClock.now()`)だけ。記録の作成日時(createdAt)はずらさない。
final class DebugClockSection: DebugMenuSection {
    override class var order: Int { 0 } // 戻し忘れに気づけるよう先頭
    override class var title: String { "日付" }
    override class var footer: String? {
        "式の評価の「今日」(期間の絞り込み・today / now・毎日リセットのチェック・連続記録・日付の既定値)だけをずらします。"
            + "記録の作成日時(createdAt)はずらしません。通知の予約・保存期間を過ぎた記録の削除はずらした日付で次に組み直す時に効きます。"
            + "起動時に METAMON_DEBUG_TODAY=YYYY-MM-DD を渡すとそちらが優先です。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugClockRows()) }
    @MainActor override class func reset() { DebugClock.apply(day: nil) }
}

/// 日付をずらす本体。保存は `DebugKnobs`(`METAMON_DEBUG_TODAY`・"YYYY-MM-DD")、効かせるのは `DSLClock.dayOffset`
enum DebugClock {
    static let knob = "METAMON_DEBUG_TODAY"

    /// 起動時: 保存した日付(または環境変数)を `DSLClock` に入れる。無ければ 0 を入れる(ずらした時に描き直されるように)
    static func applyLaunch() {
        DSLClock.dayOffset = offset(to: DebugKnobs.string(knob).flatMap(FieldCoding.dateFormatter.date(from:)))
    }

    /// 日付を変える。nil で今日に戻す
    static func apply(day: Date?) {
        DebugKnobs.set(knob, day.map(FieldCoding.dateFormatter.string(from:)))
        applyLaunch()
    }

    /// 今のずらした日から `days` 日動かす
    static func move(days: Int) {
        apply(day: Calendar.current.date(byAdding: .day, value: days, to: DSLClock.now()))
    }

    /// ずらす前の今日から `day` までの日数(nil は 0)
    static func offset(to day: Date?, base: Date = DSLClock.baseNow(), calendar: Calendar = .current) -> Int {
        guard let day else { return 0 }
        return calendar.dateComponents([.day], from: calendar.startOfDay(for: base), to: calendar.startOfDay(for: day)).day ?? 0
    }

    static var isShifted: Bool { DSLClock.dayOffset != 0 }

    /// 「2026-11-13(+1 日)」
    static var label: String {
        let d = DSLClock.dayOffset
        return "\(FieldCoding.dateFormatter.string(from: DSLClock.now()))(\(d > 0 ? "+" : "")\(d) 日)"
    }
}

private struct DebugClockRows: View {
    var body: some View {
        if DebugClock.isShifted {
            Label { Text(verbatim: "日付をずらしています: \(DebugClock.label)") } icon: { Image(systemName: "calendar.badge.exclamationmark") }
                .foregroundStyle(Theme.Colors.warning)
                .accessibilityIdentifier("debugMenu.clock.shifted")
        }
        DatePicker(selection: Binding(get: { DSLClock.now() }, set: { DebugClock.apply(day: $0) }), displayedComponents: .date) {
            DebugKnobLabel(title: "日付を選ぶ", effect: .immediate, envName: DebugClock.knob)
        }
        .accessibilityIdentifier("debugMenu.clock.picker")
        HStack {
            button("−1 日", id: "minus1") { DebugClock.move(days: -1) }
            button("+1 日", id: "plus1") { DebugClock.move(days: 1) }
            button("+7 日", id: "plus7") { DebugClock.move(days: 7) }
            Spacer()
            button("今日に戻す", id: "reset") { DebugClock.apply(day: nil) }
                .disabled(!DebugClock.isShifted)
        }
    }

    private func button(_ title: String, id: String, action: @escaping () -> Void) -> some View {
        Button(action: action) { Text(verbatim: title) }
            .buttonStyle(.bordered)
            .accessibilityIdentifier("debugMenu.clock.\(id)")
    }
}

/// 画面の端の「日付をずらしています」(戻し忘れ防止)。ずらしていない時は何も出さない
struct DebugClockBanner: ViewModifier {
    func body(content: Content) -> some View {
        content.overlay(alignment: .top) {
            if DebugClock.isShifted {
                Text(verbatim: "日付をずらしています \(DebugClock.label)")
                    .font(.caption2.weight(.semibold))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 2)
                    .background(Theme.Colors.warning.opacity(0.85), in: Capsule())
                    .foregroundStyle(.white)
                    .allowsHitTesting(false)
                    .accessibilityIdentifier("debugClock.banner")
            }
        }
    }
}
#endif
