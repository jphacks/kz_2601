import SwiftUI
import AppKit

/// Mac の VoiceOver への読み上げ(#937)。iOS の `VoiceOverAnnouncer`(H8)と同じ決まり:
/// 画面に一言を出したのと同時に読み上げ、VoiceOver の時は自動で消えるまでを長くする(3 倍・最低 15 秒)。
@MainActor
enum MacVoiceOver {
    static func announce(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        AccessibilityNotification.Announcement(trimmed).post()
    }

    static func holdSeconds(_ base: Double) -> Double {
        NSWorkspace.shared.isVoiceOverEnabled ? max(base * 3, 15) : base
    }
}
