#if os(macOS)
import AVFoundation
import CoreAudio

/// Mac の入力の装置(Core Audio)。`uid` は抜き差しや再起動をまたいで変わらない識別子(保存はこれで行う)。
public struct MacInputDevice: Equatable, Hashable, Identifiable, Sendable {
    public let id: AudioObjectID
    public let uid: String
    public let name: String

    public init(id: AudioObjectID, uid: String, name: String) {
        self.id = id
        self.uid = uid
        self.name = name
    }
}

/// Mac の音声入力(mac-005・#1221)。iOS の AVAudioSession の代わりに、設定で選んだ入力の装置(無ければシステムの既定)を使い、
/// `TranscriberDiagnostics` に残す。選んだ装置が見つからない時はシステムの既定に戻る(選んだ値は消さず、挿し直せばまた使う)。
public enum MacAudioInput {
    /// 選んだ装置の UID。無ければシステムの既定に従う
    public static let preferenceKey = "metamon.mac.inputDeviceUID"
    /// 選んだ装置の名前(見つからない時に設定で名前を出すためだけに持つ)
    public static let preferenceNameKey = "metamon.mac.inputDeviceName"

    nonisolated(unsafe) public static var defaults: UserDefaults = .standard

    public static var preferredUID: String? {
        get { defaults.string(forKey: preferenceKey).flatMap { $0.isEmpty ? nil : $0 } }
        set {
            if let newValue, !newValue.isEmpty {
                defaults.set(newValue, forKey: preferenceKey)
            } else {
                defaults.removeObject(forKey: preferenceKey)
                defaults.removeObject(forKey: preferenceNameKey)
            }
        }
    }

    public static var preferredName: String? {
        get { defaults.string(forKey: preferenceNameKey) }
        set { defaults.set(newValue, forKey: preferenceNameKey) }
    }

    /// 選んだ装置が一覧にあればそれ、無ければ nil(= システムの既定を使う)
    public static func resolve(preferredUID: String?, devices: [MacInputDevice]) -> MacInputDevice? {
        guard let preferredUID else { return nil }
        return devices.first { $0.uid == preferredUID }
    }

    /// 今の録音で使う装置(nil ならシステムの既定)
    public static func selectedDevice() -> MacInputDevice? {
        resolve(preferredUID: preferredUID, devices: devices())
    }

