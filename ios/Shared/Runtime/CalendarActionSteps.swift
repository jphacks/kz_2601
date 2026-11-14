import Foundation
import MetamonDSL
import MetamonRenderer

/// `addToCalendar` の前段(どの記録を・どんな予定にするか)と、書き込みの手順。UIKit / EventKitUI に依存しないので
/// iOS と Mac で共有する(#978)。iOS は OS の「新規イベント」画面に渡し、Mac は自前の確認シートで見せてから EventKit で書く。
@MainActor
enum CalendarActionSteps {
    enum Draft {
        /// 予定にできた
        case ready(CalendarEventDraft)
        /// 予定にしない。toast に出す一言
        case invalid(String)
    }

    /// フォーム画面なら入力中の値(未保存でも)、list の行タップならその行から予定を作る。
    static func draft(_ action: Action, _ ctx: ActionContext, dsl: AppDSL, records: (String) -> [Record]) -> Draft {
        guard let spec = action.calendarEvent, let entityId = action.entity ?? ctx.form?.entity.id,
              let entity = dsl.entity(entityId) else {
            return .invalid(L10n.tr("カレンダーに入れる記録が見つかりません"))
        }
        let data: [String: JSONValue]?
        if let form = ctx.form, form.entity.id == entityId {
            data = form.payload
        } else if let id = ctx.recordId {
            data = records(entityId).first { $0.id == id }?.data
        } else {
            data = nil
        }
        guard let data else {
            return .invalid(L10n.tr("カレンダーに入れる記録が見つかりません"))
        }
        switch CalendarEventPlan.build(spec, entity: entity, data: data) {
        case .success(let draft):
            return .ready(draft)
        case .failure(.noStart(let name)):
            return .invalid(L10n.tr("「\(name)」を入力してください"))
        }
    }

    enum SaveResult: Equatable {
        case added
        /// 許可されていない(今断った・前に断った・制限されている)
        case denied
        /// 書けなかった(既定のカレンダーが無いなど)
        case failed
    }

    /// 中身を見せて「追加」を押された後に呼ぶ: 許可を確かめ(初回だけ OS が聞く)、書く。
    static func save(_ draft: CalendarEventDraft, using writer: any CalendarWriting) async -> SaveResult {
        do {
            guard try await writer.requestWriteAccess() else { return .denied }
            try await writer.add(draft)
            return .added
        } catch {
            return .failed
        }
    }
}

/// カレンダーへの書き込み口。Mac は EventKit(`EventKitCalendarWriter`)、テストはフェイクを差す。
protocol CalendarWriting: Sendable {
    /// 予定を足す許可(書き込みだけ)。未決定なら OS が聞く。許可されていれば true
    func requestWriteAccess() async throws -> Bool
    /// 既定のカレンダーに足す
    func add(_ draft: CalendarEventDraft) async throws
}
