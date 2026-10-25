import SwiftUI

/// モチーフキャラのツル(#102)。本体の画面だけに出す装飾(生成したアプリの中には出さない)。
/// ふわっと出て軽く揺れる。「視差効果を減らす」では動かさない。VoiceOver からは隠す。
struct CraneMascot: View {
    enum Pose: String {
        case happy, joy, idea, panic, point, tea, pc, phone, bye
        var assetName: String { "crane_\(rawValue)" }
    }

    let pose: Pose
    var size: CGFloat = 88
    /// 揺れの大きさ(度)。0 なら出現アニメだけ
    var sway: Double = 4

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.colorScheme) private var scheme
    @State private var appeared = false
    @State private var swaying = false

    var body: some View {
        Image(pose.assetName)
            .resizable()
            .interpolation(.high)
            .scaledToFit()
            .padding(size * 0.08)
            .frame(width: size, height: size)
            // 線画は黒線なので、ダークでは生成りの円を敷いて見えるようにする
            .background(Circle().fill(backdrop))
            .rotationEffect(.degrees(reduceMotion || sway == 0 ? 0 : (swaying ? sway : -sway)), anchor: .bottom)
            .scaleEffect(appeared ? 1 : 0.6)
            .opacity(appeared ? 1 : 0)
            .accessibilityHidden(true)
            .allowsHitTesting(false)
            .onAppear {
                guard !reduceMotion else { appeared = true; return }
                withAnimation(.spring(duration: 0.5, bounce: 0.35)) { appeared = true }
                guard sway > 0 else { return }
                withAnimation(.easeInOut(duration: 1.6).repeatForever(autoreverses: true).delay(0.5)) { swaying = true }
            }
            .onChange(of: reduceMotion) { _, reduce in
                if reduce { swaying = false; appeared = true }
            }
            .animation(reduceMotion ? nil : .default, value: pose)
    }

    private var backdrop: Color {
        scheme == .dark
            ? Color(red: 0.95, green: 0.93, blue: 0.88).opacity(0.92)
            : Theme.Colors.brandA.opacity(0.06)
    }
}

#Preview {
    HStack {
        CraneMascot(pose: .happy)
        CraneMascot(pose: .bye, size: 64)
    }
}
