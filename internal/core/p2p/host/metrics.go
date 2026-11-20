package host

// OnRequestOutcome is a measurement hook. It does nothing until the chain
// points it at its metrics, at init, before any request is made.
var OnRequestOutcome = func(outcome string) {}
