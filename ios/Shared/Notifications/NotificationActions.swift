import Foundation
import UserNotifications

/// 通知のボタンと返信を受ける土台(#2231・親 #1802)。iOS と Mac で共有する(Mac は後で同じ登録と型を使う)。
/// 今のカテゴリは「開く」だけのもの 1 つ + 試験用の「記録する」「返信」。受けた後の動き(記録を足すなど)は決定(#1775)のあと。
/// 今は応答を `NotificationReply` にまとめて `NotificationReplyInbox` に渡し、ログ(と DEBUG のトースト)だけ。

/// 登録するカテゴリの id。通知の `categoryIdentifier` に入れる。
enum NotificationCategoryID {
    /// 押すと開くだけ(ボタン無し)
    static let open = "metamon.open"
    /// 試験用: 「記録する」ボタンと「返信」の文字入力
    static let testActions = "metamon.test.actions"
}

/// ボタンの id。
enum NotificationActionID {
    static let record = "metamon.action.record"
    static let reply = "metamon.action.reply"
}

enum NotificationCategories {
    /// 起動時に登録するカテゴリ全部。
    static var all: Set<UNNotificationCategory> {
        let open = UNNotificationCategory(identifier: NotificationCategoryID.open, actions: [], intentIdentifiers: [], options: [])
        let record = UNNotificationAction(identifier: NotificationActionID.record, title: L10n.tr("記録する"), options: [])
        let reply = UNTextInputNotificationAction(identifier: NotificationActionID.reply, title: L10n.tr("返信"), options: [],
                                                  textInputButtonTitle: L10n.tr("送信"), textInputPlaceholder: L10n.tr("メッセージ"))
        let test = UNNotificationCategory(identifier: NotificationCategoryID.testActions, actions: [record, reply],
                                          intentIdentifiers: [], options: [])
        return [open, test]
    }

    /// 起動の最初に 1 回(delegate を決めた直後)。同じ id は上書きされるので何度呼んでもよい。
    static func register(on center: UNUserNotificationCenter = .current()) {
        center.setNotificationCategories(all)
    }

    /// 試験用の通知(開発用メニューから)。`seconds` 後にボタン付きで届く。
    static func postTest(appId: String, after seconds: TimeInterval = 5, center: UNUserNotificationCenter = .current()) async throws {
        let content = UNMutableNotificationContent()
        content.title = L10n.tr("試験用の通知")
        content.body = L10n.tr("長押しして「記録する」か「返信」を押してください")
        content.categoryIdentifier = NotificationCategoryID.testActions
        content.userInfo = ["appId": appId]
        let trigger = UNTimeIntervalNotificationTrigger(timeInterval: max(1, seconds), repeats: false)
        try await center.add(UNNotificationRequest(identifier: "test.actions.\(appId)", content: content, trigger: trigger))
    }
}

/// 通知の応答を 1 つの型にまとめたもの。本体はこれだけを受け取る(`UNNotificationResponse` を持ち回らない)。
struct NotificationReply: Equatable, Sendable, CustomStringConvertible {
    enum Action: Equatable, Sendable {
        /// 通知そのものを押した(開く)
        case open
        /// 通知を消した(カテゴリに customDismissAction がある時だけ届く)
        case dismiss
        case record
        case reply
        /// 登録していない id(古い版のカテゴリなど)。中身は捨てずに渡す
        case unknown(String)
    }

    var action: Action
    /// 文字入力のボタンで打った文字(前後の空白を除く・空なら nil)
    var text: String?
    var categoryId: String
    var notificationId: String
    /// userInfo の appId(形が正しい時だけ)
    var appId: String?
    /// userInfo のうち文字列の値だけ(appId も含む)
    var info: [String: String]

    /// 打った文字は出さない
    var description: String {
        "NotificationReply(action: \(action), category: \(categoryId), appId: \(appId ?? "-"), hasText: \(text != nil))"
    }

    static func action(for identifier: String) -> Action {
        switch identifier {
        case UNNotificationDefaultActionIdentifier: .open
        case UNNotificationDismissActionIdentifier: .dismiss
        case NotificationActionID.record: .record
        case NotificationActionID.reply: .reply
        default: .unknown(identifier)
        }
    }

    /// appId の形(backend の appId と同じ: 英数字と `_.:-`・128 文字まで)。
    static func appId(from userInfo: [AnyHashable: Any]) -> String? {
        guard let id = userInfo["appId"] as? String,
              id.range(of: #"^[A-Za-z0-9_.:-]{1,128}$"#, options: .regularExpression) != nil else { return nil }
        return id
    }

    /// 単体テストの対象(`UNNotificationResponse` は作れないので中身で受ける)。
    static func make(actionIdentifier: String, categoryIdentifier: String, notificationId: String,
                     userInfo: [AnyHashable: Any], userText: String?) -> NotificationReply {
        var info: [String: String] = [:]
        for (k, v) in userInfo {
            if let k = k as? String, let v = v as? String { info[k] = v }
        }
        let trimmed = userText?.trimmingCharacters(in: .whitespacesAndNewlines)
        return NotificationReply(action: action(for: actionIdentifier),
                                 text: trimmed?.isEmpty == false ? trimmed : nil,
                                 categoryId: categoryIdentifier,
                                 notificationId: notificationId,
                                 appId: appId(from: userInfo),
                                 info: info)
    }

    init(action: Action, text: String?, categoryId: String, notificationId: String, appId: String?, info: [String: String]) {
        self.action = action
        self.text = text
        self.categoryId = categoryId
        self.notificationId = notificationId
        self.appId = appId
        self.info = info
    }

    init(response: UNNotificationResponse) {
        let request = response.notification.request
        self = Self.make(actionIdentifier: response.actionIdentifier,
                         categoryIdentifier: request.content.categoryIdentifier,
                         notificationId: request.identifier,
                         userInfo: request.content.userInfo,
                         userText: (response as? UNTextInputNotificationResponse)?.userText)
    }

    /// 開くだけの応答か(既存の「押したら開く」の流れに任せる)
    var isPlainOpen: Bool { action == .open }
}

/// 応答の受け口。本体(iOS は `MetamonApp`)が `handler` を差し込む。差し込まれる前に来た分は 1 つだけ持っておく。
@MainActor
final class NotificationReplyInbox {
    static let shared = NotificationReplyInbox()

    private(set) var pending: NotificationReply?
    private var handler: ((NotificationReply) -> Void)?

    func install(_ handler: @escaping (NotificationReply) -> Void) {
        self.handler = handler
        if let pending {
            self.pending = nil
            handler(pending)
        }
    }

    /// 開くだけの応答は渡さない(既存の流れが開く)。
    func receive(_ reply: NotificationReply) {
        guard !reply.isPlainOpen else { return }
        print("[Metamon] notification reply: \(reply)")
        if let handler { handler(reply) } else { pending = reply }
    }
}
