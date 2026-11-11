import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 中央: 選んだアプリを Renderer で描く。タブ / 単一スタックは `DSLNavigationRoot`、sheet / fullScreen はここで重ねる。
/// 確認ダイアログは `.confirmationDialog`、一言(toast)と「削除しました・元に戻す」は中央の列の上に出す(#939)。
/// アプリを切り替えたら呼び出し側の `.id(app.id)` でランタイムごと作り直す。
struct AppCanvas: View {
    @State private var runtime: MacAppRuntime?
    /// 声で頼む(#966)。生成したアプリの話して入力(#1328)も同じ認識を使う
    @Environment(MacVoiceInput.self) private var voice: MacVoiceInput?
    @State private var loadFailed = false
    let app: SampleApp
    /// 会話で作った・直したアプリの DSL(#967)。nil なら同梱の例を読む
    var dsl: AppDSL?
    /// 画面マップのシート(#970)。ツールバー・表示メニューが立てる
    var showsScreenMap: Binding<Bool> = .constant(false)
    /// 画面マップの「この画面について話す」・部品の「この部品について話す」(#1323)。マップ・モーダルを閉じ終えてから呼ぶ(シートを 2 枚同時に出さない)
    var onTalk: (ScreenContext) -> Void = { _ in }
    /// ウインドウの入力の見張り(#943)。作ったランタイムを渡す(アプリの切り替え・⌘W の前に確認する)
    var discardGuard: MacDiscardGuard?

    var body: some View {
        Group {
            if let runtime {
                AppCanvasContent(runtime: runtime, showsScreenMap: showsScreenMap, onTalk: onTalk)
            } else if loadFailed {
                ContentUnavailableView("アプリを開けませんでした", systemImage: "exclamationmark.triangle")
            } else {
                ProgressView()
            }
        }
        .navigationTitle(app.name)
        .task {
            // 見え直した時(.task は出るたびに走る)に作り直すと、メモリ上の記録が消える
            guard runtime == nil, !loadFailed else {
                discardGuard?.runtime = runtime
                return
            }
            do {
                let created = MacAppRuntime(appId: app.id, dsl: try dsl ?? app.loadDSL(), dictation: voice?.dictation)
                runtime = created
                discardGuard?.runtime = created
            } catch {
                loadFailed = true
            }
        }
    }
}

private struct AppCanvasContent: View {
    @Bindable var runtime: MacAppRuntime
    @Binding var showsScreenMap: Bool
    let onTalk: (ScreenContext) -> Void
    /// マップの「この画面について話す」・モーダルの中の部品の「この部品について話す」を押した文脈。マップ・モーダルが閉じ終わったら会話を開く
    @State private var pendingTalk: ScreenContext?

    var body: some View {
        DSLNavigationRoot()
            .background {
                // 画面マップ(#970)。アプリのシートとは別の view に付ける(同じ view に sheet を重ねない)
                Color.clear.sheet(isPresented: $showsScreenMap, onDismiss: talkAfterMap) {
                    MacScreenMapView(dsl: runtime.dsl, current: runtime.currentScreen,
                                     onOpen: { screen in
                                         runtime.reveal(screen: screen)
                                         showsScreenMap = false
                                     },
                                     onTalk: { screen in
                                         pendingTalk = screen
                                         showsScreenMap = false
                                     },
                                     onClose: { showsScreenMap = false })
                }
            }
            // Mac の fullScreenCover は無いので sheet で出す(大きめの最小サイズ)
            .sheet(item: $runtime.sheet, onDismiss: talkAfterModal) { route in
                modal(route).frame(minWidth: 420, minHeight: 480)
            }
            .sheet(item: $runtime.fullScreen, onDismiss: talkAfterModal) { route in
                modal(route).frame(minWidth: 640, minHeight: 560)
            }
            .modifier(RuntimeOverlays(runtime: runtime))
            .macNotificationsOffBar(runtime.notifications) // 通知がオフの時のバー(#965)
            .toolbar { MacUpcomingNotificationsToolbar(notifications: runtime.notifications) } // 次の通知(#1325)
            .environment(\.dslRuntime, runtime)
            .environment(\.componentRegistry, ComponentRegistry.standard)
            .environment(\.openForms, runtime.openForms)
            .environment(\.componentTalk, componentTalk)
            .focusedSceneValue(\.macAppRuntime, runtime) // メニューの「戻る」・削除の取り消し(#934)
    }

