import SwiftUI
import MetamonDSL

/// 部品の「この部品について話す」(#1323)。ホスト(Mac)が入れると、部品の右クリックと VoiceOver の操作の一覧に出る。
/// nil(iOS・プレビュー・テスト)なら何も足さない。iOS の部品の入口は部品のシートの AI ボタン(#629)。
public struct ComponentTalk {
    /// どの画面のどの部品について話すか
    public struct Target: Equatable, Sendable {
        public var route: Route
        public var component: AIButtonComponent

        public init(route: Route, component: AIButtonComponent) {
            self.route = route
            self.component = component
        }

        /// 部品のある画面の id
        public var screen: String { route.screen }

        /// 文脈に出す画面の名前。title が空・画面が見つからなければ id(iOS の `AIButtonPlacement.context` と同じ)
        public func screenTitle(in dsl: AppDSL) -> String {
            let title = dsl.screen(route.screen)?.title.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            return title.isEmpty ? route.screen : title
        }

        /// モーダル(シート・全画面)の中の部品か。会話のシートはモーダルを閉じてから開く
        public var isInModal: Bool { route.stack == "sheet" || route.stack == "full" }
    }

    /// 項目の文言(ホストの文言・iOS と共有の `ScreenMapCopy`)
    public var title: String
    public var perform: @MainActor (Target) -> Void

    public init(title: String, perform: @escaping @MainActor (Target) -> Void) {
        self.title = title
        self.perform = perform
    }

    /// 右クリックのメニューを部品に付けるか。
    /// 付けない: 入れ物(中の部品のメニューと重ねない)・余白と区切り線(指せる中身が無い)・文字の入力欄(OS の標準のメニューを残す)。
    public static func offersMenu(for type: ComponentType) -> Bool {
        offersAction(for: type) && !type.isContainer && !textInputs.contains(type)
    }

    /// VoiceOver の操作の一覧に出すか。入力欄にも出す(右クリックに出せない分をここで補う)。入れ物は中の部品に同じ操作が重なるので出さない。
    public static func offersAction(for type: ComponentType) -> Bool {
        !type.isContainer && type != .spacer && type != .divider
    }

    /// 文字の入力欄(右クリックは OS の標準: カット・コピー・ペースト)
    static let textInputs: Set<ComponentType> = [.textField, .numberField, .textEditor, .secureField, .searchBar, .transcribeField]

    public static func target(_ c: Component, route: Route) -> Target {
        Target(route: route, component: AIButtonComponent(c))
    }
}

private struct ComponentTalkKey: EnvironmentKey {
    static let defaultValue: ComponentTalk? = nil
}

public extension EnvironmentValues {
    /// 部品の「この部品について話す」(#1323)。Mac のホストが入れる
    var componentTalk: ComponentTalk? {
        get { self[ComponentTalkKey.self] }
        set { self[ComponentTalkKey.self] = newValue }
    }
}

private struct ComponentTalkTargetKey: EnvironmentKey {
    static let defaultValue: ComponentTalk.Target? = nil
}

extension EnvironmentValues {
    /// いちばん内側の部品の話す先。部品の中の画像のメニュー(`imageContextMenu`)が先頭に「この部品について話す」を置くのに使う
    var componentTalkTarget: ComponentTalk.Target? {
        get { self[ComponentTalkTargetKey.self] }
        set { self[ComponentTalkTargetKey.self] = newValue }
    }
}

/// `DSLView` が部品ごとに付ける: 右クリックのメニューと VoiceOver の操作。
struct ComponentTalkMenu: ViewModifier {
    @Environment(\.componentTalk) private var talk
    let component: Component
    let ctx: RenderContext

    func body(content: Content) -> some View {
        if let talk, ComponentTalk.offersAction(for: component.type) {
            let run = { talk.perform(ComponentTalk.target(component, route: ctx.route)) }
            if ComponentTalk.offersMenu(for: component.type) {
                content
                    .environment(\.componentTalkTarget, ComponentTalk.target(component, route: ctx.route))
                    .contextMenu { Button(talk.title, systemImage: ComponentTalkMenuItem.icon, action: run) }
                    .accessibilityAction(named: Text(talk.title), run)
            } else {
                content.accessibilityAction(named: Text(talk.title), run)
            }
        } else {
            content
        }
    }
}

/// 部品の中で行ごと・画像ごとに右クリックのメニューを持つ部品(一覧の行・チャットの吹き出し・つながり・QR コード)は、
/// そのメニューが外側のメニューより先に出るので、メニューの先頭にこれを置く(Mac にしか無い項目は先頭寄り・docs/mac/interactions.md)。
struct ComponentTalkMenuItem: View {
    static let icon = "bubble.left.and.text.bubble.right"

    @Environment(\.componentTalk) private var talk
    let component: Component
    let ctx: RenderContext

    var body: some View {
        if let talk {
            Button(talk.title, systemImage: Self.icon) { talk.perform(ComponentTalk.target(component, route: ctx.route)) }
            Divider()
        }
    }
}

/// 部品の中の画像の右クリック(#1333)は部品のメニューより先に出るので、その先頭に置く。部品の外(ホストの画面)では何も出さない
struct ComponentTalkImageMenuItem: View {
    @Environment(\.componentTalk) private var talk
    @Environment(\.componentTalkTarget) private var target

    var body: some View {
        if let talk, let target {
            Button(talk.title, systemImage: ComponentTalkMenuItem.icon) { talk.perform(target) }
            Divider()
        }
    }
}
