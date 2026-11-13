# beacon/validator

The Beacon validator client, run by `gean validator` as its own process. It
reaches a node only through the Beacon API and never imports `beacon/internal`.
Its internals: `beaconclient/` (Beacon API client), `duties/`, `signing/` (local
keys or a remote signer) and `slashingprotection/` (EIP-3076 history, persisted
apart from the chain database so pruning can never erase it).
