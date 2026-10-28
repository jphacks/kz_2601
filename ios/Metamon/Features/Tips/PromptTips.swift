import Foundation
import MetamonDSL

/// 「頼み方のコツ」(#415)の例文。タブ・画面の行き来・一覧などを正確に頼める言い方をカテゴリ別に並べる。
/// - 文はここにだけ置く。キーは日本語(英語は `Localizable.xcstrings`)
/// - 同じ文を ai-eval のケース `backend/internal/ai/eval/tips_cases.json`(`tip_<id>` / `tip_<id>_en`)で確かめる。
///   ずれは `PromptTipsTests` が落とす
/// - 審査用の版(#245)で外した機能の例は `requires` で隠す
enum PromptTips {
    /// 例が使う機能のうち、版によって無いもの。
    enum Feature: CaseIterable {
        /// 家族だけの共有・公開(共有リンク)
        case sharing
        /// photoField / videoField の撮影
        case cameraCapture
        case nearby
        case health
        /// バックグラウンドでの道のりの記録
        case locationTracking
    }

    struct Tip: Identifiable, Equatable {
        var id: String
        /// 日本語の文(= 翻訳のキー・ai-eval の transcript)
        var key: String
        var requires: Set<Feature> = []
        /// AI に送らず端末で処理する(「元に戻して」= restore)。ai-eval のケースは作らない
        var onDevice = false

        var text: String { L10n.tr(String.LocalizationValue(key)) }
    }

    struct Category: Identifiable, Equatable {
        var id: String
        var titleKey: String
        var symbol: String
        var tips: [Tip]

        var title: String { L10n.tr(String.LocalizationValue(titleKey)) }
    }

    /// この版で使える機能。
    struct Availability: Equatable {
        var features: Set<Feature>

        static let full = Availability(features: Set(Feature.allCases))

        /// AI に送る制限(`ClientCapabilities`)と共有の有無から決める。allowedComponents が nil = 全部使える。
        static func from(allowedComponents: [String]?, disabledFeatures: [String]?, sharing: Bool) -> Availability {
            func has(_ type: ComponentType) -> Bool { allowedComponents?.contains(type.rawValue) ?? true }
            var set = Set<Feature>()
            if sharing { set.insert(.sharing) }
            if !(disabledFeatures ?? []).contains("cameraCapture") { set.insert(.cameraCapture) }
            if has(.nearbyShare) { set.insert(.nearby) }
            if has(.healthStat), !(disabledFeatures ?? []).contains("fieldHealth") { set.insert(.health) }
            if has(.locationTracker) { set.insert(.locationTracking) }
            return Availability(features: set)
        }

        static var current: Availability {
            from(allowedComponents: ClientCapabilities.allowedComponents,
                 disabledFeatures: ClientCapabilities.disabledFeatures,
                 sharing: FirstRunTutorial.sharingAvailable)
        }

        static var review: Availability {
            from(allowedComponents: ClientCapabilities.reviewAllowedComponents,
                 disabledFeatures: ClientCapabilities.reviewDisabledFeatures, sharing: false)
        }
    }

    /// 版で使えない例を除いたカテゴリ。例が 0 になったカテゴリは出さない。
    static func categories(for availability: Availability = .current) -> [Category] {
        all.compactMap { category in
            var c = category
            c.tips = category.tips.filter { $0.requires.isSubset(of: availability.features) }
            return c.tips.isEmpty ? nil : c
        }
    }

