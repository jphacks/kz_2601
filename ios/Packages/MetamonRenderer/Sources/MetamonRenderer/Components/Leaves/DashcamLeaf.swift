// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(カメラでの撮影)。
#if !APP_STORE_REVIEW
import SwiftUI
import AVFoundation
import AVKit
import MetamonDSL

/// `dashcam`(#410): 背面カメラのプレビュー + 録画の開始/停止 + 保存した映像の一覧。
/// 1 分ごとの区間で録画し、直近 `keepMinutes` 分だけ残す(ループ)。衝撃・「今の映像を保存」で前後を保存済みにする。
/// `autoStart` の条件が続いたら自動で開始。録画中は画面のスリープを止める。
/// iOS は裏に回るとカメラを止めるので、画面を出している間だけ録画する(その旨を常に表示する)。
/// 録画そのものは `DashcamRecorder`、残す区間・衝撃・自動開始の判定は MetamonDSL の `DashcamLoop` などの純粋ロジック。
struct DashcamLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.openURL) private var openURL
    let c: Component
    let ctx: RenderContext

    @State private var recorder: DashcamRecorder?
    @State private var showClips = false

    private var config: DashcamConfig { DashcamConfig(c) }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            header
            preview
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .frame(minHeight: 120)
            controls
            footnote
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .renderCard()
        .sensoryFeedback(.warning, trigger: recorder?.impactCount ?? 0)
        .task {
            let r = recorder ?? DashcamRecorder(config: config, store: DashcamStore(appName: rt.dsl.app.name))
            recorder = r
            await r.appear()
        }
        .onDisappear { recorder?.disappear() }
        .onChange(of: scenePhase) { _, p in
            switch p {
            case .background: recorder?.enterBackground()
            case .active: recorder?.enterForeground()
            default: break
            }
        }
        .sheet(isPresented: $showClips) {
            if let recorder { DashcamClipsSheet(recorder: recorder) }
        }
    }

    // MARK: - parts

    private var header: some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            Text(c.title ?? L("ドラレコ"))
                .font(RenderTheme.Fonts.sectionTitle)
                .foregroundStyle(.secondary)
                .accessibilityAddTraits(.isHeader)
            Spacer(minLength: 4)
            status
        }
    }

    @ViewBuilder
    private var status: some View {
        if let recorder, recorder.isRecording, let since = recorder.recordingSince {
            TimelineView(.periodic(from: .now, by: 1)) { tl in
                HStack(spacing: 6) {
                    Circle().fill(.red).frame(width: 8, height: 8).accessibilityHidden(true)
                    Text(L("録画中")).font(.subheadline.weight(.semibold))
                    Text(Self.clock(tl.date.timeIntervalSince(since))).font(.subheadline.monospacedDigit())
                }
                .accessibilityElement(children: .combine)
            }
            .accessibilityIdentifier("dashcam.recording")
        } else if let left = recorder?.autoStartIn {
            Text(L("あと\(Int(left))秒で自動で録画")).font(.caption.weight(.semibold)).foregroundStyle(RenderTheme.Colors.brandText)
        }
    }

    @ViewBuilder
    private var preview: some View {
        let shape = RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous)
        ZStack {
            Color.black
            switch recorder?.phase {
            case .ready?, .recording?:
                if let recorder { DashcamPreview(session: recorder.capture.session, device: recorder.capture.device) }
            case .unavailable?:
                placeholder("video.slash", L("カメラが使えないため録画できません(実機のみ)"))
            case .denied?:
                placeholder("video.slash", L("カメラの許可がないため録画できません"))
            default:
                ProgressView().tint(.white)
            }
            if let at = recorder?.lastImpact, recorder?.isRecording == true {
                VStack {
                    Spacer()
                    Label(L("\(at.formatted(date: .omitted, time: .standard)) 前後の映像を保存しました"), systemImage: "exclamationmark.shield.fill")
                        .font(.caption.weight(.semibold))
                        .padding(.horizontal, 10).padding(.vertical, 6)
                        .background(.ultraThinMaterial, in: Capsule())
                        .padding(8)
                }
            }
        }
        .clipShape(shape)
        .accessibilityIdentifier("dashcam.preview")
    }

    private func placeholder(_ icon: String, _ text: String) -> some View {
        VStack(spacing: 8) {
            Image(systemName: icon).font(.title)
            Text(text).font(.footnote).multilineTextAlignment(.center)
        }
        .foregroundStyle(.white.opacity(0.85))
        .padding()
    }

    private var controls: some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            if recorder?.isRecording == true {
                Button(L("停止"), systemImage: "stop.fill") { recorder?.stop() }
                    .buttonStyle(RenderButtonStyle(kind: .destructive))
                    .accessibilityIdentifier("dashcam.stop")
                Button(L("今の映像を保存"), systemImage: "exclamationmark.shield") { recorder?.saveNow() }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    .accessibilityIdentifier("dashcam.saveNow")
            } else {
                Button(L("録画を開始"), systemImage: "record.circle") { recorder?.start() }
                    .buttonStyle(RenderButtonStyle(kind: .primary))
                    .disabled(recorder?.phase != .ready)
                    .accessibilityIdentifier("dashcam.start")
            }
            Button { showClips = true } label: {
                Label(L("保存した映像(\(recorder?.clips.count ?? 0))"), systemImage: "film.stack")
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .accessibilityIdentifier("dashcam.clips")
        }
        .lineLimit(1)
        .minimumScaleFactor(0.7)
    }

    @ViewBuilder
    private var footnote: some View {
        VStack(alignment: .leading, spacing: 4) {
            switch recorder?.phase {
            case .denied?:
                Button(L("設定を開く")) {
                    if let url = URL(string: "app-settings:") { openURL(url) }
                }
                .font(.caption.weight(.semibold))
            case .full(let deviceLow)?:
                Text(deviceLow ? L("端末の空きが少ないため録画を止めました") : L("保存した映像で容量の上限に達したため録画を止めました。いらない映像を消してください"))
                    .font(.caption).foregroundStyle(RenderTheme.Colors.negative)
            default:
                EmptyView()
            }
            if let notice = recorder?.notice {
                Text(notice).font(.caption).foregroundStyle(RenderTheme.Colors.warning)
            }
            Text(Self.footnote(config))
                .font(.caption).foregroundStyle(.secondary)
                .accessibilityIdentifier("dashcam.notice")
        }
    }

    // MARK: - text

    /// 常に出す説明: 画面を出している間だけ録画すること・残す長さ・自動開始の条件。
    static func footnote(_ config: DashcamConfig) -> String {
        let minutes = Int(config.keepSeconds / 60)
        var s = L("画面を出している間だけ録画します(アプリを閉じる・画面を消すと止まります)。直近\(minutes)分を残し、衝撃の前後は保存します。")
        if let cond = conditionText(config.autoStart) {
            s += L("\(cond)が5秒続くと自動で録画を始めます。")
        }
        return s
    }

    /// 自動開始の条件の言葉(条件なし = nil)。
    static func conditionText(_ conditions: Set<DashcamConfig.Condition>) -> String? {
        guard !conditions.isEmpty else { return nil }
        let words: [String] = DashcamConfig.Condition.allCases.filter(conditions.contains).map {
            switch $0 {
            case .landscape: L("横向き")
            case .still: L("ほぼ静止")
            case .charging: L("充電中")
            }
        }
        return words.joined(separator: "・")
    }

    static func clock(_ seconds: TimeInterval) -> String {
        let s = Int(max(0, seconds))
        return s >= 3600 ? String(format: "%d:%02d:%02d", s / 3600, s / 60 % 60, s % 60) : String(format: "%02d:%02d", s / 60, s % 60)
    }
}

