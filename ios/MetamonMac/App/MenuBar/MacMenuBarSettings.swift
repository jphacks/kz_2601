import Foundation
import Observation
import ServiceManagement

/// メニューバーの常駐(#1848)の設定。どちらも既定は OFF。
struct MacMenuBarSettings {
    static let enabledKey = "metamon.mac.menuBar.enabled"
    static let appIdKey = "metamon.mac.menuBar.appId"

    var defaults: UserDefaults = .standard

    /// メニューバーにアイコンを出すか(`MacStatusItem`)
    var isEnabled: Bool {
        get { defaults.bool(forKey: Self.enabledKey) }
        nonmutating set { defaults.set(newValue, forKey: Self.enabledKey) }
    }

    /// メニューバーで選んだアプリ。nil なら一覧の先頭
    var selectedAppId: String? {
        get { defaults.string(forKey: Self.appIdKey) }
        nonmutating set {
            if let newValue { defaults.set(newValue, forKey: Self.appIdKey) } else { defaults.removeObject(forKey: Self.appIdKey) }
        }
    }
}

/// メニューバーのアイコン。#985(タイマーの表示)もこのアイコンに相乗りし、メニューバーに 2 つ出さない
enum MacMenuBarIcon {
    static let systemImage = "square.grid.2x2"
}

// MARK: - ログイン時に開く

/// ログイン項目の口(テストでは偽物に替える)
@MainActor
protocol MacLoginItemService {
    var status: SMAppService.Status { get }
    func register() throws
    func unregister() throws
}

/// 本物: このアプリそのもの(`SMAppService.mainApp`)
struct MacMainAppLoginItem: MacLoginItemService {
    var status: SMAppService.Status { SMAppService.mainApp.status }
    func register() throws { try SMAppService.mainApp.register() }
    func unregister() throws { try SMAppService.mainApp.unregister() }
}

/// 設定の「ログイン時に開く」。自分からは登録しない(既定 OFF)。状態は毎回 OS に聞く(システム設定で外されても合わせる)
@MainActor
@Observable
final class MacLoginItem {
    static let shared = MacLoginItem()

    @ObservationIgnored private let service: any MacLoginItemService
    private(set) var isEnabled: Bool
    /// システム設定の「ログイン項目」で許可待ち
    private(set) var needsApproval: Bool
    private(set) var failed = false

    init(service: (any MacLoginItemService)? = nil) {
        let service = service ?? MacMainAppLoginItem()
        self.service = service
        isEnabled = service.status == .enabled
        needsApproval = service.status == .requiresApproval
    }

    func setEnabled(_ on: Bool) {
        do {
            if on { try service.register() } else { try service.unregister() }
            failed = false
        } catch {
            print("[Metamon] login item: \(on ? "register" : "unregister") failed \(error)")
            failed = true
        }
        refresh()
    }

    func refresh() {
        isEnabled = service.status == .enabled
        needsApproval = service.status == .requiresApproval
    }
}
