package storage

// Table represents a logical storage table.
type Table string

const (
	TableBlockHeaders    Table = "block_headers"
	TableBlockBodies     Table = "block_bodies"
	TableBlockSignatures Table = "block_signatures"
	TableStates          Table = "states"
	TableMetadata        Table = "metadata"
	TableLiveChain       Table = "live_chain"
)

// AllTables returns all 6 storage tables.
var AllTables = []Table{
	TableBlockHeaders,
	TableBlockBodies,
	TableBlockSignatures,
	TableStates,
	TableMetadata,
	TableLiveChain,
}
