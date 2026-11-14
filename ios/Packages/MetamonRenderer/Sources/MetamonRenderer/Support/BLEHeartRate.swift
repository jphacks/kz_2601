// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(Bluetooth の説明文も strip-review-plist.sh で消す)。
#if !APP_STORE_REVIEW
import CoreBluetooth
import Foundation

/// Bluetooth の心拍ベルト(標準の Heart Rate サービス 0x180D・#2237・親 #1837)を読む口。部品・DSL にはまだつながない(#1775 の決定待ち)。
/// 1 台だけにつなぎ、アプリが前にいる間だけ測定値(0x2A37)を流す。裏での接続(state restoration・bluetooth-central)はしない。
/// 値はログ・クラッシュ報告に出さない(要配慮の情報)。
@MainActor
public final class BLEHeartRate: NSObject {
    public static let serviceUUID = CBUUID(string: "180D")
    public static let measurementUUID = CBUUID(string: "2A37")
    /// 説明文が無いビルド(審査用など)で CoreBluetooth に触ると落ちるので、先に確かめる
    static let usageDescriptionKey = "NSBluetoothAlwaysUsageDescription"

    public enum State: Sendable, Equatable {
        case idle
        /// Bluetooth の無い端末・説明文の無いビルド
        case unavailable
        /// Bluetooth の使用を許可していない
        case unauthorized
        /// Bluetooth が切れている(コントロールセンター等)
        case poweredOff
        case scanning
        case connecting
        case connected(name: String?)
        /// つないでいた機器が切れた(電池・距離)。もう一度 `start()` で探し直す
        case disconnected
    }

    public enum Event: Sendable, Equatable {
        case state(State)
        case measurement(HeartRateMeasurement)
    }

    private var central: CBCentralManager?
    private var peripheral: CBPeripheral?
    private var continuation: AsyncStream<Event>.Continuation?
    /// 何本目の流れか(前の流れが閉じた知らせで今の流れを止めないため)
    private var generation = 0
    private let bundle: Bundle

    public init(bundle: Bundle = .main) {
        self.bundle = bundle
        super.init()
    }

    /// 探し始める。返す流れは `stop()` か、受け手が止めるまで続く(2 回目の `start()` は前の流れを閉じる)。
    public func start() -> AsyncStream<Event> {
        stop()
        generation += 1
        let mine = generation
        let (stream, continuation) = AsyncStream<Event>.makeStream(bufferingPolicy: .bufferingNewest(16))
        self.continuation = continuation
        continuation.onTermination = { [weak self] _ in
            Task { @MainActor in
                guard let self, self.generation == mine else { return }
                self.stop()
            }
        }
        guard bundle.object(forInfoDictionaryKey: Self.usageDescriptionKey) != nil else {
            continuation.yield(.state(.unavailable))
            continuation.finish()
            self.continuation = nil
            return stream
        }
        central = CBCentralManager(delegate: self, queue: .main) // 初回だけ OS が許可を聞く
        return stream
    }

    /// 切って止める
    public func stop() {
        if let central {
            if central.isScanning { central.stopScan() }
            if let peripheral { central.cancelPeripheralConnection(peripheral) }
        }
        peripheral = nil
        central?.delegate = nil
        central = nil
        let c = continuation
        continuation = nil
        c?.finish()
    }

    private func emit(_ event: Event) {
        continuation?.yield(event)
    }

    /// OS の Bluetooth の状態 → 口の状態(電源が入っていれば nil = 探してよい)
    nonisolated static func state(for manager: CBManagerState) -> State? {
        switch manager {
        case .poweredOn: nil
        case .poweredOff: .poweredOff
        case .unauthorized: .unauthorized
        case .unsupported: .unavailable
        case .resetting, .unknown: .idle
        @unknown default: .idle
        }
    }
}

extension BLEHeartRate: CBCentralManagerDelegate, CBPeripheralDelegate {
    // delegate は main キューで呼ばれる(CBCentralManager(queue: .main))
    public nonisolated func centralManagerDidUpdateState(_ central: CBCentralManager) {
        let s = central.state
        MainActor.assumeIsolated {
            if let state = Self.state(for: s) {
                peripheral = nil
                emit(.state(state))
                return
            }
            guard peripheral == nil else { return }
            central.scanForPeripherals(withServices: [Self.serviceUUID])
            emit(.state(.scanning))
        }
    }

    public nonisolated func centralManager(_ central: CBCentralManager, didDiscover peripheral: CBPeripheral,
                                           advertisementData: [String: Any], rssi RSSI: NSNumber) {
        MainActor.assumeIsolated {
            guard self.peripheral == nil else { return } // 1 台だけ
            central.stopScan()
            self.peripheral = peripheral
            peripheral.delegate = self
            central.connect(peripheral)
            emit(.state(.connecting))
        }
    }

    public nonisolated func centralManager(_ central: CBCentralManager, didConnect peripheral: CBPeripheral) {
        let name = peripheral.name
        MainActor.assumeIsolated {
            emit(.state(.connected(name: name)))
            peripheral.discoverServices([Self.serviceUUID])
        }
    }

    public nonisolated func centralManager(_ central: CBCentralManager, didFailToConnect peripheral: CBPeripheral, error: Error?) {
        MainActor.assumeIsolated {
            self.peripheral = nil
            emit(.state(.disconnected))
        }
    }

    public nonisolated func centralManager(_ central: CBCentralManager, didDisconnectPeripheral peripheral: CBPeripheral, error: Error?) {
        MainActor.assumeIsolated {
            self.peripheral = nil
            emit(.state(.disconnected))
        }
    }

    public nonisolated func peripheral(_ peripheral: CBPeripheral, didDiscoverServices error: Error?) {
        MainActor.assumeIsolated {
            for service in peripheral.services ?? [] where service.uuid == Self.serviceUUID {
                peripheral.discoverCharacteristics([Self.measurementUUID], for: service)
            }
        }
    }

    public nonisolated func peripheral(_ peripheral: CBPeripheral, didDiscoverCharacteristicsFor service: CBService, error: Error?) {
        MainActor.assumeIsolated {
            for c in service.characteristics ?? [] where c.uuid == Self.measurementUUID {
                peripheral.setNotifyValue(true, for: c)
            }
        }
    }

    public nonisolated func peripheral(_ peripheral: CBPeripheral, didUpdateValueFor characteristic: CBCharacteristic, error: Error?) {
        guard error == nil, let data = characteristic.value, let m = HeartRateMeasurement(data) else { return }
        let uuid = characteristic.uuid
        MainActor.assumeIsolated {
            guard uuid == Self.measurementUUID else { return }
            emit(.measurement(m))
        }
    }
}
#endif
