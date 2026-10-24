#if DEBUG
import AppIntents

/// ヘッドレス E2E 用(シミュレータはショートカットアプリのタップ自動化が無い)。
/// `METAMON_E2E_INTENT` で起動直後(bootstrap を待たずに)Intent を本物の型・Query 経由で実行する:
/// - `add:<記録先の名前>:<内容>`  例 `add:家計簿:ランチ 800円`
/// - `open:<アプリ名>` / `talk`
enum IntentE2E {
    @MainActor
    static func runIfRequested() {
        guard let spec = ProcessInfo.processInfo.environment["METAMON_E2E_INTENT"], !spec.isEmpty else { return }
        let parts = spec.split(separator: ":", maxSplits: 2).map(String.init)
        Task { @MainActor in
            try? await Task.sleep(for: .seconds(1.5))
            do {
                switch parts[0] {
                case "add" where parts.count == 3:
                    guard let target = try await RecordTargetQuery().entities(matching: parts[1]).first else {
                        print("[Metamon] E2E intent: no record target for \(parts[1])"); return
                    }
                    var intent = AddRecordIntent()
                    intent.target = target
                    intent.content = parts[2]
                    _ = try await intent.perform()
                case "open" where parts.count == 2:
                    guard let target = try await TailorAppQuery().entities(matching: parts[1]).first else {
                        print("[Metamon] E2E intent: no app for \(parts[1])"); return
                    }
                    var intent = OpenTailorAppIntent()
                    intent.target = target
                    _ = try await intent.perform()
                case "talk":
                    _ = try await TalkToTailorItIntent().perform()
                default:
                    print("[Metamon] E2E intent: bad spec \(spec)")
                    return
                }
                print("[Metamon] E2E intent: \(parts[0]) performed")
            } catch {
                print("[Metamon] E2E intent: \(parts[0]) failed: \(error)")
            }
        }
    }
}
#endif
