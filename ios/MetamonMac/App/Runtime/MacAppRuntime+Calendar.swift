import SwiftUI
import AppKit
import EventKit
import MetamonDSL
import MetamonRenderer

/// `addToCalendar` / `lookupBook` を Mac で(#978)。
/// iOS は OS の「新規イベント」画面(EventKitUI)を開くが、Mac には無いので、中身を見せる自前のシート(`MacCalendarSheet`)を出し、
/// 「追加」を押された後に初めて許可(書き込みだけ)を求めて EventKit で書く。予定の作り方は iOS と共有(`CalendarActionSteps`)。
extension MacAppRuntime {
    func addToCalendar(_ action: Action, _ ctx: ActionContext) {
        switch CalendarActionSteps.draft(action, ctx, dsl: dsl, records: { self.records(for: $0) }) {
        case .ready(let draft):
            calendarDraft = draft
        case .invalid(let message):
            toast = message
        }
    }

    /// 本の情報は iOS と同じ手順(`BookLookupSteps`)。Mac はまだ appconfig につながらない(#930)ので、
    /// `books` が nil の間は「読み込めません。手で入力してください」になる(iOS の METAMON_MOCK と同じ)。
    func lookupBook(_ action: Action, _ ctx: ActionContext) async {
        if let message = await BookLookupSteps.run(action, form: ctx.form, client: books) { toast = message }
    }
}

/// EventKit で既定のカレンダーに書く。許可は書き込みだけ(`requestWriteOnlyAccessToEvents`・macOS 14)。
final class EventKitCalendarWriter: CalendarWriting, @unchecked Sendable {
    private let store = EKEventStore()

    enum WriteError: Error { case noCalendar }

    func requestWriteAccess() async throws -> Bool {
        switch EKEventStore.authorizationStatus(for: .event) {
        case .fullAccess, .writeOnly:
            return true
        case .notDetermined:
            return try await store.requestWriteOnlyAccessToEvents()
        default: // denied・restricted
            return false
        }
    }

    func add(_ draft: CalendarEventDraft) async throws {
        guard let calendar = store.defaultCalendarForNewEvents else { throw WriteError.noCalendar }
        let event = EKEvent(eventStore: store)
        event.calendar = calendar
        event.title = draft.title
        event.startDate = draft.start
        event.endDate = draft.end
        event.isAllDay = draft.isAllDay
        event.location = draft.location
        event.notes = draft.notes
        if let offset = draft.alertOffset { event.addAlarm(EKAlarm(relativeOffset: offset)) }
        try store.save(event, span: .thisEvent, commit: true)
    }
}

/// 「カレンダーに追加」のシート。中身を見せ、「追加」を押したら許可を確かめて書く。結果はシートの中で知らせる。
struct MacCalendarSheet: View {
    let draft: CalendarEventDraft
    let writer: any CalendarWriting
    let close: () -> Void

    private enum Phase: Equatable { case review, saving, done(CalendarActionSteps.SaveResult) }
    @State private var phase: Phase = .review

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text("カレンダーに追加").font(.title2.bold())
            details
            Text(message)
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            buttons
        }
        .padding(24)
        .frame(minWidth: 420, idealWidth: 460)
    }

    private var details: some View {
        Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 16, verticalSpacing: 10) {
            row("予定") { Text(draft.title) }
            row("日時") { when }
            if let location = draft.location { row("場所") { Text(location) } }
            if let notes = draft.notes { row("メモ") { Text(notes).lineLimit(6) } }
            if let offset = draft.alertOffset { row("通知") { Text(Self.alertText(offset)) } }
        }
        .textSelection(.enabled)
    }

    private func row(_ label: LocalizedStringKey, @ViewBuilder _ value: () -> some View) -> some View {
        GridRow {
            Text(label).foregroundStyle(.secondary).gridColumnAlignment(.trailing)
            value()
        }
    }

    @ViewBuilder private var when: some View {
        if draft.isAllDay {
            let days = Calendar.current.isDate(draft.start, inSameDayAs: draft.end)
                ? draft.start.formatted(.dateTime.year().month().day().weekday())
                : (draft.start ..< draft.end).formatted(.interval.year().month().day().weekday())
            Text("\(days)(終日)")
        } else {
            Text((draft.start ..< draft.end).formatted(.interval.year().month().day().weekday().hour().minute()))
        }
    }

    static func alertText(_ offset: TimeInterval) -> String {
        let minutes = Int((-offset / 60).rounded())
        return minutes == 0 ? L10n.tr("始まる時に通知") : L10n.tr("始まる \(minutes) 分前に通知")
    }

    private var message: String {
        switch phase {
        case .review, .saving:
            return L10n.tr("「追加」を押すと、Mac のカレンダーの既定のカレンダーに入ります。初めての時は、カレンダーへの追加を許可するか聞かれます。")
        case .done(.added):
            return L10n.tr("カレンダーに追加しました")
        case .done(.denied):
            return L10n.tr("カレンダーへの追加が許可されていません。システム設定の「プライバシーとセキュリティ」→「カレンダー」で許可してください。")
        case .done(.failed):
            return L10n.tr("カレンダーに追加できませんでした。カレンダー アプリで既定のカレンダーを確かめてください。")
        }
    }

    @ViewBuilder private var buttons: some View {
        HStack {
            Spacer()
            switch phase {
            case .review, .saving:
                Button("キャンセル", role: .cancel, action: close)
                    .keyboardShortcut(.cancelAction)
                Button("追加") { Task { await add() } }
                    .keyboardShortcut(.defaultAction)
                    .disabled(phase == .saving)
            case .done(.added):
                Button("カレンダーで開く") { Self.openCalendarApp(); close() }
                Button("閉じる", action: close).keyboardShortcut(.defaultAction)
            case .done(.denied):
                Button("システム設定を開く") { Self.openPrivacySettings() }
                Button("閉じる", action: close).keyboardShortcut(.defaultAction)
            case .done(.failed):
                Button("閉じる", action: close).keyboardShortcut(.defaultAction)
            }
        }
    }

    private func add() async {
        guard phase == .review else { return } // 連打で 2 件入れない
        phase = .saving
        let result = await CalendarActionSteps.save(draft, using: writer)
        phase = .done(result)
        MacVoiceOver.announce(message)
    }

    static func openCalendarApp() {
        guard let app = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.apple.iCal") else { return }
        NSWorkspace.shared.openApplication(at: app, configuration: NSWorkspace.OpenConfiguration())
    }

    static func openPrivacySettings() {
        guard let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars") else { return }
        NSWorkspace.shared.open(url)
    }
}
