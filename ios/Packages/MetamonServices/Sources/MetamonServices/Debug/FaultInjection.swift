import Foundation
import MetamonDSL

// 応答の作り物(#2083)。サーバーの断り(402 / 409 / 429 / 401 / 5xx)・圏外・生成の途中で切れるを端末の中で作る。
// 決まりを入れるのはアプリの `#if METAMON_STG` の側だけ(開発用メニュー)。決まりが無い時は何もしない:
// URLProtocol は決まりがある間だけ `HTTP.session` に入り、一致しない通信は本物のサーバーへそのまま行く。サーバーには何も書かない。

/// どの通信に効かせるか
public struct FaultMatch: Sendable, Equatable {
    /// パスの前方一致(区切りごと・`*` は 1 区切りの何でも)。例 `v1/apps/*/ext`。nil = 全部
    public var path: String?
    /// nil = どのメソッドでも
    public var method: String?
    /// クエリに含まれる文字列(例 `stream=1`)。nil = 問わない
    public var query: String?

    public init(path: String? = nil, method: String? = nil, query: String? = nil) {
        self.path = path
        self.method = method
        self.query = query
    }

    public static let all = FaultMatch()

    public func matches(_ request: URLRequest) -> Bool {
        guard let url = request.url else { return false }
        if let method, method.caseInsensitiveCompare(request.httpMethod ?? "GET") != .orderedSame { return false }
        if let query, !(url.query ?? "").contains(query) { return false }
        guard let path else { return true }
        let want = path.split(separator: "/"), have = url.path.split(separator: "/")
        guard want.count <= have.count else { return false }
        return zip(want, have).allSatisfy { $0 == "*" || $0 == $1 }
    }
}

/// 一致した時に返すもの
public enum FaultOutcome: Sendable, Equatable {
    /// ステータスと本文(本文はサーバーの `{error:{code,message,details}}` と同じ形を `error(...)` で作る)
    case response(status: Int, body: Data, headers: [String: String])
    /// 転送のエラー(圏外 = `notConnectedToInternet`)
    case transportError(URLError.Code)
    /// SSE(`text/event-stream`)。`frames` は `event: …\ndata: …\n\n` の 1 件ずつ。`cut` なら送り終えた後に接続を切る
    case events(frames: [String], cut: Bool)
    /// 本物のサーバーへそのまま(遅らせる時に使う)
    case passThrough

    public static let offline = FaultOutcome.transportError(.notConnectedToInternet)

    /// サーバーの `httpx.ErrorWithDetails` と同じ本文の応答
    public static func error(status: Int, code: String, message: String, details: [String: JSONValue] = [:],
                             headers: [String: String] = [:]) -> FaultOutcome {
        .response(status: status, body: errorBody(code: code, message: message, details: details), headers: headers)
    }

    public static func errorBody(code: String, message: String, details: [String: JSONValue] = [:], status: Int? = nil) -> Data {
        var inner: [String: JSONValue] = ["code": .string(code), "message": .string(message)]
        if !details.isEmpty { inner["details"] = .object(details) }
        if let status { inner["status"] = .number(Double(status)) }
        let enc = JSONEncoder()
        enc.outputFormatting = .sortedKeys
        return (try? enc.encode(JSONValue.object(["error": .object(inner)]))) ?? Data()
    }

    /// SSE の 1 件
    public static func frame(event: String, data: String) -> String { "event: \(event)\ndata: \(data)\n\n" }

    /// 生成のストリームの `event: error`(ai の `serveSSE` と同じ形・status 入り)
    public static func errorFrame(status: Int, code: String, message: String, details: [String: JSONValue] = [:]) -> String {
        frame(event: "error", data: String(decoding: errorBody(code: code, message: message, details: details, status: status), as: UTF8.self))
    }

