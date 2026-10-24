import SwiftUI
import UIKit
import MetamonDSL

/// ソフトキーボードの表示状態(#143)。ホストはマイク FAB を隠すのに使う。
@MainActor
@Observable
public final class KeyboardState {
    public static let shared = KeyboardState()
    public private(set) var isVisible = false

    private init() {
        let center = NotificationCenter.default
        center.addObserver(forName: UIResponder.keyboardWillShowNotification, object: nil, queue: .main) { [weak self] note in
            let height = (note.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect)?.height ?? 0
            MainActor.assumeIsolated { self?.isVisible = KeyboardState.counts(height: height) }
        }
        center.addObserver(forName: UIResponder.keyboardWillHideNotification, object: nil, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.isVisible = false }
        }
    }

    /// ハードウェアキーボード接続時の候補バーだけ(〜70pt)は「出ていない」扱い。
    nonisolated static func counts(height: CGFloat) -> Bool { height > 120 }

    /// 今のファーストレスポンダを外してキーボードを閉じる(ツールバーの「完了」・入力欄外タップ)。
    public static func dismiss() {
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }
}

/// マイク FAB を出すか(#83 フォーム画面 / #143 キーボード表示中は隠す)。
public enum FabVisibility {
    public static func isShown(floating: Bool, formVisible: Bool, keyboardVisible: Bool) -> Bool {
        floating && !formVisible && !keyboardVisible
    }
}

/// グリッドの行高さの基準(#143)。キーボードで可視領域が縮んでもセルを潰さず、外側の ScrollView に逃がす。
/// `stable` = キーボードの領域を含めた高さ(未計測は 0)。
enum GridLayoutHeight {
    static func resolve(current: CGFloat, stable: CGFloat) -> CGFloat {
        max(current, stable)
    }
}

extension View {
    /// DEBUG: `METAMON_AUTO_FOCUS=<部品 id か field>` で表示 3 秒後にその入力欄へフォーカス(キーボードの E2E 用・#143)。
    @ViewBuilder
    func debugAutoFocus(_ c: Component, _ focused: FocusState<Bool>.Binding) -> some View {
        #if DEBUG
        task {
            guard let key = ProcessInfo.processInfo.environment["METAMON_AUTO_FOCUS"], !key.isEmpty,
                  key == c.id || key == c.field else { return }
            try? await Task.sleep(for: .seconds(3))
            focused.wrappedValue = true
            // `METAMON_AUTO_BLUR=<秒>` でさらに n 秒後に閉じる(閉じた後に余白が残らないかの確認)
            guard let s = ProcessInfo.processInfo.environment["METAMON_AUTO_BLUR"].flatMap(Double.init) else { return }
            try? await Task.sleep(for: .seconds(s))
            focused.wrappedValue = false
        }
        #else
        self
        #endif
    }
}
