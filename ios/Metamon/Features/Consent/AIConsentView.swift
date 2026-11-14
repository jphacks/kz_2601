import SwiftUI

/// 第三者 AI(Anthropic の Claude)へ送る前の説明と同意(#246・App Store 5.1.2(i))。
/// 初回起動時と、同意前に生成・対話しようとした時に出す。「あとで」でも閲覧・記録などは使える。
/// 中身は `AIConsentContent`(ios/Shared・Mac と共有・#971)。
struct AIConsentView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        AIConsentContent(needsReconsent: rt.aiConsent?.needsReconsent == true) {
            rt.grantAIConsent()
            dismiss()
        } onLater: {
            rt.aiConsent?.markPrompted() // 閉じる前に記録(閉じた直後のチュートリアル判定・#274)
            dismiss()
        }
        .interactiveDismissDisabled()
        .onDisappear { rt.aiConsent?.markPrompted() } // 閉じたあとのチュートリアル(#274)は TutorialHost
        .accessibilityIdentifier("consent.sheet")
    }
}
