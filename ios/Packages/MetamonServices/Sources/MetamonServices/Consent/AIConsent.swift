import Foundation
import Observation
import MetamonDSL

/// 第三者 AI(Anthropic の Claude)へ話した内容などを送ることへの同意(#246・App Store 5.1.2(i))。
/// 端末の UserDefaults に「同意した説明の版」と日時を保存する。説明の中身を変えたら `currentVersion` を上げると全員に出し直す。
@MainActor
@Observable
public final class AIConsentStore {
    /// 説明文(AIConsentView・プライバシーポリシー)の版。送る中身・送り先を変えたら上げる(ポリシー 6 章)。
    /// 1: 初版(#246)。2: 「AI で書く」の範囲(要約・分類・書き直し・抜き出し・提案)と 4KB・端末内 AI なら送らない(ポリシー 2026-11 改定)。
    /// 3: 長い答えは最大 16KB・いつもオンライン / aiChat 直近 20 発言・16KB / 写真のボタン 1 枚(JPEG・1024px)/ 共有は自分の記録だけ(ポリシー 2026-11-08 改定)。
    nonisolated public static let currentVersion = 3

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

    /// 保存先を直接消した後(アカウント削除の端末データ削除)に、画面の状態を保存先に合わせ直す。
    /// 合わせないと画面は「同意済み」のまま送信の砦(`ConsentGatedAIClient`)だけが止め、同意の画面も出なくなる。
    public func reload() {
        isGranted = Self.isGranted(in: defaults)
        grantedAt = isGranted ? defaults.object(forKey: Self.dateKey) as? Date : nil
    }

    /// 初回起動の説明を一度出したか(「あとで」を選んだ人に毎回は出さない。生成しようとした時はまた出る)。
    public var hasBeenPrompted: Bool { defaults.bool(forKey: Self.promptedKey) }
    public func markPrompted() {
        defaults.set(true, forKey: Self.promptedKey)
        defaults.set(Self.currentVersion, forKey: Self.promptedVersionKey)
    }
    nonisolated static let promptedKey = "metamon.aiConsent.prompted"
    /// 最後に説明を出した版。古い版に同意した人へ、新しい版の説明を起動時に 1 回だけ出すため。
    nonisolated static let promptedVersionKey = "metamon.aiConsent.promptedVersion"

    /// 古い版の説明に同意したままの人(送る中身を変えたので、もう一度同意が要る)。取り消した人は含めない。
    public var needsReconsent: Bool {
        !isGranted && defaults.integer(forKey: Self.versionKey) > 0
    }

    /// 起動時に同意の画面を出すか。初回(まだ出していない)と、版が上がって出し直す時(その版で 1 回だけ)。
    /// 「あとで」の人には毎回は出さない(生成しようとした時に出る)。
    public var shouldPromptOnLaunch: Bool {
        guard !isGranted else { return false }
        if !hasBeenPrompted { return true }
        return needsReconsent && defaults.integer(forKey: Self.promptedVersionKey) < Self.currentVersion
    }

    /// UI テスト用: 保存した同意・既読を消して初回の状態に戻す。
    public func resetForTesting() {
        revoke()
        defaults.removeObject(forKey: Self.promptedKey)
        defaults.removeObject(forKey: Self.promptedVersionKey)
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

    public func morph(transcript: String, appId: String, currentVersion: Int, locale: String?,
                      progress: @escaping @Sendable (GenerateProgress) -> Void) async throws -> MorphResult {
        guard isGranted() else { throw AIConsentError.notGranted }
        return try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion, locale: locale, progress: progress)
    }

    public func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
        guard isGranted() else { throw AIConsentError.notGranted }
        return try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
    }

    public func resetChat(appId: String) async {
        guard isGranted() else { return }
        await base.resetChat(appId: appId)
    }

    public func text(_ request: AITextRequest) async throws -> AITextResult {
        guard isGranted() else { throw AIConsentError.notGranted }
        return try await base.text(request)
    }
}
