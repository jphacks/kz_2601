import Foundation

/// 位置情報を断られた時の案内と、許可を変える設定の開き先。
/// iOS は「設定」アプリのこのアプリの欄、Mac は「システム設定 > プライバシーとセキュリティ > 位置情報サービス」(#956)。
enum LocationPermissionCopy {
    /// 「設定を開く」の行き先
    static var settingsURL: URL? {
        #if os(macOS)
        URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_LocationServices")
        #else
        URL(string: "app-settings:")
        #endif
    }

    static var notAllowed: String {
        #if os(macOS)
        L("位置情報の利用が許可されていません。「システム設定 > プライバシーとセキュリティ > 位置情報サービス」で許可してください")
        #else
        L("位置情報の利用が許可されていません")
        #endif
    }

    static var cannotGuide: String {
        #if os(macOS)
        L("位置情報がオフのため案内できません。「システム設定 > プライバシーとセキュリティ > 位置情報サービス」でこのアプリを許可してください")
        #else
        L("位置情報がオフのため案内できません。設定で「このAppの使用中のみ許可」を選んでください")
        #endif
    }

    static var cannotRecord: String {
        #if os(macOS)
        L("位置情報がオフのため記録できません。「システム設定 > プライバシーとセキュリティ > 位置情報サービス」でこのアプリを許可してください")
        #else
        L("位置情報がオフのため記録できません。設定で「このAppの使用中のみ許可」を選んでください")
        #endif
    }

    static var cannotSearchNearby: String {
        #if os(macOS)
        L("位置情報が許可されていないので近くを探せません。「システム設定 > プライバシーとセキュリティ > 位置情報サービス」でこのアプリを許可するか、探す語を入れてください")
        #else
        L("位置情報が許可されていないので近くを探せません。設定で「このAppの使用中のみ許可」を選ぶか、探す語を入れてください")
        #endif
    }
}
