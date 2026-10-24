import SwiftUI
import PhotosUI
import UIKit
import MetamonDSL

/// `photoField`(P0 カメラ): source(camera|library|both)に応じて実機カメラ / 写真選択 / 二択ダイアログ。
/// 取得した画像は `PhotoCodec` で 800px 以下・JPEG q0.5 の "data:image/jpeg;base64,..." にして photo フィールドへ。
/// カメラが無い端末(シミュレータ等)では自動で写真選択に落ちる(`PhotoSourcePolicy`)。
struct PhotoFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    @State private var librarySelection: PhotosPickerItem?
    @State private var showLibrary = false
    @State private var showCamera = false
    @State private var showChooser = false
    @State private var showDenied = false
    @Environment(\.openURL) private var openURL

    var body: some View {
        if let form = ctx.form, let field = c.field {
            content(form: form, field: field)
        } else {
            NeedsForm(c: c)
        }
    }

    @ViewBuilder
    private func content(form: FormDraft, field: String) -> some View {
        let value = form.values[field] ?? .null
        let hasPhoto = PhotoCodec.image(from: value) != nil || (value.stringValue?.hasPrefix("http") ?? false)

        VStack(alignment: .leading, spacing: 8) {
            if hasPhoto {
                PhotoView(value: value, height: 160, accessibilityLabel: "\(c.label ?? "写真")(添付済み)")
                HStack(spacing: 8) {
                    Button { start() } label: {
                        Label("撮り直す", systemImage: "camera").frame(maxWidth: .infinity)
                    }
                    .accessibilityHint(sourceHint)
                    Button(role: .destructive) { form.values[field] = .null } label: {
                        Label("削除", systemImage: "trash").frame(maxWidth: .infinity)
                    }
                    .accessibilityLabel("\(c.label ?? "写真")を削除")
                }
                .buttonStyle(.bordered)
                .controlSize(.large)
            } else {
                Button { start() } label: {
                    Label(c.label ?? "写真を撮る / 選ぶ", systemImage: mode == .library ? "photo.on.rectangle" : "camera")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .controlSize(.large)
                // VoiceOver: 「レシートを撮る, ボタン, カメラを開きます」
                .accessibilityLabel(c.label ?? "写真を撮る / 選ぶ")
                .accessibilityHint(sourceHint)
            }
        }
        .confirmationDialog(c.label ?? "写真", isPresented: $showChooser, titleVisibility: .hidden) {
            Button("撮影") { openCamera() }
            Button("写真から選ぶ") { showLibrary = true }
            Button("キャンセル", role: .cancel) {}
        }
        .fullScreenCover(isPresented: $showCamera) {
            CameraPicker { image in
                if let encoded = PhotoCodec.dataURL(from: image) { form.values[field] = .string(encoded) }
            }
            .ignoresSafeArea()
        }
        .photosPicker(isPresented: $showLibrary, selection: $librarySelection, matching: .images)
        .onChange(of: librarySelection) { _, item in
            guard let item else { return }
            Task {
                if let data = try? await item.loadTransferable(type: Data.self), let encoded = PhotoCodec.dataURL(from: data) {
                    form.values[field] = .string(encoded)
                }
                librarySelection = nil
            }
        }
        .alert("カメラを使えません", isPresented: $showDenied) {
            Button("設定を開く") {
                if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
            }
            Button("写真から選ぶ") { showLibrary = true }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("設定アプリで Tailor It のカメラへのアクセスを許可してください。")
        }
    }

    private var mode: PhotoSourceMode { PhotoSourceMode(raw: c.source) }

    private var sourceHint: String {
        switch PhotoSourcePolicy.plan(for: mode, cameraAvailable: CameraPicker.isAvailable) {
        case .camera: "カメラを開きます"
        case .library: "写真ライブラリを開きます"
        case .choose: "カメラか写真ライブラリを選びます"
        }
    }

    private func start() {
        switch PhotoSourcePolicy.plan(for: mode, cameraAvailable: CameraPicker.isAvailable) {
        case .camera: openCamera()
        case .library: showLibrary = true
        case .choose: showChooser = true
        }
    }

    private func openCamera() {
        Task { @MainActor in
            if await CameraPicker.requestAccess() { showCamera = true } else { showDenied = true }
        }
    }
}
