import MetamonServices

/// 1 日の枠が切れて作れなかった依頼(H3・Mac は #3079)。会話は消さずに残し、枠が戻った・Pro を買ったら同じ依頼で続きから作る。
struct HeldBuild: Equatable {
    /// 作るはずだった依頼文(ready の transcript / 案の instruction)
    var text: String

    /// 作れなかった理由が「利用者の 1 日の上限」の時だけ依頼を残す(混雑・オフライン等は「もう一度」で足りる。
    /// 同じネットワークの上限・全体の停止も待つ/サインインで解ける別の案内)。審査用ビルドは課金を持たないので残さない(#450)
    static func holds(for info: RateLimitInfo) -> Bool {
        #if APP_STORE_REVIEW
        return false
        #else
        return info.scope == .user
        #endif
    }
}
