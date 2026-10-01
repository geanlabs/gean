//! The SP1 guest: proves gean_stf::apply on one framed input. A rejected input
//! panics before anything is committed, so it has no proof.
#![no_main]
sp1_zkvm::entrypoint!(main);

pub fn main() {
    let input = sp1_zkvm::io::read_vec();
    match gean_stf::apply(&input) {
        Ok(pv) => sp1_zkvm::io::commit_slice(&pv),
        Err(e) => panic!("{e}"),
    }
}
