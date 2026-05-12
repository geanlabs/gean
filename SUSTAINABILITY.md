# Gean — Mission, Vision & Sustainability Plan

---

## Mission

Deliver a Lean consensus client in Go that any engineer can read, understand, and verify end-to-end, giving the Go-speaking Ethereum infrastructure community a first-class option they do not need to take on trust.

---

## Vision

A widely-deployed Lean consensus client on Ethereum mainnet that remains simple and inspectable as the protocol evolves — where protocol conformance is routine, performance is proven by measurement, and every decision from gossip arrival to state commitment is traceable without specialist help.

---

## Sustainability Plan

### 1. Technical Sustainability

**The code stays good.**

Protocol conformance is treated as a first-class invariant. Every published conformance fixture runs on every PR, with harnesses covering state transition, fork choice, signatures, SSZ containers, API endpoints, and networking codecs. No protocol change merges until fixtures pass, or any divergence is explicitly documented with an upstream reference.


### 2. Ecosystem Sustainability

**Anyone still wants to use it.**

Gean contributes upstream to the protocol. Where conformance testing surfaces upstream ambiguities, issues are filed and PRs proposed. Gean is an upstream contributor, not merely a downstream implementer.


Tooling built for Gean is designed to benefit the broader Lean consensus ecosystem. Conformance-test work, multi-client testing helpers, and operator dashboards are reusable beyond Gean itself. The commitment is to harden and upstream tools that compound value across the whole ecosystem — fixture validators, interop smoke tests, Grafana dashboard exports, checkpoint-sync health checkers, and key-management helpers.


### 3. Organizational Sustainability

**The team still exists.**

Repository hygiene is enforced through mandatory PR reviews and CI gates on every push — covering build, vet, format, race-tested suite, and conformance fixtures. The goal is a low-friction contributor experience for any engineer who can write Go.

Releases are tagged in alignment with the Ethereum hard-fork schedule. Docker images are labelled with their source revision so operators can verify the exact build they are running.

Developer-facing documentation is solid today. Operator-facing documentation is thin, and closing that gap is an explicit commitment.


### 4. Funding and Continuity

**The money keeps flowing.**

Ethereum Foundation client-diversity grants and the Protocol Guild are the natural funding paths. Gean's defensible niche — the only Go implementation in the Lean client set — aligns directly with the Foundation's stated diversity objectives and lowers the barrier for the broader Go infrastructure community to participate in Lean Ethereum.

The founding team is two people today. The goal is four to six regular contributors by end of year, with an explicit mentorship track for Go engineers new to consensus-client work. Go has the lowest language-idiom barrier of any Lean client — an advantage to be converted into sustained contribution.

Gean runs where Go runs, which is everywhere Ethereum infrastructure already lives.

---

## 12-Month Roadmap

| Quarter | Milestones |
|---|---|
| **Q2 2026** | Devnet-4 parity complete; parallel signature verification merged; multi-client interop stable. |
| **Q3 2026** | Reorg-depth and table-size metrics; checkpoint-sync hardening; operator guide v1; multi-client interop test framework extracted as a standalone, shareable tool. |
| **Q4 2026** | Devnet-5 / testnet-alpha readiness; external audit engagement; Grafana dashboard exports published for ecosystem reuse; four or more regular contributors landing PRs. |
| **Q1 2027** | Mainnet readiness review; first tagged production release candidate; fuzzing harness for SSZ codecs upstreamed. |

---