    private func talkAfterMap() {
        guard let screen = pendingTalk else { return }
        pendingTalk = nil
        onTalk(screen)
    }

    /// モーダルが閉じ終わった。全画面とシートの両方を閉じた時は、両方が閉じてから開く
    private func talkAfterModal() {
        guard runtime.sheet == nil, runtime.fullScreen == nil else { return }
        talkAfterMap()
    }

    /// 部品の右クリック・VoiceOver の操作の「この部品について話す」(#1323)
    private var componentTalk: ComponentTalk {
        ComponentTalk(title: ScreenMapCopy.talkAboutComponent) { target in talk(about: target) }
    }

    /// 中央の部品はそのまま会話を開く(入力の確認は `MacRootView` の `discardGuard` が通す・#1364)。
    /// モーダルの中の部品は、モーダルの入力を確かめてからモーダルを閉じ、閉じ終わったら開く(会話のシートをモーダルに重ねない)
    private func talk(about target: ComponentTalk.Target) {
        let screen = ScreenContext(target, dsl: runtime.dsl)
        guard runtime.sheet != nil || runtime.fullScreen != nil else { return onTalk(screen) }
        runtime.confirmDiscard(leaving: { $0.stack == "sheet" || $0.stack == "full" }) {
            pendingTalk = screen
            while runtime.sheet != nil || runtime.fullScreen != nil { runtime.dismissTopModal() }
        }
    }

    /// モーダルの中にも確認と一言を出す(根の上に重ねるとシートに隠れる・iOS の RootView と同じ理由)
    private func modal(_ route: Route) -> some View {
        ScreenStack(root: route)
            .background { EscapeToDismiss { runtime.requestDismissTopModal() } } // 入力があれば先に確認(#943)
            .modifier(RuntimeOverlays(runtime: runtime))
            .environment(\.dslRuntime, runtime)
            .environment(\.componentRegistry, ComponentRegistry.standard)
            .environment(\.openForms, runtime.openForms)
            .environment(\.componentTalk, componentTalk)
    }
}

/// 確認ダイアログ・一言・削除の取り消し。sheet が開いている時はシートの中だけに出す。
private struct RuntimeOverlays: ViewModifier {
    @Bindable var runtime: MacAppRuntime
    @Environment(\.isPresented) private var isPresented
    /// 話して入力の「音声モデルを用意する」の確認(#1328)
    @Environment(MacVoiceInput.self) private var voice: MacVoiceInput?
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var active: Bool {
        // 根(シートでない)はシートが開いている間は出さない
        isPresented || (runtime.sheet == nil && runtime.fullScreen == nil)
    }

