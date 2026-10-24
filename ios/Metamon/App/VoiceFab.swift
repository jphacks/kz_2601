import SwiftUI

/// マイク FAB(= Tailor It 本体)。位置は `Theme.Layout.fabAlignment`(左下。DSL の「追加」ボタンが右下に来るため)。
/// タップ = 「このアプリを変える」発話シート、長押し = メタ操作(変身一覧)。
/// 最下段の左端に DSL の部品がある画面では、その上端まで退避する(#83・`GridBottomObstructionKey`)。
struct VoiceFab: View {
    let hasTabs: Bool
    /// 最下段 A〜B 列に掛かる部品の上端(global y)。無ければ nil
    let obstructionTop: CGFloat?
    /// 生成中は画面と一緒に沈めて触れなくする
    let isBusy: Bool
    let onTap: () -> Void
    let onLongPress: () -> Void

    var body: some View {
        GeometryReader { geo in
            let inset = Theme.Layout.fabBottomInset(
                hasTabs: hasTabs,
                obstructionTop: obstructionTop,
                rootBottom: geo.frame(in: .global).maxY
            )
            Button(action: onTap) {
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
                    .onEnded { _ in onLongPress() }
            )
            .accessibilityLabel("話しかけて変える")
            .accessibilityHint("長押しで変身一覧")
            .accessibilityAction(named: "変身一覧") { onLongPress() }
            // 生成中は画面と一緒に沈めて触れなくする(本体だけ薄くなり FAB だけ浮いて見えるのを防ぐ)
            .opacity(isBusy ? 0.35 : 1)
            .allowsHitTesting(!isBusy)
            .animation(.easeInOut(duration: 0.3), value: isBusy)
            .padding(.horizontal, Theme.Layout.fabHorizontalInset)
            .padding(.bottom, inset)
            .animation(.easeOut(duration: 0.25), value: inset)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: Theme.Layout.fabAlignment)
        }
    }
}
