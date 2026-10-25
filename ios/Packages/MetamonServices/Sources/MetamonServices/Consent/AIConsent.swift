import Foundation
import Observation

/// 第三者 AI(Anthropic の Claude)へ話した内容などを送ることへの同意(#246・App Store 5.1.2(i))。
/// 端末の UserDefaults に「同意した説明の版」と日時を保存する。説明の中身を変えたら `currentVersion` を上げると全員に出し直す。
@MainActor
@Observable
public final class AIConsentStore {
    /// 説明文(AIConsentView・プライバシーポリシー)の版。送る中身・送り先を変えたら上げる。
    nonisolated public static let currentVersion = 1

    nonisolated static let versionKey = "metamon.aiConsent.version"
    nonisolated static let dateKey = "metamon.aiConsent.date"

    public static let shared = AIConsentStore(defaults: .standard)

    public private(set) var isGranted: Bool
    public private(set) var grantedAt: Date?

    @ObservationIgnored private let defaults: UserDefaults

    public init(defaults: UserDefaults) {
        self.defaults = defaults
        isGranted = Self.isGranted(in: defaults)
        grantedAt = isGranted ? defaults.object(forKey: Self.dateKey) as? Date : nil
    }

    public func grant(now: Date = .now) {
        defaults.set(Self.currentVersion, forKey: Self.versionKey)
        defaults.set(now, forKey: Self.dateKey)
        isGranted = true
        grantedAt = now
    }

    /// 取り消し。以後の生成・対話は、もう一度同意するまで送らない(送信済みのものは取り消せない旨を画面に書く)。
    public func revoke() {
        defaults.removeObject(forKey: Self.versionKey)
        defaults.removeObject(forKey: Self.dateKey)
        isGranted = false
        grantedAt = nil
    }

    /// 初回起動の説明を一度出したか(「あとで」を選んだ人に毎回は出さない。生成しようとした時はまた出る)。
    public var hasBeenPrompted: Bool { defaults.bool(forKey: Self.promptedKey) }
    public func markPrompted() { defaults.set(true, forKey: Self.promptedKey) }
    nonisolated static let promptedKey = "metamon.aiConsent.prompted"

    /// UI テスト用: 保存した同意・既読を消して初回の状態に戻す。
    public func resetForTesting() {
        revoke()
        defaults.removeObject(forKey: Self.promptedKey)
    }

    /// どのスレッドからでも読める判定(`ConsentGatedAIClient` 用)。
    nonisolated public static func isGranted(in defaults: UserDefaults) -> Bool {
        defaults.integer(forKey: versionKey) >= currentVersion
    }
}

public enum AIConsentError: LocalizedError, Sendable, Equatable {
    case notGranted

    public var errorDescription: String? {
        "AI への送信に同意していません。設定の「AI へのデータ送信」から同意すると使えます"
    }
}

/// 同意が無ければ ai サービスへ一切送らない包み(最後の砦)。画面側(AppRuntime)でも事前に止めるが、
/// Siri / ショートカット等ほかの入口から呼ばれても送らないよう、Services の `ai` をこれで包む。
public struct ConsentGatedAIClient: AIClient {
    public let base: any AIClient
    public let isGranted: @Sendable () -> Bool

    public init(base: any AIClient, isGranted: @escaping @Sendable () -> Bool) {
        self.base = base
        self.isGranted = isGranted
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        guard isGranted() else { throw AIConsentError.notGranted }
        return try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
    }

    public func morph(transcript: String, appId: String, currentVersion: Int, locale: String?) async throws -> MorphResult {
        guard isGranted() else { throw AIConsentError.notGranted }
        return try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion, locale: locale)
    }

    public func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
        guard isGranted() else { throw AIConsentError.notGranted }
        return try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
    }

    public func resetChat(appId: String) async {
        guard isGranted() else { return }
        await base.resetChat(appId: appId)
    }
}