    func body(content: Content) -> some View {
        content
            .confirmationDialog(runtime.confirm?.message ?? "", isPresented: confirmShown, titleVisibility: .visible,
                                presenting: runtime.confirm) { confirm in
                Button(confirm.primaryLabel, role: confirm.isDestructive ? .destructive : nil) {
                    Task { await confirm.perform() }
                }
                Button("キャンセル", role: .cancel) {}
            }
            // 保存していない入力の破棄(Esc・⌘[・⌘W・アプリの切り替え・#943)。iOS のフォームを閉じる時と同じ文言
            .discardFormConfirmation(isPresented: discardShown) {
                let request = runtime.discardRequest
                runtime.discardRequest = nil
                request?.perform()
            }
            // addToCalendar(#978): 中身を見せてから書く
            .sheet(item: calendarShown) { draft in
                MacCalendarSheet(draft: draft, writer: runtime.calendarWriter) { runtime.calendarDraft = nil }
            }
            // AI の部品(#962): 同意の無いまま送ろうとしたら説明を出す。同意したら押し直しで送る(iOS と同じ)
            .sheet(isPresented: consentShown) { MacAIConsentSheet() }
            // 話して入力(aiChat・transcribeField・#1328)の「音声モデルを用意する」: 会話のシートと同じ確認
            .modifier(MacModelDownloadConfirm(shown: modelDownloadShown))
            .overlay(alignment: .top) {
                VStack(spacing: 8) {
                    if active, let message = runtime.toast {
                        ToastCapsule(text: message) { withAnimation { runtime.toast = nil } }
                            .task(id: message) {
                                // 出したのと同時に VoiceOver で読み、VoiceOver の時は長く出しておく(#937・iOS の H8 と同じ)
                                MacVoiceOver.announce(message)
                                try? await Task.sleep(for: .seconds(MacVoiceOver.holdSeconds(3)))
                                guard !Task.isCancelled, runtime.toast == message else { return }
                                withAnimation { runtime.toast = nil }
                            }
                    }
                    if active, let undo = runtime.recordUndo {
                        UndoDeleteCapsule(runtime: runtime)
                            .task(id: undo.id) {
                                MacVoiceOver.announce(String(localized: "削除しました"))
                                try? await Task.sleep(for: .seconds(MacAppRuntime.undoSeconds))
                                guard !Task.isCancelled, runtime.recordUndo == undo else { return }
                                withAnimation { runtime.recordUndo = nil }
                            }
                    }
                }
                .padding(.top, 12)
                // 「視差効果を減らす」の時は上から滑り込ませず、その場で出す(#937)
                .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: runtime.toast)
                .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: runtime.recordUndo)
            }
    }

    private var confirmShown: Binding<Bool> {
        Binding(get: { active && runtime.confirm != nil }, set: { if !$0 { runtime.confirm = nil } })
    }

    private var consentShown: Binding<Bool> {
        Binding(get: { active && runtime.needsAIConsent }, set: { if !$0 { runtime.needsAIConsent = false } })
    }

    private var modelDownloadShown: Binding<Bool> {
        Binding(get: { active && voice?.showsAppDownloadConfirm == true },
                set: { if !$0 { voice?.showsAppDownloadConfirm = false } })
    }

    private var discardShown: Binding<Bool> {
        Binding(get: { active && runtime.discardRequest != nil }, set: { if !$0 { runtime.discardRequest = nil } })
    }

    private var calendarShown: Binding<CalendarEventDraft?> {
        Binding(get: { active ? runtime.calendarDraft : nil }, set: { runtime.calendarDraft = $0 })
    }
}

private struct ToastCapsule: View {
    let text: String
    let dismiss: () -> Void

    var body: some View {
        HStack(spacing: 8) {
            Text(text).font(.callout)
            Button(action: dismiss) { Image(systemName: "xmark") }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .help("閉じる")
                .accessibilityLabel("閉じる")
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .background(.regularMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(.separator))
        .transition(.move(edge: .top).combined(with: .opacity))
        .accessibilityElement(children: .combine)
    }
}

private struct UndoDeleteCapsule: View {
    let runtime: MacAppRuntime

    var body: some View {
        HStack(spacing: 12) {
            Text("削除しました").font(.callout)
            // ⌘Z はメニューの「削除を元に戻す」(MacCommands)が受ける
            Button("元に戻す") { Task { await runtime.undoRecordDelete() } }
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .background(.regularMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(.separator))
        .transition(.move(edge: .top).combined(with: .opacity))
    }
}

/// Esc でシートを閉じる(Mac の sheet は Esc で閉じないため)。cancelAction なので入力中の変換の取り消しは邪魔しない。
private struct EscapeToDismiss: View {
    let dismiss: () -> Void

    var body: some View {
        Button("閉じる", action: dismiss)
            .keyboardShortcut(.cancelAction)
            .opacity(0)
            .frame(width: 0, height: 0)
            .accessibilityHidden(true)
    }
}
