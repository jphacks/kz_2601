#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif

/// クリップボードの読み書き(#1332)。iOS は `UIPasteboard`、Mac は `NSPasteboard`。
/// 引数のペーストボードはテスト用(Mac は名前付きを渡し、利用者のクリップボードを汚さない)。
@MainActor
public enum PlatformPasteboard {
    #if canImport(UIKit)
    public typealias Board = UIPasteboard
    #else
    public typealias Board = NSPasteboard
    #endif

    /// 文字を書く(前の中身は置き換える)。
    public static func setString(_ text: String, on board: Board = .general) {
        #if canImport(UIKit)
        board.string = text
        #else
        board.clearContents()
        board.setString(text, forType: .string)
        #endif
    }

    /// 入っている文字。無ければ nil。
    public static func string(on board: Board = .general) -> String? {
        #if canImport(UIKit)
        board.string
        #else
        board.string(forType: .string)
        #endif
    }
}
