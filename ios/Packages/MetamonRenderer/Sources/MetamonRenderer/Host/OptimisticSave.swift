import MetamonDSL

/// 楽観的に描いた変更の保存(一覧のチェック・単語カード・トーナメントの勝者)。
/// 失敗したら 1 回の操作につき 1 回だけ `notifySaveFailed` を呼ぶ。表示を戻すのは呼び出し側。
/// 他人の記録で止まった時はホストが `notifyReadOnly` を出しているので重ねない。
@MainActor
enum OptimisticSave {
    /// 1 件の更新。保存できたら true。
    static func update(_ rt: any DSLRuntime, entity: String, recordId: String, data: [String: JSONValue]) async -> Bool {
        let saved = await rt.updateRecord(entity: entity, recordId: recordId, data: data)
        if !saved { reportFailure(rt, entity: entity, recordId: recordId) }
        return saved
    }

    /// 順に保存し、途中で失敗したら残りは送らない(`settle` は全件に呼ぶ)。失敗の一言は 1 回だけ。全部保存できたら true。
    static func updateInOrder(_ rt: any DSLRuntime, entity: String, writes: [(recordId: String, data: [String: JSONValue])],
                              settle: (String) -> Void) async -> Bool {
        var failedId: String?
        for w in writes {
            if failedId == nil, !(await rt.updateRecord(entity: entity, recordId: w.recordId, data: w.data)) {
                failedId = w.recordId
            }
            settle(w.recordId)
        }
        guard let failedId else { return true }
        reportFailure(rt, entity: entity, recordId: failedId)
        return false
    }

    private static func reportFailure(_ rt: any DSLRuntime, entity: String, recordId: String) {
        print("[Metamon] not saved: \(entity)/\(recordId)")
        if let r = rt.records(for: entity).first(where: { $0.id == recordId }), !rt.canEdit(entity: entity, record: r) { return }
        rt.notifySaveFailed(entity: entity)
    }
}
