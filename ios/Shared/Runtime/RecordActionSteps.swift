import MetamonDSL
import MetamonRenderer

/// saveRecord / deleteRecord の前段の判定と、記録まわりの一言。UIKit に依存しないので iOS と Mac で共有する。
/// 書き込み(ローカル DB → 同期キュー)は各ランタイムが `RecordSyncEngine` で行う。
@MainActor
enum RecordActionSteps {
    enum SaveTarget {
        /// 保存しない。toast に出す一言
        case invalid(String)
        /// recordId が nil なら新規
        case ready(entity: String, form: FormDraft, recordId: String?)
    }

    /// 保存先を決める。フォームが無い・必須が空なら一言を返す(空の欄には印を付ける・M10)。
    /// single(#267): 既存の 1 件があれば常にそれを上書き(フォームを開いた後に読み込まれた分も)。
    static func saveTarget(_ action: Action, _ ctx: ActionContext, dsl: AppDSL, records: (String) -> [Record]) -> SaveTarget {
        guard let form = ctx.form, let entity = action.entity ?? ctx.form?.entity.id else {
            return .invalid(L10n.tr("保存するフォームがありません"))
        }
        let missing = form.flagMissingRequired()
        guard missing.isEmpty else {
            return .invalid(L10n.tr("\(missing.joined(separator: L10n.tr("・"))) を入力してください"))
        }
        let recordId = form.recordId ?? (dsl.entity(entity)?.single == true ? SingleRecord.pick(records(entity))?.id : nil)
        return .ready(entity: entity, form: form, recordId: recordId)
    }

    /// 消す記録。single の 1 件は消さない(#267・backend も拒否)。
    static func deleteTarget(_ action: Action, _ ctx: ActionContext, dsl: AppDSL) -> (entity: String, recordId: String)? {
        guard let entity = action.entity ?? ctx.form?.entity.id,
              let id = ctx.recordId ?? ctx.form?.recordId,
              dsl.entity(entity)?.single != true else { return nil }
        return (entity, id)
    }

    /// 他人の共有レコードを変えようとした時の一言(#249)。端末で止めた時もサーバーの 403 でも同じ文言。
    static var readOnlyMessage: String { L10n.tr("このデータは作った人だけが変更できます") }
    /// その場で描いた変更を保存できず戻した時の一言(read-only と同じ toast)。
    static var saveFailedMessage: String { L10n.tr("保存できませんでした。通信を確かめてもう一度お試しください") }

    /// prefillRef の親が未保存(新規フォームの button / toolbar)。子は親の id が無いと作れないので先に保存してもらう。
    static func unsavedParentMessage(_ ctx: ActionContext) -> String {
        let name = ctx.form?.entity.name ?? ""
        return name.isEmpty ? L10n.tr("先に保存してください") : L10n.tr("先に「\(name)」を保存してください")
    }
}
