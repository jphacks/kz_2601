#if canImport(CoreNFC)
import CoreNFC
#endif
import Foundation

/// 交通系 IC カード(FeliCa・システムコード 0003)の残高と履歴を読む口(#2246・親 #1690)。
/// 部品・アクションはまだ無い(#1690 の決定待ち)。`NFCTagSessionSource`(#2233)と同じ形:
/// protocol + 実体(`LiveFeliCaTransitReader`)+ 偽物(`FakeTransitCardReader`)+ 純関数(`FeliCaTransitCodec`)。
/// カードの中身(残高・履歴)は持ち主の行動が分かるので、ログ・クラッシュ報告・サーバーに出さない(`description` も伏せる)。
public protocol TransitCardReaderSource: Sendable {
    /// この端末で交通系 IC カードを読めるか(Core NFC が使える・説明文とシステムコードの申告がある)。
    var isAvailable: Bool { get }
    /// カードを 1 枚読み、残高と履歴(古い順)を返す。
    func read() async throws -> TransitCard
}

/// 読み取った 1 枚のカード。
public struct TransitCard: Sendable, Equatable, CustomStringConvertible, CustomDebugStringConvertible {
    /// 今の残高(円)。履歴の一番新しい記録に入っている値。
    public var balance: Int
    /// 履歴(古い順)。カードに残っている分だけ(交通系は直近 20 件ほど)。
    public var history: [TransitTransaction]

    public init(balance: Int, history: [TransitTransaction]) {
        self.balance = balance
        self.history = history
    }

    /// 中身は出さない(誤ってログに入れても漏れない)
    public var description: String { "TransitCard(\(history.count) 件・非表示)" }
    public var debugDescription: String { description }
}

/// 履歴 1 件。
public struct TransitTransaction: Sendable, Equatable, CustomStringConvertible, CustomDebugStringConvertible {
    /// 利用した日。カードに日付が入っていない記録(未使用の枠など)では nil。
    public var day: TransitDay?
    public var kind: TransitTransactionKind
    /// この記録の直後の残高(円)。
    public var balance: Int
    /// 増減(円・+ がチャージ、- が支払い)。1 つ前の記録が無い一番古い記録では nil。
    public var amount: Int?

    public init(day: TransitDay?, kind: TransitTransactionKind, balance: Int, amount: Int? = nil) {
        self.day = day
        self.kind = kind
        self.balance = balance
        self.amount = amount
    }

    public var description: String { "TransitTransaction(非表示)" }
    public var debugDescription: String { description }
}

/// カードに入っている日付(年月日だけ・時刻は入っていない)。
public struct TransitDay: Sendable, Equatable {
    public var year: Int
    public var month: Int
    public var day: Int

    public init(year: Int, month: Int, day: Int) {
        self.year = year
        self.month = month
        self.day = day
    }

    /// 日本時間のその日の 0 時。カードの日付は日本時間で書かれている。
    public var date: Date? {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "Asia/Tokyo") ?? .gmt
        return calendar.date(from: DateComponents(year: year, month: month, day: day))
    }
}

/// 履歴の種類(公開されている利用種別のうち、よく出るものだけ)。
public enum TransitTransactionKind: String, Sendable, CaseIterable {
    /// 改札を出た時の運賃の支払い
    case fare
    /// チャージ(入金)
    case charge
    /// 券売機などでの購入
    case ticket
    /// 精算・のりこし
    case settlement
    /// 物販(コンビニなどでの支払い)
    case purchase
    /// バス
    case bus
    /// 新規発行
    case issued
    /// 改札に入った(出場の記録と対で残るもの)
    case entry
    /// 知らない利用種別
    case unknown
}

public enum TransitCardError: LocalizedError, Equatable {
    /// Core NFC を使えない端末・説明文やシステムコードの申告が無いビルド(審査用の版など)
    case unavailable
    case canceled
    /// 交通系 IC カードではない(システムコード 0003 を持たない)
    case notTransitCard
    /// 読めたが履歴が 1 件も無い
    case empty
    case failed

    public var errorDescription: String? {
        switch self {
        case .unavailable: L("この端末では交通系 IC カードを読み取れません")
        case .canceled: L("交通系 IC カードの読み取りを中止しました")
        case .notTransitCard: L("交通系 IC カードではないようです")
        case .empty: L("このカードから読み取れる履歴がありません")
        case .failed: L("交通系 IC カードの読み取りに失敗しました")
        }
    }
}

