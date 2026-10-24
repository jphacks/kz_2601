import UIKit
import UserNotifications

enum NativeError: LocalizedError {
    case badArgs(String)
    case notImplemented(String)
    case unknown(String)
    case denied(String)

    var errorDescription: String? {
        switch self {
        case .badArgs(let f): return "\(f): 引数が不正です"
        case .notImplemented(let f): return "\(f) は未実装です"
        case .unknown(let f): return "未知のネイティブ機能: \(f)"
        case .denied(let f): return "\(f) の権限がありません"
        }
    }
}

/// P0: camera(コンポーネント側で実装) / notification。P1: location / motion / share。
@MainActor
final class UIKitNativeBridge: NativeBridge {
    func perform(_ feature: String, args: JSONValue) async throws -> JSONValue {
        switch feature {
        case "haptic":
            let style = args[path: "style"]?.stringValue ?? "medium"
            let generator = UIImpactFeedbackGenerator(style: style == "heavy" ? .heavy : style == "light" ? .light : .medium)
            generator.impactOccurred()
            return .null

        case "openURL":
            guard let s = args[path: "url"]?.stringValue, let url = URL(string: s) else { throw NativeError.badArgs(feature) }
            return .bool(await UIApplication.shared.open(url))

        case "clipboard.copy":
            UIPasteboard.general.string = args[path: "text"]?.displayString ?? ""
            return .null

        case "notification.requestPermission":
            let granted = try await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge])
            return .bool(granted)

        case "notification.schedule":
            // args: { title, body, inSeconds } → ローカル通知
            let center = UNUserNotificationCenter.current()
            let settings = await center.notificationSettings()
            if settings.authorizationStatus == .notDetermined {
                _ = try await center.requestAuthorization(options: [.alert, .sound, .badge])
            }
            let content = UNMutableNotificationContent()
            content.title = args[path: "title"]?.displayString ?? "Morph"
            content.body = args[path: "body"]?.displayString ?? ""
            content.sound = .default
            let seconds = max(1, args[path: "inSeconds"]?.doubleValue ?? 5)
            let trigger = UNTimeIntervalNotificationTrigger(timeInterval: seconds, repeats: false)
            let id = UUID().uuidString
            try await center.add(UNNotificationRequest(identifier: id, content: content, trigger: trigger))
            return .string(id)

        case "location.current", "motion.read", "share":
            // TODO(P1): CoreLocation 1回取得 / CoreMotion / UIActivityViewController
            throw NativeError.notImplemented(feature)

        default:
            throw NativeError.unknown(feature)
        }
    }
}
