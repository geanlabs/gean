package types

type BlocksByRootRequest struct {
	Roots [][RootSize]byte `json:"roots" ssz-max:"1024" ssz-size:"?,32"`
}
