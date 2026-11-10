import SwiftUI

// iOS だけの SwiftUI の修飾子を、Mac では何もしない / Mac の同等品にする(mac-002・mac-004)。
// iOS は元の修飾子をそのまま呼ぶ。

extension ToolbarItemPlacement {
    /// iOS: ナビゲーションバーの右(`.topBarTrailing`)。Mac: ツールバーの主操作。
    static var platformTrailing: ToolbarItemPlacement {
        #if os(iOS)
        .topBarTrailing
        #else
        .primaryAction
        #endif
    }

    /// iOS: ナビゲーションバーの左(`.topBarLeading`)。Mac: ツールバーの先頭。
    static var platformLeading: ToolbarItemPlacement {
        #if os(iOS)
        .topBarLeading
        #else
        .navigation
        #endif
    }
}

extension View {
    /// iOS: `.navigationBarTitleDisplayMode(.inline)`。Mac はタイトルの表示形が 1 つなので何もしない。
    @ViewBuilder
    func inlineNavigationTitle() -> some View {
        #if os(iOS)
        navigationBarTitleDisplayMode(.inline)
        #else
        self
        #endif
    }

    /// iOS: `fullScreenCover`。Mac は全画面の覆いが無いので sheet。
    @ViewBuilder
    func platformFullScreenCover<Content: View>(isPresented: Binding<Bool>, onDismiss: (() -> Void)? = nil,
                                                @ViewBuilder content: @escaping () -> Content) -> some View {
        #if os(iOS)
        fullScreenCover(isPresented: isPresented, onDismiss: onDismiss, content: content)
        #else
        sheet(isPresented: isPresented, onDismiss: onDismiss, content: content)
        #endif
    }

    @ViewBuilder
    func platformFullScreenCover<Item: Identifiable, Content: View>(item: Binding<Item?>, onDismiss: (() -> Void)? = nil,
                                                                    @ViewBuilder content: @escaping (Item) -> Content) -> some View {
        #if os(iOS)
        fullScreenCover(item: item, onDismiss: onDismiss, content: content)
        #else
        sheet(item: item, onDismiss: onDismiss, content: content)
        #endif
    }

    /// iOS: 数値のソフトキーボード(小数あり / 整数)。Mac はハードのキーボードなので何もしない。
    @ViewBuilder
    func numericKeyboard(decimal: Bool) -> some View {
        #if os(iOS)
        keyboardType(decimal ? .decimalPad : .numberPad)
        #else
        self
        #endif
    }

    /// iOS: 長押しのメニューで浮かせる形。Mac は右クリックのメニューで浮かせないので何もしない。
    @ViewBuilder
    func contextMenuPreviewShape<S: Shape>(_ shape: S) -> some View {
        #if os(iOS)
        contentShape(.contextMenuPreview, shape)
        #else
        self
        #endif
    }

    /// iOS: ホイールの picker。Mac は既定(ポップアップ)。
    @ViewBuilder
    func wheelPickerStyle() -> some View {
        #if os(iOS)
        pickerStyle(.wheel)
        #else
        self
        #endif
    }

    /// Mac: 見出しとして VoiceOver に伝える(ローターの「見出し」で飛べる・#937)。iOS は今までどおり何もしない。
    @ViewBuilder
    func macAccessibilityHeading() -> some View {
        #if os(macOS)
        accessibilityAddTraits(.isHeader)
        #else
        self
        #endif
    }
}
