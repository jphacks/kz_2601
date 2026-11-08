import Foundation
import MetamonDSL

/// aiButton `inputFeeds`: 同じ画面の `feed` が出している行を、generate の材料の末尾に「外部データ」の節として足す。
/// 行はホストの `FeedHost`(同じ問い合わせはキャッシュ = 画面の表示と同じ行)から取る。合わせて 20 行・2 KB まで。
/// 検証は backend `dsl/ai_input_feed.go`。審査用ビルド(feed を外す)では何もしない。
enum AIFeedInput {
    static let maxRows = 20
    static let maxBytes = 2048
    /// まだ読み込んでいない時に待つ長さ(秒)。
    static let waitSeconds: Double = 8

    enum FeedInputError: LocalizedError, Equatable {
        case notLoaded

        var errorDescription: String? { "データを読み込んでから押してください" }
    }

    /// 材料と節を合わせる。節を残し、はみ出す分は元の材料(記録・フォーム)を先に削る。
    static func merge(input: String?, section: String?, maxBytes: Int) -> String? {
        guard let section, !section.isEmpty else { return input }
        let sep = "\n\n"
        guard let input, !input.isEmpty else { return AITextLimits.clip(section, maxBytes: maxBytes) }
        let room = maxBytes - section.utf8.count - sep.utf8.count
        guard room > 0 else { return AITextLimits.clip(section, maxBytes: maxBytes) }
        return AITextLimits.clip(input, maxBytes: room) + sep + section
    }
}

#if !APP_STORE_REVIEW
extension AIFeedInput {
    /// `inputFeeds` の順に、同じ画面(器の中も)の feed 部品。知らない id・feed でない部品は飛ばす。
    static func feeds(for c: Component, in screen: Screen?) -> [Component] {
        guard let ids = c.inputFeeds, !ids.isEmpty, let screen else { return [] }
        let all = screen.allComponents
        return ids.compactMap { id in all.first { $0.id == id && $0.type == .feed } }
    }

    /// 1 行: 日付 / 見出し / 補足 / 値(最高・最低・降水確率・気圧の幅・急な低下)。
    static func line(_ item: FeedItem, provider: String, english: Bool) -> String {
        var parts: [String] = []
        if let d = FeedLeaf.dateText(item.date, provider: provider) ?? item.date, !d.isEmpty { parts.append(d) }
        parts.append(item.title)
        if let sub = item.subtitle, !sub.isEmpty, sub != item.title { parts.append(sub) }
        let f = { (v: Double) in "\(Int(v.rounded()))" }
        if provider == "pressure" {
            if let range = FeedLeaf.pressureRange(item) { parts.append(range) }
            if item.drop == true { parts.append(english ? "large pressure drop" : "気圧が大きく下がる") }
        } else {
            if let max = item.max { parts.append(english ? "high \(f(max))°C" : "最高 \(f(max))°") }
            if let min = item.min { parts.append(english ? "low \(f(min))°C" : "最低 \(f(min))°") }
        }
        if let pop = item.pop { parts.append(english ? "rain \(pop)%" : "降水確率 \(pop)%") }
        return "- " + parts.joined(separator: " / ")
    }

    /// 「外部データ」の節。行は feed の数で分け合い(合わせて maxRows)、全体を maxBytes に行の単位で切る。
    static func section(_ feeds: [(title: String, result: FeedResult)], english: Bool) -> String? {
        guard !feeds.isEmpty else { return nil }
        let perFeed = Swift.max(maxRows / feeds.count, 1)
        var lines = [english ? "External data (public data shown in the app):" : "外部データ(アプリが表示している公開データ):"]
        for feed in feeds {
            lines.append("■ " + (feed.title.isEmpty ? feed.result.title : feed.title))
            let rows = feed.result.items.prefix(perFeed).map { line($0, provider: feed.result.provider, english: english) }
            lines.append(contentsOf: rows.isEmpty ? [english ? "- (nothing to show now)" : "- (今は表示するものがありません)"] : rows)
        }
        var out = ""
        for l in lines {
            let next = out.isEmpty ? l : out + "\n" + l
            if next.utf8.count > maxBytes { break }
            out = next
        }
        return out
    }

    /// `c.inputFeeds` があれば、feed の行を取って材料に足す。取れなければ `notLoaded`。
    @MainActor
    static func attach(_ c: Component, screen: Screen?, rt: any DSLRuntime, locale: String, to request: inout AITextRequest) async throws {
        let comps = feeds(for: c, in: screen)
        guard !comps.isEmpty else { return }
        guard let host = rt.feeds else { throw FeedInputError.notLoaded }
        var loaded: [(title: String, result: FeedResult)] = []
        for comp in comps {
            guard let query = FeedQuery(comp) else { continue }
            let result = try await within(waitSeconds) { try await host.feed(query, force: false) }
            loaded.append((comp.title ?? "", result))
        }
        let maxInput = AITextLimits.maxInputBytes(long: request.length == "long")
        request.input = merge(input: request.input, section: section(loaded, english: locale == "en"), maxBytes: maxInput)
    }

    /// `op` を `seconds` まで待つ。間に合わない・失敗したら `notLoaded`。
    @MainActor
    static func within<T: Sendable>(_ seconds: Double, _ op: @escaping @Sendable @MainActor () async throws -> T) async throws -> T {
        do {
            return try await withThrowingTaskGroup(of: T.self) { group in
                group.addTask { @MainActor in try await op() }
                group.addTask {
                    try await Task.sleep(nanoseconds: UInt64(seconds * 1_000_000_000))
                    throw FeedInputError.notLoaded
                }
                defer { group.cancelAll() }
                guard let first = try await group.next() else { throw FeedInputError.notLoaded }
                return first
            }
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            throw FeedInputError.notLoaded
        }
    }
}
#else
extension AIFeedInput {
    @MainActor
    static func attach(_ c: Component, screen: Screen?, rt: any DSLRuntime, locale: String, to request: inout AITextRequest) async throws {}
}
#endif
