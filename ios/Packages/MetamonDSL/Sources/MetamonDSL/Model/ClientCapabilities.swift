import Foundation

/// AI(generate / chat)に「この iOS が描ける部品」を伝える(#245・#302)。
/// 審査用ビルド(APP_STORE_REVIEW)はネイティブ API を使う部品をバイナリから外すので、AI にも使わせない。
/// 通常のビルドも描ける全部品を毎回送る(#302): 後の版で backend に足された部品は、この版には出さない。
/// 契約: docs/backend/openapi-ai.yaml・版の規則: docs/dsl/README.md §5
public enum ClientCapabilities {
    /// この版が描画できる DSL の最大 `schemaVersion`(ヘッダ `X-Client-DSL-Version`)。
    public static let supportedSchemaVersion = 1

    /// この版が描画できる全部品(`unknown` を除く全 type)。
    public static var allComponents: [String] {
        ComponentType.allCases.filter { $0 != .unknown }.map(\.rawValue)
    }

    /// 審査用ビルドでバイナリから外す部品(Renderer の登録もしない)。feed(#348)は外部の Web の内容を出すので外す。aiChat はオンライン AI(POST /v1/text)だけで動き、審査用の版はそれを持たない(#452 G4)ので外す。
    /// callButton(#447・通話)は共有と一緒に外す(マイク・カメラで他の人とつながる。審査メモの説明が要る)。
    public static let reviewExcludedComponents: Set<ComponentType> = [.healthStat, .locationTracker, .nearbyShare, .dashcam, .sensorValue, .audioRecorder, .webView, .chatThread, .matchList, .feed, .aiChat, .callButton]
    /// 審査用ビルドで無効にする機能。fieldHealth = Field.health(HealthKit への書き込み)、cameraCapture = photoField / videoField の撮影、
    /// sharedData = entity の scope shared / public(他の人と同じ記録を見る作り・#451 G2。backend が private に直す)。
    public static let reviewDisabledFeatures = ["fieldHealth", "cameraCapture", "sharedData"]

    /// 審査用ビルドのときの部品の allowlist(`unknown` を除く全 type − 外した部品)。
    public static var reviewAllowedComponents: [String] {
        ComponentType.allCases.filter { $0 != .unknown && !reviewExcludedComponents.contains($0) }.map(\.rawValue)
    }

    /// この端末の OS(ヘッダ `X-Client-Platform`・#925)。backend はログに残すだけで、部品の絞り込みは allowedComponents で行う。
    /// 値は docs/decisions/363-desktop-strategy.md の「AI への申告」に合わせて "macos"。
    public static var platform: String {
        #if os(macOS)
        "macos"
        #else
        "ios"
        #endif
    }

    /// Mac で描けない部品(#925・#947)。Mac にハード / データが無い(HealthKit・加速度 / 気圧・持ち歩く位置の追跡・ドラレコ)。
    /// 表は docs/decisions/363-desktop-strategy.md「部品の対応(Mac の列)」の ×。AI には作らせず、古い DSL で来た時は
    /// Renderer が「この Mac では使えません」の枠(MacUnavailableView)で受ける。
    public static let macUnavailableComponents: Set<ComponentType> = [.healthStat, .sensorValue, .locationTracker, .dashcam]
    /// Mac で無効にする機能。fieldHealth = Field.health(HealthKit が無い)。
    /// photoField / videoField の撮影(cameraCapture)は Mac のカメラで撮れる(#1043)ので外さない。カメラの無い Mac は Renderer が選ぶだけにする。
    public static let macDisabledFeatures = ["fieldHealth"]

    /// 通常のビルドの部品の allowlist(OS ごと)。Mac は描けない部品を外す。
    public static func allowedComponents(macOS: Bool, review: Bool) -> [String] {
        var excluded: Set<ComponentType> = [.unknown]
        if review { excluded.formUnion(reviewExcludedComponents) }
        if macOS { excluded.formUnion(macUnavailableComponents) }
        return ComponentType.allCases.filter { !excluded.contains($0) }.map(\.rawValue)
    }

    /// 無効にする機能(OS ごと)。どれも無ければ nil(送らない)。
    public static func disabledFeatures(macOS: Bool, review: Bool) -> [String]? {
        var out: [String] = []
        for f in (review ? reviewDisabledFeatures : []) + (macOS ? macDisabledFeatures : []) where !out.contains(f) { out.append(f) }
        return out.isEmpty ? nil : out
    }

    private static var isMacOS: Bool {
        #if os(macOS)
        true
        #else
        false
        #endif
    }

    private static var isReview: Bool {
        #if APP_STORE_REVIEW
        true
        #else
        false
        #endif
    }

    /// このビルドの値。通常 = 全部品(#302)、審査用 = 外した部品を除く。Mac は描けない部品も除く(#925)。
    public static var allowedComponents: [String]? {
        allowedComponents(macOS: isMacOS, review: isReview)
    }

