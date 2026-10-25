import SwiftUI
import MetamonDSL

/// `locationTracker`(#168): 「記録を開始 / 記録を停止」と状態(経過時間・距離・点の数)。
/// 開始を押した間だけ位置を `entity` に 1 点 1 レコードで足す(`locationField` = "lat,lng"、`dateField` = 日時)。
/// 記録そのものは `LocationTracking.shared`(画面を移っても・背面でも続く)。`seconds` で自動停止(既定 1 時間)。
struct LocationTrackerLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.openURL) private var openURL
    let c: Component
    let ctx: RenderContext
    private var tracking: LocationTracking { .shared }

    private var target: LocationTracking.Target? {
        guard let entity = c.entity, let field = c.locationField else { return nil }
        return .init(appName: rt.dsl.app.name, entity: entity, locationField: field, dateField: c.dateField)
    }

    private var autoStop: Int { Track.autoStopSeconds(c.seconds) }

    var body: some View {
        if let target {
            content(target)
        } else {
            UnsupportedComponent(label: "locationTracker")
        }
    }

    @ViewBuilder
    private func content(_ target: LocationTracking.Target) -> some View {
        let mine = tracking.isRecording(target)
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            if mine {
                status
                Button(L("記録を停止"), systemImage: "stop.fill") { tracking.stop() }
                    .buttonStyle(RenderButtonStyle(kind: .destructive))
                    .accessibilityIdentifier("locationTracker.stop")
            } else if tracking.isActive {
                // 別のアプリ・別の部品で記録中(同時に 2 本は取らない)
                Text(L("別の記録が進行中です")).font(.caption).foregroundStyle(.secondary)
                Button(L("その記録を停止"), systemImage: "stop.fill") { tracking.stop() }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
            } else {
                Button(c.label ?? L("記録を開始"), systemImage: "record.circle") { start(target) }
                    .buttonStyle(RenderButtonStyle(kind: .primary))
                    .accessibilityIdentifier("locationTracker.start")
                footnote
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onAppear {
            #if DEBUG
            // ヘッドレス E2E 用: `METAMON_E2E_TRACKER_START=<部品 id>` で表示時に開始(METAMON_MOCK=1 なら決まった点列)
            if ProcessInfo.processInfo.environment["METAMON_E2E_TRACKER_START"] == c.id, !tracking.isActive {
                print("[Metamon] E2E: locationTracker start id=\(c.id)")
                start(target)
            }
            #endif
        }
    }

    /// 記録中: ● 記録中 12:34 ・ 1.23 km ・ 18 点 / あと 47 分で自動停止
    private var status: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            let elapsed = context.date.timeIntervalSince(tracking.startedAt ?? context.date)
            let remaining = max(0, (tracking.endsAt ?? context.date).timeIntervalSince(context.date))
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Circle().fill(.red).frame(width: 8, height: 8).accessibilityHidden(true)
                    Text(tracking.phase == .requesting ? L("位置情報の許可を待っています") : L("記録中"))
                        .font(.subheadline.weight(.semibold))
                    Spacer(minLength: 4)
                    Text("\(Self.clock(elapsed)) ・ \(Track.formatDistance(tracking.distance))")
                        .font(.subheadline.monospacedDigit())
                }
                Text(tracking.savedCount == 0
                     ? L("位置を待っています…")
                     : L("\(tracking.savedCount)点を記録・あと\(Int(remaining / 60) + 1)分で自動停止"))
                    .font(.caption).foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .combine)
        }
    }

    @ViewBuilder
    private var footnote: some View {
        switch tracking.phase {
        case .denied:
            VStack(alignment: .leading, spacing: 4) {
                Text(L("位置情報がオフのため記録できません。設定で「このAppの使用中のみ許可」を選んでください"))
                    .font(.caption).foregroundStyle(RenderTheme.Colors.negative)
                Button(L("設定を開く")) {
                    if let url = URL(string: "app-settings:") { openURL(url) }
                }
                .font(.caption.weight(.semibold))
            }
        case .failed(let message):
            Text(message).font(.caption).foregroundStyle(RenderTheme.Colors.negative)
        default:
            Text(tracking.lastStopMessage ?? L("押した時だけ位置を記録します(最長\(autoStop / 60)分で自動停止)"))
                .font(.caption).foregroundStyle(.secondary)
        }
    }

    private func start(_ target: LocationTracking.Target) {
        let rt = rt
        let base = Self.baseData(rt.dsl.entity(target.entity), skipping: [target.locationField, target.dateField].compactMap { $0 })
        tracking.start(target, autoStopSeconds: autoStop) { point in
            // アプリを切り替えた等で記録先が無くなったら保存しない(別アプリの entity に混ぜない)
            guard rt.dsl.app.name == target.appName, rt.dsl.entity(target.entity)?.field(target.locationField)?.type == .location else { return false }
            return await rt.createRecord(entity: target.entity, data: base.merging(point) { _, new in new })
        }
    }

    /// 点のレコードに入れる他の field の既定値(defaultValue を持つ保存 field だけ)。
    static func baseData(_ entity: Entity?, skipping: [String]) -> [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        for f in entity?.fields ?? [] where f.computed == nil && f.defaultValue != nil && !skipping.contains(f.id) {
            out[f.id] = FieldCoding.defaultValue(for: f)
        }
        return out
    }

    static func clock(_ seconds: TimeInterval) -> String {
        let s = Int(max(0, seconds))
        return s >= 3600 ? String(format: "%d:%02d:%02d", s / 3600, s / 60 % 60, s % 60) : String(format: "%02d:%02d", s / 60, s % 60)
    }
}
