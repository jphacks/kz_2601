import SwiftUI
#if os(macOS)
import AppKit
#endif
import MetamonDSL

/// Mac のフォームで Tab / Shift+Tab が回る順(#2065)。DSL の並び(親 → 子の深さ優先)どおりにする。
/// キービューの並び(見た目の位置)に任せると、横並び(hstack・grid)や広いウインドウ(FormFlowLayout)で
/// ラベルの有無などで上下にずれた欄へ先に飛ぶ。
public enum FormTabOrder {
    public enum Kind: Equatable, Sendable {
        /// 文字の欄(1 行・複数行・数値・伏せ字): フルキーボードアクセスが off でも止まる
        case text
        /// 日付・時刻: 文字の欄と同じく常に試す(止まれなければ飛ばす)
        case dateField
        /// スイッチ・選択肢(segmented を含む): フルキーボードアクセスが on の時だけ
        case control
    }

    public struct Stop: Equatable, Sendable {
        public let id: String
        public let kind: Kind
        /// 中にいる折りたたみ(disclosure)の id。どれかが閉じていれば隠れている
        public let disclosures: [String]
        /// 遅延で並べる入れ物(grid・carousel)の中なら、まだ出ていない時に画面のスクロールで出す先の id
        public let anchor: String?
        public init(id: String, kind: Kind, disclosures: [String] = [], anchor: String? = nil) {
            self.id = id
            self.kind = kind
            self.disclosures = disclosures
            self.anchor = anchor
        }
    }

    /// 画面の止まり先と、折りたたみの初期状態(DSL の `expanded`)。どちらかが変われば並びを作り直す
    public struct Layout: Equatable, Sendable {
        public var stops: [Stop] = []
        public var expandedByDefault: [String: Bool] = [:]
    }

    public static func kind(of type: ComponentType) -> Kind? {
        switch type {
        case .textField, .textEditor, .numberField, .secureField: .text
        case .datePicker, .timePicker: .dateField
        case .toggle, .picker: .control
        default: nil
        }
    }

    /// 画面の部品から Tab の止まり先だけを DSL の並びで取り出す。`include` が false の部品は中ごと出さない(隠す field など)。
    public static func stops(in components: [Component], include: (Component) -> Bool = { _ in true }) -> [Stop] {
        layout(in: components, include: include).stops
    }

    public static func layout(in components: [Component], include: (Component) -> Bool = { _ in true }) -> Layout {
        var out = Layout()
        func walk(_ c: Component, disclosures: [String], anchor: String?) {
            guard include(c) else { return }
            if let k = kind(of: c.type) { out.stops.append(Stop(id: c.id, kind: k, disclosures: disclosures, anchor: anchor)) }
            if c.type == .disclosure { out.expandedByDefault[c.id] = c.expanded ?? false }
            let inner = c.type == .disclosure ? disclosures + [c.id] : disclosures
            for child in c.children {
                // LazyVGrid は子の id(ForEach)へスクロールできる。carousel は自分で横に送るので、縦は carousel まで
                let a: String? = switch c.type {
                case .grid: child.id
                case .carousel: anchor ?? c.id
                default: anchor
                }
                walk(child, disclosures: inner, anchor: a)
            }
        }
        for c in components { walk(c, disclosures: [], anchor: nil) }
        return out
    }

    /// `c` の中(自分を含む)に id の部品があるか。
    static func contains(_ id: String, in c: Component) -> Bool {
        c.id == id || c.children.contains { contains(id, in: $0) }
    }

    /// `from` から Tab(`backward` なら Shift+Tab)で移る先の候補を近い順に。前の候補に止まれなかったら次を試す。
    /// - 隠れている欄は DSL で決める(閉じた折りたたみの中。隠す field は `stops(include:)` で除く)。
    ///   遅延の入れ物で出ていないだけの欄は候補に残す(受け渡し役がスクロールで出してから止める)
    /// - フルキーボードアクセスが off の時のスイッチ・選択肢は飛ばす
    /// - 端を越える時: off なら反対の端へ回る(システムに任せると見た目の順の先頭へ飛ぶ)。
    ///   on なら候補はそこで終わり、システムが保存バーのボタンなどへ進める
    public static func candidates(from id: String, in order: [Stop], collapsed: Set<String> = [],
                                  backward: Bool, fullKeyboardAccess: Bool) -> [String] {
        let usable = order.filter {
            $0.id == id || ($0.disclosures.allSatisfy { !collapsed.contains($0) } && ($0.kind != .control || fullKeyboardAccess))
        }
        var ids: [String] = []
        for stop in usable where !ids.contains(stop.id) { ids.append(stop.id) }
        guard let i = ids.firstIndex(of: id) else { return [] }
        let after = Array(ids[(i + 1)...]), before = Array(ids[..<i])
        if backward {
            return before.reversed() + (fullKeyboardAccess ? [] : after.reversed())
        }
        return after + (fullKeyboardAccess ? [] : before)
    }
}

