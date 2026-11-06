// 審査用ビルド(APP_STORE_REVIEW)は外部の公開データ(feed・#348)を外す(外部の Web の内容を出すため)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `feed`(#348): 天気予報・祝日・許可したお知らせを読むだけの一覧。データはホスト(`FeedHost`)が appconfig から取る。
/// 行: 日付・見出し・(天気)アイコン / 最高・最低 / 降水確率、(お知らせ)タップで記事を Safari で開く。下に出典を必ず出す。
struct FeedLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.openURL) private var openURL
    let c: Component
    @State private var result: FeedResult?
    @State private var failed = false

    var body: some View {
        if let query = FeedQuery(c) {
            VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                Text(heading).font(RenderTheme.Fonts.cardTitle).accessibilityAddTraits(.isHeader)
                content
                if let result {
                    Text(Self.attribution(result))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .accessibilityLabel(Text(L("出典: \(result.source.name)")))
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .renderCard()
            .task(id: query) { await load(query, force: false) }
        } else {
            UnsupportedComponent(label: "feed")
        }
    }

    private var heading: String {
        if let t = c.title, !t.isEmpty { return t }
        return result?.title ?? ""
    }

    @ViewBuilder
    private var content: some View {
        if let result, !result.items.isEmpty {
            VStack(spacing: 0) {
                ForEach(Array(result.items.enumerated()), id: \.element.id) { index, item in
                    if index > 0 { Hairline() }
                    row(item, provider: result.provider)
                }
            }
            if result.stale {
                Label(L("最新の情報を取れなかったため、前に取った内容です"), systemImage: "clock.arrow.circlepath")
                    .font(.caption2).foregroundStyle(.secondary)
            }
        } else {
            EmptyPlaceholder(icon: Self.icon(c.provider), emptyText: c.emptyText ?? L("今は表示するものがありません"),
                             state: failed ? .unavailable : (result == nil ? .loading : .empty)) {
                if let q = FeedQuery(c) { Task { await load(q, force: true) } }
            }
        }
    }

    @ViewBuilder
    private func row(_ item: FeedItem, provider: String) -> some View {
        let link = item.link
        HStack(alignment: .center, spacing: RenderTheme.Spacing.row) {
            if let icon = item.icon {
                Image(systemName: icon).symbolRenderingMode(.multicolor).font(.title3).frame(width: 28).accessibilityHidden(true)
            }
            VStack(alignment: .leading, spacing: 2) {
                if let date = Self.dateText(item.date, provider: provider) {
                    Text(date).font(.caption.monospacedDigit()).foregroundStyle(.secondary)
                }
                Text(item.title).font(RenderTheme.Fonts.rowTitle).lineLimit(3)
                if let sub = item.subtitle, !sub.isEmpty, sub != item.title {
                    Text(sub).font(RenderTheme.Fonts.rowSubtitle).foregroundStyle(.secondary).lineLimit(2)
                }
            }
            Spacer(minLength: 0)
            if let temps = Self.temps(item) {
                Text(temps).font(RenderTheme.Fonts.rowTrailing)
            }
            if let pop = item.pop {
                Label("\(pop)%", systemImage: "umbrella").font(.caption.monospacedDigit()).foregroundStyle(.secondary)
                    .accessibilityLabel(Text(L("降水確率 \(pop) パーセント")))
            }
            if link != nil {
                Image(systemName: "arrow.up.right.square").foregroundStyle(.secondary).accessibilityHidden(true)
            }
        }
        .padding(.vertical, RenderTheme.Spacing.sm)
        .contentShape(Rectangle())
        .onTapGesture { if let link { openURL(link) } }
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(link != nil ? .isLink : [])
    }

    private func load(_ query: FeedQuery, force: Bool) async {
        guard let host = rt.feeds else {
            failed = true
            return
        }
        if force { failed = false }
        do {
            result = try await host.feed(query, force: force)
            failed = false
        } catch is CancellationError {
        } catch {
            failed = result == nil
        }
    }

    // MARK: - 表示の組み立て(テストから使う)

    static func icon(_ provider: String?) -> String {
        switch provider {
        case "weather": "cloud.sun"
        case "holidays": "calendar"
        default: "newspaper"
        }
    }

    /// 「最高 / 最低」(片方だけでも出す)。
    static func temps(_ item: FeedItem) -> String? {
        let f = { (v: Double) in "\(Int(v.rounded()))°" }
        switch (item.max, item.min) {
        case let (max?, min?): return "\(f(max)) / \(f(min))"
        case let (max?, nil): return f(max)
        case let (nil, min?): return f(min)
        default: return nil
        }
    }

    /// weather / holidays は「11/7(土)」、rss は「11/5 18:30」。
    static func dateText(_ raw: String?, provider: String) -> String? {
        guard let raw, !raw.isEmpty else { return nil }
        let jst = TimeZone(identifier: "Asia/Tokyo") ?? .current
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = jst
        let out = DateFormatter()
        out.calendar = cal
        out.timeZone = jst
        out.locale = Locale(identifier: "ja_JP")
        if provider == "rss" {
            guard let d = ISO8601DateFormatter().date(from: raw) else { return nil }
            out.dateFormat = "M/d H:mm"
            return out.string(from: d)
        }
        let p = DateFormatter()
        p.calendar = cal
        p.timeZone = jst
        p.locale = Locale(identifier: "en_US_POSIX")
        p.dateFormat = "yyyy-MM-dd"
        guard let d = p.date(from: String(raw.prefix(10))) else { return nil }
        out.dateFormat = "M/d(E)"
        return out.string(from: d)
    }

    static func attribution(_ r: FeedResult) -> String {
        L("出典: \(r.source.name)(\(r.source.license))")
    }
}
#endif
