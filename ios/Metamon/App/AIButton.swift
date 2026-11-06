import SwiftUI
import Observation
import MetamonDSL
import MetamonRenderer

/// 追加の指示をする AI ボタン(マイク)を、どの画面・シートでも出す(#629)。
///
/// SwiftUI のシートは別の階層に出て根の overlay が隠れるので、層ごとに同じ入口を置き、いちばん上の層だけが出す:
/// - main: 根(タブ・メインのスタック。push 先を含む)… 左下の FAB(`RootView`)
/// - modal: DSL のシート / 全画面(`rt.sheet` / `rt.fullScreen`。中の push を含む)… 同じ FAB(`ModalVoiceFabLayer`)
/// - component: 部品から開くシート(場所を選ぶ・曲を探す 等)… ナビバー右上の小さなマイク(`ComponentAIButtonLayer`)
/// フォーム画面は層によらず、従来どおりナビバー右上の小さなマイク(保存ボタン・削除に FAB が被るため・#83)。
/// システムのアラート・確認の選択肢・カメラは SwiftUI の外の UIKit が一番上に出すので、その上には出さない。
enum AIButtonPlacement {
    enum Layer: Equatable { case main, modal, component }
    enum Entry: Equatable { case fab, toolbar, none }

    /// 今見えている画面。`isStackRoot` = スタックの根(push していない)。
    struct Visible: Equatable {
        var route: Route
        var layer: Layer
        var isStackRoot: Bool
    }

    /// その層に出す入口。上に別の層が重なっている(`isTopLayer == false`)・初期画面(音声だけ)では出さない。
    /// FAB は `voiceEntry == .floating` の時だけ・キーボード表示中は隠す(#143)。ナビバーのマイクはキーボードに被らないので出したまま。
    static func entry(layer: Layer, voiceEntry: VoiceEntry, isForm: Bool, keyboardVisible: Bool, isTopLayer: Bool) -> Entry {
        guard isTopLayer, voiceEntry != .fullscreen else { return .none }
        if layer == .component || isForm { return .toolbar }
        return FabVisibility.isShown(floating: voiceEntry == .floating, formVisible: false, keyboardVisible: keyboardVisible) ? .fab : .none
    }

    /// 今見えている画面(全画面 > シート > メインのスタック)。
    static func visible(paths: [String: [Route]], selectedTab: String, hasTabs: Bool, initialScreen: String,
                        sheet: Route?, fullScreen: Route?) -> Visible {
        if let modal = fullScreen ?? sheet {
            let pushed = paths[modal.stack]?.last
            return Visible(route: pushed ?? modal, layer: .modal, isStackRoot: pushed == nil)
        }
        let stack = hasTabs ? Route.tabStack(selectedTab) : "main"
        let pushed = paths[stack]?.last
        return Visible(route: pushed ?? Route(screen: hasTabs ? selectedTab : initialScreen, stack: stack),
                       layer: .main, isStackRoot: pushed == nil)
    }

    /// 押した時に指示の対象にする画面(と部品)。
    /// タブの無いアプリの最初の画面(根)だけは付けない: アプリ全体への指示(「色を変えて」等)を 1 画面に縛らないため(従来どおり)。
    static func context(visible: Visible, hasTabs: Bool, screenTitle: String?, component: AIButtonComponent?) -> ScreenContext? {
        if component == nil, visible.layer == .main, visible.isStackRoot, !hasTabs { return nil }
        let title = screenTitle.flatMap { $0.isEmpty ? nil : $0 } ?? visible.route.screen
        return ScreenContext(id: visible.route.screen, title: title, component: component)
    }
}

/// 部品から開いたシート(component 層)の積み重ね。音声シートはいちばん上の層から出す(下の層から出すと二重の present で開かない)。
@MainActor
@Observable
final class AIButtonLayers {
    static let shared = AIButtonLayers()
    private(set) var stack: [UUID] = []

    var isEmpty: Bool { stack.isEmpty }
    func isTop(_ id: UUID) -> Bool { stack.last == id }
    func push(_ id: UUID) { if !stack.contains(id) { stack.append(id) } }
    func remove(_ id: UUID) { stack.removeAll { $0 == id } }
}

extension AppRuntime {
    /// 今見えている画面(全画面 > シート > メインのスタック)。
    var visibleScreen: AIButtonPlacement.Visible {
        AIButtonPlacement.visible(paths: paths, selectedTab: selectedTab, hasTabs: dsl.app.hasTabs,
                                  initialScreen: dsl.initialScreen, sheet: sheet, fullScreen: fullScreen)
    }

    /// AI ボタンを押した: 今の画面(・部品)を指示の対象にして音声シートを開く(#629)。生成中は何もしない。
    func openVoiceForVisibleScreen(component: AIButtonComponent? = nil) {
        guard morphJob == nil else { return }
        let visible = visibleScreen
        ScreenMapUI.shared.context = AIButtonPlacement.context(
            visible: visible, hasTabs: dsl.app.hasTabs,
            screenTitle: dsl.screen(visible.route.screen)?.title, component: component)
        showVoiceSheet = true
    }
}

