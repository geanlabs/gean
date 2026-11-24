---
paths:
  - "internal/core/storage/**"
  - "internal/store/**"
---

# Storage

- **Layering.** `core/storage` is a generic table-and-key store over Pebble and imports no other
  gean package; `make lint` enforces this. The chain's tables and keys are in `internal/store`.
- **On-disk layout.** A key is the table name, a `0x00` separator, then the key. Table names and key
  encodings are part of the layout, and a layout change means wiping the data directory.
- **Bounded growth.** Tables, caches and buffers that track chain data are pruned at finality; a
  new one is wired into the store's pruning.
- **Iterators and batches** are closed on every path.
- **No scans on the hot path.** A full-table or wide prefix scan does not run on the dispatch loop;
  keep an index or move the work to a worker.
