import AppIntents
import MetamonDSL
import MetamonServices

// MARK: - アプリを開く

/// 「Tailor It で家計簿を開く」: 選んだアプリに変身した状態で起動する(変身一覧で選んだのと同じ `switchApp`)。
struct OpenTailorAppIntent: OpenIntent {
    #if APP_STORE_REVIEW // 審査用の版は「作ったアプリ・変身」を避け、記録帳と言う(#247・docs/store/wording.md)。App Intents の文言はビルド時に決まるので #if で替える
    static var title: LocalizedStringResource = "記録帳を開く"
    static var description = IntentDescription("Tailor It で、選んだ記録帳を開きます。")
    #else
    static var title: LocalizedStringResource = "アプリを開く"
    static var description = IntentDescription("Tailor It で作ったアプリに変身した状態で開きます。")
    #endif
    static var openAppWhenRun = true

    #if APP_STORE_REVIEW
    @Parameter(title: "記録帳", requestValueDialog: "どの記録帳を開きますか?")
    var target: TailorAppEntity
    #else
    @Parameter(title: "アプリ", requestValueDialog: "どのアプリを開きますか?")
    var target: TailorAppEntity
    #endif

    @MainActor
    func perform() async throws -> some IntentResult {
        await IntentHost.shared.request(.open(appId: target.id))
        return .result()
    }
}

// MARK: - 記録を追加(アプリを開かない)

/// 「Tailor It で家計簿に記録」→「ランチ 800円」: アプリを開かずにレコードを 1 件追加する。
/// 本体プロセスが背面で起動され、画面と同じローカル DB に書いて SyncQueue に積む(送信は接続があれば即、無ければ次回)。
struct AddRecordIntent: AppIntent {
    static var title: LocalizedStringResource = "記録を追加"
    #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247)
    static var description = IntentDescription("Tailor It を開かずに、記録帳に記録を 1 件追加します。例: 家計簿に「ランチ 800円」。")
    #else
    static var description = IntentDescription("Tailor It で作ったアプリに、アプリを開かずに記録を 1 件追加します。例: 家計簿に「ランチ 800円」。")
    #endif
    static var openAppWhenRun = false

    #if APP_STORE_REVIEW
    @Parameter(title: "記録先", requestValueDialog: "どの記録帳に記録しますか?")
    var target: RecordTargetEntity
    #else
    @Parameter(title: "記録先", requestValueDialog: "どのアプリに記録しますか?")
    var target: RecordTargetEntity
    #endif

    @Parameter(title: "内容", description: "例: ランチ 800円", requestValueDialog: "何を記録しますか?")
    var content: String

    @Parameter(title: "数値", description: "金額・回数など。省略すると内容の中の数字を使います")
    var amount: Double?

    static var parameterSummary: some ParameterSummary {
        Summary("\(\.$target) に \(\.$content) を追加") {
            \.$amount
        }
    }

    @MainActor
    func perform() async throws -> some IntentResult & ProvidesDialog {
        let host = IntentHost.shared // iOS は IntentHost.swift、Mac は MetamonMac/App/Intents/MacIntentHost.swift(#1034)
        guard host.canWriteRecords else { throw TailorIntentError.notReady }
        guard let doc = await host.document(appId: target.appId),
              let entity = doc.dsl.entity(target.entityId) else { throw TailorIntentError.targetNotFound(target.title) }

        // ref 項目(カテゴリ台帳など)は参照先のレコードをローカル DB から読み、見出しで当てる
        var refRecords: [String: [Record]] = [:]
        for refEntity in Set(entity.fields.filter { $0.type == .ref }.compactMap(\.refEntity)) {
            refRecords[refEntity] = await host.cachedRecords(appId: target.appId, entity: refEntity)
        }
        let refChoices = QuickRecord.refChoices(for: entity, in: doc.dsl, records: refRecords)

        var number = amount
        var draft = QuickRecord.build(entity: entity, text: content, number: number, refChoices: refChoices)
        // 数値項目(金額など)だけが足りなければ Siri で聞き返す
        if let numberField = QuickRecord.numberField(of: entity), draft.missing.contains(numberField.name) {
            number = try await $amount.requestValue(IntentDialog("\(numberField.name)はいくつですか?"))
            draft = QuickRecord.build(entity: entity, text: content, number: number, refChoices: refChoices)
        }
        guard draft.missing.isEmpty else { throw TailorIntentError.missingFields(draft.missing) }

        // #233 今のアプリでなくても暗号化する
        let sent = try await host.addRecord(appId: target.appId, entity: entity.id, data: draft.data,
                                            encryptedFields: doc.dsl.encryptedFields)
        let what = draft.summary.isEmpty ? content : draft.summary
        let message = Self.addedMessage(appName: target.appName, what: what, refs: draft.refs, sent: sent)
        print("[Metamon] intent: \(String(localized: message))")
        return .result(dialog: IntentDialog(message))
    }

