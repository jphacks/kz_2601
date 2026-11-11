import EventKit
import EventKitUI
import SwiftUI
import MetamonDSL
import MetamonRenderer

/// `addToCalendar`: 今のレコードを端末のカレンダーに入れる。
/// OS の「新規イベント」画面(EventKitUI)を中身を埋めて開き、ユーザーが「追加」を押す。
/// iOS 17 以降この画面は別プロセスで動くので、カレンダーの許可・Info.plist の説明文・entitlement は要らない。
/// 予定の作り方は Mac と共有する(`Shared/Runtime/CalendarActionSteps.swift`・#978)。
extension AppRuntime {
    func addToCalendar(_ action: Action, _ ctx: ActionContext) {
        switch CalendarActionSteps.draft(action, ctx, dsl: dsl, records: { self.records[$0] ?? [] }) {
        case .ready(let draft):
            calendarDraft = draft
        case .invalid(let message):
            toast = message
        }
    }
}

/// `EKEventEditViewController` を SwiftUI のシートに載せる。閉じる・追加で `onFinish(追加したか)`。
struct CalendarEventEditor: UIViewControllerRepresentable {
    let draft: CalendarEventDraft
    let onFinish: (Bool) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(onFinish: onFinish) }

    func makeUIViewController(context: Context) -> EKEventEditViewController {
        let store = EKEventStore()
        let event = EKEvent(eventStore: store)
        event.title = draft.title
        event.startDate = draft.start
        event.endDate = draft.end
        event.isAllDay = draft.isAllDay
        event.location = draft.location
        event.notes = draft.notes
        if let offset = draft.alertOffset { event.addAlarm(EKAlarm(relativeOffset: offset)) }
        let vc = EKEventEditViewController()
        vc.eventStore = store
        vc.event = event
        vc.editViewDelegate = context.coordinator
        return vc
    }

    func updateUIViewController(_ controller: EKEventEditViewController, context: Context) {}

    final class Coordinator: NSObject, EKEventEditViewDelegate {
        let onFinish: (Bool) -> Void
        init(onFinish: @escaping (Bool) -> Void) { self.onFinish = onFinish }

        func eventEditViewController(_ controller: EKEventEditViewController, didCompleteWith action: EKEventEditViewAction) {
            onFinish(action == .saved)
        }
    }
}
