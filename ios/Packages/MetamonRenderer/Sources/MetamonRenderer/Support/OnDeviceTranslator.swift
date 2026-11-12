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
    enum Kind { case prepare, translate([String]) }

    /// `TranslationSession.Configuration` を型消去で持つ(iOS 18 限定の型は stored property に置けないため)。
    @Published private var configurationBox: Any?
    private lazy var jobs: OnDeviceTranslationJobQueue<Kind> = {
        let q = OnDeviceTranslationJobQueue<Kind>()
        q.dispatch = { [weak self] source, target in
            guard let self else { return }
            guard #available(iOS 18.0, *) else { return }
            if var current = self.configuration, current.source == source, current.target == target {
                current.invalidate() // 同じ組でも action をもう一度走らせる
                self.configuration = current
            } else {
                self.configuration = TranslationSession.Configuration(source: source, target: target)
            }
        }
        return q
    }()

    @available(iOS 18.0, *)
    var configuration: TranslationSession.Configuration? {
        get { configurationBox as? TranslationSession.Configuration }
        set { configurationBox = newValue }
    }

    @available(iOS 18.0, *)
    func submit(_ kind: Kind, from source: Locale.Language, to target: Locale.Language) async throws -> [String] {
        guard attachedCount > 0 else { throw OnDeviceTranslationError.hostMissing }
        return try await jobs.submit(kind, from: source, to: target)
    }

    /// `.translationTask` の action。今の依頼と組が合う時だけこの session で処理する。
    @available(iOS 18.0, *)
    func handle(_ session: TranslationSession) async {
        guard let job = jobs.begin(source: session.sourceLanguage, target: session.targetLanguage) else { return }
        do {
            switch job.payload {
            case .prepare:
                try await session.prepareTranslation()
                jobs.finish(job.token, .success([]))
            case .translate(let texts):
                let responses = try await session.translations(from: texts.map { .init(sourceText: $0) })
                jobs.finish(job.token, .success(responses.map(\.targetText)))
            }
        } catch {
            jobs.finish(job.token, .failure(OnDeviceTranslationError.failed))
        }
    }

    /// 見えない View が外れたら積んだ依頼を失敗で返す(待ち続けない)。
    func detach() {
        attachedCount -= 1
        guard attachedCount == 0 else { return }
        jobs.failAll(OnDeviceTranslationError.hostMissing)
    }
    #else
    func detach() { attachedCount -= 1 }
    #endif
}

// MARK: - 依頼の順番待ち(OS に依らない・テスト対象)

/// 依頼を 1 件ずつ流す。流した依頼は token で追い、別の依頼の action・遅れた結果で取り違えない。
/// `dispatch` してから `fireTimeout` 以内に action が来なければ `.failed` で返す。
@MainActor final class OnDeviceTranslationJobQueue<Payload> {
    struct Job {
        let token: Int
        let payload: Payload
        let source: Locale.Language
        let target: Locale.Language
        let continuation: CheckedContinuation<[String], Error>
    }

    /// 先頭を流す時に呼ぶ(host は configuration を立てる)。
    var dispatch: (Locale.Language, Locale.Language) -> Void = { _, _ in }
    let fireTimeout: Duration

    private var pending: [Job] = []
    private var current: Job?
    private var started = false
    private var timeout: Task<Void, Never>?
    private var nextToken = 0

    init(fireTimeout: Duration = .seconds(10)) { self.fireTimeout = fireTimeout }

    /// 待っている依頼(流し中を含む)の数
    var count: Int { pending.count + (current == nil ? 0 : 1) }

    func submit(_ payload: Payload, from source: Locale.Language, to target: Locale.Language) async throws -> [String] {
        try await withCheckedThrowingContinuation { c in
            nextToken += 1
            pending.append(Job(token: nextToken, payload: payload, source: source, target: target, continuation: c))
            pump()
        }
    }

    /// action が来た。今の依頼と組が合えば返す。合わなければ同じ依頼をもう一度流して nil(時間切れは延ばさない)。
    func begin(source: Locale.Language?, target: Locale.Language?) -> Job? {
        guard let job = current, !started else { return nil }
        guard Self.same(source, job.source), Self.same(target, job.target) else {
            dispatch(job.source, job.target)
            return nil
        }
        started = true
        timeout?.cancel()
        timeout = nil
        return job
    }

    /// 結果を返して次へ。もう今の依頼でない token(時間切れ・外れた後)は捨てる。
    func finish(_ token: Int, _ result: Result<[String], Error>) {
        guard let job = current, job.token == token else { return }
        clearCurrent()
        job.continuation.resume(with: result)
        pump()
    }

    func failAll(_ error: Error) {
        let all = (current.map { [$0] } ?? []) + pending
        clearCurrent()
        pending = []
        all.forEach { $0.continuation.resume(throwing: error) }
    }

    private func pump() {
        guard current == nil, !pending.isEmpty else { return }
        let job = pending.removeFirst()
        current = job
        started = false
        let token = job.token, wait = fireTimeout
        timeout = Task { [weak self] in
            try? await Task.sleep(for: wait)
            guard !Task.isCancelled else { return }
            self?.timedOut(token)
        }
        dispatch(job.source, job.target)
    }

    private func timedOut(_ token: Int) {
        guard current?.token == token, !started else { return }
        finish(token, .failure(OnDeviceTranslationError.failed))
    }

    private func clearCurrent() {
        timeout?.cancel()
        timeout = nil
        current = nil
        started = false
    }

    private static func same(_ a: Locale.Language?, _ b: Locale.Language) -> Bool {
        a?.minimalIdentifier == b.minimalIdentifier
    }
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
