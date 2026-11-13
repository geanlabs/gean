# beacon/internal/network/syncer

Range sync, backfill and checkpoint sync, under the network loop. Reads chain
data through named reads and delivers blocks to the chain loop; never writes
chain state itself.
