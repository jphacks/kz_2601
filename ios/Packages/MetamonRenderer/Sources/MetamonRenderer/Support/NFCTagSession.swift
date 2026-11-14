#if canImport(CoreNFC)
import CoreNFC
#endif
import Foundation

/// NFC タグ(NDEF)を 1 枚読む・書く口(#2233・親 #1838)。部品・アクションはまだ無い(決定待ち)。
/// `DeviceCalendarSource` と同じ形: protocol + 実体(`LiveNFCTagSession`)+ 偽物(`FakeNFCTagSession`)。
/// タグの中身(文字・URL)はログ・クラッシュ報告・サーバーに出さない(`description` も伏せる)。
public protocol NFCTagSessionSource: Sendable {
    /// この端末で NDEF を読めるか(`NFCNDEFReaderSession.readingAvailable`)。
    var isAvailable: Bool { get }
    /// タグを 1 枚読み、分かる種類(文字・URL)のレコードだけ順番どおりに返す。
    func read() async throws -> [NFCTagRecord]
    /// タグ 1 枚に文字か URL を 1 つ書く。
    func write(_ record: NFCTagRecord) async throws
}

/// NDEF の well-known なレコードのうち、この口が扱う 2 種類。
public enum NFCTagRecord: Sendable, Equatable, CustomStringConvertible, CustomDebugStringConvertible {
    /// 文字(`T`)。`language` は BCP 47 の言語コード(`ja`・`en-US` など)。
    case text(String, language: String)
    /// URL(`U`)。短縮の接頭辞は読み書きの時に自動で付け外しする。
    case uri(String)

    /// 中身は出さない(誤ってログに入れても漏れない)
    public var description: String {
        switch self {
        case .text: "NFCTagRecord(text)"
        case .uri: "NFCTagRecord(uri)"
        }
    }

    public var debugDescription: String { description }
}

/// NDEF の 1 レコードを CoreNFC の型を使わずに表したもの(組み立て・読み取りを純関数にするため)。
public struct NFCRawRecord: Sendable, Equatable {
    /// TNF が NFC Forum well-known(0x01)か
    public var isWellKnown: Bool
    /// 種類(`T` / `U`)
    public var type: Data
    public var payload: Data

    public init(isWellKnown: Bool, type: Data, payload: Data) {
        self.isWellKnown = isWellKnown
        self.type = type
        self.payload = payload
    }
}

public enum NFCTagError: LocalizedError, Equatable {
    /// NFC を読めない端末・審査用の版(説明文が無い)
    case unavailable
    case canceled
    case readOnly
    case tooLarge
    /// 文字・URL のレコードが 1 つも無い
    case empty
    case failed

    public var errorDescription: String? {
        switch self {
        case .unavailable: L("この端末では NFC タグを読み取れません")
        case .canceled: L("NFC タグの読み取りを中止しました")
        case .readOnly: L("このタグには書き込めません")
        case .tooLarge: L("内容がタグの容量を超えています")
        case .empty: L("このタグに読み取れる内容がありません")
        case .failed: L("NFC タグの読み取りに失敗しました")
        }
    }
}

/// NDEF の組み立て・読み取り(純関数)。CoreNFC が無い所でも動く。
public enum NFCNDEFCodec {
    /// URI レコードの短縮の接頭辞(NFC Forum URI RTD)。並び順がそのまま識別子の番号。
    public static let uriPrefixes: [String] = [
        "", "http://www.", "https://www.", "http://", "https://", "tel:", "mailto:",
        "ftp://anonymous:anonymous@", "ftp://ftp.", "ftps://", "sftp://", "smb://", "nfs://",
        "ftp://", "dav://", "news:", "telnet://", "imap:", "rtsp://", "urn:", "pop:", "sip:",
        "sips:", "tftp:", "btspp://", "btl2cap://", "btgoep://", "tcpobex://", "irdaobex://",
        "file://", "urn:epc:id:", "urn:epc:tag:", "urn:epc:pat:", "urn:epc:raw:", "urn:epc:", "urn:nfc:",
    ]

