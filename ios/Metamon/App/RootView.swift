import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 画面ツリーの根。`DSLNavigationRoot`(Renderer: タブ or 単一スタック)の上に
/// モーダル・音声シート・確認ダイアログ・生成中演出・要約カード・トーストを重ねる。
///
/// 変身の瞬間: `AppRuntime.apply` が `document` 差し替えと `morphJob = nil` を同じ withAnimation で行うので、
/// 生成中オーバーレイの fade out と新ツリーの scale(0.96→1)+fade in がクロスフェードになる。
struct RootView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.scenePhase) private var scenePhase
    /// Renderer が上げる「最下段(行 11〜12・A〜B 列)に掛かる部品の上端 y」(Route.id → global y)。FAB 退避に使う(#83)
    @State private var bottomObstructions: [String: CGFloat] = [:]
    /// 同じく右端(E〜F 列)。FAB を右に吸着させた時に使う(#52)
    @State private var trailingBottomObstructions: [String: CGFloat] = [:]
    /// マイク FAB の矩形(global・隠れている / 非表示なら nil)。Renderer が背の高い list の末尾に余白を足す(#157)
    @State private var fabFrame: CGRect?
    /// ソフトキーボードの表示状態(FAB を隠す・#143)
    private let keyboard = KeyboardState.shared
    @Environment(\.assistiveAccess) private var assistiveAccess
    /// アシスティブアクセスの一覧から開いたアプリ(#1997)
    @State private var assistiveOpenedAppId: String?

    var body: some View {
        @Bindable var rt = rt
        ZStack {
            Group {
                if let stage = rt.buildStage {
                    #if DEBUG
                    let _ = MorphProbe.rootCreateEvals += 1
                    #endif
                    // 全画面の「変身中」は下の画面に重ねず、置き換える(#200)。
                    // 重ねる方式(zIndex 固定の層)では遅い CI で、morphJob が立っているのに完成まで一度も描かれず、
                    // 下の初期画面(「アプリを考えています…」)が見えたままだった。置き換えなら隠れる先が無い。
                    // 完成時は apply が document 差し替えと morphJob = nil を同じアニメーションで行い、.id が変わってクロスフェードになる
                    // 結果が届いた後も組み上げ(#626)の間はここに留まる(`rootViewID` で作り直さない)
                    GeneratingView(job: stage.job, reveal: stage.reveal)
                        .transition(.asymmetric(insertion: .identity, removal: .opacity))
                } else if assistiveHome == .appList {
                    AssistiveAccessAppList { assistiveOpenedAppId = $0 }
                } else if assistiveHome == .openedApp, AssistiveActions.hasCandidates(in: rt.dsl) {
                    AssistiveAccessActionsView() // #1998 主な動作を大きなボタンで(選べない時は今の画面のまま)
                } else if rt.dsl.isVoiceOnly {
                    // 初期状態(voiceEntry = fullscreen)はグリッドのセル内ではなく全面に描く(背景を画面いっぱいに敷くため)。
                    // 見出しは DSL の prompt ではなく Greeting(#42: 初回/再訪/時間帯で出し分け)
                    VoiceInputView()
                } else {
                    DSLNavigationRoot()
                }
            }
                // appId か version が変わったら丸ごと作り直して transition を出す。
                // appId も含める: 別アプリが同じ version・同じ screen id(home)だと作り直されず、記録が読み直されない(#80 で発見)
                // 組み上げ演出(#626)の間は待っていた時の id に留め、終わった時に新しい id へ(= いつもの変身のクロスフェード)
                .id(rt.rootViewID)
                .transition(.asymmetric(
                    insertion: .scale(scale: 0.96).combined(with: .opacity),
                    removal: .opacity
                ))
                .opacity(rt.morphJob?.kind == .edit ? 0.45 : 1)
                .blur(radius: rt.morphJob?.kind == .edit ? 1.5 : 0)
                // create 中は中身が GeneratingView(30 秒超えのキャンセルを押せるように)。edit 中は下の画面を触らせない
                .allowsHitTesting(rt.morphJob == nil || rt.morphJob?.kind == .create)
                .onPreferenceChange(GridBottomObstructionKey.self) { bottomObstructions = $0 }
                .onPreferenceChange(GridBottomTrailingObstructionKey.self) { trailingBottomObstructions = $0 }
                .environment(\.fabFrame, fabFrame)
                .environment(\.changeHighlight, rt.changeHighlight) // #626 編集で変わった部品だけ光らせる
                .environment(\.gridEditSession, rt.gridEdit) // #1442 手で動かす編集モード
                .safeAreaInset(edge: .top, spacing: 0) { assistiveBackBar }

            // フォーム画面(push)では FAB を出さず、ナビバー右上のマイク(`formToolbarAccessory`)が入口になる(#83)
            // キーボード表示中も隠す(入力欄・キーボードのツールバーに被るため・#143)
            // シート・部品のシートが上にある間はそちらの層が出す(#629 `ModalVoiceFabLayer` / `ComponentAIButtonLayer`)
            ZStack {
                // create の全画面「変身中」の上には出さない(重ねる方式の時は GeneratingView が覆っていた)
                if mainEntry == .fab, rt.gridEdit == nil, assistiveAccess.shows(.aiButton) { // 編集モード中は FAB を出さない(部品のつまみに被る)
                    VoiceFab(
                        hasTabs: rt.dsl.app.hasTabs,
                        leadingObstructionTop: bottomObstructions[visibleMainRoute.id],
                        trailingObstructionTop: trailingBottomObstructions[visibleMainRoute.id],
                        isBusy: rt.morphJob != nil,
                        onTap: { rt.openVoiceForVisibleScreen() },
                        onLongPress: { rt.openSwitcher() },
                        coachMark: rt.fabCoachMark, // #1441
                        onCoachMark: { rt.handleCoachMark($0, $1) },
                        onGesturePerformed: { rt.noteGesturePerformed($0) }
                    )
                    .transition(.scale(scale: 0.6).combined(with: .opacity))
                }
            }
            .animation(.easeOut(duration: 0.22), value: mainEntry)
            .onPreferenceChange(VoiceFabFrameKey.self) { fabFrame = $0 }
            // edit(改変)の小さなインジケータだけを上に重ねる(create の全画面は上の Group で置き換え)
            ZStack {
                if let job = rt.morphJob, job.kind == .edit {
                    VStack {
                        EditingIndicator(job: job)
                            .padding(.top, Theme.Spacing.sm)
                        Spacer()
                    }
                    .transition(.move(edge: .top).combined(with: .opacity))
                }
            }
            .zIndex(2)
            #if DEBUG
            UITestMorphProbe()
            #endif
            if let banner = rt.summaryBanner, let anchor = banner.highlight, rt.isNoticeTurn(.summary) {
                PositionHighlight(anchor: anchor)
                    .transition(.opacity)
                    .zIndex(3)
            }
        }
        .tint(RenderTheme.Colors.brandA) // 生成アプリは常に Tailor It ブランド(docs/ios/design-system.md)
        .preferredColorScheme(colorScheme)
        .sheet(item: $rt.sheet) { route in
            ScreenStack(root: route)
                .modifier(ModalVoiceFabLayer()) // #629 シートの中にも AI ボタン
                .modifier(HostSheets(onModal: true))
                #if !APP_STORE_REVIEW
                .modifier(NearbySheets(onModal: true))
                .modifier(ModerationDialogs(onModal: true))
                .modifier(MatchingSheets(onModal: true))
                .modifier(CallSheets(onModal: true))
                #endif
                .modifier(ToastHost(onModal: true))
        }
        .fullScreenCover(item: $rt.fullScreen) { route in
            ScreenStack(root: route)
                .modifier(ModalVoiceFabLayer())
                .modifier(HostSheets(onModal: true))
                #if !APP_STORE_REVIEW
                .modifier(NearbySheets(onModal: true))
                .modifier(ModerationDialogs(onModal: true))
                .modifier(MatchingSheets(onModal: true))
                .modifier(CallSheets(onModal: true))
                #endif
                .modifier(ToastHost(onModal: true))
        }
        .modifier(HostSheets(onModal: false))
        #if !APP_STORE_REVIEW // 審査用ビルドはシェアリンク・探す・近くの端末との共有を外す(#245)
        .modifier(SharingHooks()) // シェアリンク受信・「探す」シート(#49 #50)
        .modifier(HandoffAdvertiser()) // 今の画面と記録を Mac へ(連係の Handoff・#1851)
        .modifier(NearbySheets(onModal: false)) // 近くの端末と送り合う(#36)
        .modifier(ModerationDialogs()) // 通報・ブロック(#321)
        .modifier(MatchingSheets()) // 仲間探しの 18 歳以上の確認(#507)
        .modifier(CallSheets()) // 通話の画面(#447)
        #endif
        // アシスティブアクセス(#1997)では AI ボタンを出さない
        .environment(\.formToolbarAccessory, assistiveAccess.shows(.aiButton) ? AnyView(VoiceToolbarButton()) : nil)
        .environment(\.aiButtonLayer, assistiveAccess.shows(.aiButton) ? ComponentAIButtonLayer.wrapper : nil) // #629 部品から開くシートにも AI ボタン
        .sensoryFeedback(.impact(weight: .medium), trigger: rt.showAppSwitcher) { _, opened in opened }
        .modifier(PublicDataPromptHost()) // #253 生成直後の「個人の情報も公開されます」
        .modifier(RetentionPromptHost()) // #482 記録の保存期間の確認
        .modifier(TrackingBarHost()) // #559 位置の記録中はどの画面でも上に「記録中」バー
        .modifier(NotificationsOffBarHost()) // L3 通知がオフなら上に「設定を開く」付きのバー
        #if !APP_STORE_REVIEW
        .modifier(FeedbackShakeHost()) // #319 端末を振ってフィードバック
        #endif
        .onChange(of: rt.document.version) {
            // 変身(apply)が落ち着いてから今の画面をサムネに撮る(変身一覧カード用)
            Task {
                // 組み上げ演出(#626)の間は生成中の表示なので、終わって本物の画面に切り替わってから撮る
                let reveal = rt.buildReveal.map { $0.plan.total + 0.7 } ?? 0
                try? await Task.sleep(for: .seconds(1.2 + reveal))
                guard rt.morphJob == nil, rt.buildReveal == nil, !rt.showAppSwitcher else { return }
                AppThumbnailStore.shared.capture(appId: rt.document.appId, isVoiceOnly: rt.dsl.isVoiceOnly)
            }
        }
        #if DEBUG
        .task { await ScreenMapUI.shared.autoOpenIfRequested(rt) } // `METAMON_AUTO_SCREEN_MAP`(#116)
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_SWITCHER=1` で起動 2.5 秒後に変身一覧を開く
            guard ProcessInfo.processInfo.environment["METAMON_AUTO_SWITCHER"] == "1" else { return }
            try? await Task.sleep(for: .seconds(2.5))
            rt.openSwitcher()
        }
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_FOCUS=voice-sheet` で起動 2 秒後に発話シートを開く(シート内の文字入力へフォーカス・#143)
            guard ProcessInfo.processInfo.environment["METAMON_AUTO_FOCUS"] == "voice-sheet", !rt.dsl.isVoiceOnly else { return }
            try? await Task.sleep(for: .seconds(2))
            rt.showVoiceSheet = true
        }
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_OPEN=<screen>[:push|sheet][:record]` で起動 2.5 秒後に画面を開く
            // (`record` を付けると screen.entity の先頭レコードを編集モードで開く)
            guard let spec = ProcessInfo.processInfo.environment["METAMON_AUTO_OPEN"], !spec.isEmpty else { return }
            try? await Task.sleep(for: .seconds(2.5))
            let parts = spec.split(separator: ":").map(String.init)
            let screen = parts[0]
            let recordId = parts.contains("record")
                ? rt.dsl.screen(screen)?.entity.flatMap { rt.records(for: $0).first?.id } : nil
            if parts.contains("sheet") {
                rt.sheet = Route(screen: screen, recordId: recordId, stack: "sheet")
            } else {
                let stack = visibleMainRoute.stack
                rt.paths[stack, default: []].append(Route(screen: screen, recordId: recordId, stack: stack))
            }
        }
        #endif
        // アシスティブアクセス(#1997)では起動時の案内を出さない(待たせていたリンク・Intent は下の task で放す)
        .unlessAssistiveAccess(.launchPrompts, ConsentHost()) // #246: AI への送信の同意・音声モデルのダウンロード確認
        .unlessAssistiveAccess(.launchPrompts, AccountLinkPromptHost()) // #276: 匿名で初めて作った後の「引き継ぎのためにサインイン」
        .unlessAssistiveAccess(.launchPrompts, TutorialHost()) // #274: 同意のあとに初回のチュートリアル・最初のアプリの直後に直し方の例
        #if !APP_STORE_REVIEW
        .unlessAssistiveAccess(.launchPrompts, BetaWelcomeHost()) // #313: β の版だけ、チュートリアルのあとに初回の案内
        #endif
        .task(id: assistiveAccess) {
            rt.assistiveAccess = assistiveAccess
            if !assistiveAccess.shows(.launchPrompts) { rt.launchPresentationsAbandoned() }
        }
        .alert(item: $rt.confirm) { confirm in
            let run: () -> Void = { Task { await confirm.perform() } }
            return Alert(title: Text(confirm.message),
                         primaryButton: confirm.isDestructive ? .destructive(Text(confirm.primaryLabel), action: run)
                                                              : .default(Text(confirm.primaryLabel), action: run),
                         secondaryButton: .cancel(Text("キャンセル")))
        }
        // 編集モード中は上のカードを出さない(「完了」「キャンセル」に被る)。バーは上のカードより後に重ねる
        .overlay(alignment: .top) { if Self.showsTopCards(gridEditing: rt.gridEdit != nil) { summaryCard } }
        .overlay(alignment: .top) { if Self.showsTopCards(gridEditing: rt.gridEdit != nil) { errorCardOverlay } }
        .overlay(alignment: .top) { if rt.gridEdit != nil { GridEditBar().transition(.move(edge: .top).combined(with: .opacity)) } }
        .modifier(ToastHost(onModal: false))
        .modifier(NewItemsHost()) // #268: 開いた直後に他の人の新着の数
        .modifier(UsageNoticeHost()) // #317: 記録・写真が上限に近い(作成者・1 日 1 回)
        .onChange(of: scenePhase) { _, phase in
            rt.setForeground(phase == .active) // ホットリロードのポーリングは前面時のみ
        }
    }

    /// アシスティブアクセス(#1997)の根に出すもの。
    /// アシスティブアクセスで開いたアプリの「戻る」(#1997)。式を分けて Staging の型検査の時間切れを避ける
    @ViewBuilder private var assistiveBackBar: some View {
        if assistiveHome == .openedApp {
            AssistiveAccessBackBar {
                rt.paths = [:] // 次に開いた時は最初の画面から
                assistiveOpenedAppId = nil
            }
        }
    }

    private var assistiveHome: AssistiveAccessPolicy.Home {
        assistiveAccess.home(openedAppId: assistiveOpenedAppId, currentAppId: rt.document.appId)
    }

    /// 上部の要約・失敗のカードを出すか。編集モード(#1442)中は編集のバーを隠さないよう出さない
    static func showsTopCards(gridEditing: Bool) -> Bool { !gridEditing }

    private var colorScheme: ColorScheme? { Self.colorScheme(themeMode: rt.dsl.app.theme?.mode) }

    /// L7: 端末のダークモードを優先する。`dark` のアプリだけ暗く固定し、`light` は固定しない(夜に画面が急に白くならない)
    static func colorScheme(themeMode: String?) -> ColorScheme? {
        themeMode == "dark" ? .dark : nil
    }

    /// メインスタック(タブなら選択中タブ)で今いちばん上に見えている画面。モーダルは含めない(FAB はその下に隠れる)。
    private var visibleMainRoute: Route {
        let hasTabs = rt.dsl.app.hasTabs
        let stack = hasTabs ? Route.tabStack(rt.selectedTab) : "main"
        return rt.paths[stack]?.last ?? Route(screen: hasTabs ? rt.selectedTab : rt.dsl.initialScreen, stack: stack)
    }

    /// 見えている画面がフォーム(`screen.entity` あり)か。全幅の「削除」等に FAB が被るので隠す(#83)
    private var isFormVisible: Bool {
        rt.dsl.screen(visibleMainRoute.screen)?.entity != nil
    }

    /// 根(main 層)の入口。シート・部品のシートが上にある間は出さない(#629)
    private var mainEntry: AIButtonPlacement.Entry {
        AIButtonPlacement.entry(
            layer: .main, voiceEntry: rt.dsl.app.voiceEntry, isForm: isFormVisible, keyboardVisible: keyboard.isVisible,
            isTopLayer: rt.sheet == nil && rt.fullScreen == nil && AIButtonLayers.shared.isEmpty,
            isBuilding: rt.buildStage != nil)
    }

    /// 変身直後の要約(読み上げ文)。上部カードで 3〜6 秒表示して自動で消える(VoiceOver の時は長く・H8)。
    /// 共有の版の課金画面が出ている間は待つ(H7: 出てから時間を数える。課金画面の下で「元に戻す」が時間切れにならない)
    @ViewBuilder
    private var summaryCard: some View {
        if let banner = rt.summaryBanner, rt.isNoticeTurn(.summary) {
            let dismiss = { withAnimation { rt.summaryBanner = nil } }
            HStack(alignment: .top, spacing: Theme.Spacing.sm + 2) {
                CraneMascot(pose: .joy, size: 52, sway: 6) // #102
                Text(banner.text)
                    .font(.subheadline.weight(.medium))
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityDismissAction(dismiss) // #628
                Spacer(minLength: 0)
                if let action = banner.action { SummaryBannerActionButton(action: action) } // #209 元に戻す / やり直す
            }
            .metamonCard(padding: Theme.Spacing.md)
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.top, Theme.Spacing.sm)
            .swipeToDismiss(onDismiss: dismiss) // #628 上に払って閉じる(元に戻す付きでも)
            .transition(.move(edge: .top).combined(with: .opacity))
            .onTapGesture(perform: dismiss)
            .task(id: banner.id) {
                // 文字数に応じて 3〜5 秒(読み上げを読み切れる長さ)
                // ボタン付き(#209)は押す余裕を持たせて 6 秒以上
                // VoiceOver の時は読み上げを聞いてから探して押せるよう長く(H8)
                var seconds = VoiceOverAnnouncer.holdSeconds(max(banner.action == nil ? 3.0 : 6.0, min(5.0, Double(banner.text.count) / 12)))
                #if DEBUG
                // UI テスト用: 遅い CI では出てから押すまでに 6 秒を超え、押す前に消えた(#209)
                if let hold = Double(ProcessInfo.processInfo.environment["METAMON_BANNER_SECONDS"] ?? "") { seconds = hold }
                #endif
                try? await Task.sleep(for: .seconds(seconds))
                // 次のバナーに差し替わって取り消された時は消さない(新しい方まで消えていた・#209)
                guard !Task.isCancelled else { return }
                withAnimation(.easeInOut(duration: 0.35)) { rt.summaryBanner = nil }
            }
        }
    }

    /// 生成後(edit)の失敗は音声シートが閉じているので、画面上部に状態カードを出す(#60)。初期画面と音声シートの中は `VoiceInputView` が描く。
    @ViewBuilder
    private var errorCardOverlay: some View {
        if let card = rt.errorCard, !rt.dsl.isVoiceOnly, !rt.showVoiceSheet {
            ErrorCard(presentation: card, compact: true)
                .padding(.horizontal, Theme.Spacing.md)
                .padding(.top, Theme.Spacing.sm)
                .transition(.move(edge: .top).combined(with: .opacity))
                .animation(.spring(duration: 0.45, bounce: 0.15), value: rt.errorCard)
                .zIndex(4)
        }
    }
}

/// 下部の toast。モーダル(sheet / fullScreen)が開いている間はその中に出す(根の overlay はモーダルに隠れる)。
private struct ToastHost: ViewModifier {
    let onModal: Bool
    @Environment(AppRuntime.self) private var rt
    @Environment(\.assistiveAccess) private var assistiveAccess

    func body(content: Content) -> some View {
        let active = onModal == (rt.sheet != nil || rt.fullScreen != nil)
        content.overlay(alignment: .bottom) {
            if active, let message = rt.toast {
                let dismiss = { withAnimation { rt.toast = nil } }
                let fixable = rt.fixOffer?.message == message && assistiveAccess.shows(.voiceSheet) // #1997
                HStack(spacing: Theme.Spacing.sm) {
                    Text(message)
                    if fixable { FixRequestToastButton() }
                }
                    .font(.footnote.weight(.medium))
                    .padding(.horizontal, Theme.Spacing.md).padding(.vertical, Theme.Spacing.sm + 2)
                    .background(.regularMaterial, in: Capsule())
                    .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, Theme.Spacing.md)
                    // マイク FAB と DSL の最下段(行 11〜12 のボタン)の上に出す(被せない)。タブ無しでも同じ高さ
                    .accessibilityDismissAction(dismiss) // #628
                    .swipeToDismiss(edge: .bottom, onDismiss: dismiss) // 下に出るので下に払って閉じる
                    .padding(.bottom, Theme.Layout.fabBottomInset(hasTabs: true) + Theme.Layout.fabSize + Theme.Spacing.md)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                    .task(id: rt.toastSerial) { // 同じ文言が続いても出し直す
                        // H8: VoiceOver の時は長く。「直してと頼む」付きは押す間を取る
                        try? await Task.sleep(for: .seconds(VoiceOverAnnouncer.holdSeconds(fixable ? 6 : 3)))
                        guard !Task.isCancelled else { return }
                        withAnimation { rt.toast = nil }
                    }
            }
        }
        .overlay(alignment: .bottom) {
            if active, let undo = rt.recordUndo { // M11 削除しました・元に戻す。普通のトーストと重なったらその上に積む
                RecordUndoToast(undo: undo)
                    .padding(.horizontal, Theme.Spacing.md)
                    .padding(.bottom, Theme.Layout.fabBottomInset(hasTabs: true) + Theme.Layout.fabSize + Theme.Spacing.md
                             + (rt.toast == nil ? 0 : 56))
            }
        }
    }
}

/// 失敗のトーストの「直してと頼む」。音声シートの入力欄に依頼文を入れて開く(送らない)。
private struct FixRequestToastButton: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        Button { withAnimation { rt.requestFix() } } label: {
            Label("直してと頼む", systemImage: "wand.and.stars")
                .labelStyle(.titleAndIcon)
        }
        .buttonStyle(.borderless)
        .foregroundStyle(Theme.Colors.brandText)
        .fixedSize()
        .accessibilityHint(Text("直す依頼を入力欄に入れて、話しかける画面を開きます"))
        .accessibilityIdentifier("toast.fixRequest")
    }
}

#if DEBUG
/// UI テスト用の観測点(`METAMON_UI_PROBE=1` の時だけ置く・#200)。生成中の表示が出ない時に、
/// 「morphJob が立っていたか」「GeneratingView の onAppear が呼ばれたか」をテスト側から読めるようにする。
private struct UITestMorphProbe: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        if ProcessInfo.processInfo.environment["METAMON_UI_PROBE"] == "1" {
            let job = rt.morphJob.map { $0.kind == .create ? "create" : "edit" } ?? "none"
            Color.clear
                .frame(width: 1, height: 1)
                .allowsHitTesting(false)
                .accessibilityElement()
                .accessibilityIdentifier("debug.morphProbe")
                .accessibilityLabel("job=\(job) createStarted=\(MorphProbe.createStarted) generatingAppeared=\(MorphProbe.generatingAppeared) rootCreateEvals=\(MorphProbe.rootCreateEvals) fallbackAppeared=\(MorphProbe.fallbackAppeared)")
        }
    }
}
#endif

/// 発話シート・変身一覧の present 元。フォームが sheet/fullScreen で出ている間は根からは present できない(二重 present になる)ので、
/// モーダルの中身にも同じ 2 枚を付け、「モーダルが出ているか」で片方だけが実際に開く。
private struct HostSheets: ViewModifier {
    let onModal: Bool
    @Environment(AppRuntime.self) private var rt
    @Environment(\.assistiveAccess) private var assistiveAccess

    func body(content: Content) -> some View {
        let active = onModal == (rt.sheet != nil || rt.fullScreen != nil)
        content
            // 部品から開いたシートが上にある間は、そちらが出す(#629 `ComponentAIButtonLayer`)。アシスティブアクセス(#1997)では出さない
            .sheet(isPresented: Binding(get: { active && rt.showVoiceSheet && AIButtonLayers.shared.isEmpty && assistiveAccess.shows(.voiceSheet) },
                                        set: { rt.showVoiceSheet = $0 })) {
                VoiceInputView()
                    .presentationDetents([.large])
                    .presentationDragIndicator(.visible)
            }
            .sheet(isPresented: Binding(get: { active && rt.showAppSwitcher && assistiveAccess.shows(.appSwitcher) },
                                        set: { rt.showAppSwitcher = $0 })) {
                AppSwitcherSheet()
            }
            .sheet(item: Binding(get: { active ? rt.exportFile : nil }, set: { rt.exportFile = $0 })) { file in
                ActivityView(items: [file.url])
                    .presentationDetents([.medium, .large])
                    .ignoresSafeArea()
            }
            .sheet(item: Binding(get: { active ? rt.calendarDraft : nil }, set: { rt.calendarDraft = $0 })) { draft in
                CalendarEventEditor(draft: draft) { saved in
                    rt.calendarDraft = nil
                    if saved { rt.toast = L10n.tr("カレンダーに追加しました") }
                }
                .ignoresSafeArea()
            }
            .modifier(AuthorNamePromptModifier(active: active)) // 共有アプリへの初投稿で表示名を聞く(#250)
    }
}

/// summary に「右下」等の位置語があった時、その領域を 2 回ほど淡く脈打たせる(部品単位ではなく領域ハイライト)。
struct PositionHighlight: View {
    let anchor: UnitPoint

    var body: some View {
        GeometryReader { geo in
            TimelineView(.animation(minimumInterval: 1.0 / 30)) { tl in
                let t = tl.date.timeIntervalSinceReferenceDate
                let pulse = 0.5 + 0.5 * sin(t * 4)
                Circle()
                    .fill(RadialGradient(colors: [Theme.Colors.brandB.opacity(0.35 + 0.25 * pulse), .clear],
                                         center: .center, startRadius: 0, endRadius: 120))
                    .frame(width: 260, height: 260)
                    .scaleEffect(0.9 + 0.15 * pulse)
                    .position(x: geo.size.width * anchor.x, y: geo.size.height * anchor.y)
            }
        }
        .allowsHitTesting(false)
        .ignoresSafeArea()
    }
}

#Preview("家計簿サンプル") {
    let services = BuildConfig.make(environment: .mock)
    let rt = AppRuntime(document: (try? DSLExamples.document("kakeibo")) ?? .fallback, services: services)
    return RootView()
        .environment(rt)
        .environment(\.dslRuntime, rt)
        .environment(AuthSession(service: services.auth))
        .environment(\.services, services)
        .environment(\.componentRegistry, MetamonApp.componentRegistry)
}
