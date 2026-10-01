//! The OpenVM guest: proves gean_stf::apply on one framed input. A rejected
//! input panics before anything is revealed, so it has no proof.
openvm::entry!(main);

/// The VM's public values size (see zk/openvm/host): a power-of-two number of
/// u64 words, so the 96 bytes are revealed zero-padded.
const PUBLIC_VALUES: usize = 128;

fn main() {
    let input = openvm::io::read_vec();
    let pv = match gean_stf::apply(&input) {
        Ok(pv) => pv,
        Err(e) => panic!("{e}"),
    };
    let mut out = [0u8; PUBLIC_VALUES];
    out[..pv.len()].copy_from_slice(&pv);
    for (i, word) in out.chunks_exact(8).enumerate() {
        openvm::io::reveal_u64(u64::from_le_bytes(word.try_into().unwrap()), i);
    }
}