    /// 返事の文言。Siri の言語で引けるよう書式キーにする(アプリ名・内容・項目名はデータなので訳さない)。
    static func addedMessage(appName: String, what: String, refs: [String], sent: Bool) -> LocalizedStringResource {
        let refs = refs.joined(separator: String(localized: "・")) // どのカテゴリに入れたか
        switch (refs.isEmpty, sent) {
        case (true, true): return "\(appName)に「\(what)」を追加しました"
        case (false, true): return "\(appName)に「\(what)」(\(refs))を追加しました"
        case (true, false): return "\(appName)に「\(what)」を追加しました(未送信: 接続が戻ったら自動で送ります)"
        case (false, false): return "\(appName)に「\(what)」(\(refs))を追加しました(未送信: 接続が戻ったら自動で送ります)"
        }
    }
}

// MARK: - 話しかける

/// 「Tailor It で話しかける」: 音声シートを開いた状態で起動する(初期画面ならそのまま音声画面)。
struct TalkToTailorItIntent: AppIntent {
    static var title: LocalizedStringResource = "話しかける"
    static var description = IntentDescription("Tailor It の音声入力を開きます。")
    static var openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult {
        await IntentHost.shared.request(.talk)
        return .result()
    }
}

// MARK: - エラー

enum TailorIntentError: Error, CustomLocalizedStringResourceConvertible {
    case notReady
    case targetNotFound(String)
    case missingFields([String])
    /// 数字を読む(#1994): アプリか stat が消えた / 記録が端末に無い / 数で取れない
    case statNotFound
    case statNotLoaded(String)
    case statNotNumber(String)

    var localizedStringResource: LocalizedStringResource {
        switch self {
        case .notReady: "Tailor It を一度開いてから、もう一度お試しください"
        #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247)
        case .targetNotFound(let name): "「\(name)」が見つかりませんでした。Tailor It で記録帳を確認してください"
        #else
        case .targetNotFound(let name): "「\(name)」が見つかりませんでした。Tailor It でアプリを確認してください"
        #endif
        case .missingFields(let names): "「\(names.joined(separator: String(localized: "・")))」が必要です。Tailor It を開いて追加してください"
        #if APP_STORE_REVIEW
        case .statNotFound: "その数字が見つかりませんでした。Tailor It で記録帳を確認して、選び直してください"
        #else
        case .statNotFound: "その数字が見つかりませんでした。Tailor It でアプリを確認して、選び直してください"
        #endif
        case .statNotLoaded(let name): "まだ読み込んでいません。Tailor It で\(name)を一度開いてください"
        case .statNotNumber(let label): "「\(label)」は数値で読めません。「数字を読む」を使ってください"
        }
    }
}

// MARK: - Siri のフレーズ

/// Siri / ショートカット / Spotlight に出る App Shortcuts。フレーズは必ずアプリ名(Tailor It)を含む。
/// パラメータ入りのフレーズ(`\(\.$target)`)は `updateAppShortcutParameters()` 時点の候補(作ったアプリ名)で学習される。
struct TailorItShortcuts: AppShortcutsProvider {
    static var shortcutTileColor: ShortcutTileColor = .purple

    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: AddRecordIntent(),
            phrases: [
                "\(.applicationName)で\(\.$target)に記録",
                "\(.applicationName)で\(\.$target)に記録を追加",
                "\(.applicationName)で記録を追加",
                "\(.applicationName)に記録",
            ],
            shortTitle: "記録を追加",
            systemImageName: "plus.circle"
        )
        #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247)。どのフレーズも applicationName を含む
        AppShortcut(
            intent: OpenTailorAppIntent(),
            phrases: [
                "\(.applicationName)で\(\.$target)を開く",
                "\(.applicationName)で\(\.$target)を開いて",
                "\(.applicationName)で記録帳を開く",
            ],
            shortTitle: "記録帳を開く",
            systemImageName: "square.grid.2x2"
        )
        #else
        AppShortcut(
            intent: OpenTailorAppIntent(),
            phrases: [
                "\(.applicationName)で\(\.$target)を開く",
                "\(.applicationName)で\(\.$target)を開いて",
                "\(.applicationName)でアプリを開く",
            ],
            shortTitle: "アプリを開く",
            systemImageName: "square.grid.2x2"
        )
        #endif
        #if os(iOS) // 数字を読む(#1994)は iOS だけ。Mac は #1034 の後で足す
        AppShortcut(
            intent: GetStatValueIntent(),
            phrases: [
                "\(.applicationName)で数字を読む",
                "\(.applicationName)の数字を読む",
            ],
            shortTitle: "数字を読む",
            systemImageName: "number"
        )
        #endif
        AppShortcut(
            intent: TalkToTailorItIntent(),
            phrases: [
                "\(.applicationName)で話しかける",
                "\(.applicationName)に話しかける",
            ],
            shortTitle: "話しかける",
            systemImageName: "mic"
        )
    }
}
