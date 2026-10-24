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

    var body: some View {
        @Bindable var rt = rt
        ZStack {
            Group {
                if rt.dsl.isVoiceOnly {
                    // 初期状態(voiceEntry = fullscreen)はグリッドのセル内ではなく全面に描く(背景を画面いっぱいに敷くため)。
                    // 見出しは DSL の prompt ではなく Greeting(#42: 初回/再訪/時間帯で出し分け)
                    VoiceInputView()
                } else {
                    DSLNavigationRoot()
                }
            }
                .id(rt.document.version) // version が変わったら丸ごと作り直して transition を出す
                .transition(.asymmetric(
                    insertion: .scale(scale: 0.96).combined(with: .opacity),
                    removal: .opacity
                ))
                .opacity(rt.morphJob?.kind == .edit ? 0.45 : 1)
                .blur(radius: rt.morphJob?.kind == .edit ? 1.5 : 0)
                .allowsHitTesting(rt.morphJob == nil)

            if rt.dsl.app.voiceEntry == .floating {
                voiceFab
            }
            if let job = rt.morphJob {
                switch job.kind {
                case .create:
                    GeneratingView(job: job)
                        .transition(.opacity)
                        .zIndex(2)
                case .edit:
                    VStack {
                        EditingIndicator(job: job)
                            .padding(.top, Theme.Spacing.sm)
                        Spacer()
                    }
                    .transition(.move(edge: .top).combined(with: .opacity))
                    .zIndex(2)
                }
            }
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
        }
        .fullScreenCover(item: $rt.fullScreen) { route in
            ScreenStack(root: route)
        }
        .sheet(isPresented: $rt.showVoiceSheet) {
            VoiceInputView()
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: $rt.showAppSwitcher) {
            AppSwitcherSheet()
        }
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
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_SWITCHER=1` で起動 2.5 秒後に変身一覧を開く
            guard ProcessInfo.processInfo.environment["METAMON_AUTO_SWITCHER"] == "1" else { return }
            try? await Task.sleep(for: .seconds(2.5))
            rt.openSwitcher()
        }
        #endif
        .alert(item: $rt.confirm) { confirm in
            Alert(title: Text(confirm.message),
                  primaryButton: .destructive(Text("実行")) { Task { await confirm.perform() } },
                  secondaryButton: .cancel(Text("キャンセル")))
        }
        .overlay(alignment: .top) { summaryCard }
        .overlay(alignment: .bottom) { toast }
        .onChange(of: scenePhase) { _, phase in
            rt.setForeground(phase == .active) // ホットリロードのポーリングは前面時のみ
        }
    }

    private var colorScheme: ColorScheme? {
        switch rt.dsl.app.theme?.mode { case "light": .light; case "dark": .dark; default: nil }
    }

    /// マイク FAB(= Tailor It 本体)。位置は `Theme.Layout.fabAlignment`(左下。DSL の「追加」ボタンが右下に来るため)。
    /// タップ = 「このアプリを変える」発話シート、長押し = メタ操作(変身一覧)。
    private var voiceFab: some View {
        Button { rt.showVoiceSheet = true } label: {
            Image(systemName: "mic.fill")
                .font(.title2.weight(.semibold))
                .foregroundStyle(.white)
                .frame(width: Theme.Layout.fabSize, height: Theme.Layout.fabSize)
                .background(Theme.brandGradient, in: Circle())
                .overlay(Circle().strokeBorder(.white.opacity(0.25)))
                .shadow(color: Theme.Colors.brandA.opacity(0.45), radius: 14, y: 6)
        }
        .simultaneousGesture(
            LongPressGesture(minimumDuration: Theme.Layout.fabLongPressSeconds)
                .onEnded { _ in rt.openSwitcher() }
        )
        .accessibilityLabel("話しかけて変える")
        .accessibilityHint("長押しで変身一覧")
        .accessibilityAction(named: "変身一覧") { rt.openSwitcher() }
        .padding(.horizontal, Theme.Layout.fabHorizontalInset)
        .padding(.bottom, Theme.Layout.fabBottomInset(hasTabs: rt.dsl.app.hasTabs))
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: Theme.Layout.fabAlignment)
    }

    /// 変身直後の要約(読み上げ文)。上部カードで 3 秒表示して自動で消える。
    @ViewBuilder
    private var summaryCard: some View {
        if let banner = rt.summaryBanner {
            HStack(alignment: .top, spacing: Theme.Spacing.sm + 2) {
                Image(systemName: "sparkles")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(Theme.brandGradient)
                    .padding(.top, 1)
                Text(banner.text)
                    .font(.subheadline.weight(.medium))
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
            }
            .metamonCard(padding: Theme.Spacing.md)
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.top, Theme.Spacing.sm)
            .transition(.move(edge: .top).combined(with: .opacity))
            .onTapGesture { withAnimation { rt.summaryBanner = nil } }
            .task(id: banner.id) {
                // 文字数に応じて 3〜5 秒(読み上げを読み切れる長さ)
                let seconds = min(5.0, max(3.0, Double(banner.text.count) / 12))
                try? await Task.sleep(for: .seconds(seconds))
                withAnimation(.easeInOut(duration: 0.35)) { rt.summaryBanner = nil }
            }
        }
    }

    @ViewBuilder
    private var toast: some View {
        if let message = rt.toast {
            Text(message)
                .font(.footnote.weight(.medium))
                .padding(.horizontal, Theme.Spacing.md).padding(.vertical, Theme.Spacing.sm + 2)
                .background(.regularMaterial, in: Capsule())
                .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
                .padding(.bottom, 100)
                .transition(.move(edge: .bottom).combined(with: .opacity))
                .task(id: message) {
                    try? await Task.sleep(for: .seconds(3))
                    withAnimation { rt.toast = nil }
                }
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
