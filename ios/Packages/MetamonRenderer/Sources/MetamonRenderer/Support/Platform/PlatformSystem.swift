import SwiftUI
#if canImport(UIKit)
import UIKit
#else
import AppKit
import IOKit.pwr_mgt
#endif

// iOS と Mac の OS の操作の差を吸収する(mac-002)。iOS は今までと同じ API を呼ぶだけ。

#if canImport(UIKit)
public typealias PlatformColor = UIColor
#else
public typealias PlatformColor = NSColor
#endif

extension PlatformColor {
    /// ライト / ダークで色を変える動的な色。
    static func dynamic(light: @escaping () -> PlatformColor, dark: @escaping () -> PlatformColor) -> PlatformColor {
        #if canImport(UIKit)
        UIColor { $0.userInterfaceStyle == .dark ? dark() : light() }
        #else
        NSColor(name: nil) { appearance in
            appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua ? dark() : light()
        }
        #endif
    }
}

extension Color {
    /// iOS: `systemBackground`。Mac: ウィンドウの背景。
    static var platformBackground: Color {
        #if canImport(UIKit)
        Color(.systemBackground)
        #else
        Color(nsColor: .windowBackgroundColor)
        #endif
    }

    /// iOS: `separator`。Mac: `separatorColor`。
    static var platformSeparator: Color {
        #if canImport(UIKit)
        Color(.separator)
        #else
        Color(nsColor: .separatorColor)
        #endif
    }
}

@MainActor
enum PlatformSystem {
    /// 「この端末にはありません」の印(#1340)。iOS は iPhone、Mac はコンピュータの形
    static var notOnDeviceSymbol: String {
        #if canImport(UIKit)
        "iphone.slash"
        #else
        "desktopcomputer.trianglebadge.exclamationmark"
        #endif
    }

    /// OS の設定(このアプリの権限の画面)を開く URL。Mac はシステム設定の「プライバシーとセキュリティ」。
    static var appSettingsURL: URL? {
        #if canImport(UIKit)
        URL(string: UIApplication.openSettingsURLString)
        #else
        URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy")
        #endif
    }

    /// カメラが拒否された時の「設定を開く」(#1043)。Mac はシステム設定のカメラの欄へ直接。
    static var cameraSettingsURL: URL? {
        #if canImport(UIKit)
        appSettingsURL
        #else
        URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Camera")
        #endif
    }

    /// マイクが拒否された時の「設定を開く」(#952)。Mac はシステム設定 > プライバシーとセキュリティ > マイク へ直接。
    static var microphoneSettingsURL: URL? {
        #if canImport(UIKit)
        appSettingsURL
        #else
        URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone")
        #endif
    }

    /// マイクが拒否された時の説明(Mac は「システム設定 > プライバシーとセキュリティ > マイク」)
    static var microphoneDeniedMessage: String {
        #if canImport(UIKit)
        L("設定アプリで Tailor It のマイクへのアクセスを許可してください。")
        #else
        L("システム設定 > プライバシーとセキュリティ > マイク で Tailor It を許可してください。")
        #endif
    }

    /// 権限の「設定を開く」ボタンの文言(Mac は「システム設定を開く」)
    static var openSettingsTitle: String {
        #if canImport(UIKit)
        L("設定を開く")
        #else
        L("システム設定を開く")
        #endif
    }

    /// カメラが拒否された時の説明(Mac は「システム設定」)
    static var cameraDeniedMessage: String {
        #if canImport(UIKit)
        L("設定アプリで Tailor It のカメラへのアクセスを許可してください。")
        #else
        L("システム設定で Tailor It のカメラへのアクセスを許可してください。")
        #endif
    }

    /// 外のアプリ / ブラウザで開く。
    static func open(_ url: URL) {
        #if canImport(UIKit)
        UIApplication.shared.open(url)
        #else
        NSWorkspace.shared.open(url)
        #endif
    }

    /// VoiceOver が動いているか。
    static var isVoiceOverRunning: Bool {
        #if canImport(UIKit)
        UIAccessibility.isVoiceOverRunning
        #else
        NSWorkspace.shared.isVoiceOverEnabled
        #endif
    }

    /// VoiceOver に読み上げさせる。
    static func announce(_ text: String) {
        #if canImport(UIKit)
        UIAccessibility.post(notification: .announcement, argument: text)
        #else
        guard let element = NSApp?.mainWindow ?? NSApp?.keyWindow else { return }
        NSAccessibility.post(element: element, notification: .announcementRequested,
                             userInfo: [.announcement: text, .priority: NSAccessibilityPriorityLevel.high.rawValue])
        #endif
    }

    /// 成功の触覚(iPhone)。Mac は何もしない。
    static func successHaptic() {
        #if canImport(UIKit)
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        #endif
    }

    /// 文字入力のフォーカスを外す(iOS はキーボードを閉じる)。
    static func resignFirstResponder() {
        #if canImport(UIKit)
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
        #else
        NSApp?.keyWindow?.makeFirstResponder(nil)
        #endif
    }

    /// 画面を消さない(iOS は isIdleTimerDisabled・Mac はディスプレイのスリープを止める電源の assertion)。
    static func setIdleTimerDisabled(_ disabled: Bool) {
        #if canImport(UIKit)
        UIApplication.shared.isIdleTimerDisabled = disabled
        #else
        if disabled, macAwakeAssertion == nil {
            var id: IOPMAssertionID = 0
            let r = IOPMAssertionCreateWithName(kIOPMAssertionTypePreventUserIdleDisplaySleep as CFString,
                                                IOPMAssertionLevel(kIOPMAssertionLevelOn), "Tailor It" as CFString, &id)
            if r == kIOReturnSuccess { macAwakeAssertion = id }
        } else if !disabled, let id = macAwakeAssertion {
            IOPMAssertionRelease(id)
            macAwakeAssertion = nil
        }
        #endif
    }

    #if !canImport(UIKit)
    private static var macAwakeAssertion: IOPMAssertionID?
    #endif
}
