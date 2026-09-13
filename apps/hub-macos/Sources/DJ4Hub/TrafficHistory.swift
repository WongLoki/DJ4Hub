import Foundation

struct TrafficHistory {
    private var previous: (time: Double, rx: Double, tx: Double, interface: String)?
    private(set) var download: [Double] = []
    private(set) var upload: [Double] = []
    mutating func record(_ value: HubValue) {
        guard value["available"].bool,
              let time = Double(value["sampled_at_ms"].text),
              let rx = Double(value["rx_bytes"].text), let tx = Double(value["tx_bytes"].text) else {
            previous = nil; download = []; upload = []; return
        }
        let interface = value["interface"].text
        defer { previous = (time, rx, tx, interface) }
        guard let last = previous, last.interface == interface, time > last.time,
              time - last.time <= 15000, rx >= last.rx, tx >= last.tx else {
            download = []; upload = []; return
        }
        let seconds = (time - last.time) / 1000
        download.append((rx - last.rx) / seconds); upload.append((tx - last.tx) / seconds)
        download = Array(download.suffix(36)); upload = Array(upload.suffix(36))
    }
}
