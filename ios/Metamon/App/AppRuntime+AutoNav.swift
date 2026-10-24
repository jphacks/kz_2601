#if DEBUG
import Foundation
import MetamonDSL
import MetamonRenderer

/// ヘッドレス E2E 用(#113): `METAMON_AUTO_NAV="b,c,a,c:sheet,other"` で、今見えている画面から順に navigate を実行する。
/// `:sheet` / `:fullScreen` で presentation を指定。間隔は `METAMON_AUTO_NAV_INTERVAL`(秒・既定 2)。開始は起動 3 秒後。
extension AppRuntime {
    func autoNavigateIfRequested() {
        let env = ProcessInfo.processInfo.environment
        guard let spec = env["METAMON_AUTO_NAV"], !spec.isEmpty else { return }
        let interval = env["METAMON_AUTO_NAV_INTERVAL"].flatMap(Double.init) ?? 2
        let steps = spec.split(separator: ",").map { $0.split(separator: ":").map(String.init) }
        Task { [weak self] in
            try? await Task.sleep(for: .seconds(3))
            for (i, step) in steps.enumerated() {
                guard let self, let screen = step.first else { return }
                var json: [String: String] = ["type": "navigate", "screen": screen]
                if step.count > 1 { json["presentation"] = step[1] }
                guard let action = try? JSONDecoder().decode(Action.self, from: JSONEncoder().encode(json)) else { return }
                await run(action, ActionContext(route: visibleRoute))
                let stack = visibleRoute.stack
                let line = "auto nav \(i + 1)/\(steps.count): → \(step.joined(separator: ":")) tab=\(selectedTab) stack=\(stack) depth=\(paths[stack]?.count ?? 0) sheet=\(sheet?.screen ?? "-")"
                print("[Metamon] \(line)")
                // stdout はまとめて流れることがあるので、スクショ側が同期できるよう tmp/autonav-step にも書く
                try? line.write(toFile: NSTemporaryDirectory() + "autonav-step", atomically: true, encoding: .utf8)
                try? await Task.sleep(for: .seconds(interval))
            }
        }
    }

    /// 今見えている画面(モーダル > 選択中タブ / main の最上段)。
    private var visibleRoute: Route {
        if let root = fullScreen ?? sheet { return paths[root.stack]?.last ?? root }
        let root = dsl.app.hasTabs ? Route(screen: selectedTab, stack: Route.tabStack(selectedTab)) : Route(screen: dsl.initialScreen)
        return paths[root.stack]?.last ?? root
    }
}
#endif
