// Package embedded runs geth inside the gean process and drives it through
// its consensus API, so an execution-layer network needs no second process,
// no JWT secret, and no JSON-RPC between consensus and execution.
package embedded

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

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
// transactions propagate between nodes. Both are off when zero. geth's own
// log lines go to geth.log under DataDir, or to stderr when in memory, at
// LogLevel and above; the zero value is info, so callers wanting a quiet
// run pass slog.LevelWarn.
type Config struct {
	Genesis   *core.Genesis
	DataDir   string
	HTTPPort  int
	P2PPort   int
	Bootnodes []string
	LogLevel  slog.Level
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

// LogFile is the name of geth's log under the execution data directory.
const LogFile = "geth.log"

// Engine is the execution.Engine over an in-process geth.
type Engine struct {
	stack   *node.Node
	backend *eth.Ethereum
	api     *catalyst.ConsensusAPI
	logFile io.Closer
}

var _ execution.Engine = (*Engine)(nil)

// Start brings geth up from the genesis and returns once its chain is
// readable.
func Start(cfg Config) (*Engine, error) {
	if cfg.Genesis == nil {
		return nil, fmt.Errorf("embedded execution: genesis is required")
	}
	e := &Engine{}
	logOut := io.Writer(os.Stderr)
	if cfg.DataDir != "" {
		if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			return nil, fmt.Errorf("embedded execution: %w", err)
		}
		f, err := os.OpenFile(filepath.Join(cfg.DataDir, LogFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("embedded execution: %w", err)
		}
		logOut, e.logFile = f, f
	}
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(logOut, cfg.LogLevel, false)))

	peers := make([]*enode.Node, 0, len(cfg.Bootnodes))
	for _, url := range cfg.Bootnodes {
		n, err := enode.Parse(enode.ValidSchemes, url)
		if err != nil {
			return nil, fmt.Errorf("embedded execution: bootnode %q: %w", url, err)
		}
		peers = append(peers, n)
	}

	nodeCfg := &node.Config{
		DataDir: cfg.DataDir,
		P2P: p2p.Config{
			// Discovery stays off, and geth only reaches bootstrap nodes
			// through discovery, so the peers given are dialed as static
			// nodes: connected directly and redialed if they drop.
			NoDiscovery: true,
			NoDial:      cfg.P2PPort == 0,
			MaxPeers:    maxPeers,
			StaticNodes: peers,
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
		e.Close()
		return nil, fmt.Errorf("embedded execution: %w", err)
	}
	e.stack = stack

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
		e.Close()
		return nil, fmt.Errorf("embedded execution: %w", err)
	}
	if err := stack.Start(); err != nil {
		e.Close()
		return nil, fmt.Errorf("embedded execution: %w", err)
	}
	e.backend, e.api = backend, catalyst.NewConsensusAPI(backend)
	return e, nil
}

// Close stops geth, flushes its database, and closes its log.
func (e *Engine) Close() error {
	var err error
	if e.stack != nil {
		err = e.stack.Close()
	}
	if e.logFile != nil {
		log.SetDefault(log.NewLogger(log.DiscardHandler()))
		if cerr := e.logFile.Close(); err == nil {
			err = cerr
		}
	}
	return err
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
