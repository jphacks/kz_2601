import Foundation

/// 新着(#268)。`filterField=author&filterOp=neq` で他人のレコードだけ取る(backend `buildFilter`)。
/// private の entity は author で絞れず 400 になるので空扱い。author という項目を持つ private の entity は data で絞られるが、
/// レスポンスに author が付かないので `NewItemsCounter` が数えない。
extension HTTPAppConfigClient: SharedRecordStampSource {
    private struct StampPage: Decodable {
        var items: [RecordStamp]
        var nextPageToken: String?
    }

    public func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp] {
        let value = uid.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? uid
        var items: [RecordStamp] = []
        var pageToken: String?
        for _ in 0..<Self.maxPages {
            var path = "v1/apps/\(appId)/entities/\(entity)/records?pageSize=\(Self.pageSize)&filterField=author&filterOp=neq&filterValue=\(value)"
            if let pageToken { path += "&pageToken=\(pageToken)" }
            let data: Data
            do {
                data = try await HTTP.send("GET", baseURL: baseURL, path: path, body: nil, token: token)
            } catch let error as APIError where error.status == 400 {
                return []
            }
            let page = try JSONDecoder().decode(StampPage.self, from: data)
            items += page.items
            guard let next = page.nextPageToken, !next.isEmpty else { break }
            pageToken = next
        }
        return items
    }
}
