import SwiftUI
import AppKit

/// Mac の VoiceOver への読み上げ(#937)。iOS の `VoiceOverAnnouncer`(H8)と同じ決まり:
/// 画面に一言を出したのと同時に読み上げ、VoiceOver の時は自動で消えるまでを長くする(3 倍・最低 15 秒)。
/// VoiceOver が無効なら OS 側で何も起きない。
@MainActor
enum MacVoiceOver {
    /// 実際に読み上げる関数。テストで差し替えて、呼ばれた文を確かめる。
    static var post: (String) -> Void = { text in
        // 前面のウインドウから送る(要素が無いと VoiceOver が拾わない)。シートの間も親のウインドウで届く
        guard let element = NSApp?.keyWindow ?? NSApp?.mainWindow else {
            AccessibilityNotification.Announcement(text).post()
            return
        }
        NSAccessibility.post(element: element, notification: .announcementRequested,
                             userInfo: [.announcement: text, .priority: NSAccessibilityPriorityLevel.high.rawValue])
    }

    /// VoiceOver が有効か。テストで差し替える。
    static var isVoiceOverRunning: () -> Bool = { NSWorkspace.shared.isVoiceOverEnabled }

    static func announce(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        post(trimmed)
    }

    static func announce(_ event: MacAnnouncement) {
        announce(event.text)
    }

    /// 自動で消えるまでの秒数。VoiceOver の時は読み上げを聞いてから探して押せるよう長くする。
    static func holdSeconds(_ base: Double) -> Double {
        isVoiceOverRunning() ? max(base * 3, 15) : base
    }
}

/// 殻(サイドバー・会話・中央の列)が VoiceOver に伝える出来事と、その文(#937)。
/// 文はここに集め、同じ出来事は iOS と同じ文で読む(保存・削除・失敗・完成)。
enum MacAnnouncement: Equatable {
    /// 記録を保存した(保存ボタン・画面を閉じる前に伝える)
    case saved
    /// 一覧の記録を消した(「元に戻す」が出ている間)
    case recordDeleted
    /// アプリを作った(複製など)
    case appCreated(name: String)
    /// アプリを消した
    case appDeleted(name: String)
    /// 同梱の例を元に戻した
    case appRestored(name: String)
    /// 会話から作り始めた
    case building
    /// できあがった・直した(AI の要約。無ければ iOS と同じ定型文)
    case built(summary: String?, appName: String, created: Bool)
    /// 失敗の状態カード(`ErrorPresentation.announcement`)
    case failure(String)
    /// 一言(toast)
    case message(String)

    var text: String {
        switch self {
        case .saved: L10n.tr("保存しました")
        case .recordDeleted: L10n.tr("削除しました")
        case .appCreated(let name): L10n.tr("「\(name)」を作りました")
        case .appDeleted(let name): L10n.tr("「\(name)」を削除しました")
        case .appRestored(let name): L10n.tr("「\(name)」を元の例に戻しました")
        case .building: L10n.tr("作っています")
        case .built(let summary, let appName, let created):
            if let summary, !summary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { summary }
            else { ErrorCopy.completedAnnouncement(appName: appName, created: created) }
        case .failure(let text), .message(let text): text
        }
    }
}
