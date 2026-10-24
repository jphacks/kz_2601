import SwiftUI
import AVFoundation
import MetamonDSL

/// `qrScanner`: AVFoundation で QR / バーコードを読み、文字列をフォームの string field に保存する。
/// 実機専用(カメラ権限 `NSCameraUsageDescription` は Info.plist 設定済み)。シミュレータはプレースホルダ + 手入力。
struct QrScannerLeaf: View {
    let c: Component
    let ctx: RenderContext
    @State private var showScanner = false

    var body: some View {
        if let form = ctx.form, let field = c.field {
            VStack(alignment: .leading, spacing: 4) {
                #if targetEnvironment(simulator)
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
                        .navigationBarTitleDisplayMode(.inline)
                        .toolbar { ToolbarItem(placement: .cancellationAction) { Button(L("閉じる")) { showScanner = false } } }
                    }
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

#if !targetEnvironment(simulator)
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
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        onFound?(code)
    }
}
#endif
