//! The ZisK guest: proves gean_stf::apply on one framed input. A rejected input
//! halts with an error before anything is committed, so it has no proof.
#![no_main]
ziskos::entrypoint!(main);

fn main() {
    let input = ziskos::io::read_slice();
    match gean_stf::apply(&input) {
        Ok(pv) => ziskos::io::commit_slice(&pv),
        Err(e) => {
            println!("{e}");
            halt_with_error()
        }
    }
}

/// halt_with_error stops the guest as a failure. ZisK transpiles the all-ones
/// word, a reserved encoding, to its halt_with_error operation. A panic cannot
/// be used: it does not stop ZisK's emulator, and ZisK's exit call does not
/// make a non-zero exit code unprovable.
fn halt_with_error() -> ! {
    // SAFETY: the word only halts the machine; it touches no memory.
    unsafe { core::arch::asm!(".word 0xffffffff", options(noreturn)) }
}