/// 画面ごとの Tab の受け渡し役(#2065)。leaf はそれぞれ自分の @FocusState しか持たないので、ここを通して次の欄に立てさせる。
@MainActor
@Observable
final class FormTabCoordinator {
    struct Request: Equatable {
        let id: String
        let serial: Int
    }

    @ObservationIgnored private(set) var order: [FormTabOrder.Stop] = []
    @ObservationIgnored private var expandedByDefault: [String: Bool] = [:]
    /// 折りたたみが知らせてきた今の開閉(知らせが無ければ DSL の初期状態)
    @ObservationIgnored private var expanded: [String: Bool] = [:]
    /// 出ている止まり先(同じ id が 2 か所に出ることもあるので数える)。出ていなければ先にスクロールで出す
    @ObservationIgnored private var mounted: [String: Int] = [:]
    @ObservationIgnored private(set) var focusedId: String?
    @ObservationIgnored private var serial = 0
    /// 受け渡し中の、止まれなかった時に次の候補へ進む待ち
    @ObservationIgnored private var pending: Task<Void, Never>?
    /// フォーカスを渡す欄。leaf は自分の id なら @FocusState を立てる。止まれた・利用者が動いたら nil
    private(set) var request: Request?
    /// 遅延の入れ物の中でまだ出ていない欄へ渡す時、画面のスクロールで出す先(`Stop.anchor`)
    private(set) var reveal: Request?

    /// 止まれなかったと見なすまでの時間(SwiftUI のフォーカスは次のループ以降に届く)
    @ObservationIgnored var settleDelay: Duration = .milliseconds(120)
    /// スクロールで出してから止める時の待ち(出て @FocusState が立つまで)
    @ObservationIgnored var revealDelay: Duration = .milliseconds(400)
    /// 待ち方(テストで差し替える)
    @ObservationIgnored var wait: @MainActor (Duration) async throws -> Void = { try await Task.sleep(for: $0) }
    /// 候補が尽きた時にシステムのキービューの並びで進める(テストで差し替える)
    @ObservationIgnored var systemStep: @MainActor (_ backward: Bool) -> Void = FormTabCoordinator.keyViewStep
    /// 待ちが終わるたびに呼ぶ(テスト用)
    @ObservationIgnored var didSettle: (@MainActor () -> Void)?

    var collapsed: Set<String> {
        Set(expandedByDefault.keys.filter { !(expanded[$0] ?? expandedByDefault[$0] ?? false) })
    }

    func apply(_ layout: FormTabOrder.Layout) {
        order = layout.stops
        expandedByDefault = layout.expandedByDefault
    }

    func mount(_ id: String) { mounted[id, default: 0] += 1 }

    func unmount(_ id: String) {
        guard let n = mounted[id] else { return }
        mounted[id] = n > 1 ? n - 1 : nil
        if n <= 1, focusedId == id { focusedId = nil }
    }

    func noteDisclosure(_ id: String, expanded isExpanded: Bool) { expanded[id] = isExpanded }

    func noteFocus(_ id: String, _ isFocused: Bool) {
        guard isFocused else {
            if focusedId == id { focusedId = nil }
            return
        }
        focusedId = id
        // 渡した先に止まれた、または利用者が別の欄へ動かした: どちらも受け渡しはここで終わり(待ちの後で先へ進めない)
        if request != nil { endHandOff() }
    }

    /// Tab を引き受けたら true(false ならシステムのキービューの並びに任せる)。
    /// `insertsTab`: 複数行の欄。断ると Tab の文字が入るので、候補が無い時もシステムの次へ進めて引き受ける
    func move(from id: String, backward: Bool, fullKeyboardAccess: Bool, insertsTab: Bool = false) -> Bool {
        // 受け渡し中に続けて押されたら、まだ届いていない渡し先から数える
        let origin = request?.id ?? id
        let list = FormTabOrder.candidates(from: origin, in: order, collapsed: collapsed,
                                           backward: backward, fullKeyboardAccess: fullKeyboardAccess)
        guard !list.isEmpty else {
            endHandOff()
            guard insertsTab else { return false }
            systemStep(backward)
            return true
        }
        attempt(list[...], backward: backward, fullKeyboardAccess: fullKeyboardAccess)
        return true
    }

    private func endHandOff() {
        pending?.cancel()
        pending = nil
        request = nil
    }

    private func attempt(_ list: ArraySlice<String>, backward: Bool, fullKeyboardAccess: Bool) {
        pending?.cancel()
        pending = nil
        guard let target = list.first else {
            request = nil
            // どの候補にも止まれなかった: on ならシステムに次(保存バーなど)へ進めてもらう
            if fullKeyboardAccess { systemStep(backward) }
            return
        }
        serial += 1
        let mine = Request(id: target, serial: serial)
        let anchor = mounted[target] == nil ? order.first { $0.id == target }?.anchor : nil
        if let anchor { reveal = Request(id: anchor, serial: serial) }
        request = mine
        let delay = anchor == nil ? settleDelay : revealDelay
        pending = Task { @MainActor [weak self] in
            defer { self?.didSettle?() }
            guard let wait = self?.wait else { return }
            try? await wait(delay)
            guard !Task.isCancelled, let self, self.request == mine else { return }
            self.attempt(list.dropFirst(), backward: backward, fullKeyboardAccess: fullKeyboardAccess)
        }
    }

