import Foundation
import SwiftUI
#if canImport(Translation) && os(iOS)
import Translation
#endif

/// 端末の中で文字を訳す口(#1999・親 #1825)。Apple の Translation framework(iOS 18+)で動き、文字は端末外に出ない。
/// アクション `translate`・AI の決まり・DeepL との切り替えは #1775 の決定待ちなので、ここは「訳す所」と「言語の用意の案内」だけ。
/// Translation に触るのはこのファイルだけ(iOS 17 / macOS でもビルドできるよう `canImport` + `#available` で囲う)。

// MARK: - 状態

/// 言語の組の用意の状態(`LanguageAvailability.Status` と同じ 3 つ)。
public enum OnDeviceTranslationStatus: Equatable, Sendable {
    /// 端末に入っている。機内モードでも訳せる
    case installed
    /// 対応しているが未ダウンロード。`prepare` で OS のダウンロードの画面を出す
    case supported
    /// この組・この OS では訳せない(iOS 17・macOS も)
    case unsupported

    /// 訳す前に出す案内(入っている時は nil)。
    public var guidance: String? {
        switch self {
        case .installed: return nil
        case .supported: return L("翻訳する言語をダウンロードしてください")
        case .unsupported: return L("この端末ではこの言語を翻訳できません")
        }
    }
}

public enum OnDeviceTranslationError: LocalizedError, Equatable, Sendable {
    /// 訳せない組(または iOS 17)
    case unsupported
    /// 言語が未ダウンロード(先に `prepare`)
    case notInstalled
    /// 見えない View(`onDeviceTranslationHost`)が画面に付いていない
    case hostMissing
    /// ダウンロードの画面を閉じた・OS の翻訳が失敗した
    case failed

    public var errorDescription: String? {
        switch self {
        case .unsupported: return OnDeviceTranslationStatus.unsupported.guidance
        case .notInstalled: return OnDeviceTranslationStatus.supported.guidance
        case .hostMissing, .failed: return L("翻訳できませんでした")
        }
    }
}

// MARK: - 口

public protocol OnDeviceTranslator: Sendable {
    func availability(from source: Locale.Language, to target: Locale.Language) async -> OnDeviceTranslationStatus
    /// 言語が入っていなければ OS のダウンロードの画面を出す(入っていれば何もしない)。
    func prepare(from source: Locale.Language, to target: Locale.Language) async throws
    /// `OnDeviceTranslationLimit.clip` で切ってから訳す。返す数は切った後の数。
    func translate(_ texts: [String], from source: Locale.Language, to target: Locale.Language) async throws -> [String]
}

// MARK: - 上限(純関数・テスト対象)

/// 1 回に訳す量の上限。長い記録をそのまま渡さない。
public enum OnDeviceTranslationLimit {
    /// 1 回の件数
    public static let maxTexts = 50
    /// 1 件の文字数
    public static let maxCharactersPerText = 2000
    /// 1 回の合計文字数(使い切ったら残りの件は落とす)
    public static let maxTotalCharacters = 10000

    public static func clip(_ texts: [String]) -> [String] {
        var budget = maxTotalCharacters
        var out: [String] = []
        for text in texts.prefix(maxTexts) {
            guard budget > 0 else { break }
            let piece = String(text.prefix(min(maxCharactersPerText, budget)))
            budget -= piece.count
            out.append(piece)
        }
        return out
    }
}

// MARK: - 選び方

public enum OnDeviceTranslators {
    /// mock(`METAMON_MOCK=1`・テスト)→ 偽物 / iOS 18+ → 本物 / それ以外(iOS 17・macOS)→ 使えない。
    @MainActor public static func make(mock: Bool, host: OnDeviceTranslationHost? = nil) -> any OnDeviceTranslator {
        if mock { return FakeOnDeviceTranslator() }
        #if canImport(Translation) && os(iOS)
        if #available(iOS 18.0, *), let host { return SystemOnDeviceTranslator(host: host) }
        #endif
        return UnsupportedOnDeviceTranslator()
    }
}

/// iOS 17・macOS(Mac は対象外 #1999)。常に `unsupported`。
public struct UnsupportedOnDeviceTranslator: OnDeviceTranslator {
    public init() {}
    public func availability(from source: Locale.Language, to target: Locale.Language) async -> OnDeviceTranslationStatus { .unsupported }
    public func prepare(from source: Locale.Language, to target: Locale.Language) async throws { throw OnDeviceTranslationError.unsupported }
    public func translate(_ texts: [String], from source: Locale.Language, to target: Locale.Language) async throws -> [String] {
        throw OnDeviceTranslationError.unsupported
    }
}

