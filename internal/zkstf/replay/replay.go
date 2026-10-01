// Package replay re-runs, on any zkstf backend, the state transition of blocks
// a node has imported, fetching them over the node's SSZ replay routes.
package replay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/internal/zkstf"
)

const (
	fetchTimeout = 30 * time.Second
	// maxFetchBytes bounds a node response; states are well under this.
	maxFetchBytes = 256 << 20
)

var (
	ErrNotFound = errors.New("not found")
	ErrNoBlocks = errors.New("no blocks in range")
)

// Block is one replayed block.
type Block struct {
	Slot         uint64
	Attestations int
	Input        []byte
	Result       *zkstf.ExecResult
}

// Run replays the blocks named by ids in order. With skipMissing, ids the node
// has no block for (empty slots) are skipped; otherwise they are an error.
//
// Only the first block's pre-state is fetched. Each later pre-state is the
// native post-state of the block before it, so a range needs one state
// download and works even where the node has pruned intermediate states.
// Every block must produce the native public values, its post-state root must
// be the root the block commits to, and each pre-state root must equal the
// previous post-state root.
func Run(ctx context.Context, node *Client, prover zkstf.Prover, ids []string, skipMissing bool, onBlock func(Block) error) error {
	var (
		pre      *types.State
		prevPost *[32]byte
		replayed int
	)
	for _, id := range ids {
		signed, err := node.SignedBlock(ctx, id)
		if errors.Is(err, ErrNotFound) && skipMissing {
			continue
		}
		if err != nil {
			return fmt.Errorf("block %s: %w", id, err)
		}
		block := signed.Block
		if pre == nil {
			if pre, err = node.State(ctx, fmt.Sprintf("0x%x", block.ParentRoot)); err != nil {
				return fmt.Errorf("parent state of slot %d: %w", block.Slot, err)
			}
		}

		input, err := zkstf.NewInput(pre, block)
		if err != nil {
			return fmt.Errorf("slot %d: %w", block.Slot, err)
		}
		post, err := pre.Clone()
		if err != nil {
			return err
		}
		if err := statetransition.StateTransition(post, block); err != nil {
			return fmt.Errorf("slot %d: native transition rejects a block the node imported: %w", block.Slot, err)
		}
		native, err := zkstf.Apply(input)
		if err != nil {
			return fmt.Errorf("slot %d: %w", block.Slot, err)
		}
		res, err := prover.Execute(ctx, input)
		if err != nil {
			return fmt.Errorf("slot %d: %s: %w", block.Slot, prover.ZKVM(), err)
		}

		pv := res.PublicValues
		switch {
		case pv != native:
			return fmt.Errorf("slot %d: %s public values %v differ from native %v", block.Slot, prover.ZKVM(), pv, native)
		case pv.PostStateRoot != block.StateRoot:
			return fmt.Errorf("slot %d: post-state root 0x%x, block commits 0x%x", block.Slot, pv.PostStateRoot, block.StateRoot)
		case prevPost != nil && pv.PreStateRoot != *prevPost:
			return fmt.Errorf("slot %d: pre-state root 0x%x does not chain from previous post-state 0x%x", block.Slot, pv.PreStateRoot, *prevPost)
		}
		if err := onBlock(Block{Slot: block.Slot, Attestations: len(block.Body.Attestations), Input: input, Result: res}); err != nil {
			return err
		}
		pre, prevPost = post, &pv.PostStateRoot
		replayed++
	}
	if replayed == 0 {
		return ErrNoBlocks
	}
	return nil
}

// Client reads blocks and states from a node's replay routes.
type Client struct {
	base string
	http *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: fetchTimeout}}
}

func (c *Client) SignedBlock(ctx context.Context, id string) (*types.SignedBlock, error) {
	data, err := c.get(ctx, "/lean/v0/blocks/"+id+"/ssz")
	if err != nil {
		return nil, err
	}
	var sb types.SignedBlock
	if err := sb.UnmarshalSSZ(data); err != nil {
		return nil, fmt.Errorf("decode block: %w", err)
	}
	return &sb, nil
}

func (c *Client) State(ctx context.Context, id string) (*types.State, error) {
	data, err := c.get(ctx, "/lean/v0/states/"+id)
	if err != nil {
		return nil, err
	}
	var st types.State
	if err := st.UnmarshalSSZ(data); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	return &st, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFetchBytes {
		return nil, fmt.Errorf("GET %s: response over %d bytes", path, maxFetchBytes)
	}
	return data, nil
}