/// 背面カメラのプレビュー。向きは `RotationCoordinator` に合わせる(画面の向きを固定していても水平に見える)。
private struct DashcamPreview: UIViewRepresentable {
    let session: AVCaptureSession
    let device: AVCaptureDevice?

    func makeUIView(context: Context) -> PreviewView {
        let v = PreviewView()
        v.previewLayer.session = session
        v.previewLayer.videoGravity = .resizeAspectFill
        if let device { v.follow(device) }
        return v
    }

    func updateUIView(_ uiView: PreviewView, context: Context) {}

    final class PreviewView: UIView {
        override class var layerClass: AnyClass { AVCaptureVideoPreviewLayer.self }
        var previewLayer: AVCaptureVideoPreviewLayer { layer as! AVCaptureVideoPreviewLayer } // swiftlint:disable:this force_cast
        private var coordinator: AVCaptureDevice.RotationCoordinator?
        private var observation: NSKeyValueObservation?

        func follow(_ device: AVCaptureDevice) {
            let c = AVCaptureDevice.RotationCoordinator(device: device, previewLayer: previewLayer)
            coordinator = c
            observation = c.observe(\.videoRotationAngleForHorizonLevelPreview, options: [.initial, .new]) { [weak self] c, _ in
                let angle = c.videoRotationAngleForHorizonLevelPreview
                DispatchQueue.main.async {
                    if let conn = self?.previewLayer.connection, conn.isVideoRotationAngleSupported(angle) { conn.videoRotationAngle = angle }
                }
            }
        }
    }
}

