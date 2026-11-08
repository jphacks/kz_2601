import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// `lookupBook`: フォームの ISBN から本の情報を引き、書名などの field を上書きする(保存はしない)。
/// 端末は appconfig にだけ聞く(`GET /v1/lookup/book`)。見つからない・つながらない時はメッセージだけで手入力に戻る。
extension AppRuntime {
    func lookupBook(_ action: Action, _ ctx: ActionContext) async {
        guard let spec = action.bookLookup, let form = ctx.form, !form.isReadOnly else {
            toast = L10n.tr("本の情報はフォームの画面で読み込めます")
            return
        }
        let raw = (form.values[spec.isbnField] ?? .null).displayString
        guard !raw.trimmingCharacters(in: .whitespaces).isEmpty else {
            toast = L10n.tr("先に ISBN を入力してください")
            return
        }
        guard let isbn = BookLookupPlan.normalizeISBN(raw) else {
            toast = L10n.tr("ISBN を確かめてください(10 桁か 13 桁)")
            return
        }
        guard let client = services.books else {
            toast = L10n.tr("本の情報を読み込めません。手で入力してください")
            return
        }
        do {
            let book = try await client.lookup(isbn: isbn)
            let values = BookLookupPlan.values(spec, book: book, entity: form.entity)
            for (id, v) in values { form.values[id] = v }
            form.values[spec.isbnField] = .string(isbn)
            toast = L10n.tr("「\(book.title)」の情報を入れました(出典: \(book.source.name))")
        } catch let e as APIError where e.status == 404 {
            toast = L10n.tr("この ISBN の本は見つかりませんでした。手で入力してください")
        } catch let e as APIError where e.isBookLookupOff {
            toast = L10n.tr("本の情報の取得は今使えません。手で入力してください")
        } catch let e as APIError where e.status == 400 {
            toast = L10n.tr("ISBN を確かめてください(10 桁か 13 桁)")
        } catch {
            toast = L10n.tr("本の情報を読み込めません。手で入力してください")
        }
    }
}
