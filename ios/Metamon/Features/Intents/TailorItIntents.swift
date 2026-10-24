import AppIntents
import MetamonDSL
import MetamonServices

// MARK: - アプリを開く

/// 「Tailor It で家計簿を開く」: 選んだアプリに変身した状態で起動する(変身一覧で選んだのと同じ `switchApp`)。
struct OpenTailorAppIntent: OpenIntent {
    static var title: LocalizedStringResource = "アプリを開く"
    static var description = IntentDescription("Tailor It で作ったアプリに変身した状態で開きます。")
    static var openAppWhenRun = true

    @Parameter(title: "アプリ", requestValueDialog: "どのアプリを開きますか?")
    var target: TailorAppEntity

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
    static var description = IntentDescription("Tailor It で作ったアプリに、アプリを開かずに記録を 1 件追加します。例: 家計簿に「ランチ 800円」。")
    static var openAppWhenRun = false

    @Parameter(title: "記録先", requestValueDialog: "どのアプリに記録しますか?")
    var target: RecordTargetEntity

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
        guard let runtime = IntentHost.shared.runtime else { throw TailorIntentError.notReady }
        guard let doc = await IntentHost.shared.document(appId: target.appId),
              let entity = doc.dsl.entity(target.entityId) else { throw TailorIntentError.targetNotFound(target.title) }

        var number = amount
        var draft = QuickRecord.build(entity: entity, text: content, number: number)
        // 数値項目(金額など)だけが足りなければ Siri で聞き返す
        if let numberField = QuickRecord.numberField(of: entity), draft.missing.contains(numberField.name) {
            number = try await $amount.requestValue(IntentDialog("\(numberField.name)はいくつですか?"))
            draft = QuickRecord.build(entity: entity, text: content, number: number)
        }
        guard draft.missing.isEmpty else { throw TailorIntentError.missingFields(draft.missing) }

        let sent = await runtime.addRecordFromIntent(appId: target.appId, entity: entity.id, data: draft.data)
        let what = draft.summary.isEmpty ? content : draft.summary
        let tail = sent ? "" : "(未送信: 接続が戻ったら自動で送ります)"
        let message = "\(target.appName)に「\(what)」を追加しました\(tail)"
        print("[Metamon] intent: \(message)")
        return .result(dialog: IntentDialog(stringLiteral: message))
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

    var localizedStringResource: LocalizedStringResource {
        switch self {
        case .notReady: "Tailor It を一度開いてから、もう一度お試しください"
        case .targetNotFound(let name): "「\(name)」が見つかりませんでした。Tailor It でアプリを確認してください"
        case .missingFields(let names): "「\(names.joined(separator: "・"))」が必要です。Tailor It を開いて追加してください"
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