/// DSL のシート / 全画面(modal 層)に重ねるマイク FAB。根の FAB はシートの下に隠れるので、こちらが同じ見た目で出す。
struct ModalVoiceFabLayer: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @State private var bottomObstructions: [String: CGFloat] = [:]
    @State private var trailingBottomObstructions: [String: CGFloat] = [:]
    @State private var fabFrame: CGRect?
    private let keyboard = KeyboardState.shared
    private let layers = AIButtonLayers.shared

    func body(content: Content) -> some View {
        let visible = rt.visibleScreen
        let entry = AIButtonPlacement.entry(
            layer: .modal, voiceEntry: rt.dsl.app.voiceEntry,
            isForm: rt.dsl.screen(visible.route.screen)?.entity != nil,
            keyboardVisible: keyboard.isVisible, isTopLayer: visible.layer == .modal && layers.isEmpty)
        ZStack {
            content
                .onPreferenceChange(GridBottomObstructionKey.self) { bottomObstructions = $0 }
                .onPreferenceChange(GridBottomTrailingObstructionKey.self) { trailingBottomObstructions = $0 }
                .environment(\.fabFrame, fabFrame)
            ZStack {
                if entry == .fab {
                    VoiceFab(
                        hasTabs: false, // シートにはタブバーが無い
                        leadingObstructionTop: bottomObstructions[visible.route.id],
                        trailingObstructionTop: trailingBottomObstructions[visible.route.id],
                        isBusy: rt.morphJob != nil,
                        onTap: { rt.openVoiceForVisibleScreen() },
                        onLongPress: { rt.openSwitcher() }
                    )
                    .transition(.scale(scale: 0.6).combined(with: .opacity))
                }
            }
            .animation(.easeOut(duration: 0.22), value: entry)
            .onPreferenceChange(VoiceFabFrameKey.self) { fabFrame = $0 }
        }
    }
}

/// 部品から開くシート(component 層)の中身を包む: ナビバー右上に小さなマイクを置き、音声シートをここから出す。
/// シートの下半分には「この場所に決める」等の全幅のボタン・検索欄があるので、FAB ではなくナビバーに置く。
struct ComponentAIButtonLayer: View {
    let content: AnyView
    let component: AIButtonComponent
    @Environment(AppRuntime.self) private var rt
    @State private var id = UUID()
    private let layers = AIButtonLayers.shared

    var body: some View {
        content
            .toolbar {
                if !rt.dsl.isVoiceOnly {
                    ToolbarItem(placement: .topBarTrailing) {
                        VoiceToolbarButton(target: component) { rt.openVoiceForVisibleScreen(component: component) }
                    }
                }
            }
            .onAppear { layers.push(id) }
            .onDisappear { layers.remove(id) }
            .sheet(isPresented: Binding(get: { layers.isTop(id) && rt.showVoiceSheet }, set: { rt.showVoiceSheet = $0 })) {
                VoiceInputView()
                    .presentationDetents([.large])
                    .presentationDragIndicator(.visible)
            }
    }

    /// `\.aiButtonLayer` に渡すホストの処理。
    static let wrapper: AIButtonLayerWrapper = { content, component in
        AnyView(ComponentAIButtonLayer(content: content, component: component))
    }
}

/// ナビバー右上に置く小さなマイク(フォーム画面・部品のシート)。タップ = 発話シート、長押し = 変身一覧(FAB と同じ 2 入口)。
/// 部品のシート(`target` あり)では長押しを付けない(シートを開いたまま別のアプリへ切り替えないため)。
struct VoiceToolbarButton: View {
    var target: AIButtonComponent?
    var onTap: (() -> Void)?
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        Button { if let onTap { onTap() } else { rt.openVoiceForVisibleScreen() } } label: {
            Image(systemName: "mic.fill")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(.white)
                .frame(width: Theme.Layout.toolbarMicSize, height: Theme.Layout.toolbarMicSize)
                .background(Theme.brandGradient, in: Circle())
                .overlay(Circle().strokeBorder(.white.opacity(0.25)))
        }
        .buttonStyle(.plain)
        .simultaneousGesture(
            LongPressGesture(minimumDuration: Theme.Layout.fabLongPressSeconds)
                .onEnded { _ in if target == nil, rt.morphJob == nil { rt.openSwitcher() } }
        )
        // 生成中は FAB と同じく押せない(フォームのシートは本体の暗転の外なので、ここから設定の「初期状態に戻す」等に行けていた)
        .opacity(rt.morphJob != nil ? 0.35 : 1)
        .allowsHitTesting(rt.morphJob == nil)
        .disabled(rt.morphJob != nil)
        .accessibilityLabel("話しかけて変える")
        .modifier(VoiceToolbarAccessibility(target: target))
        .accessibilityIdentifier(target == nil ? "voice.toolbarMic" : "voice.componentMic")
    }
}

private struct VoiceToolbarAccessibility: ViewModifier {
    let target: AIButtonComponent?
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        if let target {
            content.accessibilityHint(L10n.tr("「\(target.label)」について話しかけます"))
        } else {
            content
                .accessibilityHint(L10n.key("長押しで変身一覧", review: "長押しで記録帳の一覧"))
                .accessibilityAction(named: L10n.key("変身一覧", review: "記録帳の一覧")) { if rt.morphJob == nil { rt.openSwitcher() } }
        }
    }
}