    static let textType = Data("T".utf8)
    static let uriType = Data("U".utf8)
    /// 言語コードは状態バイトの下位 6 ビットに入るので 63 バイトまで
    static let maxLanguageBytes = 63

    public static func textPayload(_ text: String, language: String) -> Data {
        let code = Data(normalizedLanguage(language).utf8)
        var payload = Data([UInt8(code.count)])
        payload.append(code)
        payload.append(Data(text.utf8))
        return payload
    }

    public static func uriPayload(_ uri: String) -> Data {
        let index = uriPrefixes.indices.dropFirst()
            .filter { !uriPrefixes[$0].isEmpty && uri.hasPrefix(uriPrefixes[$0]) }
            .max { uriPrefixes[$0].count < uriPrefixes[$1].count } ?? 0
        var payload = Data([UInt8(index)])
        payload.append(Data(uri.dropFirst(uriPrefixes[index].count).utf8))
        return payload
    }

    /// 状態バイト・言語コード・本文に分ける。足りない・読めないバイト列は nil。
    public static func decodeText(payload: Data) -> (text: String, language: String)? {
        let bytes = [UInt8](payload)
        guard let status = bytes.first else { return nil }
        let length = Int(status & 0x3F)
        guard bytes.count >= 1 + length else { return nil }
        guard let language = String(bytes: bytes[1..<(1 + length)], encoding: .utf8) else { return nil }
        let body = Data(bytes[(1 + length)...])
        let text = status & 0x80 == 0 ? String(data: body, encoding: .utf8) : decodeUTF16(body)
        guard let text else { return nil }
        return (text, language)
    }

    public static func decodeURI(payload: Data) -> String? {
        let bytes = [UInt8](payload)
        guard let code = bytes.first, Int(code) < uriPrefixes.count else { return nil }
        guard let rest = String(bytes: bytes.dropFirst(), encoding: .utf8) else { return nil }
        let uri = uriPrefixes[Int(code)] + rest
        return uri.isEmpty ? nil : uri
    }

    public static func raw(from record: NFCTagRecord) -> NFCRawRecord {
        switch record {
        case let .text(text, language):
            NFCRawRecord(isWellKnown: true, type: textType, payload: textPayload(text, language: language))
        case let .uri(uri):
            NFCRawRecord(isWellKnown: true, type: uriType, payload: uriPayload(uri))
        }
    }

    /// 分かる種類だけ返す(名刺・独自の型などは nil にして捨てる)。
    public static func record(from raw: NFCRawRecord) -> NFCTagRecord? {
        guard raw.isWellKnown else { return nil }
        switch raw.type {
        case textType:
            guard let decoded = decodeText(payload: raw.payload) else { return nil }
            return .text(decoded.text, language: decoded.language)
        case uriType:
            guard let uri = decodeURI(payload: raw.payload) else { return nil }
            return .uri(uri)
        default:
            return nil
        }
    }

    public static func records(from raws: [NFCRawRecord]) -> [NFCTagRecord] {
        raws.compactMap(record(from:))
    }

    /// 空・長すぎる言語コードは `ja` に戻す(状態バイトに収まらないため)。
    static func normalizedLanguage(_ language: String) -> String {
        let trimmed = language.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, trimmed.utf8.count <= maxLanguageBytes else { return "ja" }
        return trimmed
    }

    private static func decodeUTF16(_ body: Data) -> String? {
        let bytes = [UInt8](body)
        if bytes.starts(with: [0xFF, 0xFE]) { return String(bytes: bytes.dropFirst(2), encoding: .utf16LittleEndian) }
        if bytes.starts(with: [0xFE, 0xFF]) { return String(bytes: bytes.dropFirst(2), encoding: .utf16BigEndian) }
        return String(bytes: bytes, encoding: .utf16BigEndian)
    }
}

