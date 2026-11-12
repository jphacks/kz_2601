#if os(iOS)
import SwiftUI
import Combine
import MetamonDSL

/// フォームのナビバーの「記録」メニュー(#1871): 開いている記録をコピー・クリップボードの記録を貼り付け。
/// 貼り付けは記録が載っている時・変えられるフォームの時だけ出す。貼っても保存はしない(保存は利用者が押す)。
struct RecordClipboardMenu: View {
    @Environment(\.dslRuntime) private var rt
    let form: FormDraft
    /// コピーする保存済みの記録(新規フォームは nil)。編集中の値ではなく保存済みの値を載せる
    let record: MetamonDSL.Record?
    let hasRecord: Bool

    static func isVisible(form: FormDraft, record: MetamonDSL.Record?, hasRecord: Bool, rt: any DSLRuntime) -> Bool {
        canCopy(form: form, record: record, rt: rt) || canPaste(form: form, hasRecord: hasRecord)
    }

    static func canCopy(form: FormDraft, record: MetamonDSL.Record?, rt: any DSLRuntime) -> Bool {
        record.flatMap { RecordPasteboard.payload(record: $0, entity: form.entity, rt: rt) } != nil
    }

    static func canPaste(form: FormDraft, hasRecord: Bool) -> Bool {
        hasRecord && !form.isReadOnly && !RecordClipboard.copyableFields(of: form.entity).isEmpty
    }

    var body: some View {
        Menu {
            if let record, Self.canCopy(form: form, record: record, rt: rt) {
                Button(L("記録をコピー"), systemImage: "doc.on.doc") {
                    RecordPasteboard.copy(record: record, entity: form.entity, rt: rt)
                }
            }
            if Self.canPaste(form: form, hasRecord: hasRecord) {
                Button(L("記録を貼り付け"), systemImage: "doc.on.clipboard") {
                    RecordPasteboard.paste(into: form, rt: rt)
                }
            }
        } label: {
            Label(L("記録のコピーと貼り付け"), systemImage: "doc.on.clipboard")
        }
        .accessibilityIdentifier("form.recordClipboard")
    }
}

/// クリップボードに記録が載っているかを追う(コピーした時・アプリに戻った時に見直す)。
struct RecordPasteboardWatcher: ViewModifier {
    @Binding var hasRecord: Bool

    func body(content: Content) -> some View {
        content
            .onAppear { hasRecord = RecordPasteboard.hasRecord() }
            .onReceive(NotificationCenter.default.publisher(for: UIPasteboard.changedNotification)) { _ in
                hasRecord = RecordPasteboard.hasRecord()
            }
            .onReceive(NotificationCenter.default.publisher(for: UIApplication.didBecomeActiveNotification)) { _ in
                hasRecord = RecordPasteboard.hasRecord()
            }
    }
}
#endif
