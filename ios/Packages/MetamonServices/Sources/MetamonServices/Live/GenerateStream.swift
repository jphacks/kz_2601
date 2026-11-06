import Foundation
import MetamonDSL

// 生成の途中経過(#636): `POST /v1/generate?stream=1` の SSE を読む。
// 契約: backend/internal/ai/handler.go(`serveSSE`)・generate.go(`Event`)・component_stream.go(`ComponentEvent`)。
// `event: app` / `component` / `restart` は仮置き、`event: result` が通常の応答と同じ本文、`event: error` が通常のエラー封筒。

/// SSE の 1 件(`event:` と `data:` を空行で区切ったもの)。
struct SSEFrame: Equatable {
    var event: String
    var data: String
}

/// SSE の行を組み立てる(バイトの断片を順に流し込む)。`:` で始まる行(keep-alive)と `id:` / `retry:` は読み飛ばす。
/// 区切りの空行が来るまでは 1 件にしない(途中で切れた最後の 1 件は出さない)。
struct SSEParser {
    private var line: [UInt8] = []
    private var event = ""
    private var data: [String] = []

    /// 断片を流し込み、区切りの空行で揃った分を返す。
    mutating func feed<S: Sequence>(_ bytes: S) -> [SSEFrame] where S.Element == UInt8 {
        var out: [SSEFrame] = []
        for b in bytes {
            if b == UInt8(ascii: "\n") {
                if let f = takeLine() { out.append(f) }
            } else {
                line.append(b)
            }
        }
        return out
    }

    /// 1 バイトずつ流す時用(`AsyncBytes`)。
    mutating func feed(byte b: UInt8) -> SSEFrame? {
        if b == UInt8(ascii: "\n") { return takeLine() }
        line.append(b)
        return nil
    }

    private mutating func takeLine() -> SSEFrame? {
        if line.last == UInt8(ascii: "\r") { line.removeLast() }
        let text = String(decoding: line, as: UTF8.self)
        line.removeAll(keepingCapacity: true)
        if text.isEmpty {
            defer { event = ""; data = [] }
            guard !data.isEmpty else { return nil }
            return SSEFrame(event: event.isEmpty ? "message" : event, data: data.joined(separator: "\n"))
        }
        if text.hasPrefix(":") { return nil }
        let field: Substring, value: Substring
        if let colon = text.firstIndex(of: ":") {
            field = text[..<colon]
            var v = text[text.index(after: colon)...]
            if v.first == " " { v = v.dropFirst() }
            value = v
        } else {
            field = Substring(text)
            value = ""
        }
        switch field {
        case "event": event = String(value)
        case "data": data.append(String(value))
        default: break
        }
        return nil
    }
}

/// 生成の SSE の 1 件を型にしたもの。
enum GenerateStreamEvent {
    case progress(GenerateProgress)
    /// 通常の応答と同じ本文(`GenerateResponse`)
    case result(Data)
    case failure(APIError)

    /// 知らない event・壊れた本文は nil(読み飛ばす)。
    static func decode(_ frame: SSEFrame) -> GenerateStreamEvent? {
        let data = Data(frame.data.utf8)
        let dec = JSONDecoder()
        switch frame.event {
        case "app":
            guard let e = try? dec.decode(AppEvent.self, from: data) else { return nil }
            let name = e.name.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty else { return nil }
            return .progress(.app(name: name, tabs: e.tabs ?? [], attempt: e.attempt ?? 0))
        case "component":
            guard let e = try? dec.decode(ComponentEnvelope.self, from: data),
                  !e.component.type.trimmingCharacters(in: .whitespaces).isEmpty else { return nil }
            let c = e.component
            return .progress(.component(StreamedComponent(attempt: e.attempt ?? 0, screen: e.screen ?? "", index: c.index,
                                                          id: c.id, type: c.type, area: c.area, label: c.label)))
        case "restart":
            let attempt = (try? dec.decode(RestartEvent.self, from: data))?.attempt ?? 0
            return .progress(.restart(attempt: attempt))
        case "result":
            return .result(data)
        case "error":
            guard let e = try? dec.decode(ErrorEvent.self, from: data) else {
                return .failure(.http(status: 500, body: frame.data))
            }
            return .failure(.server(status: e.error.status ?? 500, code: e.error.code, message: e.error.message,
                                    details: e.error.details ?? [:]))
        default:
            return nil // summary など(iOS は使わない)
        }
    }