// MARK: - 偽物

/// テスト・`METAMON_MOCK=1` 用。組ごとの状態を決めておけ、訳は `[<target>] <原文>` を返す。
public actor FakeOnDeviceTranslator: OnDeviceTranslator {
    public struct Pair: Hashable, Sendable {
        public var source: String
        public var target: String
        public init(_ source: String, _ target: String) { self.source = source; self.target = target }
        init(_ source: Locale.Language, _ target: Locale.Language) {
            self.init(source.minimalIdentifier, target.minimalIdentifier)
        }
    }

    private var statuses: [Pair: OnDeviceTranslationStatus]
    private let defaultStatus: OnDeviceTranslationStatus
    /// `prepare` でダウンロードを終えるか(false = 画面を閉じた)
    private let downloadSucceeds: Bool
    public private(set) var translatedBatches: [[String]] = []
    public private(set) var prepareCount = 0

    public init(statuses: [Pair: OnDeviceTranslationStatus] = [:], defaultStatus: OnDeviceTranslationStatus = .installed,
                downloadSucceeds: Bool = true) {
        self.statuses = statuses
        self.defaultStatus = defaultStatus
        self.downloadSucceeds = downloadSucceeds
    }

    public func availability(from source: Locale.Language, to target: Locale.Language) async -> OnDeviceTranslationStatus {
        statuses[Pair(source, target)] ?? defaultStatus
    }

    public func prepare(from source: Locale.Language, to target: Locale.Language) async throws {
        prepareCount += 1
        switch await availability(from: source, to: target) {
        case .installed: return
        case .unsupported: throw OnDeviceTranslationError.unsupported
        case .supported:
            guard downloadSucceeds else { throw OnDeviceTranslationError.failed }
            statuses[Pair(source, target)] = .installed
        }
    }

    public func translate(_ texts: [String], from source: Locale.Language, to target: Locale.Language) async throws -> [String] {
        switch await availability(from: source, to: target) {
        case .unsupported: throw OnDeviceTranslationError.unsupported
        case .supported: throw OnDeviceTranslationError.notInstalled
        case .installed: break
        }
        let clipped = OnDeviceTranslationLimit.clip(texts)
        translatedBatches.append(clipped)
        return clipped.map { "[\(target.minimalIdentifier)] \($0)" }
    }
}

// MARK: - 本物(iOS 18+)

/// `TranslationSession` は iOS 18〜25 では SwiftUI の `.translationTask` の中でしか作れない。
/// 見えない View(`onDeviceTranslationHost`)に載せ、ここに積んだ依頼を 1 件ずつ流す橋渡し。
/// iOS 26+ で言語が入っていれば `TranslationSession(installedSource:target:)` で直接作る(ダウンロードの画面は出せないので prepare は橋渡し)。
@MainActor public final class OnDeviceTranslationHost: ObservableObject {
    public init() {}

    /// 画面に付いている見えない View の数。0 なら依頼を積まずに `hostMissing`。
    var attachedCount = 0

    #if canImport(Translation) && os(iOS)
    struct Job {
        enum Kind { case prepare, translate([String]) }
        let kind: Kind
        let source: Locale.Language
        let target: Locale.Language
        let continuation: CheckedContinuation<[String], Error>
    }

    /// `TranslationSession.Configuration` を型消去で持つ(iOS 18 限定の型は stored property に置けないため)。
    @Published private var configurationBox: Any?
    private var queue: [Job] = []
    private var running = false

    @available(iOS 18.0, *)
    var configuration: TranslationSession.Configuration? {
        get { configurationBox as? TranslationSession.Configuration }
        set { configurationBox = newValue }
    }

    @available(iOS 18.0, *)
    func submit(_ kind: Job.Kind, from source: Locale.Language, to target: Locale.Language) async throws -> [String] {
        guard attachedCount > 0 else { throw OnDeviceTranslationError.hostMissing }
        return try await withCheckedThrowingContinuation { c in
            queue.append(Job(kind: kind, source: source, target: target, continuation: c))
            pump()
        }
    }

