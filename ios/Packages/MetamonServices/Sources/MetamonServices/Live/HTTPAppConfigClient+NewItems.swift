import Foundation

/// 新着(#268)。`filterField=author&filterOp=neq` で他人のレコードだけ取る(backend `buildFilter`)。
/// 条件(自分の uid を含む)は URL に載せず `POST .../records/query` の本文で送る。
/// private の entity は author で絞れず 400 になるので空扱い。author という項目を持つ private の entity は data で絞られるが、
/// レスポンスに author が付かないので `NewItemsCounter` が数えない。
extension HTTPAppConfigClient: SharedRecordStampSource {
    private struct StampPage: Decodable {
        var items: [RecordStamp]
        var nextPageToken: String?
    }

    /// `POST .../records/query` の本文(未送信のキーは省く・サーバーは未知キーを 400)。
    struct RecordsQueryBody: Encodable {
        var filterField: String
        var filterOp: String
        var filterValue: String
        var pageSize: Int
        var pageToken: String?
    }

    public func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp] {
        let path = "v1/apps/\(appId)/entities/\(entity)/records/query"
        var items: [RecordStamp] = []
        var pageToken: String?
        for _ in 0..<Self.maxPages {
            let body = RecordsQueryBody(filterField: "author", filterOp: "neq", filterValue: uid, pageSize: Self.pageSize, pageToken: pageToken)
            let data: Data
            do {
                data = try await HTTP.send("POST", baseURL: baseURL, path: path, body: body, token: token)
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
