import Foundation
import MetamonDSL
import MetamonRenderer

/// 記録の写真・動画・録音を Quick Look で見る(#1850)。スペース・ファイル > クイックルック(⌘Y)。
/// 写真は data URL なので一時フォルダに書き出してから渡す。https の写真は Quick Look が読めないので出さない。
enum MacQuickLook {
    enum Source: Equatable {
        /// data URL の写真(書き出してから見せる)
        case image(data: Data, fileName: String)
        /// 端末にある動画・録音のファイル
        case file(URL)
    }

    /// 項目の並び(DSL の順)で見せる物を集める。隠す項目(#262)・算出項目は見せない
    static func sources(data: [String: JSONValue], fields: [Field],
                        videos: LocalVideoStore = .shared, audio: LocalAudioStore = .shared,
                        fileExists: (URL) -> Bool = { FileManager.default.fileExists(atPath: $0.path) }) -> [Source] {
        fields.filter { !$0.hidden && !$0.isComputed }.compactMap { field in
            guard let value = data[field.id] else { return nil }
            switch field.type {
            case .photo:
                return image(value.stringValue, name: field.name)
            case .video:
                guard let video = VideoValue(value) else { return nil }
                if let id = video.id, let url = videos.url(for: id), fileExists(url) { return .file(url) }
                return image(video.thumbnail, name: field.name) // 動画の無い端末ではサムネイルだけ
            case .audio:
                guard let id = AudioValue(value)?.id, let url = audio.url(for: id), fileExists(url) else { return nil }
                return .file(url)
            default:
                return nil
            }
        }
    }

    /// `data:image/<種類>;base64,...` を読む。それ以外(https・空)は nil
    static func image(_ dataURL: String?, name: String) -> Source? {
        guard let s = dataURL, s.hasPrefix("data:image/"), let comma = s.firstIndex(of: ","),
              let data = Data(base64Encoded: String(s[s.index(after: comma)...])), !data.isEmpty else { return nil }
        let mime = s[s.index(s.startIndex, offsetBy: "data:image/".count)..<comma].split(separator: ";").first.map(String.init) ?? "jpeg"
        let ext = switch mime {
        case "png": "png"
        case "gif": "gif"
        case "heic": "heic"
        default: "jpg"
        }
        return .image(data: data, fileName: "\(safeName(name)).\(ext)")
    }

    /// ファイル名に使えない文字を落とす(Quick Look の見出しに出る)
    static func safeName(_ name: String) -> String {
        let cleaned = name.components(separatedBy: CharacterSet(charactersIn: "/:\\\0")).joined()
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return cleaned.isEmpty ? L10n.tr("写真") : String(cleaned.prefix(60))
    }

    /// Quick Look に渡すファイルの URL。写真は `directory` に書き出す(同じ名前は番号で分ける)
    static func urls(for sources: [Source], in directory: URL) -> [URL] {
        let fm = FileManager.default
        try? fm.removeItem(at: directory) // 前に見た分は残さない
        try? fm.createDirectory(at: directory, withIntermediateDirectories: true)
        var used: Set<String> = []
        return sources.compactMap { source in
            switch source {
            case .file(let url):
                return url
            case .image(let data, let fileName):
                var name = fileName
                var n = 2
                while !used.insert(name).inserted {
                    let url = URL(fileURLWithPath: fileName)
                    name = "\(url.deletingPathExtension().lastPathComponent) \(n).\(url.pathExtension)"
                    n += 1
                }
                let url = directory.appendingPathComponent(name)
                do { try data.write(to: url) } catch { return nil }
                return url
            }
        }
    }
}

extension MacAppRuntime {
    /// いちばん上の画面が開いている記録の値(フォームなら入力中の値)と項目
    private var quickLookRecord: (data: [String: JSONValue], fields: [Field])? {
        let top = NavigationResolver.top(navigationState, dsl: dsl)
        if let form = openForms.form(at: top) { return (form.values, form.entity.fields) }
        guard let recordId = top.recordId, let screen = dsl.screen(top.screen) else { return nil }
        let candidates = [screen.entity].compactMap { $0 } + screen.referencedEntities.sorted()
        for entity in candidates {
            if let record = records[entity]?.first(where: { $0.id == recordId }), let def = dsl.entity(entity) {
                return (record.data, def.fields)
            }
        }
        return nil
    }

    var quickLookSources: [MacQuickLook.Source] {
        guard let record = quickLookRecord else { return [] }
        return MacQuickLook.sources(data: record.data, fields: record.fields)
    }

    /// スペース・⌘Y で見られるか。シート・確認の間は出さない
    var canQuickLook: Bool { !isShowingDialog && !quickLookSources.isEmpty }

    /// 開く(もう開いていれば閉じる・Finder と同じ)
    func toggleQuickLook() {
        if quickLookURL != nil {
            quickLookURL = nil
            return
        }
        guard canQuickLook else { return }
        let urls = MacQuickLook.urls(for: quickLookSources, in: quickLookDirectory)
        quickLookItems = urls
        quickLookURL = urls.first
    }
}