@MainActor
public enum NFCTag {
    /// 読む・書く口の実体。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeNFCTagSession.sample`)。
    public static var source: any NFCTagSessionSource = LiveNFCTagSession()
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。読める中身と端末の状態を外から決められる。
public final class FakeNFCTagSession: NFCTagSessionSource, @unchecked Sendable {
    public var isAvailable: Bool
    public var tag: [NFCTagRecord]
    public var writable: Bool
    public var capacity: Int
    /// 書いた中身(テストで確かめる)
    public private(set) var written: [NFCTagRecord] = []

    public init(isAvailable: Bool = true, tag: [NFCTagRecord] = [], writable: Bool = true, capacity: Int = 512) {
        self.isAvailable = isAvailable
        self.tag = tag
        self.writable = writable
        self.capacity = capacity
    }

    public func read() async throws -> [NFCTagRecord] {
        guard isAvailable else { throw NFCTagError.unavailable }
        let records = NFCNDEFCodec.records(from: tag.map(NFCNDEFCodec.raw(from:)))
        guard !records.isEmpty else { throw NFCTagError.empty }
        return records
    }

    public func write(_ record: NFCTagRecord) async throws {
        guard isAvailable else { throw NFCTagError.unavailable }
        guard writable else { throw NFCTagError.readOnly }
        guard NFCNDEFCodec.raw(from: record).payload.count <= capacity else { throw NFCTagError.tooLarge }
        written.append(record)
        tag = [record]
    }

    public static var sample: FakeNFCTagSession {
        FakeNFCTagSession(tag: [.uri("https://tailor-it.app"), .text("ようこそ", language: "ja")])
    }
}

// MARK: - 実体

#if canImport(CoreNFC)

/// Core NFC の `NFCNDEFReaderSession` で 1 枚だけ読む / 書く。
/// Info.plist に説明文が無いビルド(審査用の版など)では読み取りを始めずに `.unavailable` を返す(始めると落ちるため)。
public final class LiveNFCTagSession: NFCTagSessionSource, @unchecked Sendable {
    public init() {}

    static let usageKey = "NFCReaderUsageDescription"

    /// 説明文の有無は生の Info.plist で見る(`object(forInfoDictionaryKey:)` は訳を返すので、審査用ビルドで
    /// Info.plist から消しても `.lproj` に訳が残っていると「ある」と判定してしまう)。
    static func hasUsageDescription(in info: [String: Any]?) -> Bool {
        guard let text = info?[usageKey] as? String else { return false }
        return !text.isEmpty
    }

    public var isAvailable: Bool {
        NFCNDEFReaderSession.readingAvailable && Self.hasUsageDescription(in: Bundle.main.infoDictionary)
    }

    public func read() async throws -> [NFCTagRecord] {
        guard isAvailable else { throw NFCTagError.unavailable }
        let raws = try await NFCNDEFRunner.run(prompt: L("iPhone をタグに近づけてください"), write: nil)
        let records = NFCNDEFCodec.records(from: raws)
        guard !records.isEmpty else { throw NFCTagError.empty }
        return records
    }

    public func write(_ record: NFCTagRecord) async throws {
        guard isAvailable else { throw NFCTagError.unavailable }
        _ = try await NFCNDEFRunner.run(prompt: L("書き込むタグに iPhone を近づけてください"), write: NFCNDEFCodec.raw(from: record))
    }
}

/// 1 回のセッションぶんの delegate。読む時は最初の 1 枚、書く時は最初の 1 枚に書いて終わる。
private final class NFCNDEFRunner: NSObject, NFCNDEFReaderSessionDelegate, @unchecked Sendable {
    private var continuation: CheckedContinuation<[NFCRawRecord], Error>?
    private var session: NFCNDEFReaderSession?
    private let write: NFCRawRecord?
    private var finished = false

    private init(write: NFCRawRecord?) { self.write = write }

