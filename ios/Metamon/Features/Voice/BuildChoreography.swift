import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 生成中の「パーツを選んで置く」演出の段取り(#626)。View を持たない純粋なロジックだけをここに置き、単体テストする。
/// - 待っている間(`WaitingScript`): パーツ箱から部品のシルエットを選んで空の画面に置く → 入れ替える → 並べ直す、を繰り返す
/// - 結果が届いた時(`AssemblyPlan`): 本物の結果の部品を上から 1 つずつ置いて、本物の画面に切り替える(最長 1.5 秒)
/// - 編集(`EditHighlight`): 変わった部品だけを光らせて置き直す
/// 待っている間の部品は依頼文から当てた「下書き」。SSE で本物の部品が届いたら(#636・`LiveDraft`)届いた順に下書きと置き換える(仮置き)。
/// 最後の結果で `AssemblyPlan` が置き直すので、仮置きと結果が違っても破綻しない。
enum BuildChoreography {
    /// 部品のシルエットの種類。DSL の `ComponentType` を見た目の近いものにまとめる。
    enum PartKind: String, CaseIterable, Equatable, Sendable {
        case heading, text, button, field, toggle, list, chart, stat, calendar, map, media, card

        static func of(_ type: ComponentType) -> PartKind {
            switch type {
            case .text, .markdown, .label, .badge, .link: .text
            case .button, .openUrlButton, .shareButton, .locationButton, .aiButton, .randomPicker, .nearbyShare: .button
            case .textField, .numberField, .datePicker, .picker, .textEditor, .secureField, .timePicker, .colorPicker,
                 .slider, .stepper, .rating, .searchBar, .locationPicker, .voiceInput: .field
            case .toggle: .toggle
            case .list, .table, .timeline, .gridTable, .matchList, .chatThread, .aiChat, .settlement, .feed: .list
            case .chart: .chart
            case .stat, .gauge, .progressBar, .healthStat, .sensorValue, .timerDisplay: .stat
            case .calendar: .calendar
            case .mapView, .routeGuide, .locationTracker: .map
            case .photoField, .drawingField, .image, .videoField, .videoPlayer, .audioRecorder, .qrCode, .qrScanner, .dashcam,
                 .musicPlayer, .avatar, .icon, .webView: .media
            default: .card
            }
        }
    }

    // MARK: - 依頼文からの当て推量

    /// 依頼文の語から「出てきそうな部品」を最大 3 つ(出てきた順)。当てずっぽうなので控えめにする。
    static func hints(for transcript: String) -> [PartKind] {
        let table: [(words: [String], kind: PartKind)] = [
            (["グラフ", "推移", "チャート", "graph", "chart"], .chart),
            (["チェック", "やること", "TODO", "ToDo", "todo", "完了", "check"], .toggle),
            (["一覧", "リスト", "履歴", "list"], .list),
            (["記録", "入力", "メモ", "日記", "record", "log"], .field),
            (["合計", "集計", "平均", "残り", "回数", "total"], .stat),
            (["カレンダー", "予定", "スケジュール", "calendar"], .calendar),
            (["地図", "場所", "散歩", "ルート", "map"], .map),
            (["写真", "画像", "カメラ", "photo"], .media),
        ]
        var found: [(index: String.Index, kind: PartKind)] = []
        for row in table {
            let hits = row.words.compactMap { transcript.range(of: $0, options: .caseInsensitive)?.lowerBound }
            if let first = hits.min() { found.append((first, row.kind)) }
        }
        return Array(found.sorted { $0.index < $1.index }.map(\.kind).prefix(3))
    }

    /// パーツ箱の中身(6 個)。当て推量を先頭に、残りはよく使う部品で埋める。
    static func tray(hints: [PartKind]) -> [PartKind] {
        var out: [PartKind] = []
        for k in hints + [.heading, .field, .button, .list, .stat, .toggle, .chart] where !out.contains(k) {
            out.append(k)
        }
        return Array(out.prefix(6))
    }

    // MARK: - 待っている間

    /// 画面に置かれた 1 つの部品(下書き)。`id` は置いた時に決まり、入れ替え・並べ直しの間も変わらない(動きのアニメ用)。
    struct PlacedPart: Equatable, Identifiable {
        var id: String
        var kind: PartKind
        var area: GridArea
        var caption: String? = nil
    }

    /// 待っている間の 1 コマ。`picking` は今パーツ箱から選んでいる部品(箱の中で持ち上がる)。
    struct WaitingFrame: Equatable {
        var placed: [PlacedPart]
        var picking: PartKind?
        var move: Move
        /// 画面の縦の行数(仮置きが 12 行を超えたら伸ばす)
        var rows: Int = GridArea.visibleRows
    }

    enum Move: Equatable { case place, swap, reorder, relayout }

    /// 待っている間の台本。`step` ごとに 1 手進む(置く → 入れ替える → 並べ直す → 配置を変える…)。
    struct WaitingScript: Equatable {
        /// 下書きの配置案(実 DSL と同じ area 記法・上から順)
        static let layouts: [[String]] = [
            ["A1:F2", "A3:F6", "A7:C9", "D7:F9", "A10:F12"],
            ["A1:F2", "A3:C6", "D3:F6", "A7:F10", "A11:F12"],
            ["A1:F3", "A4:F5", "A6:F9", "A10:C12", "D10:F12"],
        ]
        /// 1 手の秒数(Reduce Motion ではゆっくり・フェードだけ)
        static func stepSeconds(reduceMotion: Bool) -> Double { reduceMotion ? 2.4 : 0.85 }

        let tray: [PartKind]

        init(transcript: String) {
            tray = BuildChoreography.tray(hints: BuildChoreography.hints(for: transcript))
        }

        static func step(elapsed: TimeInterval, reduceMotion: Bool) -> Int {
            max(0, Int(elapsed / stepSeconds(reduceMotion: reduceMotion)))
        }

        /// 仮置き(#636)を混ぜたコマ。本物の部品を置き、重ならない所だけ下書きを残す(下書きは 5 枠まで)。
        func frame(at step: Int, live: LiveDraft?) -> WaitingFrame {
            let guess = frame(at: step)
            guard let live, !live.components.isEmpty else { return guess }
            let real = live.parts
            let rest = guess.placed.filter { g in !real.contains { $0.area.overlaps(g.area) } }
            let keep = max(0, Self.layouts[0].count - real.count)
            return WaitingFrame(placed: real + rest.prefix(keep), picking: live.latestKind ?? guess.picking, move: .place,
                                rows: max(GridArea.visibleRows, real.map(\.area.row2).max() ?? 0))
        }

        /// `step` 手目のコマ。0 手目から順に再生して決める(決定的・数十手なので軽い)。
        func frame(at step: Int) -> WaitingFrame {
            let slots = Self.layouts[0].compactMap(GridArea.init)
            var placed: [PlacedPart] = []
            var serial = 0
            var nextTray = 0
            func takeFromTray() -> PartKind {
                defer { nextTray += 1 }
                return tray[nextTray % tray.count]
            }
            var current = WaitingFrame(placed: [], picking: nil, move: .place)
            var layout = 0
            for s in 0...max(0, step) {
                if placed.count < slots.count {
                    // 置く: 空いている枠に上から
                    let kind = takeFromTray()
                    placed.append(PlacedPart(id: "p\(serial)", kind: kind, area: slots[placed.count]))
                    serial += 1
                    current = WaitingFrame(placed: placed, picking: kind, move: .place)
                    continue
                }
                let k = s - slots.count
                switch k % 3 {
                case 0:
                    // 入れ替える: 1 枠を別の部品に(同じ種類なら次の部品)
                    let i = (k / 3 + 1) % placed.count
                    var kind = takeFromTray()
                    if kind == placed[i].kind { kind = takeFromTray() }
                    placed[i] = PlacedPart(id: "p\(serial)", kind: kind, area: placed[i].area)
                    serial += 1
                    current = WaitingFrame(placed: placed, picking: kind, move: .swap)
                case 1:
                    // 並べ直す: 隣り合う 2 つの場所を交換
                    let i = (k / 3 + 2) % placed.count, j = (i + 1) % placed.count
                    let a = placed[i].area
                    placed[i].area = placed[j].area
                    placed[j].area = a
                    current = WaitingFrame(placed: placed, picking: nil, move: .reorder)
                default:
                    // 配置を変える: 同じ部品のまま別の配置案へ
                    layout = (layout + 1) % Self.layouts.count
                    let areas = Self.layouts[layout].compactMap(GridArea.init)
                    let order = placed.sorted { ($0.area.row1, $0.area.col1) < ($1.area.row1, $1.area.col1) }
                    placed = order.enumerated().map { n, p in
                        PlacedPart(id: p.id, kind: p.kind, area: areas[n % areas.count])
                    }
                    current = WaitingFrame(placed: placed, picking: nil, move: .relayout)
                }
            }
            return current
        }
    }

    // MARK: - 結果が届いた時

    /// 結果の部品を置いていく段取り。
    struct AssemblyPlan: Equatable {
        struct Step: Equatable {
            var part: PlacedPart
            /// 演出の開始から置くまでの秒数
            var delay: Double
        }
        var steps: [Step]
        /// 演出の全体の秒数(この後に本物の画面へ切り替える)
        var total: Double
        /// 画面の縦の行数(12 未満にしない。積むだけの部品は下に足す)
        var rows: Int
        /// パーツ箱に出す種類(結果に出てくる順・最大 6)
        var tray: [PartKind]
        /// タブの見出し(タブが 2 つ以上の時だけ・画面の下に出す)
        var tabs: [String] = []

        /// 置く部品の上限。超えた分は最後の手でまとめて置く
        static let maxSteps = 10
        static let leadIn = 0.12
        static let hold = 0.3
        /// 1 手の秒数の上限・置く時間の合計の上限(全体は leadIn + 0.9 + hold ≒ 1.3 秒以内)
        static let maxPerStep = 0.16
        static let maxPlacing = 0.9
        /// Reduce Motion: 全部を一度にフェードで出す
        static let reducedTotal = 0.45

        /// `shown`: 仮置きで既に見せた部品(#636)。結果と同じ部品(id・種類・場所が一致)は最初から置いておき、残りだけ 1 つずつ置く。
        static func make(for dsl: AppDSL, reduceMotion: Bool, shown: [PlacedPart] = []) -> AssemblyPlan {
            let parts = initialParts(of: dsl)
            var tray: [PartKind] = []
            for p in parts where !tray.contains(p.kind) { tray.append(p.kind) }
            let rows = max(GridArea.visibleRows, parts.map(\.area.row2).max() ?? 0)
            let tabs = dsl.app.hasTabs ? dsl.app.tabs.map(\.title) : []
            if reduceMotion || parts.isEmpty {
                return AssemblyPlan(steps: parts.map { Step(part: $0, delay: 0) },
                                    total: reduceMotion ? reducedTotal : leadIn + hold, rows: rows, tray: Array(tray.prefix(6)), tabs: tabs)
            }
            let kept = parts.filter { p in shown.contains { $0.id == p.id && $0.kind == p.kind && $0.area == p.area } }
            let rest = parts.filter { p in !kept.contains(p) }
            if rest.isEmpty {
                return AssemblyPlan(steps: kept.map { Step(part: $0, delay: 0) }, total: leadIn + hold, rows: rows,
                                    tray: Array(tray.prefix(6)), tabs: tabs)
            }
            let slots = min(rest.count, maxSteps)
            let per = min(maxPerStep, maxPlacing / Double(slots))
            let steps = kept.map { Step(part: $0, delay: 0) } + rest.enumerated().map { i, p in
                Step(part: p, delay: leadIn + Double(min(i, slots - 1)) * per)
            }
            return AssemblyPlan(steps: steps, total: leadIn + Double(slots) * per + hold, rows: rows,
                                tray: Array(tray.prefix(6)), tabs: tabs)
        }

        /// 最初に見える画面(タブがあれば初期タブ)の直下の部品を、上から(行 → 列)の順に。area の無い部品は下に全幅で積む。
        static func initialParts(of dsl: AppDSL) -> [PlacedPart] {
            let sid = dsl.app.hasTabs ? dsl.initialTab : dsl.initialScreen
            guard let screen = dsl.screens.first(where: { $0.id == sid }) ?? dsl.screens.first else { return [] }
            let placement = screen.gridPlacement
            var out = placement.placed
                .sorted { ($0.area.row1, $0.area.col1) < ($1.area.row1, $1.area.col1) }
                .map { PlacedPart(id: $0.component.id, kind: kind(of: $0.component, area: $0.area), area: $0.area, caption: caption(of: $0.component)) }
            var row = (placement.placed.map(\.area.row2).max() ?? 0) + 1
            for c in placement.stacked {
                let height = c.type == .list || c.type == .chart ? 4 : 2
                if let area = GridArea("A\(row):F\(row + height - 1)") {
                    out.append(PlacedPart(id: c.id, kind: kind(of: c, area: nil), area: area, caption: caption(of: c)))
                }
                row += height
            }
            return out
        }

        /// 見出しらしい text(1〜2 行の高さ・style が title / headline)は heading にする。
        private static func kind(of c: Component, area: GridArea?) -> PartKind {
            if c.type == .text, c.style == "title" || c.style == "headline" || (area?.rowSpan ?? 3) <= 1 { return .heading }
            return PartKind.of(c.type)
        }

        /// シルエットに添える短い文字(本物の結果の部品の名前)。長い文は出さない。
        static func caption(of c: Component) -> String? {
            shortCaption([c.label, c.title, c.type == .text ? c.text : nil])
        }

        static func shortCaption(_ candidates: [String?]) -> String? {
            let raw = candidates.compactMap { $0 }
                .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                .first { !$0.isEmpty }
            guard let raw else { return nil }
            return raw.count > 12 ? String(raw.prefix(11)) + "…" : raw
        }
    }

    // MARK: - 届いた本物の部品(仮置き・#636)

    /// SSE で届いた途中経過。`GenerateProgress` を届いた順に `apply` する。どれも仮置きで、最後の結果が正。
    struct LiveDraft: Equatable {
        /// アプリ名(`app`)
        var appName: String?
        /// タブの見出し(2 つ以上の時だけ画面の下に出す)
        var tabs: [String] = []
        /// 今の作り直しの回。古い回の部品は捨てる
        private(set) var attempt = 0
        /// 届いた部品(届いた順・同じ index は 1 回だけ)
        private(set) var components: [StreamedComponent] = []

        mutating func apply(_ p: GenerateProgress) {
            switch p {
            case let .app(name, tabs, attempt):
                guard attempt >= self.attempt else { return }
                appName = name
                self.tabs = tabs
            case let .component(c):
                if c.attempt < attempt { return }
                if c.attempt > attempt { // restart を取りこぼしても、新しい回が来たら前の回を捨てる
                    attempt = c.attempt
                    components = []
                }
                guard !components.contains(where: { $0.index == c.index }) else { return }
                components.append(c)
            case let .restart(attempt):
                // 0 = iOS 側のやり直し(stream を諦めた・409)。次の stream は 1 回目から数え直す
                self.attempt = attempt
                components = []
            }
        }

        /// 届いた部品のシルエット。area のある部品はその場所、無い部品はその下に全幅で積む(`initialParts` と同じ)。
        var parts: [PlacedPart] {
            var out: [PlacedPart] = []
            var stacked: [StreamedComponent] = []
            for c in components.sorted(by: { $0.index < $1.index }) {
                if let raw = c.area, let area = GridArea(raw) {
                    out.append(part(c, area: area))
                } else {
                    stacked.append(c)
                }
            }
            var row = (out.map(\.area.row2).max() ?? 0) + 1
            for c in stacked {
                let type = ComponentType(rawValue: c.type)
                let height = type == .list || type == .chart ? 4 : 2
                if let area = GridArea("A\(row):F\(row + height - 1)") { out.append(part(c, area: area)) }
                row += height
            }
            // 同じ id の部品が来ても描画の id は重ねない
            var seen: Set<String> = []
            for i in out.indices where !seen.insert(out[i].id).inserted { out[i].id = "stream-dup-\(i)" }
            return out
        }

        /// 最後に届いた部品の種類(パーツ箱で持ち上げる)
        var latestKind: PartKind? { components.last.map { Self.kind(of: $0, area: $0.area.flatMap(GridArea.init)) } }

        /// パーツ箱: 届いた部品の種類を先頭に、残りは下書きの箱で埋める
        func tray(base: [PartKind]) -> [PartKind] {
            var out: [PartKind] = []
            for k in components.map({ Self.kind(of: $0, area: $0.area.flatMap(GridArea.init)) }) + base where !out.contains(k) {
                out.append(k)
            }
            return Array(out.prefix(6))
        }

        private func part(_ c: StreamedComponent, area: GridArea) -> PlacedPart {
            let id = c.id.flatMap { $0.isEmpty ? nil : $0 } ?? "stream-\(c.index)"
            return PlacedPart(id: id, kind: Self.kind(of: c, area: area), area: area,
                              caption: AssemblyPlan.shortCaption([c.label]))
        }

        /// 見出しらしい text(1 行の高さ)は heading。style は届かないので高さだけで見る
        private static func kind(of c: StreamedComponent, area: GridArea?) -> PartKind {
            guard let type = ComponentType(rawValue: c.type) else { return .card }
            if type == .text, (area?.rowSpan ?? 3) <= 1 { return .heading }
            return PartKind.of(type)
        }
    }

    // MARK: - 編集

    /// 編集の前後で、変わった(足された・中身が変わった・場所が変わった)直下の部品を `画面 id/部品 id` で返す。
    /// 部品 id を持たない DSL もあるので、id ではなく中身の印で比べる。新しい画面・ほぼ全部が変わった画面は光らせない(普段の出現に任せる)。
    static func editHighlight(old: AppDSL, new: AppDSL) -> ChangeHighlight {
        var keys: Set<String> = []
        var screens: Set<String> = []
        for screen in new.screens {
            guard let before = old.screens.first(where: { $0.id == screen.id }), !screen.components.isEmpty else { continue }
            var remaining = before.components.map(signature)
            var changed: [String] = []
            for c in screen.components {
                let sig = signature(c)
                if let i = remaining.firstIndex(of: sig) {
                    remaining.remove(at: i)
                } else {
                    changed.append(c.id)
                }
            }
            guard !changed.isEmpty, Double(changed.count) / Double(screen.components.count) <= 0.7 else { continue }
            screens.insert(screen.id)
            for id in changed { keys.insert(ChangeHighlight.key(screen: screen.id, component: id)) }
        }
        return ChangeHighlight(components: keys, screens: screens)
    }

    /// 部品の中身の印(見た目に効く項目と子の印)。
    static func signature(_ c: Component) -> String {
        let fields: [String?] = [c.rawType, c.area, c.text, c.label, c.title, c.icon, c.style, c.field, c.entity,
                                 c.kind, c.color, c.placeholder, c.aggregate, c.format, c.groupBy, c.align, c.size]
        let own = fields.map { $0 ?? "" }.joined(separator: "\u{1F}")
        return own + "[" + c.children.map(signature).joined(separator: "\u{1E}") + "]"
    }
}
