#if os(macOS)
import SwiftUI
import AppKit
@preconcurrency import AVFoundation
import AVKit
import Vision
import UniformTypeIdentifiers

// Mac のカメラ(#1043・#1045)。UIImagePickerController が無いので AVCaptureSession で撮る・読む。
// 流れ(撮る → 撮り直す / 使う)は `MacCaptureFlow`、カメラそのものは `MacCaptureDriving`(テストは偽物を渡す)。

/// 画像から QR / バーコードを読む(Vision)。カメラの無い Mac でも画像ファイル・貼り付け・ドロップから使う。
enum QrImageDecoder {
    /// iOS の `QrCameraController` と同じ種類
    static let symbologies: [VNBarcodeSymbology] = [.qr, .ean13, .ean8, .code128, .code39, .pdf417, .aztec, .dataMatrix]

    static func decode(_ image: CGImage) -> String? {
        perform(VNImageRequestHandler(cgImage: image, options: [:]))
    }

    static func decode(_ buffer: CVPixelBuffer) -> String? {
        perform(VNImageRequestHandler(cvPixelBuffer: buffer, options: [:]))
    }

    static func decode(data: Data) -> String? {
        NSImage(data: data)?.cgImage.flatMap(decode)
    }

    static func decode(fileAt url: URL) -> String? {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        return (try? Data(contentsOf: url)).flatMap { decode(data: $0) }
    }

    private static func perform(_ handler: VNImageRequestHandler) -> String? {
        let request = VNDetectBarcodesRequest()
        request.symbologies = symbologies
        try? handler.perform([request])
        return request.results?.compactMap(\.payloadStringValue).first { !$0.isEmpty }
    }
}

/// 撮る物
enum MacCaptureMode: Equatable {
    case photo
    /// 上限の秒数(iOS の `videoMaximumDuration` と同じ)
    case video(maxDuration: TimeInterval)
    /// QR / バーコードを 1 回読んだら返す
    case qr
}

struct MacCaptureDevice: Identifiable, Equatable {
    let id: String
    let name: String
}

enum MacCaptureError: Error, Equatable {
    case noCamera
    case denied
    case failed
}

/// カメラそのもの。本物は `MacCameraSession`、テストは偽物。
@MainActor
protocol MacCaptureDriving: AnyObject {
    var devices: [MacCaptureDevice] { get }
    /// QR を見つけた時(`.qr` の時だけ呼ぶ)
    var onCode: ((String) -> Void)? { get set }
    func start(mode: MacCaptureMode, deviceID: String?) async throws
    func stop()
    func capturePhoto() async throws -> PlatformImage
    /// 録り終わる(`stopRecording()` か上限の秒数)まで待ってファイルを返す
    func record(maxDuration: TimeInterval) async throws -> URL
    func stopRecording()
}

/// 撮る sheet の状態。starting → live →(写真)capturing → reviewPhoto /(動画)recording → reviewVideo。
enum MacCapturePhase: Equatable {
    case starting
    case live
    case capturing
    case recording
    case reviewPhoto
    case reviewVideo
    /// カメラが無い・許可されていない
    case unavailable(MacCaptureError)
    case failed
}

@MainActor
@Observable
final class MacCaptureFlow {
    let mode: MacCaptureMode
    private let driver: MacCaptureDriving
    private(set) var phase: MacCapturePhase = .starting
    private(set) var photo: PlatformImage?
    private(set) var video: URL?
    private(set) var code: String?
    private(set) var deviceID: String?

    init(mode: MacCaptureMode, driver: MacCaptureDriving) {
        self.mode = mode
        self.driver = driver
    }

    var devices: [MacCaptureDevice] { driver.devices }

    func start() async {
        phase = .starting
        if mode == .qr {
            driver.onCode = { [weak self] in self?.found($0) }
        }
        do {
            try await driver.start(mode: mode, deviceID: deviceID)
            phase = .live
        } catch let e as MacCaptureError where e != .failed {
            phase = .unavailable(e)
        } catch {
            phase = .failed
        }
    }

    func select(device id: String) async {
        guard id != deviceID, phase == .live || phase == .failed else { return }
        deviceID = id
        driver.stop()
        await start()
    }