    /// これを受けた `HTTP.send` / 生成のストリームが投げるエラー(自動の送り直しを除く)。エラーにならない(切れて通常の呼び方に戻る・通す)時は nil。
    /// 開発用メニューの見本が、どの案内(`FailureKind` / `ExtDenial` / `AppLimit` / `CoinDenial`)に届くかのテストに使う
    public var expectedError: Error? {
        switch self {
        case .response(let status, let body, let headers):
            do {
                _ = try HTTP.check(HTTP.Response(status: status, data: body, headers: headers))
                return nil
            } catch { return error }
        case .transportError(let code):
            return URLError(code)
        case .events(let frames, _):
            var parser = SSEParser()
            for frame in parser.feed(Data(frames.joined().utf8)) {
                if case .failure(let e)? = GenerateStreamEvent.decode(frame) { return e }
            }
            return nil
        case .passThrough:
            return nil
        }
    }
}

/// 決まり = 対象 × 結果 × 回数
public struct FaultRule: Sendable, Equatable, Identifiable {
    public enum Times: Sendable, Equatable {
        /// やめるまでずっと
        case always
        /// 次の 1 回。`followUp` 秒の間は続く通信にも効かせる(アプリ自身の送り直し: ストリーム → 通常の呼び方・401 / 429 の 1 回再送)
        case once(followUp: TimeInterval)
    }

    public var id: UUID
    /// メニューに出す名前
    public var label: String
    public var match: FaultMatch
    public var outcome: FaultOutcome
    /// 返すまで待つ秒
    public var delay: TimeInterval
    public var times: Times
    /// 最初に効いた時刻(`once` の期限に使う)
    public internal(set) var firstHitAt: Date?
    public internal(set) var hits = 0

    public init(id: UUID = UUID(), label: String, match: FaultMatch, outcome: FaultOutcome, delay: TimeInterval = 0,
                times: Times = .once(followUp: 0)) {
        self.id = id
        self.label = label
        self.match = match
        self.outcome = outcome
        self.delay = delay
        self.times = times
    }

    /// まだ効くか
    func isAlive(at now: Date) -> Bool {
        guard case .once(let followUp) = times, let first = firstHitAt else { return true }
        return now.timeIntervalSince(first) <= followUp
    }
}

/// 決まりの置き場。アプリの `#if METAMON_STG` の側からだけ触る
public enum FaultInjection {
    private static let lock = NSLock()
    nonisolated(unsafe) private static var store: [FaultRule] = []
    nonisolated(unsafe) private static var savedSession: URLSession?
    /// 決まりが変わった時(メニューの表示の更新)。どのスレッドからも呼ばれる
    nonisolated(unsafe) public static var onChange: (@Sendable () -> Void)?
    /// 決まりの有無で `HTTP.session` を差し替えるか(テストは自分のセッションを使うので切る)
    nonisolated(unsafe) static var installsSession = true
    nonisolated(unsafe) static var now: @Sendable () -> Date = { Date() }

    /// 今効いている決まり(期限の切れた `once` は除く)
    public static var rules: [FaultRule] {
        let (list, changed) = lock.withLock { () -> ([FaultRule], Bool) in
            let changed = pruneLocked()
            return (store, changed)
        }
        if changed { onChange?() }
        return list
    }

    public static var isActive: Bool { !rules.isEmpty }

    public static func add(_ rule: FaultRule) {
        lock.withLock {
            store.append(rule)
            syncSessionLocked()
        }
        onChange?()
    }

    public static func remove(id: UUID) {
        lock.withLock {
            store.removeAll { $0.id == id }
            syncSessionLocked()
        }
        onChange?()
    }

    public static func removeAll() {
        lock.withLock {
            store.removeAll()
            syncSessionLocked()
        }
        onChange?()
    }

    /// 一致する決まりがあるか(消費しない・`URLProtocol.canInit`)
    static func hasMatch(_ request: URLRequest) -> Bool {
        lock.withLock {
            let t = now()
            return store.contains { $0.isAlive(at: t) && $0.match.matches(request) }
        }
    }

