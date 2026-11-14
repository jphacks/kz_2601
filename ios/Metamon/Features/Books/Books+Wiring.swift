import Foundation
import MetamonServices

extension LiveWiring {
    /// ISBN から本の情報(lookupBook)。書誌の文字をフォームに入れるだけなので審査用ビルドにも残す(docs/ios/review-build.md)。
    static let books = LiveWiring { services, appConfigURL, token in
        services.books = appConfigURL.map { HTTPBookLookupClient(baseURL: $0, token: token) }
    }
}
