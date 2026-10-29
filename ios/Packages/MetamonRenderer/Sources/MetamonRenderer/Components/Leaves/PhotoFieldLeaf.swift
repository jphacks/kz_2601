import SwiftUI
import PhotosUI
import UIKit
import MetamonDSL

/// `photoField`(P0 カメラ): source(camera|library|both)に応じて実機カメラ / 写真選択 / 二択ダイアログ。
/// 取得した画像は `PhotoCodec` で 800px 以下・JPEG q0.5 の "data:image/jpeg;base64,..." にして photo フィールドへ。
/// カメラが無い端末(シミュレータ等)では自動で写真選択に落ちる(`PhotoSourcePolicy`)。
/// `analyze`(#418)があれば、撮った・選んだ直後に端末内で読み取って他の field のドラフトに入れる(`PhotoReader`)。
struct PhotoFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    @State private var librarySelection: PhotosPickerItem?
    @State private var showLibrary = false
    @State private var showCamera = false
    @State private var showChooser = false
    @State private var showDenied = false
    @State private var reading = false
    @State private var readStatus: ReadStatus?
    @State private var pose: BodyPose?
    @Environment(\.openURL) private var openURL

    private enum ReadStatus: Equatable {
        /// 読めなかった項目の名前(空 = 全部読めた)
        case done(missed: [String])
        case unreadable
    }

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
        let decoded = PhotoCodec.image(from: value)
        let hasPhoto = decoded != nil || (value.stringValue?.hasPrefix("http") ?? false)

        VStack(alignment: .leading, spacing: 8) {
            if hasPhoto {
                if wantsPose, let pose, let decoded {
                    PosePhotoView(image: decoded, pose: pose, height: 200)
                } else {
                    PhotoView(value: value, height: 160, accessibilityLabel: L("\(c.label ?? L("写真"))(添付済み)"))
                }
                HStack(spacing: RenderTheme.Spacing.sm) {
                    Button { start() } label: {
                        Label(L("撮り直す"), systemImage: "camera").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    .accessibilityHint(sourceHint)
                    Button(role: .destructive) {
                        form.values[field] = .null
                        pose = nil
                        readStatus = nil
                    } label: {
                        Label(L("削除"), systemImage: "trash").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .destructive))
                    .accessibilityLabel(L("\(c.label ?? L("写真"))を削除"))
                }
                if !c.analyze.isEmpty {
                    readControls(form: form, field: field, canRead: decoded != nil)
                }
            } else {
                Button { start() } label: {
                    Label(c.label ?? L("写真を撮る / 選ぶ"), systemImage: mode == .library ? "photo.on.rectangle" : "camera")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                // VoiceOver: 「レシートを撮る, ボタン, カメラを開きます」
                .accessibilityLabel(c.label ?? L("写真を撮る / 選ぶ"))
                .accessibilityHint(sourceHint)
            }
        }
        #if !APP_STORE_REVIEW
        .confirmationDialog(c.label ?? L("写真"), isPresented: $showChooser, titleVisibility: .hidden) {
            Button(L("撮影")) { openCamera() }
            Button(L("写真から選ぶ")) { showLibrary = true }
            Button(L("キャンセル"), role: .cancel) {}
        }
        .fullScreenCover(isPresented: $showCamera) {
            CameraPicker { image in
                if let encoded = PhotoCodec.dataURL(from: image) { setPhoto(encoded, form: form, field: field) }
            }
            .ignoresSafeArea()
        }
        #endif
        .photosPicker(isPresented: $showLibrary, selection: $librarySelection, matching: .images)
        .onChange(of: librarySelection) { _, item in
            guard let item else { return }
            Task {
                if let data = try? await item.loadTransferable(type: Data.self), let encoded = PhotoCodec.dataURL(from: data) {
                    setPhoto(encoded, form: form, field: field)
                }
                librarySelection = nil
            }
        }
        #if !APP_STORE_REVIEW
        .alert(L("カメラを使えません"), isPresented: $showDenied) {
            Button(L("設定を開く")) {
                if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
            }
            Button(L("写真から選ぶ")) { showLibrary = true }
            Button(L("キャンセル"), role: .cancel) {}
        } message: {
            Text(L("設定アプリで Tailor It のカメラへのアクセスを許可してください。"))
        }
        #endif
        // 既存の写真(編集で開いた時)にも骨格を重ねる。値は書き換えない
        .task(id: value) { await detectPoseForOverlay(value) }
    }

    // MARK: - AI で読み取る(#418)

    private var wantsPose: Bool { c.analyze.contains { $0.kind == .jointAngle } }

    @ViewBuilder
    private func readControls(form: FormDraft, field: String, canRead: Bool) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Button { read(form: form, field: field) } label: {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    if reading { ProgressView().controlSize(.small) } else { Image(systemName: "sparkles") }
                    Text(reading ? L("AI で読み取っています…") : L("もう一度読み取る"))
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .disabled(reading || !canRead)
            .accessibilityHint(L("端末内の AI で写真を読み取り、入力欄に入れます"))
            switch readStatus {
            case .done(let missed) where missed.isEmpty:
                Text(L("端末内の AI が読み取りました。確認して保存してください")).font(.caption).foregroundStyle(.secondary)
            case .done(let missed):
                Text(L("読み取れなかった項目: \(missed.joined(separator: "、"))")).font(.caption).foregroundStyle(.secondary)
            case .unreadable:
                Text(L("この写真は読み取れません")).font(.caption).foregroundStyle(.secondary)
            case nil:
                EmptyView()
            }
        }
    }

    /// 撮った・選んだ写真を入れ、analyze があればすぐ読み取る。
    private func setPhoto(_ encoded: String, form: FormDraft, field: String) {
        form.values[field] = .string(encoded)
        pose = nil
        readStatus = nil
        if !c.analyze.isEmpty { read(form: form, field: field) }
    }

    /// 読めた項目だけドラフトに入れる(読めなかった項目は今の値のまま)。保存はユーザー。
    private func read(form: FormDraft, field: String) {
        guard !reading else { return }
        guard let image = PhotoCodec.image(from: form.values[field] ?? .null) else {
            readStatus = .unreadable
            return
        }
        reading = true
        readStatus = nil
        let specs = c.analyze
        let entity = form.entity
        Task { @MainActor in
            let out = await PhotoReader.read(image, specs: specs, entity: entity)
            withAnimation(RenderTheme.Motion.select) {
                for (id, v) in out.values { form.values[id] = v }
            }
            if wantsPose { pose = out.pose }
            readStatus = .done(missed: out.missed.map { entity.field($0)?.name ?? $0 })
            reading = false
        }
    }

    private func detectPoseForOverlay(_ value: JSONValue) async {
        guard wantsPose, !reading else { return }
        pose = nil
        guard let cg = PhotoCodec.image(from: value)?.cgImage else { return }
        let detected = await Task.detached(priority: .utility) { PhotoVision.bodyPose(in: cg) }.value
        if !Task.isCancelled, !reading { pose = detected }
    }

    private var mode: PhotoSourceMode { PhotoSourceMode(raw: c.source) }

    /// 審査用ビルド(APP_STORE_REVIEW・#245)は撮影をバイナリから外し、常にライブラリから選ぶ。
    private static var cameraAvailable: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        CameraPicker.isAvailable
        #endif
    }

    private var sourceHint: String {
        switch PhotoSourcePolicy.plan(for: mode, cameraAvailable: Self.cameraAvailable) {
        case .camera: L("カメラを開きます")
        case .library: L("写真ライブラリを開きます")
        case .choose: L("カメラか写真ライブラリを選びます")
        }
    }

    private func start() {
        switch PhotoSourcePolicy.plan(for: mode, cameraAvailable: Self.cameraAvailable) {
        case .camera: openCamera()
        case .library: showLibrary = true
        case .choose: showChooser = true
        }
    }

    private func openCamera() {
        Task { @MainActor in
            #if APP_STORE_REVIEW
            showLibrary = true
            #else
            if await CameraPicker.requestAccess() { showCamera = true } else { showDenied = true }
            #endif
        }
    }
}