/// 履歴のバイト列を残高・履歴に変える純関数。Core NFC が無い所でも動く。
/// 元にしたのは公開されている交通系 IC カード(サイバネ規格)の履歴 16 バイトの並び。
/// 分からない所(連番・駅コードなど)には触らない。
public enum FeliCaTransitCodec {
    /// 履歴のサービスコード(0x090F)。FeliCa のコマンドにはリトルエンディアンで渡す。
    public static let historyServiceCode: UInt16 = 0x090F
    /// 交通系 IC カードのシステムコード(0x0003)。
    public static let transitSystemCode: UInt16 = 0x0003
    /// Info.plist に書くシステムコードの文字(`com.apple.developer.nfc.readersession.felica.systemcodes`)。
    public static let transitSystemCodeText = "0003"
    /// 履歴 1 件のバイト数。
    public static let blockSize = 16
    /// 読みにいく履歴の件数(交通系は 20 件ほどしか残らない)。
    public static let historyBlockCount = 20

    /// 利用種別(2 バイト目)から種類へ。知らない値は `.unknown`。
    public static func kind(processCode: UInt8) -> TransitTransactionKind {
        switch processCode {
        case 0x01, 0x13: .fare
        case 0x02, 0x17, 0x1F, 0x48, 0x49: .charge
        case 0x03, 0x23: .ticket
        case 0x04, 0x05, 0x06, 0x07, 0x08, 0x84, 0x85: .settlement
        case 0x46, 0x4A, 0xC6: .purchase
        case 0x0D, 0x0F, 0x8F: .bus
        case 0x10, 0x11: .issued
        case 0x14, 0x4B, 0xCB: .entry
        default: .unknown
        }
    }

    /// 年月日の 2 バイト(上位 7 ビット = 2000 年からの年・次の 4 ビット = 月・下位 5 ビット = 日)。
    /// 0 や月日がありえない値なら nil(未使用の枠)。年は 2000 年から 127 年ぶん数えられる。
    public static func day(high: UInt8, low: UInt8) -> TransitDay? {
        let packed = UInt16(high) << 8 | UInt16(low)
        let year = 2000 + Int(packed >> 9)
        let month = Int((packed >> 5) & 0x0F)
        let day = Int(packed & 0x1F)
        guard (1...12).contains(month), (1...31).contains(day) else { return nil }
        return TransitDay(year: year, month: month, day: day)
    }

    /// 残高の 2 バイト(11・12 バイト目・リトルエンディアン)。
    public static func balance(low: UInt8, high: UInt8) -> Int {
        Int(UInt16(high) << 8 | UInt16(low))
    }

    /// 履歴 1 件を読む。16 バイトでない・すべて 0(未使用の枠)なら nil。増減はここでは入らない。
    public static func transaction(fromBlock block: Data) -> TransitTransaction? {
        let bytes = [UInt8](block)
        guard bytes.count == blockSize, bytes.contains(where: { $0 != 0 }) else { return nil }
        return TransitTransaction(
            day: day(high: bytes[4], low: bytes[5]),
            kind: kind(processCode: bytes[1]),
            balance: balance(low: bytes[10], high: bytes[11])
        )
    }

    /// 履歴のバイト列(カードから読んだ順 = 新しい順)から 1 枚ぶんにまとめる。
    /// 返す履歴は古い順で、増減は 1 つ前の記録との残高の差。読める記録が 1 件も無ければ nil。
    public static func card(fromBlocks blocks: [Data]) -> TransitCard? {
        let newestFirst = blocks.compactMap(transaction(fromBlock:))
        guard let balance = newestFirst.first?.balance else { return nil }
        var history = newestFirst.reversed().map { $0 }
        for index in history.indices.dropFirst() {
            history[index].amount = history[index].balance - history[index - 1].balance
        }
        return TransitCard(balance: balance, history: history)
    }

    /// Info.plist にシステムコード 0003 の申告があるか。
    /// 申告が無いまま `NFCTagReaderSession` を始めると落ちるので、始める前にここで見る。
    public static func hasTransitSystemCode(in info: [String: Any]?) -> Bool {
        let codes = info?["com.apple.developer.nfc.readersession.felica.systemcodes"] as? [String] ?? []
        return codes.contains { $0.caseInsensitiveCompare(transitSystemCodeText) == .orderedSame }
    }
}

