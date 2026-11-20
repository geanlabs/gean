package keys

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/geanlabs/gean/internal/core/crypto/xmss"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Manager holds a node's validator keys: an attestation key and a proposal key
// per validator.
type Manager struct {
	attestationKeys map[uint64]*xmss.ValidatorKeyPair
	proposalKeys    map[uint64]*xmss.ValidatorKeyPair
}

func NewManager(attestationKeys, proposalKeys map[uint64]*xmss.ValidatorKeyPair) *Manager {
	return &Manager{
		attestationKeys: attestationKeys,
		proposalKeys:    proposalKeys,
	}
}

func (km *Manager) ValidatorIDs() []uint64 {
	if km == nil {
		return nil
	}
	ids := make([]uint64, 0, len(km.attestationKeys))
	for id := range km.attestationKeys {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

func (km *Manager) GetAttestationKey(validatorID uint64) *xmss.ValidatorKeyPair {
	if km == nil {
		return nil
	}
	return km.attestationKeys[validatorID]
}

func (km *Manager) GetProposalKey(validatorID uint64) *xmss.ValidatorKeyPair {
	if km == nil {
		return nil
	}
	return km.proposalKeys[validatorID]
}

// SignAttestationRoot signs an attestation's message root at slot with the
// validator's attestation key. The caller computes the root; this package
// knows nothing about the chain's attestation format.
func (km *Manager) SignAttestationRoot(validatorID uint64, slot uint64, msgRoot [32]byte) ([xmss.SignatureSize]byte, error) {
	if km == nil {
		return [xmss.SignatureSize]byte{}, fmt.Errorf("key manager is nil")
	}
	kp := km.GetAttestationKey(validatorID)
	if kp == nil {
		return [xmss.SignatureSize]byte{}, fmt.Errorf("attestation key for validator %d not found", validatorID)
	}

	s := uint32(slot)
	if uint64(s) != slot {
		return [xmss.SignatureSize]byte{}, fmt.Errorf("slot %d overflows uint32", slot)
	}

	return kp.Sign(s, msgRoot)
}

func (km *Manager) Close() {
	if km == nil {
		return
	}
	for _, kp := range km.attestationKeys {
		kp.Close()
	}
	for _, kp := range km.proposalKeys {
		kp.Close()
	}
}

// MatchRegistry checks every loaded key against its validator's registry pubkeys, so a swapped,
// shared, or foreign key file stops the node before it signs anything. validators is indexed by
// validator ID.
func (km *Manager) MatchRegistry(validators []*types.Validator) error {
	for _, id := range km.ValidatorIDs() {
		if id >= uint64(len(validators)) {
			return fmt.Errorf("validator %d not in registry of %d validators", id, len(validators))
		}
		if err := matchPubkey(km.attestationKeys[id], validators[id].AttestationPubkey, id, "attestation"); err != nil {
			return err
		}
		if err := matchPubkey(km.proposalKeys[id], validators[id].ProposalPubkey, id, "proposal"); err != nil {
			return err
		}
	}
	return nil
}

func matchPubkey(kp *xmss.ValidatorKeyPair, want [types.PubkeySize]byte, id uint64, role string) error {
	got, err := kp.PublicKeyBytes()
	if err != nil {
		return fmt.Errorf("validator %d %s key: %w", id, role, err)
	}
	if got != want {
		return fmt.Errorf("validator %d %s key 0x%x does not match registry pubkey 0x%x", id, role, got, want)
	}
	return nil
}

type annotatedValidator struct {
	Index             uint64 `yaml:"index"`
	PrivkeyFile       string `yaml:"privkey_file"`
	AttestationSkFile string `yaml:"attestation_sk_file"`
	ProposalSkFile    string `yaml:"proposal_sk_file"`
}

// Load loads a node's attestation and proposal keys. Entries use either gean keygen's
// attestation_sk_file/proposal_sk_file pair or lean-quickstart's one privkey_file per role, with
// the role named in the file name. Every validator must end up with exactly one key per role.
func Load(annotatedPath, keysDir, nodeID string) (*Manager, error) {
	data, err := os.ReadFile(annotatedPath)
	if err != nil {
		return nil, fmt.Errorf("read annotated validators: %w", err)
	}

	var allValidators map[string][]annotatedValidator
	if err := yaml.Unmarshal(data, &allValidators); err != nil {
		return nil, fmt.Errorf("parse annotated validators: %w", err)
	}

	validators, ok := allValidators[nodeID]
	if !ok {
		return nil, fmt.Errorf("node ID %q not found in annotated validators", nodeID)
	}

	attestationFiles := make(map[uint64]string)
	proposalFiles := make(map[uint64]string)
	assign := func(files map[uint64]string, role string, index uint64, file string) error {
		if file == "" {
			return fmt.Errorf("%s key file missing for validator %d", role, index)
		}
		if files[index] != "" {
			return fmt.Errorf("duplicate %s key for validator %d", role, index)
		}
		files[index] = file
		return nil
	}

	for _, v := range validators {
		if v.PrivkeyFile != "" {
			if v.AttestationSkFile != "" || v.ProposalSkFile != "" {
				return nil, fmt.Errorf("validator %d: privkey_file cannot be combined with attestation_sk_file or proposal_sk_file", v.Index)
			}
			name := filepath.Base(v.PrivkeyFile)
			attestation := strings.Contains(name, "attester") || strings.Contains(name, "attestation")
			proposal := strings.Contains(name, "proposer") || strings.Contains(name, "proposal")
			if attestation == proposal {
				return nil, fmt.Errorf("validator %d: key file %q must name exactly one role (attester or proposer)", v.Index, v.PrivkeyFile)
			}
			if attestation {
				err = assign(attestationFiles, "attestation", v.Index, v.PrivkeyFile)
			} else {
				err = assign(proposalFiles, "proposal", v.Index, v.PrivkeyFile)
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		if err := assign(attestationFiles, "attestation", v.Index, v.AttestationSkFile); err != nil {
			return nil, err
		}
		if err := assign(proposalFiles, "proposal", v.Index, v.ProposalSkFile); err != nil {
			return nil, err
		}
	}
	for index := range attestationFiles {
		if proposalFiles[index] == "" {
			return nil, fmt.Errorf("proposal key file missing for validator %d", index)
		}
	}
	for index := range proposalFiles {
		if attestationFiles[index] == "" {
			return nil, fmt.Errorf("attestation key file missing for validator %d", index)
		}
	}

	attestationKeys := make(map[uint64]*xmss.ValidatorKeyPair, len(attestationFiles))
	proposalKeys := make(map[uint64]*xmss.ValidatorKeyPair, len(proposalFiles))
	for index, file := range attestationFiles {
		kp, err := loadKeypair(keysDir, file, index)
		if err != nil {
			return nil, fmt.Errorf("load attestation key for validator %d (%s): %w", index, file, err)
		}
		attestationKeys[index] = kp
	}
	for index, file := range proposalFiles {
		kp, err := loadKeypair(keysDir, file, index)
		if err != nil {
			return nil, fmt.Errorf("load proposal key for validator %d (%s): %w", index, file, err)
		}
		proposalKeys[index] = kp
	}

	return NewManager(attestationKeys, proposalKeys), nil
}

func loadKeypair(keysDir, skFile string, index uint64) (*xmss.ValidatorKeyPair, error) {
	skPath := skFile
	if !filepath.IsAbs(skPath) {
		skPath = filepath.Join(keysDir, skFile)
	}

	skBytes, err := os.ReadFile(skPath)
	if err != nil {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	if len(skBytes) == 0 {
		return nil, fmt.Errorf("secret key is empty")
	}

	return xmss.KeyPairFromSecret(skBytes, index)
}
