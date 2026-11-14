import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// `lookupBook`: フォームの ISBN から本の情報を引き、書名などの field を上書きする(保存はしない)。
/// 中身は Mac と共有する(`Shared/Runtime/BookLookupSteps.swift`・#978)。
extension AppRuntime {
    func lookupBook(_ action: Action, _ ctx: ActionContext) async {
        if let message = await BookLookupSteps.run(action, form: ctx.form, client: services.books) { toast = message }
    }
}
