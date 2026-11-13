import Foundation

/// 断られた権限の案内から開く OS の設定の欄(#2257)。
/// Mac はシステム設定の「プライバシーとセキュリティ」のその権限の欄(通知はシステム設定の「通知」のこのアプリ)へ直接。
/// iOS はどの権限も今までどおり「設定」アプリのこのアプリの画面(`UIApplication.openSettingsURLString`)。
public enum PrivacySettingsPane: String, CaseIterable, Sendable {
    case camera
    case microphone
    case location
    case photos
    case notifications
    case speechRecognition
    case calendars

    public enum Platform: Sendable {
        case iOS
        case macOS

        public static var current: Platform {
            #if os(macOS)
            .macOS
            #else
            .iOS
            #endif
        }
    }

    /// この OS の開き先
    public var url: URL { url(on: .current) }

    /// OS ごとの開き先(テストで両方を確かめられるように OS を引数に取る)
    public func url(on platform: Platform, bundleIdentifier: String? = Bundle.main.bundleIdentifier) -> URL {
        switch platform {
        case .iOS:
            return URL(string: "app-settings:")! // UIApplication.openSettingsURLString と同じ値
        case .macOS:
            if self == .notifications {
                return URL(string: "x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=\(bundleIdentifier ?? "")")!
            }
            return URL(string: "x-apple.systempreferences:com.apple.preference.security?\(macPrivacyAnchor)")!
        }
    }

    /// システム設定の「プライバシーとセキュリティ」の欄の名前
    var macPrivacyAnchor: String {
        switch self {
        case .camera: "Privacy_Camera"
        case .microphone: "Privacy_Microphone"
        case .location: "Privacy_LocationServices"
        case .photos: "Privacy_Photos"
        case .notifications: "Privacy"
        case .speechRecognition: "Privacy_SpeechRecognition"
        case .calendars: "Privacy_Calendars"
        }
    }
}
