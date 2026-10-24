import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 画面ツリーの根。`DSLNavigationRoot`(Renderer: タブ or 単一スタック)の上に
/// モーダル・音声シート・確認ダイアログ・モーフ中オーバーレイ・トーストを重ねる。
struct RootView: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        @Bindable var rt = rt
        ZStack {
            DSLNavigationRoot()
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
            ScreenStack(root: route)
        }
        .fullScreenCover(item: $rt.fullScreen) { route in
            ScreenStack(root: route)
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

#Preview("家計簿サンプル") {
    let services = BuildConfig.make(environment: .mock)
    let rt = AppRuntime(document: (try? DSLExamples.document("kakeibo")) ?? .fallback, services: services)
    return RootView()
        .environment(rt)
        .environment(\.dslRuntime, rt)
        .environment(\.services, services)
        .environment(\.componentRegistry, MetamonApp.componentRegistry)
}
