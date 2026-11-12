import SwiftUI
import AVFoundation
import MetamonDSL

/// `qrScanner`: AVFoundation で QR / バーコードを読み、文字列をフォームの string field に保存する。
/// 実機専用(カメラ権限 `NSCameraUsageDescription` は Info.plist 設定済み)。シミュレータはプレースホルダ + 手入力。
/// Mac(#1045)はカメラ(`MacCameraSheet`)と、画像ファイル・貼り付け・ドロップの画像(Vision)から読む。手入力の欄も残す。
struct QrScannerLeaf: View {
    let c: Component
    let ctx: RenderContext
    @State private var showScanner = false

    var body: some View {
        if let form = ctx.form, let field = c.field {
            VStack(alignment: .leading, spacing: 4) {
                #if os(macOS)
                MacQrReader(c: c, form: form, field: field)
                #elseif targetEnvironment(simulator)
                HStack {
                    Label(c.label ?? L("QRを読み取る"), systemImage: "qrcode.viewfinder").foregroundStyle(.secondary)
                    Spacer()
                    Text(L("実機のみ")).font(.caption2).foregroundStyle(.secondary)
                }
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RenderTheme.Colors.surface, in: RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
                TextField(L("シミュレータ用: 読み取り結果を手入力"), text: form.string(field))
                    .textFieldStyle(.roundedBorder)
                    .font(.caption)
                #else
                Button {
                    showScanner = true
                } label: {
                    Label(c.label ?? L("QRを読み取る"), systemImage: "qrcode.viewfinder").frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .sheet(isPresented: $showScanner) {
                    NavigationStack {
                        QrCameraView { code in
                            form.values[field] = .string(code)
                            showScanner = false
                        }
                        .ignoresSafeArea()
                        .navigationTitle(c.label ?? L("QRを読み取る"))
                        .inlineNavigationTitle()
                        .toolbar { SheetCloseToolbarItem { showScanner = false } }
                        .aiButtonLayer()
                    }
                    .aiButtonTarget(c) // #629
                }
                #endif
                if let s = (form.values[field] ?? .null).stringValue, !s.isEmpty {
                    Text(s).font(.caption).foregroundStyle(.secondary).lineLimit(2)
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}

#if os(iOS) && !targetEnvironment(simulator)
/// AVCaptureSession + メタデータ出力でコードを 1 回検出して返す。
struct QrCameraView: UIViewControllerRepresentable {
    let onFound: (String) -> Void

    func makeUIViewController(context: Context) -> QrCameraController {
        let vc = QrCameraController()
        vc.onFound = onFound
        return vc
    }

    func updateUIViewController(_ uiViewController: QrCameraController, context: Context) {}
}

final class QrCameraController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
    var onFound: ((String) -> Void)?
    private let session = AVCaptureSession()
    private var found = false
    /// 端末の向きに合わせてプレビューを回す(#625: 横向きで映像が 90° 倒れて見えた)。DashcamLeaf と同じ
    private var rotation: AVCaptureDevice.RotationCoordinator?
    private var rotationObservation: NSKeyValueObservation?

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black
        guard let device = AVCaptureDevice.default(for: .video),
              let input = try? AVCaptureDeviceInput(device: device),
              session.canAddInput(input) else { return }
        session.addInput(input)
        let output = AVCaptureMetadataOutput()
        guard session.canAddOutput(output) else { return }
        session.addOutput(output)
        output.setMetadataObjectsDelegate(self, queue: .main)
        output.metadataObjectTypes = [.qr, .ean13, .ean8, .code128, .code39, .pdf417, .aztec, .dataMatrix]
        let layer = AVCaptureVideoPreviewLayer(session: session)
        layer.videoGravity = .resizeAspectFill
        layer.frame = view.bounds
        view.layer.addSublayer(layer)
        let coordinator = AVCaptureDevice.RotationCoordinator(device: device, previewLayer: layer)
        rotation = coordinator
        rotationObservation = coordinator.observe(\.videoRotationAngleForHorizonLevelPreview, options: [.initial, .new]) { [weak layer] c, _ in
            let angle = c.videoRotationAngleForHorizonLevelPreview
            DispatchQueue.main.async {
                if let conn = layer?.connection, conn.isVideoRotationAngleSupported(angle) { conn.videoRotationAngle = angle }
            }
        }
    }

    override func viewWillLayoutSubviews() {
        super.viewWillLayoutSubviews()
        view.layer.sublayers?.first?.frame = view.bounds
    }

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        let session = self.session
        Task.detached {
            switch AVCaptureDevice.authorizationStatus(for: .video) {
            case .notDetermined:
                if await AVCaptureDevice.requestAccess(for: .video) { session.startRunning() }
            case .authorized:
                session.startRunning()
            default:
                break
            }
        }
    }

    override func viewWillDisappear(_ animated: Bool) {
        super.viewWillDisappear(animated)
        let session = self.session
        Task.detached { session.stopRunning() }
    }

    func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput metadataObjects: [AVMetadataObject], from connection: AVCaptureConnection) {
        guard !found, let code = metadataObjects.compactMap({ ($0 as? AVMetadataMachineReadableCodeObject)?.stringValue }).first else { return }
        found = true
        PlatformSystem.successHaptic()
        onFound?(code)
    }
}
#endif

#if os(macOS)
/// Mac の読み取り(#1045): カメラ・画像ファイル・貼り付け・ドロップ。どれも読めなければ手入力。
private struct MacQrReader: View {
    let c: Component
    let form: FormDraft
    let field: String
    @State private var showCamera = false
    @State private var showImporter = false
    @State private var dropTargeted = false
    @State private var notFound = false

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: RenderTheme.Spacing.sm) {
                if MacCameraSession.hasCamera {
                    Button { showCamera = true } label: {
                        Label(c.label ?? L("QRを読み取る"), systemImage: "qrcode.viewfinder").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    .help(L("Mac のカメラに QR コードをかざして読み取ります"))
                }
                Button { showImporter = true } label: {
                    Label(L("画像から読み取る"), systemImage: "photo").frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .help(L("QR コードが写った画像ファイルを選んで読み取ります。画像をここにドロップしても読めます"))
                Button { readPasteboard() } label: {
                    Label(L("貼り付け"), systemImage: "doc.on.clipboard")
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .help(L("コピーした画像から QR コードを読み取ります"))
            }
            if notFound {
                Text(L("QR コードが見つかりませんでした")).font(.caption).foregroundStyle(.secondary)
            }
            TextField(L("読み取り結果を手入力"), text: form.string(field))
                .textFieldStyle(.roundedBorder)
                .font(.caption)
                .help(L("読み取れない時は、ここに直接入力できます"))
        }
        .padding(dropTargeted ? 4 : 0)
        .overlay {
            if dropTargeted {
                RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous)
                    .strokeBorder(Color.accentColor, lineWidth: 2)
            }
        }
        .dropDestination(for: URL.self) { urls, _ in
            guard let url = urls.first else { return false }
            apply(QrImageDecoder.decode(fileAt: url))
            return true
        } isTargeted: { dropTargeted = $0 }
        .fileImporter(isPresented: $showImporter, allowedContentTypes: [.image], allowsMultipleSelection: false) { result in
            guard case .success(let urls) = result, let url = urls.first else { return }
            apply(QrImageDecoder.decode(fileAt: url))
        }
        .sheet(isPresented: $showCamera) {
            MacCameraSheet(mode: .qr, onCode: { apply($0) })
                .aiButtonTarget(c) // #629
        }
    }

    private func readPasteboard() {
        apply(PlatformPasteboard.image()?.cgImage.flatMap(QrImageDecoder.decode))
    }

    private func apply(_ code: String?) {
        if let code {
            form.values[field] = .string(code)
            notFound = false
        } else {
            notFound = true
        }
    }
}
#endif