/// 保存した映像の一覧: 再生・写真に保存・共有・削除。
private struct DashcamClipsSheet: View {
    let recorder: DashcamRecorder
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Group {
                if recorder.clips.isEmpty {
                    ContentUnavailableView(L("保存した映像はまだありません"), systemImage: "film.stack",
                                           description: Text(L("衝撃があった時や「今の映像を保存」を押した時の前後が残ります")))
                } else {
                    List {
                        ForEach(recorder.clips) { clip in
                            NavigationLink {
                                DashcamClipView(recorder: recorder, clip: clip)
                            } label: {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(clip.start.formatted(date: .abbreviated, time: .standard))
                                    Text("\(DashcamLeaf.clock(clip.duration)) ・ \(ByteCountFormatter.string(fromByteCount: clip.bytes, countStyle: .file))")
                                        .font(.caption).foregroundStyle(.secondary)
                                }
                            }
                        }
                        .onDelete { idx in idx.map { recorder.clips[$0] }.forEach(recorder.delete) }
                    }
                }
            }
            .navigationTitle(L("保存した映像"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button(L("閉じる")) { dismiss() } } }
        }
    }
}

private struct DashcamClipView: View {
    let recorder: DashcamRecorder
    let clip: DashcamSegment
    @State private var player: AVPlayer?
    @State private var savedToPhotos: Bool?
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        let url = recorder.url(for: clip)
        VStack(spacing: RenderTheme.Spacing.md) {
            VideoPlayer(player: player)
                .aspectRatio(16 / 9, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
            HStack(spacing: RenderTheme.Spacing.sm) {
                Button(L("写真に保存"), systemImage: "square.and.arrow.down") {
                    Task { savedToPhotos = await recorder.saveToPhotos(clip) }
                }
                .buttonStyle(RenderButtonStyle(kind: .primary))
                ShareLink(item: url) { Label(L("共有"), systemImage: "square.and.arrow.up") }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
            }
            if let ok = savedToPhotos {
                Text(ok ? L("写真に保存しました") : L("写真に保存できませんでした(写真への追加を許可してください)"))
                    .font(.caption).foregroundStyle(ok ? RenderTheme.Colors.positive : RenderTheme.Colors.negative)
            }
            Spacer()
        }
        .padding()
        .navigationTitle(clip.start.formatted(date: .omitted, time: .standard))
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button(L("削除"), systemImage: "trash", role: .destructive) {
                    recorder.delete(clip)
                    dismiss()
                }
            }
        }
        .onAppear { player = AVPlayer(url: url) }
        .onDisappear { player?.pause() }
    }
}
#endif
