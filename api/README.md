# gean API

Lightweight HTTP API for Lean endpoints using Go's standard library.

## Base URL

`http://<host>:<port>` (defaults to `http://0.0.0.0:5058`)

## Routes

### `GET /lean/v0/health`

**Purpose:** Liveness probe.

**Response (200, application/json):**
```json
{"status":"healthy","service":"lean-rpc-api"}
```

---

### `GET /lean/v0/states/finalized`

**Purpose:** Fetch finalized state as raw SSZ bytes.

**Response (200, application/octet-stream):**
- Body: SSZ bytes of the finalized `State`.

**Common responses:**
- `404`: Finalized state not available yet.
- `503`: Store not initialized.

**Example:**
```sh
curl -o finalized_states.ssz http://localhost:5058/lean/v0/states/finalized
```

---

### `GET /lean/v0/checkpoints/justified`

**Purpose:** Latest justified checkpoint.

**Response (200, application/json):**
```json
{"slot":56,"root":"0x..."}
```

**Common responses:**
- `503`: Store not initialized.

---

### `GET /lean/v0/fork_choice`

**Purpose:** Fork choice snapshot for monitoring.

**Response (200, application/json):**
```json
{
  "nodes": [
    {
      "root": "0x...",
      "slot": 62,
      "parent_root": "0x...",
      "proposer_index": 2,
      "weight": 5
    }
  ],
  "head": "0x...",
  "justified": {"slot": 63, "root": "0x..."},
  "finalized": {"slot": 62, "root": "0x..."},
  "safe_target": "0x...",
  "validator_count": 5
}
```

**Common responses:**
- `503`: Store not initialized.

## Configuration

Enable and configure via CLI flags:

- `--api-host` (default `0.0.0.0`)
- `--api-port` (default `5058`, `0` disables)
- `--api-enabled` (default `true`)

## Notes

- `/states/finalized` is binary output. Use `curl -o` to avoid terminal warnings.
- `weight` reflects current fork choice vote weight based on latest known attestations.
