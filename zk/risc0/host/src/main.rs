//! risc0host runs, proves and verifies the state-transition guest on RISC Zero.
//!
//! Usage:
//!
//!   risc0host execute <guest.bin> <input.bin>
//!   risc0host prove   <guest.bin> <input.bin> <proof.bin>
//!   risc0host verify  <guest.bin> <proof.bin>
//!
//! The input file is the guest's stdin. `execute` prints `cycles=<n> pv=<hex>`;
//! `prove` writes a composite receipt and prints `vk=<hex> pv=<hex>`; `verify`
//! checks a receipt against the program's image id, which requires the guest to
//! have halted with exit code 0, and prints `vk=<hex> pv=<hex>`. `vk` is the
//! image id. A guest that fails exits 3 and an invalid proof exits 4, so the
//! caller can tell them from a host error (exit 1).

use std::process::ExitCode;

use risc0_zkvm::{
    compute_image_id, default_executor, default_prover, Digest, ExecutorEnv, ExitCode as GuestExit,
    ProverOpts, Receipt,
};

const GUEST_FAILED: u8 = 3;
const INVALID_PROOF: u8 = 4;

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    match args.as_slice() {
        ["execute", elf, input] => execute(elf, input),
        ["prove", elf, input, proof] => prove(elf, input, proof),
        ["verify", elf, proof] => verify(elf, proof),
        _ => {
            eprintln!(
                "usage: risc0host execute <elf> <input> | prove <elf> <input> <proof> | verify <elf> <proof>"
            );
            ExitCode::from(1)
        }
    }
}

fn execute(elf: &str, input: &str) -> ExitCode {
    let elf = read(elf);
    match default_executor().execute(env(input), &elf) {
        Ok(session) if session.exit_code == GuestExit::Halted(0) => {
            println!(
                "cycles={} pv={}",
                session.cycles(),
                hex(&session.journal.bytes)
            );
            ExitCode::SUCCESS
        }
        Ok(session) => {
            eprintln!("guest exited with {:?}", session.exit_code);
            ExitCode::from(GUEST_FAILED)
        }
        Err(e) => {
            eprintln!("guest failed: {e}");
            ExitCode::from(GUEST_FAILED)
        }
    }
}

fn prove(elf: &str, input: &str, out: &str) -> ExitCode {
    let elf = read(elf);
    let image_id = image_id(&elf);
    // A failed guest has no proof: proving executes it first and errors.
    let receipt = match default_prover().prove_with_opts(env(input), &elf, &ProverOpts::composite())
    {
        Ok(info) => info.receipt,
        Err(e) => {
            eprintln!("guest failed: {e}");
            return ExitCode::from(GUEST_FAILED);
        }
    };
    if let Err(e) = receipt.verify(image_id) {
        eprintln!("fresh proof does not verify: {e}");
        return ExitCode::from(INVALID_PROOF);
    }
    let bytes = bincode::serialize(&receipt).unwrap_or_else(|e| fatal(&format!("encode: {e}")));
    std::fs::write(out, bytes).unwrap_or_else(|e| fatal(&format!("save {out}: {e}")));
    println!(
        "vk={} pv={}",
        hex(image_id.as_bytes()),
        hex(&receipt.journal.bytes)
    );
    ExitCode::SUCCESS
}

fn verify(elf: &str, path: &str) -> ExitCode {
    let image_id = image_id(&read(elf));
    let receipt: Receipt = match bincode::deserialize(&read(path)) {
        Ok(receipt) => receipt,
        Err(e) => {
            eprintln!("invalid proof: {e}");
            return ExitCode::from(INVALID_PROOF);
        }
    };
    if let Err(e) = receipt.verify(image_id) {
        eprintln!("invalid proof: {e}");
        return ExitCode::from(INVALID_PROOF);
    }
    println!(
        "vk={} pv={}",
        hex(image_id.as_bytes()),
        hex(&receipt.journal.bytes)
    );
    ExitCode::SUCCESS
}

fn env(input: &str) -> ExecutorEnv<'static> {
    ExecutorEnv::builder()
        .write_slice(&read(input))
        .build()
        .unwrap_or_else(|e| fatal(&format!("executor env: {e}")))
}

fn image_id(elf: &[u8]) -> Digest {
    compute_image_id(elf).unwrap_or_else(|e| fatal(&format!("image id: {e}")))
}

fn read(path: &str) -> Vec<u8> {
    std::fs::read(path).unwrap_or_else(|e| fatal(&format!("read {path}: {e}")))
}

fn fatal(msg: &str) -> ! {
    eprintln!("{msg}");
    std::process::exit(1)
}

fn hex(b: &[u8]) -> String {
    b.iter().map(|x| format!("{x:02x}")).collect()
}
