//! The RISC Zero guest: proves gean_stf::apply on one framed input. A rejected
//! input panics before anything is committed, so it has no proof.
use std::io::Read;

use risc0_zkvm::guest::env;

fn main() {
    let mut input = Vec::new();
    env::stdin().read_to_end(&mut input).unwrap();
    match gean_stf::apply(&input) {
        Ok(pv) => env::commit_slice(&pv),
        Err(e) => panic!("{e}"),
    }
}