    @available(iOS 18.0, *)
    private func pump() {
        guard !running, let job = queue.first else { return }
        running = true
        if var current = configuration, current.source == job.source, current.target == job.target {
            current.invalidate() // 同じ組でも action をもう一度走らせる
            configuration = current
        } else {
            configuration = TranslationSession.Configuration(source: job.source, target: job.target)
        }
    }

    /// `.translationTask` の action。先頭の依頼をこの session で処理する。
    @available(iOS 18.0, *)
    func handle(_ session: TranslationSession) async {
        guard !queue.isEmpty else { running = false; return }
        let job = queue.removeFirst()
        do {
            switch job.kind {
            case .prepare:
                try await session.prepareTranslation()
                job.continuation.resume(returning: [])
            case .translate(let texts):
                let responses = try await session.translations(from: texts.map { .init(sourceText: $0) })
                job.continuation.resume(returning: responses.map(\.targetText))
            }
        } catch {
            job.continuation.resume(throwing: OnDeviceTranslationError.failed)
        }
        running = false
        pump()
    }

    /// 見えない View が外れたら積んだ依頼を失敗で返す(待ち続けない)。
    func detach() {
        attachedCount -= 1
        guard attachedCount == 0 else { return }
        let pending = queue
        queue = []
        running = false
        pending.forEach { $0.continuation.resume(throwing: OnDeviceTranslationError.hostMissing) }
    }
    #else
    func detach() { attachedCount -= 1 }
    #endif
}

#if canImport(Translation) && os(iOS)
@available(iOS 18.0, *)
final class SystemOnDeviceTranslator: OnDeviceTranslator, @unchecked Sendable {
    private let host: OnDeviceTranslationHost

    init(host: OnDeviceTranslationHost) { self.host = host }

    func availability(from source: Locale.Language, to target: Locale.Language) async -> OnDeviceTranslationStatus {
        switch await LanguageAvailability().status(from: source, to: target) {
        case .installed: return .installed
        case .supported: return .supported
        case .unsupported: return .unsupported
        @unknown default: return .unsupported
        }
    }

    func prepare(from source: Locale.Language, to target: Locale.Language) async throws {
        switch await availability(from: source, to: target) {
        case .installed: return
        case .unsupported: throw OnDeviceTranslationError.unsupported
        case .supported: _ = try await host.submit(.prepare, from: source, to: target)
        }
    }

    func translate(_ texts: [String], from source: Locale.Language, to target: Locale.Language) async throws -> [String] {
        switch await availability(from: source, to: target) {
        case .unsupported: throw OnDeviceTranslationError.unsupported
        case .supported: throw OnDeviceTranslationError.notInstalled
        case .installed: break
        }
        let clipped = OnDeviceTranslationLimit.clip(texts)
        guard !clipped.isEmpty else { return [] }
        if #available(iOS 26.0, *) {
            let session = TranslationSession(installedSource: source, target: target)
            do {
                return try await session.translations(from: clipped.map { .init(sourceText: $0) }).map(\.targetText)
            } catch {
                throw OnDeviceTranslationError.failed
            }
        }
        return try await host.submit(.translate(clipped), from: source, to: target)
    }
}
#endif

// MARK: - 見えない View

struct OnDeviceTranslationHostModifier: ViewModifier {
    @ObservedObject var host: OnDeviceTranslationHost

    func body(content: Content) -> some View {
        content.background {
            Color.clear
                .frame(width: 0, height: 0)
                .accessibilityHidden(true)
                .modifier(TranslationTaskModifier(host: host))
                .onAppear { host.attachedCount += 1 }
                .onDisappear { host.detach() }
        }
    }
}

private struct TranslationTaskModifier: ViewModifier {
    @ObservedObject var host: OnDeviceTranslationHost

    func body(content: Content) -> some View {
        #if canImport(Translation) && os(iOS)
        if #available(iOS 18.0, *) {
            content.translationTask(host.configuration) { session in
                await host.handle(session)
            }
        } else {
            content
        }
        #else
        content
        #endif
    }
}

public extension View {
    /// 端末の中の翻訳の橋渡しを画面に付ける(root に 1 つ)。iOS 17・macOS では何もしない。
    func onDeviceTranslationHost(_ host: OnDeviceTranslationHost) -> some View {
        modifier(OnDeviceTranslationHostModifier(host: host))
    }
}
