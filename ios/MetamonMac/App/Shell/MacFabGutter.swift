import SwiftUI
import MetamonRenderer

/// 生成したアプリの FAB を中央の列の外の余白に置く試作(#2327・既定 off の `MacFabExperiment`)。
/// 押すとツールバーの「声で頼む」と同じ会話を開く(`openVoice`)。余白が無い窓ではグリッドの右下に戻し、
/// 最下段の右端の部品の上へ逃がす(#83)・`fabFrame` を渡して一覧の末尾に余白を足す(#157)。
struct MacFabGutter: ViewModifier {
    let runtime: MacAppRuntime
    @State private var columnWidth: CGFloat?
    @State private var obstructions: [String: CGFloat] = [:]
    /// 余白に収まらない時だけ渡す(余白にある時は部品に重ならないので末尾の余白は要らない)
    @State private var overlapFrame: CGRect?

    func body(content: Content) -> some View {
        if MacFabExperiment.isEnabled() {
            content
                .environment(\.fabFrame, overlapFrame)
                .onPreferenceChange(GridColumnWidthKey.self) { columnWidth = $0 }
                .onPreferenceChange(GridBottomTrailingObstructionKey.self) { obstructions = $0 }
                .overlay { fab }
        } else {
            content
        }
    }

    private var fab: some View {
        GeometryReader { geo in
            let global = geo.frame(in: .global)
            let routeId = NavigationResolver.top(runtime.navigationState, dsl: runtime.dsl).id
            let placement = MacFabPlacement.placement(container: geo.size, columnWidth: columnWidth,
                                                      obstructionTop: obstructions[routeId].map { $0 - global.minY },
                                                      inset: RenderTheme.Spacing.screenInset)
            Button { runtime.openVoice() } label: {
                Image(systemName: "mic.fill")
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(.white)
                    .frame(width: MacFabPlacement.size, height: MacFabPlacement.size)
                    .background(Theme.Colors.brandA, in: Circle())
                    .shadow(color: .black.opacity(0.2), radius: 6, y: 3)
            }
            .buttonStyle(.plain)
            .help("声でこのアプリを直す")
            .accessibilityLabel(Text("声で頼む"))
            .accessibilityIdentifier("macFab.voice")
            .position(x: placement.frame.midX, y: placement.frame.midY)
            .onChange(of: placement, initial: true) { _, now in
                overlapFrame = now.inGutter ? nil : now.frame.offsetBy(dx: global.minX, dy: global.minY)
            }
        }
    }
}
