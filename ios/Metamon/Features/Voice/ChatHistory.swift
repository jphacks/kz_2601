import Foundation
import MetamonServices

/// アプリごとのチャット履歴(#210)。反映(「これでいく」)で会話を閉じても、ここに残して次にシートを開いた時に見せる。
/// - user / assistant = 吹き出し、applied = 反映の区切り(「ここで反映しました(v3)・〜」)
struct ChatHistoryEntry: Codable, Equatable, Identifiable, Sendable {
    enum Kind: String, Codable, Sendable { case user, assistant, applied }
    var id = UUID()
    var kind: Kind
    var text: String
    /// applied の時だけ。反映後の version
    var version: Int?
}

/// 履歴の保存先。UserDefaults に appId ごとの JSON(直近 `limit` 件)。
/// アプリ削除(#195 purge)で `remove(appId:)`、アカウント削除(wipe)で `removeAll()`。
/// モック起動(`METAMON_MOCK=1`・UI テスト)とユニットテストは、前の起動・別のテストの履歴を持ち込まないようメモリだけに置く
/// (`AppRuntime` ごとに新しい保存先。`METAMON_KEEP_CHAT_HISTORY=1` なら実機と同じ UserDefaults)。
final class ChatHistoryStore: @unchecked Sendable {
    static let limit = 30
    static let keyPrefix = "metamon.chatHistory."
    static let shared = ChatHistoryStore(defaults: .standard)

    /// `AppRuntime` の既定の保存先。
    static func forRuntime() -> ChatHistoryStore {
        let env = ProcessInfo.processInfo.environment
        let isTest = env["METAMON_MOCK"] == "1" || env["XCTestConfigurationFilePath"] != nil
        return isTest && env["METAMON_KEEP_CHAT_HISTORY"] != "1" ? ChatHistoryStore() : shared
    }

    private let defaults: UserDefaults?
    private var memory: [String: Data] = [:]
    private let lock = NSLock()

    init(defaults: UserDefaults) {
        self.defaults = defaults
    }

    /// メモリだけ(テスト・モック)。
    init() {
        self.defaults = nil
    }

    func load(appId: String) -> [ChatHistoryEntry] {
        guard let data = read(Self.keyPrefix + appId),
              let entries = try? JSONDecoder().decode([ChatHistoryEntry].self, from: data) else { return [] }
        return Self.trimmed(entries)
    }

    func save(_ entries: [ChatHistoryEntry], appId: String) {
        let kept = Self.trimmed(entries)
        guard !kept.isEmpty else { return remove(appId: appId) }
        if let data = try? JSONEncoder().encode(kept) { write(data, Self.keyPrefix + appId) }
    }

    func remove(appId: String) {
        write(nil, Self.keyPrefix + appId)
    }

    func removeAll() {
        guard let defaults else { return lock.withLock { memory = [:] } }
        defaults.dictionaryRepresentation().keys
            .filter { $0.hasPrefix(Self.keyPrefix) }
            .forEach(defaults.removeObject(forKey:))
    }

    /// 直近 `limit` 件(区切りも 1 件と数える)。
    static func trimmed(_ entries: [ChatHistoryEntry]) -> [ChatHistoryEntry] {
        entries.count > limit ? Array(entries.suffix(limit)) : entries
    }

    private func read(_ key: String) -> Data? {
        guard let defaults else { return lock.withLock { memory[key] } }
        return defaults.data(forKey: key)
    }

    private func write(_ data: Data?, _ key: String) {
        guard let defaults else { return lock.withLock { memory[key] = data } }
        if let data { defaults.set(data, forKey: key) } else { defaults.removeObject(forKey: key) }
    }
}

/// AI に送る文脈の組み立て(#210)。backend の契約(`POST /v1/chat` の messages)は変えない。
/// 反映済みの古いやり取りは今の DSL と食い違う指示を含むので送らない。送るのは
/// 1. 過去の変更の要約(区切りの 1 行ずつ・最大 `maxSummaries` 件)を assistant の 1 メッセージにして先頭に
///    (user にすると backend の turn 数・依頼文の予備に混ざるため assistant)
/// 2. 最後の反映より後のやり取り(切替で閉じた未反映の会話)
/// 3. 今の会話
enum ChatContext {
    static let maxSummaries = 5

    static func messages(history: [ChatHistoryEntry], live: [ChatMessage], maxSummaries: Int = maxSummaries) -> [ChatMessage] {
        let start = history.lastIndex { $0.kind == .applied }.map { $0 + 1 } ?? history.startIndex
        let tail = history[start...].compactMap { entry -> ChatMessage? in
            switch entry.kind {
            case .user: ChatMessage(role: .user, text: entry.text)
            case .assistant: ChatMessage(role: .assistant, text: entry.text)
            case .applied: nil
            }
        }
        let summaries = history.filter { $0.kind == .applied }.suffix(maxSummaries)
        var out: [ChatMessage] = []
        if !summaries.isEmpty {
            let lines = summaries.map { "- " + summaryLine($0) }.joined(separator: "\n")
            out.append(ChatMessage(role: .assistant, text: L10n.tr("(これまでに反映した変更)") + "\n" + lines))
        }
        return out + tail + live
    }

    static func summaryLine(_ entry: ChatHistoryEntry) -> String {
        let version = entry.version.map { "v\($0)" } ?? ""
        let text = entry.text.trimmingCharacters(in: .whitespacesAndNewlines)
        if text.isEmpty { return version }
        return version.isEmpty ? text : "\(version): \(text)"
    }

    /// 区切りの表示文。「ここで反映しました(v3)・〜」
    static func dividerText(_ entry: ChatHistoryEntry) -> String {
        let head = entry.version.map { L10n.tr("ここで反映しました(v\($0))") } ?? L10n.tr("ここで反映しました")
        let text = entry.text.trimmingCharacters(in: .whitespacesAndNewlines)
        return text.isEmpty ? head : head + "・" + text
    }
}
