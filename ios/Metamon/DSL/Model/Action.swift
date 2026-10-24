import Foundation

enum ActionType: String, Decodable, Sendable {
    case navigate, back, saveRecord, deleteRecord, openVoice, openUrl, resetForm
    case scheduleNotification, cancelNotification
    case unknown

    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .unknown) }
}

enum Presentation: String, Decodable, Sendable {
    case push, sheet, fullScreen
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .push) }
}

/// ボタン / ツールバー / list タップから実行される副作用(ネストしない・Go の `dsl.Action` と 1:1)。
struct Action: Decodable, Sendable {
    var type: ActionType
    var rawType: String
    /// navigate: 遷移先 / saveRecord: 保存後の遷移先(省略時は back)
    var screen: String?
    var presentation: Presentation?
    /// saveRecord / deleteRecord の対象 entity
    var entity: String?
    var url: String?
    /// 指定時は確認ダイアログを先に出す
    var confirmMessage: String?
    /// scheduleNotification
    var notification: NotificationSpec?
    /// cancelNotification
    var notificationId: String?

    private enum Keys: String, CodingKey {
        case type, screen, presentation, entity, url, confirmMessage, notification, notificationId
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        rawType = try c.decodeIfPresent(String.self, forKey: .type) ?? ""
        type = ActionType(rawValue: rawType) ?? .unknown
        screen = try? c.decodeIfPresent(String.self, forKey: .screen)
        presentation = try? c.decodeIfPresent(Presentation.self, forKey: .presentation)
        entity = try? c.decodeIfPresent(String.self, forKey: .entity)
        url = try? c.decodeIfPresent(String.self, forKey: .url)
        confirmMessage = try? c.decodeIfPresent(String.self, forKey: .confirmMessage)
        notification = try? c.decodeIfPresent(NotificationSpec.self, forKey: .notification)
        notificationId = try? c.decodeIfPresent(String.self, forKey: .notificationId)
    }
}
