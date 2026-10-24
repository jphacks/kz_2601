import SwiftUI
import MetamonDSL

/// `app.tabs` があれば TabView(タブ毎に NavigationStack)、無ければ `initialScreen` を根にした単一 NavigationStack。
/// その上にモーダル・音声シート・確認ダイアログ・モーフ中オーバーレイを重ねる。
struct RootView: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        @Bindable var rt = rt
        ZStack {
            navigationRoot
                .id(rt.document.version) // version が変わったら丸ごと作り直して transition を出す
                .transition(.asymmetric(
                    insertion: .scale(scale: 0.94).combined(with: .opacity),
                    removal: .opacity
                ))

            if rt.dsl.app.voiceEntry == .floating {
                voiceFab
            }
            if rt.isMorphing {
                morphingOverlay
            }
        }
        .tint(Color(hex: rt.dsl.app.theme?.primaryColor) ?? .accentColor)
        .preferredColorScheme(colorScheme)
        .sheet(item: $rt.sheet) { route in
            NavigationStack(path: rt.pathBinding("sheet")) {
                ScreenView(route: route)
                    .navigationDestination(for: Route.self) { ScreenView(route: $0) }
            }
        }
        .fullScreenCover(item: $rt.fullScreen) { route in
            NavigationStack(path: rt.pathBinding("full")) {
                ScreenView(route: route)
                    .navigationDestination(for: Route.self) { ScreenView(route: $0) }
            }
        }
        .sheet(isPresented: $rt.showVoiceSheet) {
            VoiceInputView(prompt: "どう変えたいですか？")
                .presentationDetents([.medium, .large])
        }
        .alert(item: $rt.confirm) { confirm in
            Alert(title: Text(confirm.message),
                  primaryButton: .destructive(Text("実行")) { Task { await confirm.perform() } },
                  secondaryButton: .cancel(Text("キャンセル")))
        }
        .overlay(alignment: .bottom) { toast }
    }

    @ViewBuilder
    private var navigationRoot: some View {
        if rt.dsl.app.hasTabs {
            @Bindable var rt = rt
            TabView(selection: $rt.selectedTab) {
                ForEach(rt.dsl.app.tabs) { tab in
                    let stack = AppRuntime.tabStack(tab.screen)
                    NavigationStack(path: rt.pathBinding(stack)) {
                        ScreenView(route: Route(screen: tab.screen, stack: stack))
                            .navigationDestination(for: Route.self) { ScreenView(route: $0) }
                    }
                    .tabItem { Label(tab.title, systemImage: tab.icon ?? "square") }
                    .tag(tab.screen)
                }
            }
        } else {
            NavigationStack(path: rt.pathBinding("main")) {
                ScreenView(route: Route(screen: rt.dsl.initialScreen))
                    .navigationDestination(for: Route.self) { ScreenView(route: $0) }
            }
        }
    }

    private var colorScheme: ColorScheme? {
        switch rt.dsl.app.theme?.mode { case "light": .light; case "dark": .dark; default: nil }
    }

    private var voiceFab: some View {
        VStack {
            Spacer()
            HStack {
                Spacer()
                Button { rt.showVoiceSheet = true } label: {
                    Image(systemName: "mic.fill")
                        .font(.title2)
                        .padding(18)
                        .background(.tint, in: Circle())
                        .foregroundStyle(.white)
                        .shadow(radius: 8, y: 4)
                }
                .padding(.trailing, 20)
                .padding(.bottom, rt.dsl.app.hasTabs ? 70 : 24)
            }
        }
    }

    private var morphingOverlay: some View {
        ZStack {
            Rectangle().fill(.ultraThinMaterial).ignoresSafeArea()
            VStack(spacing: 16) {
                ProgressView().controlSize(.large)
                Text("アプリを生成中…").font(.headline)
            }
        }
        .transition(.opacity)
    }

    @ViewBuilder
    private var toast: some View {
        if let message = rt.toast {
            Text(message)
                .font(.footnote)
                .padding(.horizontal, 16).padding(.vertical, 10)
                .background(.regularMaterial, in: Capsule())
                .padding(.bottom, 100)
                .transition(.move(edge: .bottom).combined(with: .opacity))
                .task(id: message) {
                    try? await Task.sleep(for: .seconds(3))
                    withAnimation { rt.toast = nil }
                }
        }
    }
}
