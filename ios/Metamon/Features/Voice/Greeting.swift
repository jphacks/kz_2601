import Foundation

/// 音声画面の問いかけ文言(#42)。見出し・サブ文言はここにだけ置き、多言語化(#44)ではこのファイルを差し替える。
/// 時間帯の挨拶(おはよう/こんにちは/こんばんは)を前置きし、初回・再訪・新規作成・既存アプリの改変で出し分ける。
enum Greeting {
    enum Context: Equatable {
        /// 初回起動(まだ何も作っていない)
        case firstVisit
        /// 2 回目以降の起動で初期画面
        case returning
        /// 変身一覧の「+ 新しいアプリ」/ 発話「別のアプリ」
        case newApp
        /// 変身一覧の「Tailor It に戻る」/ アプリのリセット
        case home
        /// 既にアプリがある(FAB からの音声シート)
        case editing
    }

    struct Copy: Equatable {
        var title: String
        var subtitle: String
    }

    // MARK: 文言

    static func copy(for context: Context, date: Date = .now, calendar: Calendar = .current) -> Copy {
        let hello = timeOfDay(date, calendar: calendar)
        switch context {
        case .firstVisit:
            return Copy(title: L10n.pick(L10n.tr("どんなアプリが欲しいですか?"), review: L10n.tr("何を記録したいですか?")),
                        subtitle: L10n.pick(L10n.tr("\(hello)。話しかけるだけで、アプリが生まれ変わります"), review: L10n.tr("\(hello)。話すだけで、記録帳の形が変わります")))
        case .returning:
            return Copy(title: L10n.pick(L10n.tr("おかえりなさい。\n今日は何を作りますか?"), review: L10n.tr("おかえりなさい。\n今日は何を記録しますか?")),
                        subtitle: L10n.pick(L10n.tr("\(hello)。「アプリ一覧」と話すか、マイクを長押しすると前に作ったアプリを呼び出せます"), review: L10n.tr("\(hello)。マイクを長押しすると、前に用意した記録帳を呼び出せます")))
        case .newApp:
            return Copy(title: L10n.pick(L10n.tr("新しいアプリですね。\nどんなアプリにしますか?"), review: L10n.tr("新しい記録帳ですね。\n何を記録しますか?")),
                        subtitle: L10n.pick(L10n.tr("前のアプリは、マイクの長押しからいつでも戻せます"), review: L10n.tr("前の記録帳は、マイクの長押しからいつでも戻せます")))
        case .home:
            return Copy(title: L10n.pick(L10n.tr("おかえりなさい。\n次は何を作りますか?"), review: L10n.tr("おかえりなさい。\n次は何を記録しますか?")),
                        subtitle: L10n.pick(L10n.tr("\(hello)。前のアプリは、マイクの長押しからいつでも戻せます"), review: L10n.tr("\(hello)。前の記録帳は、マイクの長押しからいつでも戻せます")))
        case .editing:
            return Copy(title: L10n.pick(L10n.tr("このアプリをどう変えますか?"), review: L10n.tr("この記録帳をどう直しますか?")),
                        subtitle: L10n.pick(L10n.tr("話しかけると、このアプリが育ちます"), review: L10n.tr("話すと、この記録帳を直せます")))
        }
    }

    /// 変身一覧シートの見出し。
    static var switcherTitle: String { L10n.pick(L10n.tr("変身一覧"), review: L10n.tr("記録帳の一覧")) }
    static var switcherSubtitle: String { L10n.tr("タップで切り替え。マイクを長押しすると、いつでもここに戻れます") }
    static var switcherEmpty: String { L10n.pick(L10n.tr("まだ他のアプリはありません。\n「+ 新しいアプリ」から 2 つ目を作れます"), review: L10n.tr("まだ他の記録帳はありません。\n「+ 新しい記録帳」から 2 つ目を用意できます")) }
    static var currentBadge: String { L10n.tr("使用中") }
    static var newAppAction: String { L10n.pick(L10n.tr("新しいアプリ"), review: L10n.tr("新しい記録帳")) }
    static var homeAction: String { L10n.tr("Tailor It に戻る(初期画面)") }

    /// 例文チップのカテゴリ(#274)。初期画面で上の行がカテゴリ、下の行がそのカテゴリの例。
    struct ExampleCategory: Identifiable, Equatable {
        var id: String
        var title: String
        var examples: [String]
    }

    /// 記録・習慣・お金・家族/友達と・ゲーム。家族/友達と は共有して使う例(割り勘・出欠・みんなの掲示板)。
    /// 審査用の版(シェアが外れる・#245)では「みんなの掲示板」を出さない(割り勘・出欠は 1 人でも使える)。
    static func createExampleCategories(sharingAvailable: Bool = FirstRunTutorial.sharingAvailable) -> [ExampleCategory] {
        var together = [L10n.tr("友達と割り勘"), L10n.tr("サークルの出欠")]
        if sharingAvailable { together.append(L10n.tr("みんなの掲示板")) }
        return [
            ExampleCategory(id: "record", title: L10n.tr("記録"),
                            examples: [L10n.tr("筋トレの記録"), L10n.tr("散歩の写真メモ"), L10n.tr("読んだ本のメモ")]),
            ExampleCategory(id: "habit", title: L10n.tr("習慣"),
                            examples: [L10n.tr("毎日の水の量"), L10n.tr("薬の飲み忘れ防止"), L10n.tr("早起きチェック")]),
            ExampleCategory(id: "money", title: L10n.tr("お金"),
                            examples: [L10n.pick(L10n.tr("家計簿アプリが欲しい"), review: L10n.tr("家計簿をつけたい")), L10n.tr("おこづかい帳"), L10n.tr("サブスクの管理")]),
            ExampleCategory(id: "together", title: L10n.tr("家族・友達と"), examples: together),
            ExampleCategory(id: "game", title: L10n.tr("ゲーム"),
                            examples: [L10n.tr("ボードゲームの勝ち負け記録"), L10n.tr("ボウリングのスコア表")]),
        ]
    }

    /// 例文チップ(最初のカテゴリ)。
    static var createExamples: [String] { createExampleCategories().first?.examples ?? [] }
    static var editExamples: [String] { [L10n.tr("ダークモードにして"), L10n.tr("グラフを追加して"), L10n.tr("メモ欄を増やして")] }
    static var examplesLabel: String { L10n.tr("たとえば") }
    static var editExamplesLabel: String { L10n.tr("たとえば、こんな変更") }

    /// 5〜10 時 おはよう / 11〜17 時 こんにちは / それ以外 こんばんは
    static func timeOfDay(_ date: Date = .now, calendar: Calendar = .current) -> String {
        switch calendar.component(.hour, from: date) {
        case 5..<11: return L10n.tr("おはようございます")
        case 11..<18: return L10n.tr("こんにちは")
        default: return L10n.tr("こんばんは")
        }
    }

    // MARK: 初回 / 再訪の判定

    private static let visitsKey = "metamon.visitCount"

    /// このプロセスの起動が再訪か。最初に参照した時点で判定して訪問回数を 1 増やす(起動中は値が変わらない)。
    static let launchIsReturning: Bool = {
        let defaults = UserDefaults.standard
        let count = defaults.integer(forKey: visitsKey)
        defaults.set(count + 1, forKey: visitsKey)
        return count > 0
    }()

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
