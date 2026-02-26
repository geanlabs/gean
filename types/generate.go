package types

// NOTE: State encoding is checked in from previous generation and unchanged for devnet-2.
// fastssz v0.1.3 currently panics when regenerating State in this package.
//go:generate sszgen --path . --objs Checkpoint,Config,Validator,AttestationData,Attestation,SignedAttestation,AggregatedAttestation,AggregatedSignatureProof,BlockHeader,BlockBody,Block,BlockWithAttestation,BlockSignatures,SignedBlockWithAttestation
