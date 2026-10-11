//! openvmhost runs, proves and verifies the state-transition guest ELF on
//! OpenVM (RV64, v2.x.0-preview.2). The VM has the SHA-2 extension, whose
//! SHA-256 compress instruction the guest's SSZ hashing uses.
//!
//! Usage:
//!
//!   openvmhost execute <guest.elf> <input.bin>
//!   openvmhost prove   <guest.elf> <input.bin> <proof.bin>
//!   openvmhost verify  <guest.elf> <proof.bin>
//!
//! The input file is passed to the guest as one stdin vector. `execute` prints
//! `cycles=<n> pv=<hex>`; `prove` writes an app proof and prints
//! `vk=<hex> pv=<hex>`; `verify` checks a proof against the app verifying key
//! and the program's execution commitment, requiring exit code 0, and prints
//! `vk=<hex> pv=<hex>`. `vk` is the app execution commitment of the ELF, which
//! identifies the program. A guest that fails (an execution error or a non-zero
//! exit code) exits 3 and an invalid proof exits 4, so the caller can tell them
//! from a host error (exit 1).

use std::process::ExitCode;
use std::sync::Arc;

use openvm_circuit::arch::{instructions::exe::VmExe, SystemConfig};
use openvm_continuations::CommitBytes;
use openvm_sdk::config::{AggregationSystemParams, AppConfig};
use openvm_sdk::prover::verify_app_proof_with_expected_exe_commit;
use openvm_sdk::{fs, DefaultStarkEngine, Sdk, StdIn};
use openvm_sdk_config::SdkVmConfig;
use openvm_stark_sdk::config::{app_params_with_100_bits_security, MAX_APP_LOG_STACKED_HEIGHT};
use openvm_stark_sdk::openvm_stark_backend::p3_field::PrimeField32;

type AppProof = openvm_circuit::arch::ContinuationVmProof<openvm_sdk::SC>;

const GUEST_FAILED: u8 = 3;
const INVALID_PROOF: u8 = 4;

/// The guest reveals its 96-byte public values zero-padded to 128: OpenVM's
/// public values size is a power-of-two number of dwords.
const PUBLIC_VALUES: usize = 128;

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    match args.as_slice() {
        ["execute", elf, input] => execute(elf, input),
        ["prove", elf, input, proof] => prove(elf, input, proof),
        ["verify", elf, proof] => verify(elf, proof),
        _ => {
            eprintln!(
                "usage: openvmhost execute <elf> <input> | prove <elf> <input> <proof> | verify <elf> <proof>"
            );
            ExitCode::from(1)
        }
    }
}

fn sdk() -> Sdk {
    let vm = SdkVmConfig::builder()
        .system(
            SystemConfig::default()
                .with_public_values(PUBLIC_VALUES)
                .into(),
        )
        .rv64i(Default::default())
        .rv64m(Default::default())
        .io(Default::default())
        .sha2(Default::default())
        .build()
        .optimize();
    let app = AppConfig::new(
        vm,
        app_params_with_100_bits_security(MAX_APP_LOG_STACKED_HEIGHT),
    );
    Sdk::new(app, AggregationSystemParams::default())
        .unwrap_or_else(|e| fatal(&format!("sdk: {e}")))
}

fn execute(elf: &str, input: &str) -> ExitCode {
    let sdk = sdk();
    let exe = exe(&sdk, elf);
    match sdk.compile_and_execute_metered_cost(exe, stdin(input)) {
        Ok((pv, (_cost, instret))) => {
            println!("cycles={instret} pv={}", hex(&pv));
            ExitCode::SUCCESS
        }
        Err(e) => {
            eprintln!("guest failed: {e}");
            ExitCode::from(GUEST_FAILED)
        }
    }
}

fn prove(elf: &str, input: &str, out: &str) -> ExitCode {
    let sdk = sdk();
    let exe = exe(&sdk, elf);
    let mut prover = sdk
        .app_prover(exe)
        .unwrap_or_else(|e| fatal(&format!("app prover: {e}")));
    let commit = prover.app_exe_commit();
    // A failed guest has no proof: proving re-executes it and errors.
    let proof = match prover.prove(stdin(input)) {
        Ok(proof) => proof,
        Err(e) => {
            eprintln!("guest failed: {e}");
            return ExitCode::from(GUEST_FAILED);
        }
    };
    if let Err(e) = verify_app_proof_with_expected_exe_commit::<DefaultStarkEngine>(
        &sdk.app_vk(),
        &proof,
        Some(commit),
    ) {
        eprintln!("fresh proof does not verify: {e}");
        return ExitCode::from(INVALID_PROOF);
    }
    fs::write_object_to_file(out, &proof).unwrap_or_else(|e| fatal(&format!("save {out}: {e}")));
    println!(
        "vk={} pv={}",
        hex(CommitBytes::from(commit).as_slice()),
        hex(&public_values(&proof))
    );
    ExitCode::SUCCESS
}

fn verify(elf: &str, path: &str) -> ExitCode {
    let proof: AppProof = match fs::read_object_from_file(path) {
        Ok(proof) => proof,
        Err(e) => {
            eprintln!("invalid proof: {e}");
            return ExitCode::from(INVALID_PROOF);
        }
    };
    let sdk = sdk();
    let exe = exe(&sdk, elf);
    let commit = exe_commit(&sdk, exe);
    // Verification requires the last segment to terminate with exit code 0.
    if let Err(e) = verify_app_proof_with_expected_exe_commit::<DefaultStarkEngine>(
        &sdk.app_vk(),
        &proof,
        Some(commit.into()),
    ) {
        eprintln!("invalid proof: {e}");
        return ExitCode::from(INVALID_PROOF);
    }
    println!(
        "vk={} pv={}",
        hex(commit.as_slice()),
        hex(&public_values(&proof))
    );
    ExitCode::SUCCESS
}

fn exe(sdk: &Sdk, elf: &str) -> Arc<VmExe> {
    sdk.convert_to_exe(read(elf))
        .unwrap_or_else(|e| fatal(&format!("load {elf}: {e}")))
}

fn exe_commit(sdk: &Sdk, exe: Arc<VmExe>) -> CommitBytes {
    let prover = sdk
        .app_prover(exe)
        .unwrap_or_else(|e| fatal(&format!("app prover: {e}")));
    CommitBytes::from(prover.app_exe_commit())
}

/// public_values returns the proof's user public values, one byte per field
/// element.
fn public_values(proof: &AppProof) -> Vec<u8> {
    proof
        .user_public_values
        .public_values
        .iter()
        .map(|f| {
            u8::try_from(f.as_canonical_u32())
                .unwrap_or_else(|_| fatal("public value is not a byte"))
        })
        .collect()
}

fn stdin(path: &str) -> StdIn {
    let mut stdin = StdIn::default();
    stdin.write_bytes(&read(path));
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
