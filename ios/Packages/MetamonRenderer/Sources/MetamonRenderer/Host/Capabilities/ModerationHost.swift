import MetamonDSL

/// 他人の共有レコードの通報・ブロック(#321・審査 1.2)。持たないホストではメニューを出さない。
@MainActor
public protocol ModerationHost: AnyObject {
    /// 通報・ブロックできるか(シェアの無い版・自分の投稿・private の entity は false)。
    func canReport(entity: String, record: Record) -> Bool
    /// 通報の理由を選ぶ画面をホストが出す。
    func reportRecord(entity: String, record: Record)
    /// 作者のブロックの確認をホストが出す(ブロックすると、その人の投稿はすぐ見えなくなる)。
    func blockAuthor(entity: String, record: Record)
}

public extension DSLRuntime {
    var moderation: (any ModerationHost)? { self as? any ModerationHost }
}