    /// シャッター。写真は撮って確かめる画面へ、動画は録画の開始 / 停止。
    func shutter() async {
        switch (mode, phase) {
        case (.photo, .live):
            phase = .capturing
            do {
                photo = try await driver.capturePhoto()
                phase = .reviewPhoto
                driver.stop()
            } catch {
                phase = .failed
            }
        case (.video(let max), .live):
            phase = .recording
            do {
                video = try await driver.record(maxDuration: max)
                phase = .reviewVideo
                driver.stop()
            } catch {
                phase = .failed
            }
        case (.video, .recording):
            driver.stopRecording()
        default:
            break
        }
    }

    /// 撮り直す(撮った動画の一時ファイルは消す)
    func retake() async {
        guard phase == .reviewPhoto || phase == .reviewVideo || phase == .failed else { return }
        discardVideo()
        photo = nil
        await start()
    }

    /// QR を 1 回だけ受け取る
    func found(_ value: String) {
        guard mode == .qr, code == nil, phase == .live, !value.isEmpty else { return }
        code = value
        driver.stop()
    }

    /// 閉じる時。使わなかった動画は消す。
    func cancel() {
        driver.stop()
        if phase == .recording { driver.stopRecording() }
        discardVideo()
    }

    /// 「使う」で渡した動画は取り込み側(`VideoFileImport`・.temporary)が消すので、ここでは手放すだけ。
    func takeVideo() -> URL? {
        defer { video = nil }
        return video
    }

    private func discardVideo() {
        if let video { try? FileManager.default.removeItem(at: video) }
        video = nil
    }
}

/// 本物のカメラ(AVCaptureSession)。内蔵・連係カメラ・外付けから選べる。
@MainActor
final class MacCameraSession: NSObject, MacCaptureDriving {
    let session = AVCaptureSession()
    private let queue = DispatchQueue(label: "jp.kz2601.metamon.mac-camera")
    private let photoOutput = AVCapturePhotoOutput()
    private let movieOutput = AVCaptureMovieFileOutput()
    private let frameOutput = AVCaptureVideoDataOutput()
    private let frameReader = QrFrameReader()
    private var photoContinuation: CheckedContinuation<PlatformImage, Error>?
    private var recordContinuation: CheckedContinuation<URL, Error>?
    var onCode: ((String) -> Void)?

    private static var discovery: AVCaptureDevice.DiscoverySession {
        AVCaptureDevice.DiscoverySession(deviceTypes: [.builtInWideAngleCamera, .continuityCamera, .external],
                                         mediaType: .video, position: .unspecified)
    }

    /// この Mac にカメラがあるか(無ければ撮影のボタンを出さない)
    static var hasCamera: Bool { !discovery.devices.isEmpty }

    var devices: [MacCaptureDevice] {
        Self.discovery.devices.map { MacCaptureDevice(id: $0.uniqueID, name: $0.localizedName) }
    }

    func start(mode: MacCaptureMode, deviceID: String?) async throws {
        let all = Self.discovery.devices
        guard let device = all.first(where: { $0.uniqueID == deviceID }) ?? AVCaptureDevice.default(for: .video) ?? all.first else {
            throw MacCaptureError.noCamera
        }
        guard await CameraPicker.requestAccess() else { throw MacCaptureError.denied }
        let input = try AVCaptureDeviceInput(device: device)
        var audio: AVCaptureDeviceInput?
        if case .video = mode, AVCaptureDevice.authorizationStatus(for: .audio) == .authorized,
           let mic = AVCaptureDevice.default(for: .audio) {
            audio = try? AVCaptureDeviceInput(device: mic)
        }
        frameReader.onCode = { @Sendable [weak self] code in
            Task { @MainActor in self?.onCode?(code) }
        }
        let session = session, photoOutput = photoOutput, movieOutput = movieOutput, frameOutput = frameOutput, frameReader = frameReader
        let ok: Bool = await withCheckedContinuation { cont in
            queue.async {
                session.beginConfiguration()
                session.inputs.forEach(session.removeInput)
                session.outputs.forEach(session.removeOutput)
                // 動画は iOS の `.typeMedium` に合わせて中程度の画質
                session.sessionPreset = mode == .photo ? .photo : (mode == .qr ? .high : .medium)
                var ok = session.canAddInput(input)
                if ok { session.addInput(input) }
                if let audio, session.canAddInput(audio) { session.addInput(audio) }
                let output: AVCaptureOutput
                switch mode {
                case .photo: output = photoOutput
                case .video(let max):
                    movieOutput.maxRecordedDuration = CMTime(seconds: max, preferredTimescale: 600)
                    output = movieOutput
                case .qr:
                    frameOutput.alwaysDiscardsLateVideoFrames = true
                    frameOutput.setSampleBufferDelegate(frameReader, queue: frameReader.queue)
                    output = frameOutput
                }
                ok = ok && session.canAddOutput(output)
                if ok { session.addOutput(output) }
                session.commitConfiguration()
                if ok { session.startRunning() }
                cont.resume(returning: ok)
            }
        }
        if !ok { throw MacCaptureError.failed }
    }