    /// 入力の装置の一覧(入力のチャンネルを持つものだけ)
    public static func devices() -> [MacInputDevice] {
        var address = AudioObjectPropertyAddress(mSelector: kAudioHardwarePropertyDevices,
                                                 mScope: kAudioObjectPropertyScopeGlobal,
                                                 mElement: kAudioObjectPropertyElementMain)
        var size: UInt32 = 0
        guard AudioObjectGetPropertyDataSize(AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size) == noErr else { return [] }
        var ids = [AudioObjectID](repeating: 0, count: Int(size) / MemoryLayout<AudioObjectID>.size)
        guard AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size, &ids) == noErr else { return [] }
        return ids.compactMap { id in
            guard hasInput(id), let uid = string(id, kAudioDevicePropertyDeviceUID) else { return nil }
            return MacInputDevice(id: id, uid: uid, name: string(id, kAudioObjectPropertyName) ?? uid)
        }
    }

    /// システムの既定の入力の装置
    public static func systemDefaultID() -> AudioObjectID? {
        var address = AudioObjectPropertyAddress(mSelector: kAudioHardwarePropertyDefaultInputDevice,
                                                 mScope: kAudioObjectPropertyScopeGlobal,
                                                 mElement: kAudioObjectPropertyElementMain)
        var id = AudioObjectID(0)
        var size = UInt32(MemoryLayout<AudioObjectID>.size)
        guard AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size, &id) == noErr,
              id != kAudioObjectUnknown else { return nil }
        return id
    }

    /// AVAudioEngine の入力を選んだ装置にする(選んでいなければ・見つからなければ既定のまま)
    public static func apply(to engine: AVAudioEngine) throws {
        guard let device = selectedDevice() else { return }
        try engine.inputNode.auAudioUnit.setDeviceID(device.id)
    }

    public static func fill(_ diag: inout TranscriberDiagnostics) {
        let all = devices()
        let selected = resolve(preferredUID: preferredUID, devices: all)
        let defaultID = systemDefaultID()
        let device = selected ?? all.first { $0.id == defaultID }
        diag.sessionCategory = "macOS"
        diag.sessionMode = selected == nil ? "default" : "selected"
        diag.inputAvailable = device != nil || AVCaptureDevice.default(for: .audio) != nil
        diag.inputRoute = device?.name ?? AVCaptureDevice.default(for: .audio)?.localizedName ?? ""
        let engine = AVAudioEngine()
        try? apply(to: engine)
        diag.inputSampleRate = engine.inputNode.outputFormat(forBus: 0).sampleRate
    }

    /// 装置の抜き差し・既定の入力の変更を知らせる。返した値を `stopObserving` に渡すまで続く
    public static func observeChanges(_ handler: @escaping @Sendable () -> Void) -> AnyObject {
        let token = ChangeObserver(handler: handler)
        token.start()
        return token
    }

    public static func stopObserving(_ token: AnyObject) {
        (token as? ChangeObserver)?.stop()
    }

    // MARK: - Core Audio

    private static func hasInput(_ id: AudioObjectID) -> Bool {
        var address = AudioObjectPropertyAddress(mSelector: kAudioDevicePropertyStreamConfiguration,
                                                 mScope: kAudioObjectPropertyScopeInput,
                                                 mElement: kAudioObjectPropertyElementMain)
        var size: UInt32 = 0
        guard AudioObjectGetPropertyDataSize(id, &address, 0, nil, &size) == noErr, size > 0 else { return false }
        let raw = UnsafeMutableRawPointer.allocate(byteCount: Int(size), alignment: MemoryLayout<AudioBufferList>.alignment)
        defer { raw.deallocate() }
        guard AudioObjectGetPropertyData(id, &address, 0, nil, &size, raw) == noErr else { return false }
        let list = UnsafeMutableAudioBufferListPointer(raw.assumingMemoryBound(to: AudioBufferList.self))
        return list.reduce(0) { $0 + Int($1.mNumberChannels) } > 0
    }

    private static func string(_ id: AudioObjectID, _ selector: AudioObjectPropertySelector) -> String? {
        var address = AudioObjectPropertyAddress(mSelector: selector,
                                                 mScope: kAudioObjectPropertyScopeGlobal,
                                                 mElement: kAudioObjectPropertyElementMain)
        var value: Unmanaged<CFString>?
        var size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
        guard AudioObjectGetPropertyData(id, &address, 0, nil, &size, &value) == noErr, let value else { return nil }
        return value.takeRetainedValue() as String
    }

    private final class ChangeObserver {
        private let handler: @Sendable () -> Void
        private var block: AudioObjectPropertyListenerBlock?
        private let selectors = [kAudioHardwarePropertyDevices, kAudioHardwarePropertyDefaultInputDevice]

        init(handler: @escaping @Sendable () -> Void) { self.handler = handler }

        func start() {
            let handler = handler
            let block: AudioObjectPropertyListenerBlock = { _, _ in handler() }
            self.block = block
            for selector in selectors {
                var address = AudioObjectPropertyAddress(mSelector: selector, mScope: kAudioObjectPropertyScopeGlobal,
                                                         mElement: kAudioObjectPropertyElementMain)
                AudioObjectAddPropertyListenerBlock(AudioObjectID(kAudioObjectSystemObject), &address, .main, block)
            }
        }

        func stop() {
            guard let block else { return }
            for selector in selectors {
                var address = AudioObjectPropertyAddress(mSelector: selector, mScope: kAudioObjectPropertyScopeGlobal,
                                                         mElement: kAudioObjectPropertyElementMain)
                AudioObjectRemovePropertyListenerBlock(AudioObjectID(kAudioObjectSystemObject), &address, .main, block)
            }
            self.block = nil
        }

        deinit { stop() }
    }
}
#endif
