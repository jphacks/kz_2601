import SwiftUI
import PhotosUI
import MetamonDSL

/// `photoField`(P0 カメラ): 撮影/選択 → 800px 以下・JPEG q0.5 に縮小 → "data:image/jpeg;base64,..." を photo フィールドへ。
/// v1 は PhotosPicker(シミュレータでも動く)。実機の `source: camera` は UIImagePickerController(.camera) に差し替え予定。
struct PhotoFieldLeaf: View {
    let c: Component
    let ctx: RenderContext
    @State private var selection: PhotosPickerItem?

    var body: some View {
        if let form = ctx.form, let field = c.field {
            PhotosPicker(selection: $selection, matching: .images) {
                Label(c.label ?? "写真を撮る / 選ぶ", systemImage: "camera")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .controlSize(.large)
            .onChange(of: selection) { _, item in
                guard let item else { return }
                Task {
                    if let data = try? await item.loadTransferable(type: Data.self), let encoded = PhotoCodec.dataURL(from: data) {
                        form.values[field] = .string(encoded)
                    }
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
