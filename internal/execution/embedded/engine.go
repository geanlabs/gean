// Package embedded runs geth inside the gean process and drives it through
// its consensus API, so an execution-layer network needs no second process,
// no JWT secret, and no JSON-RPC between consensus and execution.
package embedded

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/catalyst"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"

	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/types"
)

// Config describes the embedded client. An empty DataDir keeps the chain in
// memory, which tests use. HTTPPort exposes the eth, net, and web3 RPC on
// loopback for wallets and tools; P2PPort joins the execution p2p mesh so
// transactions propagate between nodes. Both are off when zero.
type Config struct {
	Genesis   *core.Genesis
	DataDir   string
	HTTPPort  int
	P2PPort   int
	Bootnodes []string
}

// Cache sizes in MiB. geth's defaults reserve gigabytes; the XMSS prover
// shares this process and needs that headroom more.
const (
	trieCleanCacheMiB = 128
	trieDirtyCacheMiB = 256
	snapshotCacheMiB  = 64
	databaseCacheMiB  = 128
	maxPeers          = 8
)

// LoadGenesis reads the same genesis file geth's init command takes.
func LoadGenesis(path string) (*core.Genesis, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	genesis := new(core.Genesis)
	if err := json.Unmarshal(raw, genesis); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if genesis.Config == nil || genesis.Config.ChainID == nil {
		return nil, fmt.Errorf("%s: genesis declares no chain id", path)
	}
	return genesis, nil
}

// Engine is the execution.Engine over an in-process geth.
type Engine struct {
	stack   *node.Node
	backend *eth.Ethereum
	api     *catalyst.ConsensusAPI
}

var _ execution.Engine = (*Engine)(nil)

// Start brings geth up from the genesis and returns once its chain is
// readable. geth logs at warn level and above on stderr, so a healthy run
// is silent.
func Start(cfg Config) (*Engine, error) {
	if cfg.Genesis == nil {
		return nil, fmt.Errorf("embedded execution: genesis is required")
	}
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelWarn, false)))

	bootnodes := make([]*enode.Node, 0, len(cfg.Bootnodes))
	for _, url := range cfg.Bootnodes {
		n, err := enode.Parse(enode.ValidSchemes, url)
		if err != nil {
			return nil, fmt.Errorf("embedded execution: bootnode %q: %w", url, err)
		}
		bootnodes = append(bootnodes, n)
	}

	nodeCfg := &node.Config{
		DataDir: cfg.DataDir,
		P2P: p2p.Config{
			// Peers are the bootnodes given explicitly; discovery stays off.
			NoDiscovery:    true,
			NoDial:         cfg.P2PPort == 0,
			MaxPeers:       maxPeers,
			BootstrapNodes: bootnodes,
		},
	}
	if cfg.P2PPort > 0 {
		nodeCfg.P2P.ListenAddr = fmt.Sprintf(":%d", cfg.P2PPort)
	}
	if cfg.HTTPPort > 0 {
		nodeCfg.HTTPHost = "127.0.0.1"
		nodeCfg.HTTPPort = cfg.HTTPPort
		nodeCfg.HTTPModules = []string{"eth", "net", "web3"}
	}
	stack, err := node.New(nodeCfg)
	if err != nil {
		return nil, fmt.Errorf("embedded execution: %w", err)
	}

	ethCfg := ethconfig.Defaults
	ethCfg.Genesis = cfg.Genesis
	ethCfg.NetworkId = cfg.Genesis.Config.ChainID.Uint64()
	ethCfg.SyncMode = ethconfig.FullSync
	ethCfg.TrieCleanCache = trieCleanCacheMiB
	ethCfg.TrieDirtyCache = trieDirtyCacheMiB
	ethCfg.SnapshotCache = snapshotCacheMiB
	ethCfg.DatabaseCache = databaseCacheMiB
	backend, err := eth.New(stack, &ethCfg)
	if err != nil {
		stack.Close()
		return nil, fmt.Errorf("embedded execution: %w", err)
	}
	if err := stack.Start(); err != nil {
		stack.Close()
		return nil, fmt.Errorf("embedded execution: %w", err)
	}
	return &Engine{stack: stack, backend: backend, api: catalyst.NewConsensusAPI(backend)}, nil
}

// Close stops geth and flushes its database.
func (e *Engine) Close() error {
	return e.stack.Close()
}

// Enode is this client's execution p2p address, for other nodes' bootnode
// lists.
func (e *Engine) Enode() string {
	return e.stack.Server().Self().URLv4()
}

func (e *Engine) ForkchoiceUpdated(ctx context.Context, state execution.ForkchoiceState, attrs *execution.PayloadAttributes) (execution.ForkchoiceUpdatedResult, error) {
	return e.api.ForkchoiceUpdatedV3(ctx, state, attrs)
}

func (e *Engine) GetPayload(_ context.Context, id execution.PayloadID) (*types.ExecutionPayload, error) {
	envelope, err := e.api.GetPayloadV3(id)
	if err != nil {
		return nil, err
	}
	payload, err := execution.FromExecutableData(envelope.ExecutionPayload)
	if err != nil {
		return nil, err
	}
	if err := payload.ValidateExecutionFeatures(); err != nil {
		return nil, err
	}
	return payload, nil
}

func (e *Engine) NewPayload(ctx context.Context, payload *types.ExecutionPayload, parentBeaconBlockRoot [32]byte) (execution.PayloadStatus, error) {
	if err := payload.ValidateExecutionFeatures(); err != nil {
		return execution.PayloadStatus{}, err
	}
	beaconRoot := common.Hash(parentBeaconBlockRoot)
	// The feature check above guarantees there are no blob versioned hashes.
	return e.api.NewPayloadV3(ctx, *execution.ToExecutableData(payload), []common.Hash{}, &beaconRoot)
}

func (e *Engine) GenesisBlockHash(context.Context) ([32]byte, error) {
	return e.backend.BlockChain().Genesis().Hash(), nil
}

func (e *Engine) ExchangeCapabilities(_ context.Context, offered []string) ([]string, error) {
	return e.api.ExchangeCapabilities(offered), nil
}
