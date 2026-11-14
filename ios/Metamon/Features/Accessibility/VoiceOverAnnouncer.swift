import SwiftUI
import UIKit

/// VoiceOver への読み上げ(H8)。AI の返事・完成の要約・失敗の状態カード・トーストを、画面に出したのと同時に伝える。
/// VoiceOver が無効なら OS 側で何も起きない。読み上げの設定(#43・`SpeechReader`)とは別物。
@MainActor
enum VoiceOverAnnouncer {
    /// 実際に読み上げる関数。テストで差し替えて、呼ばれた文を確かめる。
    static var post: (String) -> Void = { AccessibilityNotification.Announcement($0).post() }
    /// VoiceOver が有効か。テストで差し替える。
    static var isVoiceOverRunning: () -> Bool = { UIAccessibility.isVoiceOverRunning }

    static func announce(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        post(trimmed)
    }

    /// 自動で消えるまでの秒数。VoiceOver の時は読み上げを聞いてから探して押せるよう長くする。
    static func holdSeconds(_ base: Double) -> Double {
        isVoiceOverRunning() ? max(base * 3, 15) : base
    }
}