    /// 一致した最初の決まりを使う(回数を進める)。無ければ nil
    static func take(_ request: URLRequest) -> FaultRule? {
        let (rule, expiry) = lock.withLock { () -> (FaultRule?, TimeInterval?) in
            let t = now()
            _ = pruneLocked()
            guard let i = store.firstIndex(where: { $0.match.matches(request) }) else { return (nil, nil) }
            var expiry: TimeInterval?
            if store[i].firstHitAt == nil {
                store[i].firstHitAt = t
                if case .once(let followUp) = store[i].times { expiry = followUp }
            }
            store[i].hits += 1
            let rule = store[i]
            if case .once(let followUp) = rule.times, followUp <= 0 { store.remove(at: i) }
            syncSessionLocked()
            return (rule, expiry)
        }
        if rule != nil { onChange?() }
        if let expiry, expiry > 0 {
            // 期限で消えたことをメニューに知らせる(`rules` を読むと消える)
            DispatchQueue.global().asyncAfter(deadline: .now() + expiry + 0.1) { _ = rules }
        }
        return rule
    }

    /// 期限の切れた決まりを消す。消したら true
    private static func pruneLocked() -> Bool {
        let t = now()
        let before = store.count
        store.removeAll { !$0.isAlive(at: t) }
        guard store.count != before else { return false }
        syncSessionLocked()
        return true
    }

    private static func syncSessionLocked() {
        guard installsSession else { return }
        if !store.isEmpty, savedSession == nil {
            savedSession = HTTP.session
            HTTP.session = HTTP.makeSession(protocolClasses: [FaultURLProtocol.self] + (URLSessionConfiguration.ephemeral.protocolClasses ?? []))
        } else if store.isEmpty, let saved = savedSession {
            HTTP.session.finishTasksAndInvalidate()
            HTTP.session = saved
            savedSession = nil
        }
    }
}

/// 決まりに一致した通信だけ受ける URLProtocol(`HTTP.session` に決まりがある間だけ入る)
final class FaultURLProtocol: URLProtocol, @unchecked Sendable {
    /// 本物へ通す時のセッション(この URLProtocol は入らない)
    private static let real = HTTP.makeSession()
    private let lock = NSLock()
    private var stopped = false
    private var inner: URLSessionDataTask?

    override class func canInit(with request: URLRequest) -> Bool { FaultInjection.hasMatch(request) }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let rule = FaultInjection.take(request) else { return passThrough() }
        guard rule.delay > 0 else { return deliver(rule.outcome) }
        DispatchQueue.global().asyncAfter(deadline: .now() + rule.delay) { [weak self] in
            self?.deliver(rule.outcome)
        }
    }

    override func stopLoading() {
        lock.withLock {
            stopped = true
            inner?.cancel()
        }
    }

    private var isStopped: Bool { lock.withLock { stopped } }

    private func deliver(_ outcome: FaultOutcome) {
        guard !isStopped, let url = request.url else { return }
        switch outcome {
        case .response(let status, let body, let headers):
            var fields = ["Content-Type": "application/json"]
            fields.merge(headers) { _, h in h }
            respond(url, status: status, headers: fields)
            client?.urlProtocol(self, didLoad: body)
            client?.urlProtocolDidFinishLoading(self)
        case .transportError(let code):
            client?.urlProtocol(self, didFailWithError: URLError(code))
        case .events(let frames, let cut):
            respond(url, status: 200, headers: ["Content-Type": "text/event-stream"])
            DispatchQueue.global().async { [self] in
                for frame in frames {
                    guard !isStopped else { return }
                    client?.urlProtocol(self, didLoad: Data(frame.utf8))
                    Thread.sleep(forTimeInterval: 0.3) // 途中経過が画面に出る間
                }
                guard !isStopped else { return }
                if cut {
                    client?.urlProtocol(self, didFailWithError: URLError(.networkConnectionLost))
                } else {
                    client?.urlProtocolDidFinishLoading(self)
                }
            }
        case .passThrough:
            passThrough()
        }
    }

    private func respond(_ url: URL, status: Int, headers: [String: String]) {
        let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: "HTTP/1.1", headerFields: headers)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
    }

    private func passThrough() {
        let task = Self.real.dataTask(with: request) { [weak self] data, response, error in
            guard let self, !self.isStopped else { return }
            if let error {
                self.client?.urlProtocol(self, didFailWithError: error)
                return
            }
            if let response { self.client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed) }
            if let data { self.client?.urlProtocol(self, didLoad: data) }
            self.client?.urlProtocolDidFinishLoading(self)
        }
        lock.withLock { inner = task }
        task.resume()
    }
}
