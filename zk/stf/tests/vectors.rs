//! Holds the port equal to the Go transition: every vector written by
//! `stfprove vectors` (make zk-vectors) must reach the outcome Go recorded.

use std::path::PathBuf;

use gean_stf::{apply, Error};

/// Inputs on which typed SSZ is stricter than fastssz, as leanSpec's typed
/// containers are. The port must reject each of them.
const DIVERGENCES: &[&str] = &[
    // The transition extends JustifiedSlots past its 2^18-bit limit, which
    // fastssz hashes regardless.
    "divergence/justified-slots-over-limit",
    // An empty list encoded as a 4-byte zero offset, which fastssz decodes.
    "divergence/empty-list-offset",
];

#[test]
fn port_matches_go_transition() {
    let dir = std::env::var_os("GEAN_STF_VECTORS")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../out/vectors"));
    let manifest = std::fs::read_to_string(dir.join("manifest.txt"))
        .unwrap_or_else(|e| panic!("{}: {e} (run make zk-vectors)", dir.display()));

    let mut failures = Vec::new();
    for line in manifest.lines() {
        let fields: Vec<&str> = line.split(' ').collect();
        let (file, name, want) = (fields[0], fields[1], &fields[2..]);
        let input = std::fs::read(dir.join(file)).unwrap();
        let got = apply(&input);
        let ok = if DIVERGENCES.contains(&name) {
            got.is_err()
        } else {
            match (want, &got) {
                (["ok", pv], Ok(got)) => *pv == hex(got),
                (["err", kind, computed], Err(Error::StateRootMismatch { computed: got, .. })) => {
                    *kind == "state_root_mismatch" && *computed == hex(got)
                }
                (["err", kind], Err(e)) => *kind == e.kind(),
                _ => false,
            }
        };
        if !ok {
            failures.push(format!(
                "{name}: Go {want:?}, Rust {:?}",
                got.map(|pv| hex(&pv))
            ));
        }
    }
    assert!(
        failures.is_empty(),
        "{} of {} vectors differ:\n{}",
        failures.len(),
        manifest.lines().count(),
        failures.join("\n")
    );
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}
