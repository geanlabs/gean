package reqresp

// Measurement hooks. They do nothing until the chain points them at its
// metrics, at init, before any request is served.
var (
	OnTimeout       = func(protocol, direction string) {}
	OnRequest       = func(protocol string, bytes int) {}
	OnResponseChunk = func(protocol string, bytes int) {}
)
