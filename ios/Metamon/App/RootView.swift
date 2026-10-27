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

    var body: some View {
        @Bindable var rt = rt
        ZStack {
            Group {
                if let job = rt.morphJob, job.kind == .create {
                    #if DEBUG
                    let _ = MorphProbe.rootCreateEvals += 1
                    #endif
                    // 全画面の「変身中」は下の画面に重ねず、置き換える(#200)。
                    // 重ねる方式(zIndex 固定の層)では遅い CI で、morphJob が立っているのに完成まで一度も描かれず、
                    // 下の初期画面(「アプリを考えています…」)が見えたままだった。置き換えなら隠れる先が無い。
                    // 完成時は apply が document 差し替えと morphJob = nil を同じアニメーションで行い、.id が変わってクロスフェードになる
                    GeneratingView(job: job)
                        .transition(.asymmetric(insertion: .identity, removal: .opacity))
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
                .id("\(rt.document.appId)#\(rt.document.version)")
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

            // フォーム画面(push)では FAB を出さず、ナビバー右上のマイク(`formToolbarAccessory`)が入口になる(#83)
            // キーボード表示中も隠す(入力欄・キーボードのツールバーに被るため・#143)
            ZStack {
                // create の全画面「変身中」の上には出さない(重ねる方式の時は GeneratingView が覆っていた)
                if rt.morphJob?.kind != .create,
                   FabVisibility.isShown(floating: rt.dsl.app.voiceEntry == .floating, formVisible: isFormVisible,
                                         keyboardVisible: keyboard.isVisible) {
                    VoiceFab(
                        hasTabs: rt.dsl.app.hasTabs,
                        leadingObstructionTop: bottomObstructions[visibleMainRoute.id],
                        trailingObstructionTop: trailingBottomObstructions[visibleMainRoute.id],
                        isBusy: rt.morphJob != nil,
                        onTap: { rt.showVoiceSheet = true },
                        onLongPress: { rt.openSwitcher() }
                    )
                    .transition(.scale(scale: 0.6).combined(with: .opacity))
                }
            }
            .animation(.easeOut(duration: 0.22), value: isFormVisible)
            .animation(.easeOut(duration: 0.22), value: keyboard.isVisible)
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
            if let banner = rt.summaryBanner, let anchor = banner.highlight {
                PositionHighlight(anchor: anchor)
                    .transition(.opacity)
                    .zIndex(3)
            }
        }
        .tint(RenderTheme.Colors.brandA) // 生成アプリは常に Tailor It ブランド(docs/ios/design-system.md)
        .preferredColorScheme(colorScheme)
        .sheet(item: $rt.sheet) { route in
            ScreenStack(root: route)
                .modifier(HostSheets(onModal: true))
                #if !APP_STORE_REVIEW
                .modifier(NearbySheets(onModal: true))
                #endif
                .modifier(ToastHost(onModal: true))
        }
        .fullScreenCover(item: $rt.fullScreen) { route in
            ScreenStack(root: route)
                .modifier(HostSheets(onModal: true))
                #if !APP_STORE_REVIEW
                .modifier(NearbySheets(onModal: true))
                #endif
                .modifier(ToastHost(onModal: true))
        }
        .modifier(HostSheets(onModal: false))
        #if !APP_STORE_REVIEW // 審査用ビルドはシェアリンク・探す・近くの端末との共有を外す(#245)
        .modifier(SharingHooks()) // シェアリンク受信・「探す」シート(#49 #50)
        .modifier(NearbySheets(onModal: false)) // 近くの端末と送り合う(#36)
        #endif
        .environment(\.formToolbarAccessory, AnyView(VoiceToolbarButton()))
        .sensoryFeedback(.impact(weight: .medium), trigger: rt.showAppSwitcher) { _, opened in opened }
        .onChange(of: rt.document.version) {
            // 変身(apply)が落ち着いてから今の画面をサムネに撮る(変身一覧カード用)
            Task {
                try? await Task.sleep(for: .milliseconds(1200))
                guard rt.morphJob == nil, !rt.showAppSwitcher else { return }
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
        .modifier(ConsentHost()) // #246: AI への送信の同意・音声モデルのダウンロード確認
        .modifier(AccountLinkPromptHost()) // #276: 匿名で初めて作った後の「引き継ぎのためにサインイン」
        .alert(item: $rt.confirm) { confirm in
            Alert(title: Text(confirm.message),
                  primaryButton: .destructive(Text("実行")) { Task { await confirm.perform() } },
                  secondaryButton: .cancel(Text("キャンセル")))
        }
        .overlay(alignment: .top) { summaryCard }
        .overlay(alignment: .top) { errorCardOverlay }
        .modifier(ToastHost(onModal: false))
        .onChange(of: scenePhase) { _, phase in
            rt.setForeground(phase == .active) // ホットリロードのポーリングは前面時のみ
        }
    }

    private var colorScheme: ColorScheme? {
        switch rt.dsl.app.theme?.mode { case "light": .light; case "dark": .dark; default: nil }
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

    /// 変身直後の要約(読み上げ文)。上部カードで 3 秒表示して自動で消える。
    @ViewBuilder
    private var summaryCard: some View {
        if let banner = rt.summaryBanner {
            HStack(alignment: .top, spacing: Theme.Spacing.sm + 2) {
                CraneMascot(pose: .joy, size: 52, sway: 6) // #102
                Text(banner.text)
                    .font(.subheadline.weight(.medium))
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
                if let action = banner.action { SummaryBannerActionButton(action: action) } // #209 元に戻す / やり直す
            }
            .metamonCard(padding: Theme.Spacing.md)
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.top, Theme.Spacing.sm)
            .transition(.move(edge: .top).combined(with: .opacity))
            .onTapGesture { withAnimation { rt.summaryBanner = nil } }
            .task(id: banner.id) {
                // 文字数に応じて 3〜5 秒(読み上げを読み切れる長さ)
                // ボタン付き(#209)は押す余裕を持たせて 6 秒以上
                let seconds = max(banner.action == nil ? 3.0 : 6.0, min(5.0, Double(banner.text.count) / 12))
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

    func body(content: Content) -> some View {
        let active = onModal == (rt.sheet != nil || rt.fullScreen != nil)
        content.overlay(alignment: .bottom) {
            if active, let message = rt.toast {
                Text(message)
                    .font(.footnote.weight(.medium))
                    .padding(.horizontal, Theme.Spacing.md).padding(.vertical, Theme.Spacing.sm + 2)
                    .background(.regularMaterial, in: Capsule())
                    .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, Theme.Spacing.md)
                    // マイク FAB と DSL の最下段(行 11〜12 のボタン)の上に出す(被せない)。タブ無しでも同じ高さ
                    .padding(.bottom, Theme.Layout.fabBottomInset(hasTabs: true) + Theme.Layout.fabSize + Theme.Spacing.md)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                    .task(id: message) {
                        try? await Task.sleep(for: .seconds(3))
                        withAnimation { rt.toast = nil }
                    }
            }
        }
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

/// フォーム画面のナビバー右上に置く小さなマイク(FAB の代わり・#83)。タップ = 発話シート、長押し = 変身一覧(FAB と同じ 2 入口)。
private struct VoiceToolbarButton: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        Button { rt.showVoiceSheet = true } label: {
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
                .onEnded { _ in rt.openSwitcher() }
        )
        .accessibilityLabel("話しかけて変える")
        .accessibilityHint("長押しで変身一覧")
        .accessibilityAction(named: "変身一覧") { rt.openSwitcher() }
    }
}

/// 発話シート・変身一覧の present 元。フォームが sheet/fullScreen で出ている間は根からは present できない(二重 present になる)ので、
/// モーダルの中身にも同じ 2 枚を付け、「モーダルが出ているか」で片方だけが実際に開く。
private struct HostSheets: ViewModifier {
    let onModal: Bool
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        let active = onModal == (rt.sheet != nil || rt.fullScreen != nil)
        content
            .sheet(isPresented: Binding(get: { active && rt.showVoiceSheet }, set: { rt.showVoiceSheet = $0 })) {
                VoiceInputView()
                    .presentationDetents([.large])
                    .presentationDragIndicator(.visible)
            }
            .sheet(isPresented: Binding(get: { active && rt.showAppSwitcher }, set: { rt.showAppSwitcher = $0 })) {
                AppSwitcherSheet()
            }
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
