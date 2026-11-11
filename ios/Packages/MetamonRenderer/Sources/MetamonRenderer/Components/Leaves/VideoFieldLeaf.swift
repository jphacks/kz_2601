import SwiftUI
import PhotosUI
import AVKit
import MetamonDSL

/// `videoField`(#206): source(camera|library|both)に応じて動画撮影 / ライブラリ選択 / 二択。
/// 撮影は標準カメラ UI(内カメ・外カメ切替可・上限 maxDuration 秒・中画質)。取り込んだ動画は端末内に保存し、
/// field には `"video:<id>|<サムネイル>"` を入れる(`VideoCodec`)。カメラが無い端末(シミュレータ等)はライブラリに落ちる。
struct VideoFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    @State private var librarySelection: PhotosPickerItem?
    @State private var showLibrary = false
    @State private var showCamera = false
    @State private var showChooser = false
    @State private var showDenied = false
    @State private var saving = false
    @State private var failed = false
    #if os(macOS)
    @State private var showMacChooser = false
    @State private var showFileImporter = false
    @State private var dropTargeted = false
    @State private var dropRejected = false
    #endif
    @Environment(\.openURL) private var openURL

    var body: some View {
        if let form = ctx.form, let field = c.field {
            content(form: form, field: field)
        } else {
            NeedsForm(c: c)
        }
    }

    private var maxDuration: Int { max(1, c.maxDuration ?? VideoCodec.defaultMaxDuration) }

    @ViewBuilder
    private func content(form: FormDraft, field: String) -> some View {
        let value = form.values[field] ?? .null
        let video = VideoValue(value)

        VStack(alignment: .leading, spacing: 8) {
            if saving {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    ProgressView()
                    Text(L("動画を保存しています…")).font(.subheadline).foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, minHeight: 44)
            } else if video != nil {
                VideoPreview(value: value, height: 200)
                HStack(spacing: RenderTheme.Spacing.sm) {
                    Button { start() } label: {
                        Label(picksOnly ? L("選び直す") : L("撮り直す"), systemImage: picksOnly ? "film" : "video").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    .accessibilityHint(sourceHint)
                    Button(role: .destructive) { form.values[field] = .null } label: {
                        Label(L("削除"), systemImage: "trash").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .destructive))
                    .accessibilityLabel(L("\(c.label ?? L("動画"))を削除"))
                }
            } else {
                Button { start() } label: {
                    Label(c.label ?? emptyTitle, systemImage: picksOnly || mode == .library ? "film" : "video")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .accessibilityLabel(c.label ?? emptyTitle)
                .accessibilityHint(sourceHint)
                .accessibilityIdentifier("videoField.\(field)")
            }
            if failed {
                Text(L("動画を保存できませんでした。もう一度お試しください。")).font(.caption).foregroundStyle(.red)
            }
            #if os(macOS)
            if dropRejected {
                Text(L("動画のファイルだけ入れられます")).font(.caption).foregroundStyle(.secondary)
            }
            #endif
        }
        #if os(macOS)
        .padding(dropTargeted ? 4 : 0)
        .overlay {
            if dropTargeted {
                RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous)
                    .strokeBorder(Color.accentColor, lineWidth: 2)
            }
        }
        .dropDestination(for: URL.self) { urls, _ in
            guard let url = urls.first, VideoFileImport.accepts(url) else {
                dropRejected = true
                return false
            }
            dropRejected = false
            ingest(url, origin: .userFile, form: form, field: field)
            return true
        } isTargeted: { dropTargeted = $0 }
        .confirmationDialog(c.label ?? L("動画"), isPresented: $showMacChooser, titleVisibility: .hidden) {
            Button(L("写真ライブラリから選ぶ")) { showLibrary = true }
            Button(L("ファイルから選ぶ")) { showFileImporter = true }
            Button(L("キャンセル"), role: .cancel) {}
        }
        .fileImporter(isPresented: $showFileImporter, allowedContentTypes: VideoFileImport.contentTypes, allowsMultipleSelection: false) { result in
            guard case .success(let urls) = result, let url = urls.first else { return }
            dropRejected = false
            ingest(url, origin: .userFile, form: form, field: field)
        }
        #endif
        #if !APP_STORE_REVIEW
        .confirmationDialog(c.label ?? L("動画"), isPresented: $showChooser, titleVisibility: .hidden) {
            Button(L("撮影")) { openCamera() }
            Button(L("ライブラリから選ぶ")) { showLibrary = true }
            Button(L("キャンセル"), role: .cancel) {}
        }
        .platformFullScreenCover(isPresented: $showCamera) {
            VideoCameraPicker(maxDuration: TimeInterval(maxDuration)) { url in
                ingest(url, origin: .temporary, form: form, field: field)
            }
            .ignoresSafeArea()
        }
        #endif
        .photosPicker(isPresented: $showLibrary, selection: $librarySelection, matching: .videos)
        .onChange(of: librarySelection) { _, item in
            guard let item else { return }
            saving = true
            Task {
                if let movie = try? await item.loadTransferable(type: PickedMovie.self) {
                    ingest(movie.url, origin: .temporary, form: form, field: field)
                } else {
                    saving = false
                    failed = true
                }
                librarySelection = nil
            }
        }
        #if !APP_STORE_REVIEW
        .alert(L("カメラを使えません"), isPresented: $showDenied) {
            Button(L("設定を開く")) {
                if let url = PlatformSystem.appSettingsURL { openURL(url) }
            }
            Button(L("ライブラリから選ぶ")) { showLibrary = true }
            Button(L("キャンセル"), role: .cancel) {}
        } message: {
            Text(L("設定アプリで Tailor It のカメラへのアクセスを許可してください。"))
        }
        #endif
    }

    /// 一時ファイル(撮影・写真ライブラリ)は取り込んだら消す。ユーザーのファイル(Mac のファイル選択・ドロップ)は読むだけ。
    private func ingest(_ url: URL, origin: VideoFileImport.Origin, form: FormDraft, field: String) {
        saving = true
        failed = false
        Task { @MainActor in
            defer { saving = false }
            do {
                let v = try await VideoFileImport.ingest(url, origin: origin, maxDuration: maxDuration)
                form.values[field] = .string(v.encoded)
            } catch {
                print("[Metamon] video ingest failed: \(error)")
                failed = true
            }
        }
    }

    private var mode: PhotoSourceMode { PhotoSourceMode(raw: c.source) }

    /// 審査用ビルド(APP_STORE_REVIEW・#245)は撮影をバイナリから外し、常にライブラリから選ぶ。
    private static var cameraAvailable: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        VideoCameraPicker.isAvailable
        #endif
    }

    private var plan: PhotoCapturePlan { PhotoSourcePolicy.plan(for: mode, cameraAvailable: Self.cameraAvailable) }

    /// Mac で撮れない時(#1379)は「選ぶ」の文言・アイコンにする。iOS は今のまま。
    private var picksOnly: Bool {
        #if os(macOS)
        plan == .library
        #else
        false
        #endif
    }

    private var emptyTitle: String { picksOnly ? L("動画を選ぶ") : L("動画を撮る / 選ぶ") }

    private var sourceHint: String {
        #if os(macOS)
        if plan == .library { return L("写真ライブラリかファイルから動画を選びます") }
        #endif
        return switch plan {
        case .camera: L("カメラで動画を撮ります")
        case .library: L("ライブラリから動画を選びます")
        case .choose: L("動画を撮るかライブラリから選びます")
        }
    }

    private func start() {
        switch plan {
        case .camera: openCamera()
        case .library:
            #if os(macOS)
            showMacChooser = true
            #else
            showLibrary = true
            #endif
        case .choose: showChooser = true
        }
    }

    private func openCamera() {
        Task { @MainActor in
            #if APP_STORE_REVIEW
            showLibrary = true
            #else
            if await VideoCameraPicker.requestAccess() { showCamera = true } else { showDenied = true }
            #endif
        }
    }
}