    /// 全部の例(版で絞らない)。ai-eval との突き合わせに使う。
    static let all: [Category] = [
        Category(id: "tabs", titleKey: "下のタブ", symbol: "menubar.dock.rectangle", tips: [
            Tip(id: "tab_add", key: "下に「履歴」のタブを足して、記録を新しい順に全部見られるようにして"),
            Tip(id: "tab_rename", key: "「集計」のタブの名前を「グラフ」に変えて"),
            Tip(id: "tab_reorder", key: "タブの順番を入れ替えて、「集計」をいちばん左にして"),
        ]),
        Category(id: "nav", titleKey: "ボタンで別の画面へ", symbol: "rectangle.portrait.and.arrow.right", tips: [
            Tip(id: "nav_button", key: "集計の画面に「記録する」ボタンを置いて、押したら入力の画面が開くようにして"),
            Tip(id: "nav_new_screen", key: "ホームに「使い方」ボタンを置いて、押したら説明を書いた画面に移るようにして"),
            Tip(id: "nav_back", key: "保存したら、一覧の画面に自動で戻るようにして"),
        ]),
        Category(id: "detail", titleKey: "一覧 → タップで詳しく・直す", symbol: "list.bullet.rectangle", tips: [
            Tip(id: "detail_open", key: "一覧の項目をタップしたら、詳しい内容の画面が開くようにして"),
            Tip(id: "detail_edit", key: "詳しい画面で、内容を直したり消したりできるようにして"),
            Tip(id: "detail_swipe", key: "一覧を左にスワイプして消せるようにして"),
        ]),
        Category(id: "add", titleKey: "追加ボタン・入力欄", symbol: "plus.circle", tips: [
            Tip(id: "add_button", key: "右下に「+ 追加」ボタンを置いて、押したら入力の画面が開くようにして"),
            Tip(id: "add_field", key: "入力の画面に「メモ」の欄を足して"),
            Tip(id: "add_move", key: "追加ボタンを左上に移して"),
        ]),
        Category(id: "stats", titleKey: "グラフ・合計", symbol: "chart.bar.xaxis", tips: [
            Tip(id: "stats_total", key: "今月の合計を、画面のいちばん上に大きく出して"),
            Tip(id: "stats_pie", key: "カテゴリごとの割合を円グラフで見せて"),
            Tip(id: "stats_line", key: "1日ごとの変化を折れ線グラフで見せて"),
            Tip(id: "stats_steps", key: "毎日の歩数をグラフで見せて", requires: [.health]),
        ]),
        Category(id: "notify", titleKey: "通知", symbol: "bell.badge", tips: [
            Tip(id: "notify_daily", key: "毎晩9時に、記録を忘れないよう通知して"),
            Tip(id: "notify_weekly", key: "毎週月曜の朝8時にお知らせして"),
            Tip(id: "notify_change", key: "通知の時間を朝7時に変えて"),
        ]),
        Category(id: "share", titleKey: "共有(家族だけ / みんなに公開)", symbol: "person.2", tips: [
            Tip(id: "share_family", key: "この記録を家族だけで共有して", requires: [.sharing]),
            Tip(id: "share_public", key: "みんなに公開して、友達のスマホからも書き込めるようにして", requires: [.sharing]),
            Tip(id: "share_nearby", key: "近くにいる人の端末に、まとめて送れるようにして", requires: [.sharing, .nearby]),
        ]),
        Category(id: "map", titleKey: "地図・場所", symbol: "map", tips: [
            Tip(id: "map_pins", key: "記録した場所を地図にピンで出して"),
            Tip(id: "map_here", key: "ボタンを押したら、今いる場所を記録できるようにして"),
            Tip(id: "map_route", key: "散歩した道のりを地図に記録して", requires: [.locationTracking]),
        ]),
        Category(id: "photo", titleKey: "カメラ・写真", symbol: "camera", tips: [
            Tip(id: "photo_attach", key: "記録に写真を付けられるようにして"),
            Tip(id: "photo_camera", key: "その場でカメラで撮って残せるようにして", requires: [.cameraCapture]),
            Tip(id: "photo_qr", key: "QRコードを読み取って記録できるようにして"),
        ]),
        Category(id: "fix", titleKey: "直し方", symbol: "wand.and.stars", tips: [
            Tip(id: "fix_bigger", key: "合計をもっと大きく表示して"),
            Tip(id: "fix_color", key: "ボタンの色をもっと目立つ色にして"),
            Tip(id: "fix_words", key: "「メモ」という言葉を「ひとこと」に変えて"),
            Tip(id: "fix_undo", key: "元に戻して", onDevice: true),
        ]),
    ]

    static let titleKey = "頼み方のコツ"
    static let introKey = "できた画面に向かって、こう話すと思いどおりに直せます。例をタップすると入力欄に入るので、言葉を足してから送ってください。"
    static var title: String { L10n.tr(String.LocalizationValue(titleKey)) }
    static var intro: String { L10n.tr(String.LocalizationValue(introKey)) }
}
