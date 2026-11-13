# beacon/consensus/types

SSZ containers. Containers no fork changes are defined once here (`shared.go`).
Each fork gets a package (`phase0/`, `altair/`, `bellatrix/`, `capella/`,
`deneb/`, `electra/`, `fulu/`, `gloas/`) defining only the containers that fork
changed; a fork that leaves a container unchanged reuses the previous fork's
type. A small `State` interface (`state.go`) exposes only the fields every fork
has. SSZ codecs are generated with `sszgen`, never edited by hand.

Do not use one struct carrying every fork's fields: per-fork structs make reading
a field the fork does not have a compile error.
