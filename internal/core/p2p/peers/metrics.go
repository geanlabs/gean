package peers

// Measurement hooks. They do nothing until the chain points them at its
// metrics, at init, before any peer is tracked.
var (
	OnScore  = func(peer string, score int) {}
	OnRemove = func(peer string) {}
)
