import Foundation

/// iOS の初期画面の挨拶の出し分け。文言は共有の `Greeting`(Shared/Voice・#1211)。
extension Greeting {
    /// 初期画面でどの挨拶を出すか(`AppRuntime.homeEntry` と起動回数から)。
    static func context(homeEntry: AppRuntime.HomeEntry, isInitial: Bool) -> Context {
        guard isInitial else { return .editing }
        switch homeEntry {
        case .launch: return launchIsReturning ? .returning : .firstVisit
        case .newApp: return .newApp
        case .home: return .home
        }
    }
}
