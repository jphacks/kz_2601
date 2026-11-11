import SwiftUI
import MetamonServices

/// AI への送信の説明と同意(#246)を Mac のシートで出す(#971)。中身は iOS と同じ `AIConsentContent`。
/// 同意の記録は端末ごと(iOS と同じ `AIConsentStore`・Mac の UserDefaults)。iPhone で同意していても Mac では改めて出す。
struct MacAIConsentSheet: View {
    private let consent = AIConsentStore.shared
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        AIConsentContent(needsReconsent: consent.needsReconsent) {
            consent.grant()
            dismiss()
        } onLater: {
            consent.markPrompted() // 閉じる前に記録(閉じた直後のチュートリアル判定・#274)
            dismiss()
        }
        .interactiveDismissDisabled()
        .onDisappear { consent.markPrompted() }
        .frame(width: 520, height: 640)
        .accessibilityIdentifier("consent.sheet")
    }
}

/// 規約・ポリシーをアプリ内で読む(#971)。同梱の docs/legal を表示し、上から公開ページも開ける。
struct MacLegalDocumentSheet: View {
    let document: LegalDocument
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            LegalDocumentView(document: document)
                .toolbar {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("閉じる") { dismiss() }
                    }
                }
        }
        .frame(minWidth: 560, minHeight: 600)
    }
}
