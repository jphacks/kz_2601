import SwiftUI

/// DSL の navigation 定義から Tab / Stack を組み、モーダル・音声シート・モーフ中オーバーレイを重ねる。
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

            if !rt.document.isVoiceOnly {
                voiceFab
            }
            if rt.isMorphing {
                morphingOverlay
            }
        }
        .tint(Color(hex: rt.document.theme?.accent) ?? .accentColor)
        .sheet(item: $rt.sheet) { route in
            NavigationStack { ScreenView(route: route) }
        }
        .fullScreenCover(item: $rt.fullScreen) { route in
            NavigationStack { ScreenView(route: route) }
        }
        .sheet(isPresented: $rt.showVoiceSheet) {
            VoiceInputView(prompt: "どう変えたいですか？")
                .presentationDetents([.medium, .large])
        }
        .overlay(alignment: .bottom) { toast }
    }

    @ViewBuilder
    private var navigationRoot: some View {
        switch rt.document.navigation.type {
        case .tab:
            @Bindable var rt = rt
            TabView(selection: $rt.selectedTab) {
                ForEach(rt.document.navigation.tabs) { tab in
                    NavigationStack(path: rt.pathBinding(tab.id)) {
                        ScreenView(route: Route(screen: tab.screen))
                            .navigationDestination(for: Route.self) { ScreenView(route: $0) }
                    }
                    .tabItem { Label(tab.title, systemImage: tab.icon ?? "square") }
                    .tag(tab.id)
                }
            }
        case .stack:
            NavigationStack(path: rt.pathBinding("main")) {
                ScreenView(route: Route(screen: rt.document.rootScreenId))
                    .navigationDestination(for: Route.self) { ScreenView(route: $0) }
            }
        }
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
                .padding(.bottom, rt.document.navigation.type == .tab ? 70 : 24)
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
