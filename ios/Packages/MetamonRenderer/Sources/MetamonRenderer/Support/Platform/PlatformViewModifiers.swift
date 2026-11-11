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

    /// アイコンだけのボタンの名前(#1335)。VoiceOver の名前を付け、Mac では同じ文言をヘルプのタグ(ホバーの説明)にし、
    /// `hoverHighlight` ならホバー中に背景を薄く敷く(borderless のボタンに近い見た目)。iOS は `accessibilityLabel` だけ。
    func iconButtonLabel(_ label: String, hoverHighlight: Bool = true) -> some View {
        accessibilityLabel(label).macIconButtonHelp(label, hoverHighlight: hoverHighlight)
    }

    /// クリックできる行・カードのヘルプのタグ(ホバーの説明・#1373)。nil(押しても何も起きない)なら付けない。iOS は何もしない。
    @ViewBuilder
    func macHelp(_ label: String?) -> some View {
        #if os(macOS)
        if let label { help(label) } else { self }
        #else
        self
        #endif
    }

    /// `iconButtonLabel` の Mac の部分だけ(名前は `Label` や行の操作が既に持つボタン用)。iOS は何もしない。
    func macIconButtonHelp(_ label: String, hoverHighlight: Bool = true) -> some View {
        modifier(MacIconButtonHelp(label: label, hoverHighlight: hoverHighlight))
    }
}

/// Mac のヘルプのタグとホバーの背景。「視差効果を減らす」の時は背景をアニメーションさせずに切り替える(#937)。
private struct MacIconButtonHelp: ViewModifier {
    let label: String
    let hoverHighlight: Bool
    #if os(macOS)
    @State private var hovering = false
    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    #endif

    func body(content: Content) -> some View {
        #if os(macOS)
        content
            .help(label)
            .background {
                if hoverHighlight {
                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                        .fill(Color.primary.opacity(hovering && isEnabled ? 0.08 : 0))
                        .padding(-4)
                }
            }
            .onHover { inside in
                guard hoverHighlight else { return }
                if reduceMotion { hovering = inside } else { withAnimation(.easeOut(duration: 0.12)) { hovering = inside } }
            }
        #else
        content
        #endif
    }
}