@MainActor
public enum TransitCardReader {
    /// 読み取り口の実体。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeTransitCardReader.sample`)。
    public static var source: any TransitCardReaderSource = LiveFeliCaTransitReader()
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。読めるカードと端末の状態を外から決められる。
public final class FakeTransitCardReader: TransitCardReaderSource, @unchecked Sendable {
    public var isAvailable: Bool
    /// 読めるカード。nil なら `.empty`。
    public var card: TransitCard?
    /// 読んだ回数(テストで確かめる)
    public private(set) var readCount = 0

    public init(isAvailable: Bool = true, card: TransitCard? = nil) {
        self.isAvailable = isAvailable
        self.card = card
    }

    public func read() async throws -> TransitCard {
        guard isAvailable else { throw TransitCardError.unavailable }
        readCount += 1
        guard let card else { throw TransitCardError.empty }
        return card
    }

    /// 固定のカード(残高 2,480 円・履歴 4 件)。バイト列から作るので、並びの決まりと食い違わない。
    public static var sample: FakeTransitCardReader {
        FakeTransitCardReader(card: FeliCaTransitCodec.card(fromBlocks: sampleBlocks))
    }

    /// 新しい順(カードから読める順)。古い方から チャージ 3,000 → 運賃 -220 → 物販 -160 → 運賃 -140。
    public static let sampleBlocks: [Data] = [
        Data([0x16, 0x01, 0x00, 0x02, 0x35, 0x2A, 0x00, 0x7A, 0x00, 0x91, 0xB0, 0x09, 0x00, 0x00, 0x12, 0x00]),
        Data([0xC7, 0x46, 0x00, 0x00, 0x35, 0x2A, 0x5B, 0x40, 0x00, 0x00, 0x3C, 0x0A, 0x00, 0x00, 0x11, 0x00]),
        Data([0x16, 0x01, 0x00, 0x02, 0x35, 0x29, 0x00, 0x7A, 0x00, 0x91, 0xDC, 0x0A, 0x00, 0x00, 0x10, 0x00]),
        Data([0x16, 0x02, 0x00, 0x00, 0x35, 0x29, 0x00, 0x7A, 0x00, 0x00, 0xB8, 0x0B, 0x00, 0x00, 0x0F, 0x00]),
    ]
}

// MARK: - 実体

#if canImport(CoreNFC)

/// Core NFC の `NFCTagReaderSession`(`.iso18092`)で交通系 IC カードを 1 枚読む。
/// Info.plist に説明文とシステムコード 0003 が無いビルド(審査用の版など)では始めずに `.unavailable` を返す。
public final class LiveFeliCaTransitReader: TransitCardReaderSource, @unchecked Sendable {
    public init() {}

    static let usageKey = "NFCReaderUsageDescription"

    /// 説明文の有無は生の Info.plist で見る(`object(forInfoDictionaryKey:)` は訳を返すので、審査用ビルドで
    /// Info.plist から消しても `.lproj` に訳が残っていると「ある」と判定してしまう)。
    static func hasUsageDescription(in info: [String: Any]?) -> Bool {
        guard let text = info?[usageKey] as? String else { return false }
        return !text.isEmpty
    }

    public var isAvailable: Bool {
        NFCTagReaderSession.readingAvailable
            && Self.hasUsageDescription(in: Bundle.main.infoDictionary)
            && FeliCaTransitCodec.hasTransitSystemCode(in: Bundle.main.infoDictionary)
    }

    public func read() async throws -> TransitCard {
        guard isAvailable else { throw TransitCardError.unavailable }
        let blocks = try await FeliCaHistoryRunner.run(prompt: L("iPhone を交通系 IC カードに近づけてください"))
        guard let card = FeliCaTransitCodec.card(fromBlocks: blocks) else { throw TransitCardError.empty }
        return card
    }
}

/// 1 回のセッションぶんの delegate。最初に見つけた FeliCa のカードから履歴を読んで終わる。
private final class FeliCaHistoryRunner: NSObject, NFCTagReaderSessionDelegate, @unchecked Sendable {
    /// 1 回の `readWithoutEncryption` で頼む件数(多くのカードは 1 コマンド 4 件まで)。
    private static let chunkSize = 4

    private var continuation: CheckedContinuation<[Data], Error>?
    private var finished = false

    private override init() {}

