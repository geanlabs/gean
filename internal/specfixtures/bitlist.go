package specfixtures

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func ParseBoolBitlist(data []json.RawMessage) ([]byte, error) {
	length := uint64(len(data))
	if length == 0 {
		return types.NewBitlistSSZ(0), nil
	}
	bl := types.NewBitlistSSZ(length)
	for i, raw := range data {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("bitlist index %d: null value", i)
		}
		var val bool
		if err := json.Unmarshal(raw, &val); err != nil {
			var intVal int
			if err2 := json.Unmarshal(raw, &intVal); err2 != nil {
				return nil, fmt.Errorf("bitlist index %d: not bool or int: %w / %w", i, err, err2)
			}
			if intVal != 0 && intVal != 1 {
				return nil, fmt.Errorf("bitlist index %d: integer value %d is not 0 or 1", i, intVal)
			}
			val = intVal != 0
		}
		if val {
			types.BitlistSet(bl, uint64(i))
		}
	}
	return bl, nil
}
