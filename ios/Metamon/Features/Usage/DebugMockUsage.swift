#if DEBUG || METAMON_STG
import Foundation
import MetamonServices

/// 手元の確認用の DEBUG・stg フック(stg の版は開発用メニューでも・#2080) `METAMON_MOCK_USAGE=apps:9/10,records:4200/5000,photos:10/500[,pro][,participant]`。
/// サーバー無しで「使っている量」とお知らせを出す。
enum DebugMockUsage {
    static var spec: String? { DebugPlanKnobs.string(DebugPlanKnobs.mockUsage).flatMap { $0.isEmpty ? nil : $0 } }
    static var source: (any PlanUsageSource)? { spec.map(Source.init) }

    struct Source: PlanUsageSource {
        var meters: [String: UsageMeter] = [:]
        var pro = false
        var participant = false

        init(_ spec: String) {
            for part in spec.split(separator: ",") {
                let kv = part.split(separator: ":")
                if kv.count == 1 {
                    if kv[0] == "pro" { pro = true }
                    if kv[0] == "participant" { participant = true }
                    continue
                }
                let nums = kv[1].split(separator: "/").compactMap { Int($0) }
                guard nums.count == 2 else { continue }
                meters[String(kv[0])] = UsageMeter(used: nums[0], limit: nums[1])
            }
        }

        func myUsage() async throws -> MyUsage? {
            MyUsage(plan: pro ? "pro" : "free", apps: meters["apps"])
        }

        func appUsage(appId: String) async throws -> AppUsage? {
            AppUsage(appId: appId, plan: pro ? "pro" : "free", payer: participant ? "creator" : "self", upgrade: !pro,
                     records: meters["records"], photos: meters["photos"])
        }
    }
}
#endif
