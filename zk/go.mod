module github.com/geanlabs/gean/zk

go 1.25.7

require github.com/geanlabs/gean v0.0.0

require (
	github.com/emicklei/dot v1.6.2 // indirect
	github.com/ferranbt/fastssz v1.0.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/geanlabs/gean => ../

// Guest SSZ hashing runs on the zkVM SHA-256 precompiles.
replace github.com/minio/sha256-simd => ./sha256zk
