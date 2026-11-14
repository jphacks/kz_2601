/// ウィジェット側の口(#1993)。コントロールの Intent は本体のプロセスで動くので、ここには来ない。
/// 本体側は Metamon/Features/Intents/ControlIntentBridge.swift
enum ControlIntentBridge {
    @MainActor
    static func open(appId: String?, entityId: String?) async {}
}