    private static func keyViewStep(backward: Bool) {
        #if os(macOS)
        if backward { NSApp?.keyWindow?.selectPreviousKeyView(nil) } else { NSApp?.keyWindow?.selectNextKeyView(nil) }
        #endif
    }
}

private struct FormTabCoordinatorKey: EnvironmentKey {
    static let defaultValue: FormTabCoordinator? = nil
}

extension EnvironmentValues {
    var formTabCoordinator: FormTabCoordinator? {
        get { self[FormTabCoordinatorKey.self] }
        set { self[FormTabCoordinatorKey.self] = newValue }
    }
}

/// 画面(ScreenView)に付ける: その画面の DSL の並びで Tab を回す(Mac だけ)。
struct MacFormTabScope: ViewModifier {
    let screen: Screen
    let ctx: RenderContext
    #if os(macOS)
    @Environment(\.dslRuntime) private var rt
    @State private var tabs = FormTabCoordinator()
    #endif

    func body(content: Content) -> some View {
        #if os(macOS)
        // id だけでなく種類・隠す field・折りたたみの初期状態が変わっても作り直す
        let layout = Self.layout(screen: screen, ctx: ctx, currentUid: rt.currentUserId)
        content
            .environment(\.formTabCoordinator, tabs)
            .onChange(of: layout, initial: true) { _, layout in tabs.apply(layout) }
        #else
        content
        #endif
    }

    /// `DSLView` と入力 leaf が描かない部品(隠す field・フォーム無し・computed / ref の行)は止まり先にしない。
    @MainActor
    static func layout(screen: Screen, ctx: RenderContext, currentUid: String?) -> FormTabOrder.Layout {
        FormTabOrder.layout(in: screen.components) { c in
            if let form = ctx.form, form.hides(c, currentUid: currentUid) { return false }
            guard FormTabOrder.kind(of: c.type) != nil else { return true }
            return ctx.form != nil && c.field != nil && !FormFieldOverride.applies(to: c, ctx: ctx)
        }
    }
}

/// 画面の ScrollViewReader の中に付ける: 遅延の入れ物で出ていない Tab の渡し先まで先にスクロールする(Mac だけ)。
struct MacFormTabReveal: ViewModifier {
    let proxy: ScrollViewProxy
    @Environment(\.formTabCoordinator) private var tabs

    func body(content: Content) -> some View {
        #if os(macOS)
        content.onChange(of: tabs?.reveal) { _, reveal in
            if let reveal { proxy.scrollTo(reveal.id, anchor: .center) }
        }
        #else
        content
        #endif
    }
}

/// 入力 leaf に付ける: Tab / Shift+Tab を DSL の並びの次の欄へ渡し、渡された時は自分にフォーカスを立てる(Mac だけ)。
struct MacFormTabStop: ViewModifier {
    let id: String
    let focused: FocusState<Bool>.Binding
    /// 複数行の欄(断ると Tab の文字が入る)
    let insertsTab: Bool
    @Environment(\.formTabCoordinator) private var tabs

    /// Shift+Tab のキーの文字(NSBackTabCharacter)
    static let backTab = KeyEquivalent("\u{19}")

    func body(content: Content) -> some View {
        #if os(macOS)
        if let tabs {
            content
                .onKeyPress(keys: [.tab, Self.backTab], phases: .down) { press in
                    guard press.modifiers.isDisjoint(with: [.command, .option, .control]) else { return .ignored }
                    // 日本語の変換中の Tab は候補の選択に使う
                    if KeyboardState.focusedTextView()?.hasMarkedText() == true { return .ignored }
                    let backward = press.key == Self.backTab || press.modifiers.contains(.shift)
                    let fka = NSApp?.isFullKeyboardAccessEnabled ?? false
                    return tabs.move(from: id, backward: backward, fullKeyboardAccess: fka, insertsTab: insertsTab) ? .handled : .ignored
                }
                .onAppear {
                    tabs.mount(id)
                    // スクロールで出てきた渡し先(出る前に渡されていた)
                    if tabs.request?.id == id { focused.wrappedValue = true }
                }
                .onDisappear { tabs.unmount(id) }
                .onChange(of: focused.wrappedValue) { _, isFocused in tabs.noteFocus(id, isFocused) }
                .onChange(of: tabs.request) { _, request in
                    if request?.id == id { focused.wrappedValue = true }
                }
        } else {
            content
        }
        #else
        content
        #endif
    }
}

extension View {
    /// Mac のフォームの Tab の止まり先にする(#2065)。iOS は何もしない。`insertsTab` は複数行の欄。
    func macFormTabStop(_ c: Component, _ focused: FocusState<Bool>.Binding, insertsTab: Bool = false) -> some View {
        modifier(MacFormTabStop(id: c.id, focused: focused, insertsTab: insertsTab))
    }
}
