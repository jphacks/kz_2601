import SwiftUI

/// シートを閉じるボタン。形は文字の「閉じる」、位置は右上にそろえる(UX L2)。
/// ナビバーのあるシートは `SheetCloseToolbarItem`、無いシートは見出しの右端に置く。
public struct SheetCloseButton: View {
    private let identifier: String
    private let action: () -> Void

    public init(identifier: String = "sheet.close", action: @escaping () -> Void) {
        self.identifier = identifier
        self.action = action
    }

    public var body: some View {
        Button(action: action) {
            Text(L("閉じる"))
                .lineLimit(1)
                .fixedSize()
                // レイアウトは変えず、見出しの中でも押せる高さを 44pt にする
                .padding(.vertical, Self.touchInset)
                .contentShape(Rectangle())
                .padding(.vertical, -Self.touchInset)
        }
        .accessibilityIdentifier(identifier)
    }

    private static let touchInset: CGFloat = 12
}

/// ナビバーの右上に置く「閉じる」。ほかの右上のボタンより後に書くと右端に並ぶ。
public struct SheetCloseToolbarItem: ToolbarContent {
    private let identifier: String
    private let action: () -> Void

    public init(identifier: String = "sheet.close", action: @escaping () -> Void) {
        self.identifier = identifier
        self.action = action
    }

    public var body: some ToolbarContent {
        ToolbarItem(placement: .platformTrailing) {
            SheetCloseButton(identifier: identifier, action: action)
        }
    }
}
