import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

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
    /// 作ったアプリ(#969)。記録を端末に残す(#1284)。同梱の例は今まで通り一時フォルダ
    var keepsRecords = false
    /// サーバーにある作ったアプリの記録の読み書き先(#1284 の続き)。nil なら端末だけ(`MacRecordVault`)
    var server: (any AppConfigClient)?
    /// 画面マップのシート(#970)。ツールバー・表示メニューが立てる
    var showsScreenMap: Binding<Bool> = .constant(false)
    /// 画面マップの「この画面について話す」・部品の「この部品について話す」(#1323)。マップ・モーダルを閉じ終えてから呼ぶ(シートを 2 枚同時に出さない)
    var onTalk: (ScreenContext) -> Void = { _ in }
    /// openVoice アクション(#946): 声で頼む会話を開く(⇧⌘M と同じ)。nil なら「使えません」を出す
    var onOpenVoice: (@MainActor () -> Void)?
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
        #if !APP_STORE_REVIEW
        // 連係の Handoff(#1851): 今の画面と記録を iPhone へ。届いた Handoff の画面は、ランタイムができてから・開いている時はすぐ開く
        .macHandoff(handoffTarget, appName: app.name)
        .onChange(of: MacHandoffInbox.shared.pending) { revealHandoff() }
        #endif
        .onReceive(NotificationCenter.default.publisher(for: MacAppRuntime.retentionConfirmed)) { note in
            guard let runtime, note.object as? String == runtime.appId else { return }
            Task { await runtime.purgeExpiredRecords() }
        }
        .task {
            // 見え直した時(.task は出るたびに走る)に作り直すと、メモリ上の記録が消える
            guard runtime == nil, !loadFailed else {
                discardGuard?.runtime = runtime
                return
            }
            do {
                let appDSL = try dsl ?? app.loadDSL()
                // 同じアプリを開いた別のウインドウとはエンジンを共有する(同じフォルダを 2 つのエンジンで書くと行が消える)
                let lease = keepsRecords ? MacRecordEngines.shared.acquire(appId: app.id, server: server) : nil
                // 暗号化して保存する項目(#233): サーバーに送る時だけ iOS と同じく端末の鍵で包む(端末だけの記録は包まない)
                if server != nil { lease?.engine.encryptedFields[app.id] = appDSL.encryptedFields }
                let created = MacAppRuntime(appId: app.id, dsl: appDSL, recordLease: lease, dictation: voice?.dictation)
                created.onOpenVoice = onOpenVoice
                created.books = MacServices.shared.books // 本の情報(#1276)。モックの時は nil
                #if !APP_STORE_REVIEW
                created.feedClient = MacServices.shared.feeds // 天気・祝日などの feed(#1288)。モックの時は nil
                #endif
                runtime = created
                discardGuard?.runtime = created
                #if !APP_STORE_REVIEW
                revealHandoff()
                #endif
                await created.purgeExpiredRecords() // 確認済みの保存期間を過ぎた記録を消す(#482・#979)
            } catch {
                loadFailed = true
            }
        }
    }
}

#if !APP_STORE_REVIEW
extension AppCanvas {
    /// Handoff に出す今の画面。iPhone でも開けるサーバーにある自分のアプリだけ(同梱の例・端末だけのアプリは出さない)
    private var handoffTarget: HandoffTarget? {
        guard let runtime, server != nil else { return nil }
        return HandoffTarget(appId: app.id, navigation: runtime.navigationState, dsl: runtime.dsl)
    }

    /// このアプリ宛ての Handoff の画面を開く。保存していない入力があれば確かめてから(#943)
    private func revealHandoff() {
        guard let runtime, let target = MacHandoffInbox.shared.take(appId: app.id) else { return }
        if let discardGuard {
            discardGuard.run { runtime.reveal(target) }
        } else {
            runtime.reveal(target)
        }
    }
}
#endif

private struct AppCanvasContent: View {
    @Bindable var runtime: MacAppRuntime
    @Binding var showsScreenMap: Bool
    let onTalk: (ScreenContext) -> Void
    /// マップの「この画面について話す」・モーダルの中の部品の「この部品について話す」を押した文脈。マップ・モーダルが閉じ終わったら会話を開く
    @State private var pendingTalk: ScreenContext?
    /// 表示メニューの拡大 / 縮小(#1206)。生成したアプリの文字だけを Dynamic Type の段で変える
    @AppStorage(MacTextZoom.storageKey) private var zoomStep = MacTextZoom.defaultStep

    private var textSize: DynamicTypeSize { MacTextZoom.size(at: zoomStep) }

    /// 画面マップで選べる記録(#1322)。表示用の記録(隠した値は落とす)で、見出しは参照の見出しと同じ
    private func recordChoices(_ entity: String) -> [ScreenMapRecordChoice] {
        runtime.viewRecords(for: entity).enumerated().map { index, record in
            let title = runtime.refTitle(record.id, entity: entity).flatMap { $0.isEmpty || $0 == record.id ? nil : $0 }
            return ScreenMapRecordChoice(id: record.id, title: title ?? L10n.tr("記録 \(index + 1)"))
        }
    }

