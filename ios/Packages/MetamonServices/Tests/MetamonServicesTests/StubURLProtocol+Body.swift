import Foundation

extension StubURLProtocol {
    /// 記録したリクエストの JSON 本文(URLProtocol には httpBodyStream で届くことがある)。
    static func bodyJSON(_ req: URLRequest) -> NSDictionary? {
        var data = req.httpBody
        if data == nil, let stream = req.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var buf = [UInt8](repeating: 0, count: 4096)
            var out = Data()
            while stream.hasBytesAvailable {
                let n = stream.read(&buf, maxLength: buf.count)
                if n <= 0 { break }
                out.append(buf, count: n)
            }
            data = out
        }
        guard let data, !data.isEmpty else { return nil }
        return try? JSONSerialization.jsonObject(with: data) as? NSDictionary
    }
}
