// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `nearbyShare` mode:passby(すれ違い通信・#425)。オンの間、この画面を開いていて前面にいる時だけ近くの同じアプリの人と
/// `fields` の項目を自動で交換し、会った人と日時を並べる。初回オンの前に同意を取る(相手に渡る表示名と項目を見せ、匿名も選べる)。
/// 通信はホスト(`NearbyPassbyHost.controller`)。
struct NearbyPassbyLeaf: View {
    @Environment(\.dslRuntime) private var runtime
    @Environment(\.scenePhase) private var scenePhase
    let c: Component
    let config: NearbyPassby.Config
    @State private var askConsent = false

    private var host: any NearbyPassbyController { NearbyPassbyHost.controller }

    private var fieldNames: [String] {
        guard let e = runtime.dsl.entity(config.entityId) else { return [] }
        return NearbyPassby.allowedFields(config, in: e).map(\.name)
    }

    private var isOn: Binding<Bool> {
        Binding(get: { host.isOptedIn(config) }, set: { on in
            switch NearbyPassby.toggle(on: on, consentedVersion: host.consentedVersion) {
            case .askConsent: askConsent = true
            case .start: host.setOptIn(true, config: config)
            case .stop: host.setOptIn(false, config: config)
            }
        })
    }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            header
            status
            if config.distance { distanceSection }
            Divider()
            encounterList
        }
        .padding(RenderTheme.Spacing.md)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: 0)
        .task(id: scenePhase == .background) {
            // 裏に回ると止める(iOS は裏で MultipeerConnectivity を続けられない)
            if scenePhase == .background { host.detach(config) } else { host.attach(config) }
        }
        .onDisappear { host.detach(config) }
        .sheet(isPresented: $askConsent) {
            PassbyConsentSheet(displayName: host.displayName, anonymous: host.isAnonymous, fieldNames: fieldNames) { anonymous in
                host.agree(anonymous: anonymous)
                host.setOptIn(true, config: config)
            }
        }
    }

    private var header: some View {
        Toggle(isOn: isOn) {
            Label(config.label, systemImage: "figure.2.arms.open")
                .font(.headline)
        }
        .accessibilityIdentifier("passby.toggle.\(config.componentId)")
    }

    @ViewBuilder
    private var status: some View {
        if host.isAdvertising {
            Label(L("近くの人を探しています"), systemImage: "dot.radiowaves.left.and.right")
                .symbolEffect(.variableColor.iterative, options: .repeating)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(RenderTheme.Colors.brandA)
                .accessibilityIdentifier("passby.advertising")
        } else if host.isOptedIn(config) {
            Label(L("一時停止中"), systemImage: "pause.circle")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        } else {
            Text(fieldNames.isEmpty ? L("送る項目がありません")
                 : L("オンにすると、この画面を開いている間 近くの同じアプリの人と\(fieldNames.map { "「\($0)」" }.joined())を交換します"))
                .font(.footnote)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        if (host.consentedVersion ?? 0) >= NearbyPassby.consentVersion {
            Toggle(isOn: Binding(get: { host.isAnonymous }, set: { host.setAnonymous($0) })) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(L("名前を出さない")).font(.subheadline)
                    Text(L("相手に見える名前: \(NearbyLimits.displayName(host.publicName))"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .accessibilityIdentifier("passby.anonymous.\(config.componentId)")
        }
        Label(L("画面を閉じる・アプリを裏に回すと止まります(iOS の制約)"), systemImage: "info.circle")
            .font(.caption)
            .foregroundStyle(.secondary)
            .fixedSize(horizontal: false, vertical: true)
        if let notice = host.notice {
            Label(notice, systemImage: "exclamationmark.triangle")
                .font(.caption)
                .foregroundStyle(.orange)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    @ViewBuilder
    private var distanceSection: some View {
        switch host.distanceSupport {
        case .unsupported(let reason):
            Text(reason)
                .font(.caption)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("passby.distance.unsupported")
        case .supported:
            ForEach(host.peers) { peer in
                HStack(spacing: RenderTheme.Spacing.sm) {
                    Image(systemName: "location.north.fill")
                        .rotationEffect(.radians(Double(peer.azimuth ?? 0)))
                        .opacity(peer.azimuth == nil ? 0.3 : 1)
                        .foregroundStyle(RenderTheme.Colors.brandA)
                        .accessibilityHidden(true)
                    Text(NearbyLimits.displayName(peer.name)).font(.subheadline.weight(.semibold)).lineLimit(1)
                    Spacer()
                    Text(peer.distance.map(NearbyPassby.distanceText) ?? L("測定中"))
                        .font(.subheadline.monospacedDigit())
                    if let az = peer.azimuth {
                        Text(NearbyPassby.directionText(azimuth: az)).font(.caption).foregroundStyle(.secondary)
                    }
                }
                .accessibilityElement(children: .combine)
            }
        }
    }

    @ViewBuilder
    private var encounterList: some View {
        if host.encounters.isEmpty {
            Text(L("まだ誰ともすれ違っていません"))
                .font(.footnote)
                .foregroundStyle(.secondary)
        } else {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                    ForEach(host.encounters) { e in
                        HStack {
                            Image(systemName: "person.crop.circle").foregroundStyle(.secondary).accessibilityHidden(true)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(NearbyLimits.displayName(e.name)).font(.subheadline.weight(.semibold)).lineLimit(1)
                                Text(e.lastMet.formatted(.dateTime.month().day().hour().minute()))
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Text(L("\(e.times) 回")).font(.caption).foregroundStyle(.secondary)
                        }
                        .accessibilityElement(children: .combine)
                    }
                }
            }
            .accessibilityIdentifier("passby.encounters")
        }
    }
}
/// すれ違い通信の同意。相手に渡るもの(表示名・項目)を並べ、名前を出さない(匿名)を選べる。
struct PassbyConsentSheet: View {
    @Environment(\.dismiss) private var dismiss
    let displayName: String
    @State var anonymous: Bool
    let fieldNames: [String]
    let onAgree: (Bool) -> Void

    init(displayName: String, anonymous: Bool, fieldNames: [String], onAgree: @escaping (Bool) -> Void) {
        self.displayName = displayName
        _anonymous = State(initialValue: anonymous)
        self.fieldNames = fieldNames
        self.onAgree = onAgree
    }

    /// 相手に渡る名前(画面の言語で)
    private var shownName: String { NearbyLimits.displayName(NearbyLimits.publicName(displayName, anonymous: anonymous)) }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    LabeledContent(L("表示名"), value: shownName)
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("passby.consent.name")
                    if fieldNames.isEmpty {
                        Text(L("送る項目がありません")).foregroundStyle(.secondary)
                    } else {
                        ForEach(fieldNames, id: \.self) { name in
                            Label(name, systemImage: "checkmark.circle")
                        }
                    }
                } header: {
                    Text(L("相手に渡るもの"))
                }
                Section {
                    Toggle(L("名前を出さない"), isOn: $anonymous)
                        .accessibilityIdentifier("passby.consent.anonymous")
                } footer: {
                    Text(L("オンにすると、表示名の代わりに「近くの人」と渡します。"))
                }
                Section {
                    Text(NearbyPassby.consentText(fieldNames: fieldNames, name: shownName))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle(L("すれ違い通信をはじめますか？"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("やめる")) { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("同意してはじめる")) {
                        onAgree(anonymous)
                        dismiss()
                    }
                    .accessibilityIdentifier("passby.consent.agree")
                }
            }
        }
        .presentationDetents([.medium, .large])
    }
}
#endif