    var body: some View {
        DSLNavigationRoot()
            .dynamicTypeSize(textSize)
            .background { SaveFormShortcut(runtime: runtime) } // ⌘Return で保存(#1138)。⌘S はメニュー(MacCommands)
            .background { ZoomInAlias(step: $zoomStep) } // ⌘= でも拡大(#1206・メニューは ⌘+)
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
                                     onClose: { showsScreenMap = false },
                                     records: recordChoices,
                                     onOpenRecord: { screen, recordId in
                                         runtime.reveal(screen: screen, recordId: recordId)
                                         showsScreenMap = false
                                     })
                }
            }
            // Mac の fullScreenCover は無いので sheet で出す(大きめの最小サイズ)
            .sheet(item: $runtime.sheet, onDismiss: talkAfterModal) { route in
                modal(route).dynamicTypeSize(textSize).frame(minWidth: 420, minHeight: 480)
            }
            .sheet(item: $runtime.fullScreen, onDismiss: talkAfterModal) { route in
                modal(route).dynamicTypeSize(textSize).frame(minWidth: 640, minHeight: 560)
            }
            .modifier(RuntimeOverlays(runtime: runtime))
            .macNotificationsOffBar(runtime.notifications) // 通知がオフの時のバー(#965)
            .toolbar { MacUpcomingNotificationsToolbar(notifications: runtime.notifications) } // 次の通知(#1325)
            .toolbar { MacSyncStatusToolbar(engine: runtime.recordSync) } // 未送信 N 件(#976)
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
        runtime.openVoiceAfterModal()
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
            .background { SaveFormShortcut(runtime: runtime) } // ⌘Return で保存(#1138)
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
            #if !APP_STORE_REVIEW
            // 通報・ブロック・18 歳以上の確認(#963)
            .modifier(MacSocialDialogs(runtime: runtime, active: active))
            #endif
            .overlay(alignment: .top) {
                VStack(spacing: 8) {
                    if active, let message = runtime.toast {
                        ToastCapsule(text: message) { withMacAnimation { runtime.toast = nil } }
                            .task(id: message) {
                                // 出したのと同時に VoiceOver で読み、VoiceOver の時は長く出しておく(#937・iOS の H8 と同じ)
                                if runtime.announcedToast != message { MacVoiceOver.announce(.message(message)) }
                                runtime.announcedToast = nil
                                try? await Task.sleep(for: .seconds(MacVoiceOver.holdSeconds(3)))
                                guard !Task.isCancelled, runtime.toast == message else { return }
                                withMacAnimation { runtime.toast = nil }
                            }
                    }
                    if active, let undo = runtime.recordUndo {
                        UndoDeleteCapsule(runtime: runtime)
                            .task(id: undo.id) {
                                MacVoiceOver.announce(.recordDeleted)
                                try? await Task.sleep(for: .seconds(MacAppRuntime.undoSeconds))
                                guard !Task.isCancelled, runtime.recordUndo == undo else { return }
                                withMacAnimation { runtime.recordUndo = nil }
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
                .macSecondaryText() // コントラストを上げる時は本文の色(#1206)
                .help("閉じる")
                .accessibilityLabel("閉じる")
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .macRaisedSurface(in: Capsule()) // 透明度を下げる・コントラストを上げる(#1206)
        .macTopTransition()
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
        .macRaisedSurface(in: Capsule()) // 透明度を下げる・コントラストを上げる(#1206)
        .macTopTransition()
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

/// ⌘Return でフォームを保存する(#1138)。⌘S はメニューの「保存」が受け、⌘Return は会話のシートの「送る」と同じ指の形で押せるよう
/// 中央の列(根とモーダル)にだけ置く。メニューの「保存」と同じく、フォームが無い・何も変えていない時は押せない
/// 表示メニューの「拡大」(⌘+)の別のキー ⌘=(US 配列では ⇧ 無しで押せる)。メニューには出さない
private struct ZoomInAlias: View {
    @Binding var step: Int

    var body: some View {
        Button("拡大") { step = MacTextZoom.zoomedIn(step) }
            .keyboardShortcut("=", modifiers: .command)
            .disabled(!MacTextZoom.canZoomIn(step))
            .opacity(0)
            .frame(width: 0, height: 0)
            .accessibilityHidden(true)
    }
}

private struct SaveFormShortcut: View {
    let runtime: MacAppRuntime

    var body: some View {
        Button("保存") { Task { await runtime.saveFromMenu() } }
            .keyboardShortcut(.return, modifiers: .command)
            .disabled(!runtime.canSaveFromMenu)
            .opacity(0)
            .frame(width: 0, height: 0)
            .accessibilityHidden(true)
    }
}
