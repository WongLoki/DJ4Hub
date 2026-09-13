import Foundation

struct IncomingAlertTracker {
    private var initialized = false
    private var messages = Set<String>()
    private var calls = Set<String>()
    mutating func update(sms: [String], ringing: [String]) -> (sms: Bool, call: Bool) {
        let nextMessages = Set(sms), nextCalls = Set(ringing)
        let result = (sms: initialized && !nextMessages.subtracting(messages).isEmpty,
                      call: !nextCalls.subtracting(calls).isEmpty)
        initialized = true
        messages.formUnion(nextMessages)
        calls.formUnion(nextCalls)
        return result
    }
}
