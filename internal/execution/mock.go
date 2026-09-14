package execution

import (
	"context"
	"sync"

	"github.com/geanlabs/gean/internal/types"
)

// Mock is a scriptable Engine for tests. Each hook, when set, decides the
// reply; unset hooks return benign defaults. Calls are recorded so a test can
// assert what the node sent.
type Mock struct {
	mu sync.Mutex

	OnForkchoiceUpdated func(state ForkchoiceState, attrs *PayloadAttributes) (ForkchoiceUpdatedResult, error)
	OnGetPayload        func(id PayloadID) (*types.ExecutionPayload, error)
	OnNewPayload        func(payload *types.ExecutionPayload, parentBeaconBlockRoot [32]byte) (PayloadStatus, error)
	Genesis             [32]byte
	Supported           []string

	ForkchoiceCalls []ForkchoiceCall
	NewPayloadCalls []NewPayloadCall
	GetPayloadCalls []PayloadID
}

type ForkchoiceCall struct {
	State ForkchoiceState
	Attrs *PayloadAttributes
}

type NewPayloadCall struct {
	Payload               *types.ExecutionPayload
	ParentBeaconBlockRoot [32]byte
}

var _ Engine = (*Mock)(nil)

func (m *Mock) ForkchoiceUpdated(_ context.Context, state ForkchoiceState, attrs *PayloadAttributes) (ForkchoiceUpdatedResult, error) {
	m.mu.Lock()
	m.ForkchoiceCalls = append(m.ForkchoiceCalls, ForkchoiceCall{State: state, Attrs: attrs})
	hook := m.OnForkchoiceUpdated
	m.mu.Unlock()
	if hook != nil {
		return hook(state, attrs)
	}
	return ForkchoiceUpdatedResult{PayloadStatus: PayloadStatus{Status: StatusValid}}, nil
}

func (m *Mock) GetPayload(_ context.Context, id PayloadID) (*types.ExecutionPayload, error) {
	m.mu.Lock()
	m.GetPayloadCalls = append(m.GetPayloadCalls, id)
	hook := m.OnGetPayload
	m.mu.Unlock()
	if hook != nil {
		return hook(id)
	}
	return &types.ExecutionPayload{}, nil
}

func (m *Mock) NewPayload(_ context.Context, payload *types.ExecutionPayload, parentBeaconBlockRoot [32]byte) (PayloadStatus, error) {
	m.mu.Lock()
	m.NewPayloadCalls = append(m.NewPayloadCalls, NewPayloadCall{Payload: payload, ParentBeaconBlockRoot: parentBeaconBlockRoot})
	hook := m.OnNewPayload
	m.mu.Unlock()
	if hook != nil {
		return hook(payload, parentBeaconBlockRoot)
	}
	return PayloadStatus{Status: StatusValid}, nil
}

func (m *Mock) GenesisBlockHash(context.Context) ([32]byte, error) {
	return m.Genesis, nil
}

func (m *Mock) ExchangeCapabilities(_ context.Context, offered []string) ([]string, error) {
	if m.Supported != nil {
		return m.Supported, nil
	}
	return offered, nil
}

// Calls returns a snapshot of the recorded calls, safe to read while the node
// is still running.
func (m *Mock) Calls() (fcu []ForkchoiceCall, newPayload []NewPayloadCall, getPayload []PayloadID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ForkchoiceCall(nil), m.ForkchoiceCalls...),
		append([]NewPayloadCall(nil), m.NewPayloadCalls...),
		append([]PayloadID(nil), m.GetPayloadCalls...)
}
