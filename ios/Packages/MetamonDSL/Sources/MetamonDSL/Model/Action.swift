import Foundation

public enum ActionType: String, Decodable, Sendable {
    case navigate, back, saveRecord, deleteRecord, openVoice, openUrl, resetForm
    case scheduleNotification, cancelNotification
    case unknown

    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .unknown) }
}

public enum Presentation: String, Decodable, Sendable {
    case push, sheet, fullScreen
    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .push) }
}

/// ボタン / ツールバー / list タップから実行される副作用(ネストしない・Go の `dsl.Action` と 1:1)。
public struct Action: Decodable, Sendable {
    public var type: ActionType
    public var rawType: String
    /// navigate: 遷移先 / saveRecord: 保存後の遷移先(省略時は back)
    public var screen: String?
    public var presentation: Presentation?
    /// navigate: 親から子を作る(#256)。遷移先フォームの ref field id。今のレコードの id を入れて新規で開く
    public var prefillRef: String?
    /// saveRecord / deleteRecord の対象 entity
    public var entity: String?
    public var url: String?
    /// 指定時は確認ダイアログを先に出す
    public var confirmMessage: String?
    /// scheduleNotification
    public var notification: NotificationSpec?
    /// cancelNotification
    public var notificationId: String?

    private enum Keys: String, CodingKey {
        case type, screen, presentation, prefillRef, entity, url, confirmMessage, notification, notificationId
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        rawType = try c.decodeIfPresent(String.self, forKey: .type) ?? ""
        type = ActionType(rawValue: rawType) ?? .unknown
        screen = try? c.decodeIfPresent(String.self, forKey: .screen)
        presentation = try? c.decodeIfPresent(Presentation.self, forKey: .presentation)
        prefillRef = try? c.decodeIfPresent(String.self, forKey: .prefillRef)
        entity = try? c.decodeIfPresent(String.self, forKey: .entity)
        url = try? c.decodeIfPresent(String.self, forKey: .url)
        confirmMessage = try? c.decodeIfPresent(String.self, forKey: .confirmMessage)
        notification = try? c.decodeIfPresent(NotificationSpec.self, forKey: .notification)
        notificationId = try? c.decodeIfPresent(String.self, forKey: .notificationId)
    }

    /// DSL 外でクライアントが合成するアクション(例: list の長押し削除)。
    public init(type: ActionType, entity: String? = nil, screen: String? = nil,
                presentation: Presentation? = nil, prefillRef: String? = nil) {
        self.type = type
        self.rawType = type.rawValue
        self.entity = entity
        self.screen = screen
        self.presentation = presentation
        self.prefillRef = prefillRef
    }
}
