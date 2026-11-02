// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、仲間探し(#507)も外す。
#if !APP_STORE_REVIEW
import MetamonDSL

/// 仲間探し(#507)。「つながりたい」を送る・承認・見送る・取り下げ・解除。持たないホスト(モック・プレビュー)ではボタンを出さない。
/// 18 歳以上の確認・失敗の理由の表示はホストが持つ。通報・ブロックは `ModerationHost` をプロフィールのレコードで使う。
@MainActor
public protocol MatchingHost: AnyObject {
    /// プロフィールの行の状態。nil = ボタンを出さない(自分の行・プロフィールでない entity・サインインしていない)。
    func interestState(entity: String, record: Record) -> InterestState?
    /// 送信中(ボタンを押せなくする)。
    func interestBusy(record: Record) -> Bool
    /// 「つながりたい」(相手から届いていれば承認と同じ)。
    func sendInterest(entity: String, record: Record) async
    /// box の一覧。nil = まだ読んでいない。
    func matchItems(_ box: MatchBox) -> [MatchInterest]?
    /// 読めず、一覧も無い。
    func matchLoadFailed(_ box: MatchBox) -> Bool
    func refreshMatches(_ boxes: Set<MatchBox>) async
    func acceptInterest(_ item: MatchInterest) async
    /// 届いたものを見送る(incoming)/ 送ったものを取り下げる(outgoing)。
    func declineInterest(_ item: MatchInterest, box: MatchBox) async
    func unmatch(_ item: MatchInterest) async
}

public extension DSLRuntime {
    var matching: (any MatchingHost)? { self as? any MatchingHost }
}
#endif
