import Foundation
import Observation
import SwiftUI
import MetamonServices

/// 声で頼むの入力の装置(#1221)。一覧は Core Audio から取り、抜き差しで更新する。選んだ装置は UID で保存し(`MacAudioInput.preferredUID`)、
/// 見つからない時はシステムの既定を使う(選んだ値は消さないので、挿し直せばまた使う)。
@MainActor
@Observable
final class MacInputDevices {
    static let shared = MacInputDevices()

    private(set) var devices: [MacInputDevice] = []
    /// 選んだ装置の UID(nil = システムの既定に従う)
    private(set) var preferredUID: String?
    /// 選んだ装置の名前(見つからない時の表示用)
    private(set) var preferredName: String?
    /// 一覧が変わった時(録音中に装置が抜けたかを見る)
    @ObservationIgnored var onChange: ((Set<String>) -> Void)?

    @ObservationIgnored private let provider: () -> [MacInputDevice]
    @ObservationIgnored private let store: Store
    @ObservationIgnored private var token: AnyObject?

    struct Store {
        var load: () -> (uid: String?, name: String?)
        var save: (_ uid: String?, _ name: String?) -> Void

        static let system = Store(
            load: { (MacAudioInput.preferredUID, MacAudioInput.preferredName) },
            save: { uid, name in
                MacAudioInput.preferredUID = uid
                if uid != nil { MacAudioInput.preferredName = name }
            })
    }

    init(provider: @escaping () -> [MacInputDevice] = { MacAudioInput.devices() }, store: Store = .system) {
        self.provider = provider
        self.store = store
        let saved = store.load()
        preferredUID = saved.uid
        preferredName = saved.name
        devices = provider()
    }

    /// 選んだ装置が今は無い(システムの既定を使っている)
    var isPreferredMissing: Bool {
        guard let preferredUID else { return false }
        return !devices.contains { $0.uid == preferredUID }
    }

    /// 今の録音で使う装置(nil = システムの既定)
    var effectiveDevice: MacInputDevice? {
        MacAudioInput.resolve(preferredUID: preferredUID, devices: devices)
    }

    func select(_ uid: String?) {
        let name = uid.flatMap { uid in devices.first { $0.uid == uid }?.name }
        preferredUID = uid
        preferredName = uid == nil ? nil : name
        store.save(uid, name)
    }

    func refresh() {
        devices = provider()
        onChange?(Set(devices.map(\.uid)))
    }

    /// 抜き差しを見始める(起動時に 1 回)
    func startObserving() {
        guard token == nil else { return }
        refresh()
        token = MacAudioInput.observeChanges { Task { @MainActor in MacInputDevices.shared.refresh() } }
    }

    /// 録音に使う装置の UID(選んだ装置か、システムの既定)
    nonisolated static func inUseUID() -> String? {
        if let selected = MacAudioInput.selectedDevice() { return selected.uid }
        let defaultID = MacAudioInput.systemDefaultID()
        return MacAudioInput.devices().first { $0.id == defaultID }?.uid
    }
}

/// 設定 > 一般の「マイク」(#1221)
struct MacInputDeviceSection: View {
    @State private var list = MacInputDevices.shared

    var body: some View {
        let selection = Binding<String>(get: { list.preferredUID ?? "" },
                                        set: { list.select($0.isEmpty ? nil : $0) })
        Section {
            Picker("入力の装置", selection: selection) {
                Text("システムの設定に従う").tag("")
                ForEach(list.devices) { device in
                    Text(verbatim: device.name).tag(device.uid)
                }
                if list.isPreferredMissing, let uid = list.preferredUID {
                    Text("\(list.preferredName ?? uid)(見つかりません)").tag(uid)
                }
            }
            .help("声で頼む時に使うマイクを選びます。選んだマイクが見つからない時はシステムの設定のマイクを使います")
            .accessibilityIdentifier("settings.inputDevice")
            .onAppear { list.refresh() }
        } header: {
            Text("マイク")
        } footer: {
            if list.isPreferredMissing {
                Text("選んだマイクが見つからないため、今はシステムの設定のマイクを使っています。つなぎ直すと、また選んだマイクを使います。")
            } else {
                Text("外付けのマイクや AirPods を選べます。録音中にマイクが外れた時は録音を止めてお知らせします。")
            }
        }
    }
}
