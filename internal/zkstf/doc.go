// Package zkstf proves gean's state transition function inside a zkVM.
//
// The claim is: applying an unsigned block B to pre-state S succeeds and yields
// post-state root R. Signatures and the leanVM block proof are not part of the
// claim; they are verified on import, outside the state transition, exactly as
// the node already does.
//
// Apply is the only logic the guest runs. The host runs the same function
// natively, so a guest's public values can be compared byte for byte with the
// native result. This package is imported by the TamaGo guest, so it must stay
// free of cgo, networking and anything else a bare-metal target cannot build.
package zkstf