    func stop() {
        let session = session
        queue.async { if session.isRunning { session.stopRunning() } }
    }

    func capturePhoto() async throws -> PlatformImage {
        try await withCheckedThrowingContinuation { cont in
            photoContinuation = cont
            photoOutput.capturePhoto(with: AVCapturePhotoSettings(), delegate: self)
        }
    }

    func record(maxDuration: TimeInterval) async throws -> URL {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("mov")
        return try await withCheckedThrowingContinuation { cont in
            recordContinuation = cont
            movieOutput.startRecording(to: url, recordingDelegate: self)
        }
    }

    func stopRecording() {
        if movieOutput.isRecording { movieOutput.stopRecording() }
    }

    fileprivate func finishPhoto(_ result: Result<PlatformImage, Error>) {
        photoContinuation?.resume(with: result)
        photoContinuation = nil
    }

    fileprivate func finishRecording(_ result: Result<URL, Error>) {
        recordContinuation?.resume(with: result)
        recordContinuation = nil
    }
}

extension MacCameraSession: AVCapturePhotoCaptureDelegate, AVCaptureFileOutputRecordingDelegate {
    nonisolated func photoOutput(_ output: AVCapturePhotoOutput, didFinishProcessingPhoto photo: AVCapturePhoto, error: Error?) {
        let data = photo.fileDataRepresentation()
        Task { @MainActor in
            if let data, let image = NSImage(data: data) {
                finishPhoto(.success(image))
            } else {
                finishPhoto(.failure(error ?? MacCaptureError.failed))
            }
        }
    }

    nonisolated func fileOutput(_ output: AVCaptureFileOutput, didFinishRecordingTo outputFileURL: URL, from connections: [AVCaptureConnection], error: Error?) {
        // 上限の秒数で止まった時も error が入る(録れたファイルは使える)
        let finished = (error as NSError?)?.userInfo[AVErrorRecordingSuccessfullyFinishedKey] as? Bool
        let ok = error == nil || finished == true
        Task { @MainActor in
            finishRecording(ok ? .success(outputFileURL) : .failure(error ?? MacCaptureError.failed))
        }
    }
}

/// QR を読む時のフレーム。毎フレームは重いので 0.25 秒おきに Vision へ。
private final class QrFrameReader: NSObject, AVCaptureVideoDataOutputSampleBufferDelegate, @unchecked Sendable {
    let queue = DispatchQueue(label: "jp.kz2601.metamon.mac-qr")
    var onCode: (@Sendable (String) -> Void)?
    private var last: CFAbsoluteTime = 0

    func captureOutput(_ output: AVCaptureOutput, didOutput sampleBuffer: CMSampleBuffer, from connection: AVCaptureConnection) {
        let now = CFAbsoluteTimeGetCurrent()
        guard now - last > 0.25, let buffer = CMSampleBufferGetImageBuffer(sampleBuffer) else { return }
        last = now
        if let code = QrImageDecoder.decode(buffer) { onCode?(code) }
    }
}

/// プレビュー(AVCaptureVideoPreviewLayer)
struct MacCameraPreview: NSViewRepresentable {
    let session: AVCaptureSession

    func makeNSView(context: Context) -> NSView {
        let view = NSView()
        view.wantsLayer = true
        let layer = AVCaptureVideoPreviewLayer(session: session)
        layer.videoGravity = .resizeAspect
        layer.autoresizingMask = [.layerWidthSizable, .layerHeightSizable]
        view.layer = layer
        view.layer?.backgroundColor = NSColor.black.cgColor
        return view
    }

    func updateNSView(_ nsView: NSView, context: Context) {}
}

/// 撮る sheet(プレビュー・カメラの切り替え・シャッター・撮り直す / 使う)。写真・動画・QR で共有。
struct MacCameraSheet: View {
    let mode: MacCaptureMode
    var onPhoto: (PlatformImage) -> Void = { _ in }
    var onVideo: (URL) -> Void = { _ in }
    var onCode: (String) -> Void = { _ in }
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL
    @State private var camera = MacCameraSession()
    @State private var flow: MacCaptureFlow?
    @State private var reviewPlayer: AVPlayer?

