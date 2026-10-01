//! sp1host runs, proves and verifies the state-transition guest ELF on SP1.
//!
//! Usage:
//!
//!   sp1host execute <guest.elf> <input.bin>
//!   sp1host prove   <guest.elf> <input.bin> <proof.bin>
//!   sp1host verify  <guest.elf> <proof.bin>
//!
//! The input file is passed to the guest as one hint record. `execute` prints
//! `cycles=<n> pv=<hex>`; `prove` writes a core proof and prints
//! `vk=<hex> pv=<hex>`; `verify` checks a proof against the program's
//! verifying key, requiring exit code 0, and prints `vk=<hex> pv=<hex>`. A
//! guest that fails (an execution error or a non-zero exit code) exits 3 and an
//! invalid proof exits 4, so the caller can tell them from a host error (exit 1).

use std::process::ExitCode;

use sp1_sdk::blocking::{ProveRequest, Prover, ProverClient};
use sp1_sdk::{Elf, HashableKey, ProvingKey, SP1ProofWithPublicValues, SP1Stdin, SP1VerifyingKey};

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
                "usage: sp1host execute <elf> <input> | prove <elf> <input> <proof> | verify <elf> <proof>"
            );
            ExitCode::from(1)
        }
    }
}

fn execute(elf: &str, input: &str) -> ExitCode {
    match run(elf, input) {
        Ok((pv, cycles)) => {
            println!("cycles={cycles} pv={}", hex(&pv));
            ExitCode::SUCCESS
        }
        Err(code) => code,
    }
}

/// run executes the guest and returns its public values and cycle count, or
/// GUEST_FAILED when it errors or exits with a code other than 0.
fn run(elf: &str, input: &str) -> Result<(Vec<u8>, u64), ExitCode> {
    let client = ProverClient::builder().light().build();
    match client.execute(read_elf(elf), stdin(input)).run() {
        Ok((pv, report)) if report.exit_code == 0 => {
            let cycles = report.total_instruction_count() + report.total_syscall_count();
            Ok((pv.as_slice().to_vec(), cycles))
        }
        Ok((_, report)) => {
            eprintln!("guest exited with code {}", report.exit_code);
            Err(ExitCode::from(GUEST_FAILED))
        }
        Err(e) => {
            eprintln!("guest failed: {e}");
            Err(ExitCode::from(GUEST_FAILED))
        }
    }
}

fn prove(elf: &str, input: &str, out: &str) -> ExitCode {
    let client = ProverClient::builder().cpu().build();
    let pk = client
        .setup(read_elf(elf))
        .unwrap_or_else(|e| fatal(&format!("setup: {e}")));
    // SP1 proves a run that halts with any exit code, so a failed guest is
    // refused before proving.
    if let Err(code) = run(elf, input) {
        return code;
    }
    let proof = match client.prove(&pk, stdin(input)).core().run() {
        Ok(proof) => proof,
        Err(e) => {
            eprintln!("guest failed: {e}");
            return ExitCode::from(GUEST_FAILED);
        }
    };
    if let Err(e) = client.verify(&proof, pk.verifying_key(), None) {
        eprintln!("fresh proof does not verify: {e}");
        return ExitCode::from(INVALID_PROOF);
    }
    proof
        .save(out)
        .unwrap_or_else(|e| fatal(&format!("save {out}: {e}")));
    println!(
        "vk={} pv={}",
        hex(&pk.verifying_key().bytes32_raw()),
        hex(proof.public_values.as_slice())
    );
    ExitCode::SUCCESS
}

fn verify(elf: &str, path: &str) -> ExitCode {
    let vk = vkey(elf);
    let proof = match SP1ProofWithPublicValues::load(path) {
        Ok(proof) => proof,
        Err(e) => {
            eprintln!("invalid proof: {e}");
            return ExitCode::from(INVALID_PROOF);
        }
    };
    let client = ProverClient::builder().light().build();
    // None requires exit code 0: a halt with any other code is not a success.
    if let Err(e) = client.verify(&proof, &vk, None) {
        eprintln!("invalid proof: {e}");
        return ExitCode::from(INVALID_PROOF);
    }
    println!(
        "vk={} pv={}",
        hex(&vk.bytes32_raw()),
        hex(proof.public_values.as_slice())
    );
    ExitCode::SUCCESS
}

fn vkey(elf: &str) -> SP1VerifyingKey {
    let client = ProverClient::builder().light().build();
    let pk = client
        .setup(read_elf(elf))
        .unwrap_or_else(|e| fatal(&format!("setup: {e}")));
    pk.verifying_key().clone()
}

fn read_elf(path: &str) -> Elf {
    Elf::from(read(path))
}

fn stdin(path: &str) -> SP1Stdin {
    let mut stdin = SP1Stdin::new();
    stdin.write_vec(read(path));
    stdin
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