    private struct AppEvent: Decodable {
        var attempt: Int?
        var name: String
        var tabs: [String]?
    }

    private struct ComponentEnvelope: Decodable {
        struct Part: Decodable {
            var index: Int
            var id: String?
            var type: String
            var area: String?
            var label: String?
        }
        var attempt: Int?
        var screen: String?
        var component: Part
    }

    private struct RestartEvent: Decodable { var attempt: Int? }

    private struct ErrorEvent: Decodable {
        struct Inner: Decodable {
            var code: String
            var message: String
            var status: Int?
            var details: [String: JSONValue]?
        }
        var error: Inner
    }
}

extension HTTPAIClient {
    /// stream を諦めて通常の呼び方に戻す理由(エラーとしては外に出さない)。
    enum StreamUnavailable: Error {
        /// 401 / 429 など、通常の呼び方の再送に任せたい応答
        case status(Int)
        /// `result` の前に切れた
        case truncated
    }

    /// 途中経過付きの生成(#636)。SSE が使えない・途中で切れた時は、仮置きを捨てて(`restart` attempt 0)通常の呼び方に戻る。
    /// サーバーが返したエラー(`event: error`・401 / 429 以外の 4xx/5xx)と読めない結果はそのまま投げる(もう一度は呼ばない)。
    func generateStreaming(transcript: String, appId: String, baseVersion: Int?, locale: String?,
                           progress: @escaping @Sendable (GenerateProgress) -> Void) async throws -> MorphResult {
        let body = GenerateBody(appId: appId, baseVersion: baseVersion, transcript: transcript, locale: locale)
        do {
            return try await streamOnce(body: body, appId: appId, progress: progress)
        } catch let error as APIError {
            throw error
        } catch let error as DecodingError {
            throw error // 結果は届いたが読めない: 通常の呼び方と同じく失敗(生成をやり直さない)
        } catch {
            if Task.isCancelled || error is CancellationError { throw CancellationError() }
            print("[Metamon] generate stream unavailable (\(error)) → plain request")
        }
        progress(.restart(attempt: 0))
        return try await generate(transcript: transcript, appId: appId, baseVersion: baseVersion, locale: locale)
    }

    private func streamOnce(body: GenerateBody, appId: String,
                            progress: @Sendable (GenerateProgress) -> Void) async throws -> MorphResult {
        let (bytes, response) = try await HTTP.openStream("POST", baseURL: baseURL, path: "v1/generate?stream=1", body: body,
                                                          token: token, timeout: timeout)
        let status = response?.statusCode ?? -1
        if status == 401 || status == 429 { throw StreamUnavailable.status(status) }
        let contentType = response?.value(forHTTPHeaderField: "Content-Type") ?? ""
        guard (200..<300).contains(status), contentType.contains("text/event-stream") else {
            // stream を知らないサーバー(JSON で返った)・エラーの応答: 本文を全部読んで通常どおりに扱う
            var data = Data()
            for try await b in bytes { data.append(b) }
            let r = HTTP.Response(status: status, data: data, headers: response?.allHeaderFields ?? [:])
            return try finish(HTTP.check(r), appId: appId)
        }
        var parser = SSEParser()
        func handle(_ frame: SSEFrame) throws -> MorphResult? {
            switch GenerateStreamEvent.decode(frame) {
            case .progress(let p): progress(p)
            case .result(let data): return try finish(data, appId: appId)
            case .failure(let e): throw e
            case nil: break
            }
            return nil
        }
        for try await b in bytes {
            if let frame = parser.feed(byte: b), let result = try handle(frame) { return result }
        }
        // 区切りの空行が来ないまま終わった最後の 1 件は捨てる(SSE の決まり)。result が無ければ通常の呼び方へ
        throw StreamUnavailable.truncated
    }

    /// 応答本文 → 結果。キャッシュにも入れる(通常の呼び方と同じ)。
    func finish(_ data: Data, appId: String) throws -> MorphResult {
        let result = try Self.parse(data, appId: appId)
        if let cache, let envelope = Self.envelope(from: data, appId: result.document.appId, version: result.document.version) {
            cache.store(appId: result.document.appId, envelope: envelope)
        }
        return result
    }
}
