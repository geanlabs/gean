include!(concat!(env!("OUT_DIR"), "/methods.rs"));

fn main() {
    let out = std::env::args()
        .nth(1)
        .expect("usage: stf-risc0-methods <out>");
    std::fs::write(&out, STF_RISC0_ELF).unwrap_or_else(|e| panic!("write {out}: {e}"));
}