    var body: some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            stage
                .frame(minWidth: 480, minHeight: 320)
            controls
        }
        .padding()
        .frame(minWidth: 520, minHeight: 440)
        .task {
            let f = MacCaptureFlow(mode: mode, driver: camera)
            flow = f
            await f.start()
        }
        .onDisappear { flow?.cancel() }
        .onChange(of: flow?.video) { _, url in
            reviewPlayer?.pause()
            reviewPlayer = url.map(AVPlayer.init(url:))
        }
        .onChange(of: flow?.code) { _, code in
            guard let code else { return }
            PlatformSystem.successHaptic()
            onCode(code)
            dismiss()
        }
    }

    @ViewBuilder
    private var stage: some View {
        switch flow?.phase ?? .starting {
        case .reviewPhoto:
            if let photo = flow?.photo { Image(nsImage: photo).resizable().scaledToFit() }
        case .reviewVideo:
            MacVideoPlayerView(player: reviewPlayer)
        case .unavailable(.denied):
            ContentUnavailableView {
                Label(L("カメラを使えません"), systemImage: "video.slash")
            } description: {
                Text(PlatformSystem.cameraDeniedMessage)
            } actions: {
                Button(L("設定を開く")) {
                    if let url = PlatformSystem.cameraSettingsURL { openURL(url) }
                }
                .help(L("システム設定のカメラの許可を開きます"))
            }
        case .unavailable:
            MacUnavailableView(feature: L("カメラ"))
        case .failed:
            ContentUnavailableView(L("カメラを開けませんでした"), systemImage: "exclamationmark.triangle")
        case .starting:
            ZStack { Color.black; ProgressView() }
        default:
            MacCameraPreview(session: camera.session)
                .overlay(alignment: .topTrailing) {
                    if flow?.phase == .recording {
                        Label(L("録画中"), systemImage: "record.circle").padding(6)
                            .foregroundStyle(.red).background(.ultraThinMaterial, in: Capsule()).padding(8)
                    }
                }
        }
    }

    private var controls: some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            Button(L("キャンセル"), role: .cancel) { dismiss() }
                .keyboardShortcut(.cancelAction)
                .help(L("撮らずに閉じます"))
            if let flow, flow.devices.count > 1, flow.phase == .live {
                Picker(L("カメラ"), selection: Binding(get: { flow.deviceID ?? flow.devices.first?.id ?? "" },
                                                     set: { id in Task { await flow.select(device: id) } })) {
                    ForEach(flow.devices) { Text($0.name).tag($0.id) }
                }
                .fixedSize()
                .help(L("使うカメラを選びます"))
            }
            Spacer()
            switch flow?.phase {
            case .reviewPhoto, .reviewVideo:
                Button(L("撮り直す")) { Task { await flow?.retake() } }
                    .help(L("撮ったものを捨てて、もう一度撮ります"))
                Button(L("使う")) { use() }
                    .keyboardShortcut(.defaultAction)
                    .help(L("撮ったものを入れて閉じます"))
            case .failed:
                Button(L("もう一度試す")) { Task { await flow?.retake() } }
                    .help(L("カメラをもう一度開きます"))
            case .live, .capturing, .recording:
                if mode != .qr { shutter }
            default:
                EmptyView()
            }
        }
    }

    private var shutter: some View {
        let recording = flow?.phase == .recording
        let title: String = switch mode {
        case .video: recording ? L("録画を停止") : L("録画を開始")
        default: L("撮影")
        }
        return Button { Task { await flow?.shutter() } } label: {
            Label(title, systemImage: recording ? "stop.circle.fill" : (mode == .photo ? "camera.circle.fill" : "record.circle"))
        }
        .keyboardShortcut(.space, modifiers: [])
        .disabled(flow?.phase == .capturing)
        .help(mode == .photo ? L("写真を撮ります(スペース)") : L("録画を開始・停止します(スペース)"))
    }

    private func use() {
        guard let flow else { return }
        if flow.phase == .reviewPhoto, let photo = flow.photo {
            onPhoto(photo)
        } else if flow.phase == .reviewVideo, let url = flow.takeVideo() {
            onVideo(url)
        }
        dismiss()
    }
}
#endif