/// `videoPlayer`(#206): フォーム / 詳細画面で video field を再生する。height(small|medium|large)。
struct VideoPlayerLeaf: View {
    let c: Component
    let ctx: RenderContext

    private var height: CGFloat {
        switch c.height { case "small": 140; case "large": 320; default: 220 }
    }

    var body: some View {
        let value = c.field.flatMap { ctx.form?.values[$0] } ?? .null
        VideoPreview(value: value, height: height)
    }
}

/// 動画の枡。この端末にファイルがあれば AVKit で再生、無ければサムネイル +「この端末にはありません」。
struct VideoPreview: View {
    let value: JSONValue
    var height: CGFloat
    var store: LocalVideoStore = .shared

    @State private var player: AVPlayer?

    private var video: VideoValue? { VideoValue(value) }

    private var playableURL: URL? {
        guard let video else { return nil }
        if let remote = video.remoteURL { return remote }
        guard let id = video.id, store.exists(id) else { return nil }
        return store.url(for: id)
    }

    var body: some View {
        Group {
            if let url = playableURL {
                playerView
                    .onAppear { if player == nil { player = AVPlayer(url: url) } }
                    .onChange(of: url) { _, new in player = AVPlayer(url: new) }
                    .onDisappear { player?.pause() }
                    .accessibilityLabel(L("動画"))
            } else if video == nil {
                Rectangle().fill(.quaternary)
                    .overlay(Image(systemName: "video.slash").font(.title2).foregroundStyle(.secondary))
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(L("動画なし"))
            } else {
                PhotoView(value: value, height: height, accessibilityLabel: L("この端末にはありません"))
                    .overlay(alignment: .bottom) {
                        if video != nil {
                            Label(L("この端末にはありません"), systemImage: PlatformSystem.notOnDeviceSymbol)
                                .font(.caption.weight(.semibold))
                                .padding(.horizontal, 10).padding(.vertical, 6)
                                .background(.ultraThinMaterial, in: Capsule())
                                .padding(8)
                        }
                    }
            }
        }
        .frame(height: height)
        .frame(maxWidth: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
    }

    /// Mac は全画面のボタン・しまった時に止める `AVPlayerView`(#1340)。iOS は SwiftUI の `VideoPlayer`
    @ViewBuilder
    private var playerView: some View {
        #if os(macOS)
        MacVideoPlayerView(player: player)
        #else
        VideoPlayer(player: player)
        #endif
    }
}
