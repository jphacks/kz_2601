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
        public init(id: String, kind: Kind) {
            self.id = id
            self.kind = kind
        }
    }

    public static func kind(of type: ComponentType) -> Kind? {
        switch type {
        case .textField, .textEditor, .numberField, .secureField: .text
        case .datePicker, .timePicker: .dateField
        case .toggle, .picker: .control
        default: nil
        }
    }

    /// 画面の部品から Tab の止まり先だけを DSL の並びで取り出す。
    public static func stops(in components: [Component]) -> [Stop] {
        components.flatMap { c -> [Stop] in
            let own = kind(of: c.type).map { [Stop(id: c.id, kind: $0)] } ?? []
            return own + stops(in: c.children)
        }
    }

    /// `from` から Tab(`backward` なら Shift+Tab)で移る先の候補を近い順に。前の候補に止まれなかったら次を試す。
    /// - 出ていない欄(`mounted` に無い)と、フルキーボードアクセスが off の時のスイッチ・選択肢は飛ばす
    /// - 端を越える時: off なら反対の端へ回る(システムに任せると見た目の順の先頭へ飛ぶ)。
    ///   on なら候補はそこで終わり、システムが保存バーのボタンなどへ進める
    public static func candidates(from id: String, in order: [Stop], mounted: Set<String>,
                                  backward: Bool, fullKeyboardAccess: Bool) -> [String] {
        let usable = order.filter { $0.id == id || (mounted.contains($0.id) && ($0.kind != .control || fullKeyboardAccess)) }
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

    @ObservationIgnored var order: [FormTabOrder.Stop] = []
    /// 出ている止まり先(同じ id が 2 か所に出ることもあるので数える)
    @ObservationIgnored private var mounted: [String: Int] = [:]
    @ObservationIgnored private(set) var focusedId: String?
    @ObservationIgnored private var serial = 0
    /// フォーカスを渡す欄。leaf は自分の id なら @FocusState を立てる
    private(set) var request: Request?

    /// 止まれなかったと見なすまでの時間(SwiftUI のフォーカスは次のループ以降に届く)
    @ObservationIgnored var settleDelay: Duration = .milliseconds(120)

    func mount(_ id: String) { mounted[id, default: 0] += 1 }

    func unmount(_ id: String) {
        guard let n = mounted[id] else { return }
        mounted[id] = n > 1 ? n - 1 : nil
        if n <= 1, focusedId == id { focusedId = nil }
    }

    func noteFocus(_ id: String, _ isFocused: Bool) {
        if isFocused { focusedId = id } else if focusedId == id { focusedId = nil }
    }

    /// Tab を引き受けたら true(false ならシステムのキービューの並びに任せる)。
    func move(from id: String, backward: Bool, fullKeyboardAccess: Bool) -> Bool {
        let list = FormTabOrder.candidates(from: id, in: order, mounted: Set(mounted.keys),
                                           backward: backward, fullKeyboardAccess: fullKeyboardAccess)
        guard !list.isEmpty else { return false }
        attempt(list[...], backward: backward, fullKeyboardAccess: fullKeyboardAccess)
        return true
    }

    private func attempt(_ list: ArraySlice<String>, backward: Bool, fullKeyboardAccess: Bool) {
        guard let target = list.first else {
            // どの候補にも止まれなかった: on ならシステムに次(保存バーなど)へ進めてもらう
            if fullKeyboardAccess { Self.systemStep(backward: backward) }
            return
        }
        serial += 1
        let mine = serial
        request = Request(id: target, serial: mine)
        Task { @MainActor [weak self] in
            guard let delay = self?.settleDelay else { return }
            try? await Task.sleep(for: delay)
            guard let self, self.serial == mine, self.focusedId != target else { return }
            self.attempt(list.dropFirst(), backward: backward, fullKeyboardAccess: fullKeyboardAccess)
        }
    }

    private static func systemStep(backward: Bool) {
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
    #if os(macOS)
    @State private var tabs = FormTabCoordinator()
    #endif

    func body(content: Content) -> some View {
        #if os(macOS)
        content
            .environment(\.formTabCoordinator, tabs)
            .onAppear { tabs.order = FormTabOrder.stops(in: screen.components) }
            .onChange(of: screen.components.map(\.id)) { tabs.order = FormTabOrder.stops(in: screen.components) }
        #else
        content
        #endif
    }
}

/// 入力 leaf に付ける: Tab / Shift+Tab を DSL の並びの次の欄へ渡し、渡された時は自分にフォーカスを立てる(Mac だけ)。
struct MacFormTabStop: ViewModifier {
    let id: String
    let focused: FocusState<Bool>.Binding
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
                    return tabs.move(from: id, backward: backward, fullKeyboardAccess: fka) ? .handled : .ignored
                }
                .onAppear { tabs.mount(id) }
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
    /// Mac のフォームの Tab の止まり先にする(#2065)。iOS は何もしない。
    func macFormTabStop(_ c: Component, _ focused: FocusState<Bool>.Binding) -> some View {
        modifier(MacFormTabStop(id: c.id, focused: focused))
    }
}
