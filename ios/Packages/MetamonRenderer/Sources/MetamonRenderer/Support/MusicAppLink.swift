import Foundation
import MetamonDSL
#if os(macOS)
import AppKit
#endif

/// Mac の「ミュージック アプリで開く」(#1348)。アプリの中で再生できない時(断られた・未契約・使えない)の逃げ道。
/// 曲の URL は `MusicLinks.appleMusicURL`(id があれば曲のページ、無ければ検索)。iOS では出さない。
enum MusicAppLink {
    /// 再生できない時に「ミュージック アプリで開く」を前に出すか。Mac だけ。制限(ペアレンタル)は出さない。
    static func offered(for availability: MusicAvailability) -> Bool {
        #if os(macOS)
        switch availability {
        case .denied, .noSubscription, .unavailable: true
        case .notDetermined, .ready, .restricted: false
        }
        #else
        false
        #endif
    }

    /// ミュージック アプリが受ける URL。Apple Music の https を `music:` に変える。曲が無ければアプリを開くだけ。
    static func appURL(for song: MusicSong?) -> URL? {
        guard let song else { return URL(string: "music://") }
        guard let web = MusicLinks.appleMusicURL(song),
              var c = URLComponents(url: web, resolvingAgainstBaseURL: false) else { return nil }
        c.scheme = "music"
        return c.url
    }

    /// 開く先。ミュージック アプリが無ければ(受け手が無ければ)ブラウザで Apple Music の https を開く。
    static func url(for song: MusicSong?) -> URL? {
        #if os(macOS)
        if let app = appURL(for: song), NSWorkspace.shared.urlForApplication(toOpen: app) != nil { return app }
        #endif
        return song.flatMap(MusicLinks.appleMusicURL)
    }

    /// ボタンの文言: 曲があれば「ミュージック アプリで開く」、無ければ「ミュージック アプリを開く」。
    static func label(for song: MusicSong?) -> String {
        song == nil ? L("ミュージック アプリを開く") : L("ミュージック アプリで開く")
    }
}