    /// 部品以外で、この版が実装している DSL の動作(#482)。backend は allowedComponents を送るのにここに無い動作(= v1.0)を AI に使わせない。
    /// recordRetention・notification.when は審査用の版も同じ(記録の自動削除・条件の通知は端末内の処理だけ)。matching(#507・仲間探し)は審査用の版では送らない(共有と一緒に外す)。
    /// notification.when(#265): 知らない版は残りを 9:00 の 1 回きりとして予約してしまうので、無い版には生成させない。
    /// singleEntity(#267・設定値の置き場): 知らない版は設定のフォームで保存するたびに記録を足してしまうので、無い版には生成させない。審査用の版も同じ。
    /// dataRoles(#272・参加者を選ぶ member と管理者だけのボタン visibleTo): 参加者の一覧を読むので、共有を外す審査用の版は送らない。
    /// addressedMessages(#271・宛先を選ぶ members と宛先付きの記録): 同じく参加者を選ぶので審査用の版は送らない。
    /// notification.near(場所に着いたら知らせる・時刻の jitterMinutes): 知らない版は near の通知を 9:00 の 1 回きりとして予約してしまうので、無い版には生成させない。
    /// 審査用の版も同じ(位置の判定は端末の中だけ・使用中のみの許可)。
    /// aiButton.long・aiButton.inputPhoto・aiButton.saveTo・aiButton.inputFeeds(画面の feed の行を AI に読ませる)・aiButton.outputs(fields を項目ごとにフォームへ)・action.lookupBook・text.highlight(text の {{fieldId}} も)・feed.v2(feed の warnings / pressure): 知らない版は黙って無視するか描けないので、無い版には生成させない。審査用の版も同じ(feed は部品ごと外す)。ただし long・inputPhoto は送らない(下の #1640)。
    /// expr.addMinutes(計算式の op addMinutes・到着 − 所要分 = 出発): 知らない版は式を「—」にするので、無い版には生成させない。審査用の版も同じ(端末内の計算だけ)。
    /// aiChat.inputEntity(aiChat の inputEntity と limit・自分の記録を読んで話す): 知らない版は黙って無視するので、無い版には生成させない。審査用の版も同じ(aiChat は部品ごと外す)。
    /// component.bracket(トーナメント表): 知らない版は描けないので、無い版には生成させない。審査用の版も同じ。
    /// progressBar.settingTarget(progressBar の target に {"setting": …}・変えられる目標でバーが動く): 知らない版はバーが空になるので、無い版には生成させない。審査用の版も同じ(端末内の計算だけ)。
    /// photoField.illustrate(端末内の Image Playground でイラストを作る): 知らない版はキーを無視するだけだが、ボタンが出ないので無い版には生成させない。
    /// 審査用の版は Apple Intelligence に頼る作りを審査で確かめにくいので、ボタンをバイナリから外し、キーも送らない。
    /// component.callButton(#447・通話): 知らない版は描けないので、無い版には生成させない。審査用の版は部品ごと外し、キーも送らない。
    /// openUrlButton.app(openUrlButton の app / to・shareButton の shareTo x / mail): 知らない版は url の無いボタンを描けないので、無い版には生成させない。
    /// 審査用の版も送る(公開されているリンクを Safari / そのアプリで開くだけ)。
    /// 審査用の版は aiButton.long・aiButton.inputPhoto を送らない(#1640): どちらもオンライン AI(POST /v1/text)だけで動き、審査用の版はそれを持たないので、
    /// 送ると押せないボタンができる。送らなければ backend が length / inputPhoto を外し、端末内 AI で動く形にする。
    /// field.encrypted(#233・項目を端末で暗号化して保存・鍵は iCloud キーチェーン): 知らない版は平文で保存してしまうので、無い版には生成させない。
    /// Tailor It Pro の機能なので、App 内課金の無い審査用の版は送らない。
    public static var supportedFeatures: [String] {
        #if APP_STORE_REVIEW
        ["recordRetention", "notification.when", "singleEntity", "notification.near", "aiButton.saveTo", "aiButton.inputFeeds", "aiButton.outputs", "action.lookupBook", "text.highlight", "feed.v2", "expr.addMinutes", "aiChat.inputEntity", "component.bracket", "progressBar.settingTarget", "openUrlButton.app"]
        #else
        ["recordRetention", "matching", "notification.when", "singleEntity", "dataRoles", "addressedMessages", "notification.near", "aiButton.long", "aiButton.inputPhoto", "aiButton.saveTo", "aiButton.inputFeeds", "aiButton.outputs", "action.lookupBook", "text.highlight", "feed.v2", "expr.addMinutes", "aiChat.inputEntity", "component.bracket", "progressBar.settingTarget", "photoField.illustrate", "component.callButton", "field.encrypted", "openUrlButton.app"]
        #endif
    }

    public static var disabledFeatures: [String]? {
        disabledFeatures(macOS: isMacOS, review: isReview)
    }
}
