import AppIntents

// コントロール(iOS 18・#1993)を押した時の動き。本体と MetamonWidget の両方にコンパイルする(project.yml)。
// 両方にあると、押した時に OS が本体を開いて本体のプロセスで perform する(ウィジェットでは動かない)。
// 中身は本体の `ControlIntentBridge`(IntentHost 経由で「開く」と同じ経路)。ウィジェット側の Bridge は何もしない。
// 選んだアプリを消した後でも落ちないよう、Entity でなく id の文字列で持つ(無ければ本体が一覧を開く)。

/// 選んだ生成アプリを開く。未選択・消したアプリなら一覧を開く
struct OpenAppFromControlIntent: AppIntent {
    #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247・docs/store/wording.md)
    static var title: LocalizedStringResource = "記録帳を開く"
    #else
    static var title: LocalizedStringResource = "アプリを開く"
    #endif
    static var openAppWhenRun = true
    static var isDiscoverable = false // ショートカットの一覧には出さない(出すのは OpenTailorAppIntent)

    #if APP_STORE_REVIEW
    @Parameter(title: "記録帳")
    var appId: String?
    #else
    @Parameter(title: "アプリ")
    var appId: String?
    #endif

    init() {}

    init(appId: String?) {
        self.appId = appId
    }

    @MainActor
    func perform() async throws -> some IntentResult {
        await ControlIntentBridge.open(appId: appId, entityId: nil)
        return .result()
    }
}

/// 選んだ記録先のアプリを開き、その記録の入力画面を出す(記録そのものは画面で入れる)
struct OpenRecordFromControlIntent: AppIntent {
    static var title: LocalizedStringResource = "記録の画面を開く"
    static var openAppWhenRun = true
    static var isDiscoverable = false

    @Parameter(title: "記録先のアプリ")
    var appId: String?

    @Parameter(title: "記録の種類")
    var entityId: String?

    init() {}

    init(appId: String?, entityId: String?) {
        self.appId = appId
        self.entityId = entityId
    }

    @MainActor
    func perform() async throws -> some IntentResult {
        await ControlIntentBridge.open(appId: appId, entityId: entityId)
        return .result()
    }
}