    static func run(prompt: String) async throws -> [Data] {
        let runner = FeliCaHistoryRunner()
        guard let session = NFCTagReaderSession(pollingOption: .iso18092, delegate: runner, queue: nil) else {
            throw TransitCardError.unavailable
        }
        return try await withCheckedThrowingContinuation { continuation in
            runner.continuation = continuation
            session.alertMessage = prompt
            runner.retain()
            session.begin()
        }
    }

    private static let lock = NSLock()
    private static var held: [ObjectIdentifier: FeliCaHistoryRunner] = [:]

    private func retain() {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        Self.held[ObjectIdentifier(self)] = self
    }

    private func finish(_ result: Result<[Data], Error>) {
        Self.lock.lock()
        let pending = finished ? nil : continuation
        finished = true
        continuation = nil
        Self.held[ObjectIdentifier(self)] = nil
        Self.lock.unlock()
        pending?.resume(with: result)
    }

    func tagReaderSessionDidBecomeActive(_ session: NFCTagReaderSession) {}

    func tagReaderSession(_ session: NFCTagReaderSession, didInvalidateWithError error: Error) {
        let canceled = (error as? NFCReaderError)?.code == .readerSessionInvalidationErrorUserCanceled
        finish(.failure(canceled ? TransitCardError.canceled : TransitCardError.failed))
    }

    func tagReaderSession(_ session: NFCTagReaderSession, didDetect tags: [NFCTag]) {
        guard let tag = tags.first else { return }
        guard case let .feliCa(card) = tag else {
            finish(.failure(TransitCardError.notTransitCard))
            session.invalidate()
            return
        }
        session.connect(to: tag) { [weak self] error in
            guard let self else { return }
            if error != nil {
                session.invalidate(errorMessage: L("カードに近づけ直してください"))
                return
            }
            guard card.currentSystemCode == Self.systemCodeData else {
                self.finish(.failure(TransitCardError.notTransitCard))
                session.invalidate()
                return
            }
            self.readHistory(from: card, startingAt: 0, collected: [], session: session)
        }
    }

    /// システムコード(0x0003)は上位バイトから並ぶ。
    private static let systemCodeData = Data([
        UInt8(FeliCaTransitCodec.transitSystemCode >> 8),
        UInt8(FeliCaTransitCodec.transitSystemCode & 0xFF),
    ])

    /// サービスコードはリトルエンディアンで渡す(0x090F → 0F 09)。
    private static let serviceCodeData = Data([
        UInt8(FeliCaTransitCodec.historyServiceCode & 0xFF),
        UInt8(FeliCaTransitCodec.historyServiceCode >> 8),
    ])

    /// 4 件ずつ読み、エラー・空になった所で止める(残っている履歴の数はカードによって違う)。
    private func readHistory(from card: NFCFeliCaTag, startingAt start: Int, collected: [Data], session: NFCTagReaderSession) {
        guard start < FeliCaTransitCodec.historyBlockCount else {
            finishHistory(collected, session: session)
            return
        }
        let count = min(Self.chunkSize, FeliCaTransitCodec.historyBlockCount - start)
        let blockList = (start..<(start + count)).map { Data([0x80, UInt8($0)]) }
        card.readWithoutEncryption(serviceCodeList: [Self.serviceCodeData], blockList: blockList) { [weak self] status1, _, blocks, error in
            guard let self else { return }
            guard error == nil, status1 == 0, !blocks.isEmpty else {
                self.finishHistory(collected, session: session)
                return
            }
            let next = collected + blocks
            // 未使用の枠(すべて 0)まで来たら、そこから先は読まない
            if blocks.contains(where: { FeliCaTransitCodec.transaction(fromBlock: $0) == nil }) {
                self.finishHistory(next, session: session)
            } else {
                self.readHistory(from: card, startingAt: start + count, collected: next, session: session)
            }
        }
    }

    private func finishHistory(_ blocks: [Data], session: NFCTagReaderSession) {
        if blocks.contains(where: { FeliCaTransitCodec.transaction(fromBlock: $0) != nil }) {
            finish(.success(blocks))
            session.invalidate()
        } else {
            finish(.failure(TransitCardError.empty))
            session.invalidate()
        }
    }
}

#else

/// Core NFC が無い環境(macOS)では常に使えない。
public struct LiveFeliCaTransitReader: TransitCardReaderSource {
    public init() {}
    public var isAvailable: Bool { false }
    public func read() async throws -> TransitCard { throw TransitCardError.unavailable }
}

#endif