    static func run(prompt: String, write: NFCRawRecord?) async throws -> [NFCRawRecord] {
        let runner = NFCNDEFRunner(write: write)
        return try await withCheckedThrowingContinuation { continuation in
            runner.continuation = continuation
            let session = NFCNDEFReaderSession(delegate: runner, queue: nil, invalidateAfterFirstRead: false)
            session.alertMessage = prompt
            runner.session = session
            runner.retain()
            session.begin()
        }
    }

    private static let lock = NSLock()
    private static var held: [ObjectIdentifier: NFCNDEFRunner] = [:]

    private func retain() {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        Self.held[ObjectIdentifier(self)] = self
    }

    private func finish(_ result: Result<[NFCRawRecord], Error>) {
        Self.lock.lock()
        let pending = finished ? nil : continuation
        finished = true
        continuation = nil
        Self.held[ObjectIdentifier(self)] = nil
        Self.lock.unlock()
        pending?.resume(with: result)
    }

    func readerSessionDidBecomeActive(_ session: NFCNDEFReaderSession) {}

    func readerSession(_ session: NFCNDEFReaderSession, didDetectNDEFs messages: [NFCNDEFMessage]) {}

    func readerSession(_ session: NFCNDEFReaderSession, didDetect tags: [NFCNDEFTag]) {
        guard let tag = tags.first else { return }
        session.connect(to: tag) { [weak self] error in
            guard let self else { return }
            if error != nil {
                session.invalidate(errorMessage: L("タグに近づけ直してください"))
                return
            }
            tag.queryNDEFStatus { status, capacity, _ in
                switch status {
                case .notSupported:
                    self.finish(.failure(NFCTagError.failed))
                    session.invalidate()
                case .readOnly where self.write != nil:
                    self.finish(.failure(NFCTagError.readOnly))
                    session.invalidate()
                default:
                    if let raw = self.write {
                        self.perform(write: raw, capacity: capacity, on: tag, session: session)
                    } else {
                        self.perform(read: tag, session: session)
                    }
                }
            }
        }
    }

    func readerSession(_ session: NFCNDEFReaderSession, didInvalidateWithError error: Error) {
        let code = (error as? NFCReaderError)?.code
        let canceled = code == .readerSessionInvalidationErrorUserCanceled
        finish(.failure(canceled ? NFCTagError.canceled : NFCTagError.failed))
    }

    private func perform(read tag: NFCNDEFTag, session: NFCNDEFReaderSession) {
        tag.readNDEF { [weak self] message, _ in
            guard let self else { return }
            guard let message else {
                self.finish(.failure(NFCTagError.empty))
                session.invalidate()
                return
            }
            self.finish(.success(message.records.map(Self.raw)))
            session.invalidate()
        }
    }

    private func perform(write raw: NFCRawRecord, capacity: Int, on tag: NFCNDEFTag, session: NFCNDEFReaderSession) {
        let message = NFCNDEFMessage(records: [
            NFCNDEFPayload(format: .nfcWellKnown, type: raw.type, identifier: Data(), payload: raw.payload),
        ])
        guard message.length <= capacity else {
            finish(.failure(NFCTagError.tooLarge))
            session.invalidate()
            return
        }
        tag.writeNDEF(message) { [weak self] error in
            guard let self else { return }
            self.finish(error == nil ? .success([raw]) : .failure(NFCTagError.failed))
            session.invalidate()
        }
    }

    private static func raw(_ payload: NFCNDEFPayload) -> NFCRawRecord {
        NFCRawRecord(isWellKnown: payload.typeNameFormat == .nfcWellKnown, type: payload.type, payload: payload.payload)
    }
}

#else

/// CoreNFC が無い環境(macOS)では常に使えない。
public struct LiveNFCTagSession: NFCTagSessionSource {
    public init() {}
    public var isAvailable: Bool { false }
    public func read() async throws -> [NFCTagRecord] { throw NFCTagError.unavailable }
    public func write(_ record: NFCTagRecord) async throws { throw NFCTagError.unavailable }
}

#endif
