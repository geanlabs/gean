//! ziskhost proves and verifies the state-transition guest ELF on ZisK.
//!
//! Usage:
//!
//!   ziskhost prove  <guest.elf> <input.bin> <proof.bin>
//!   ziskhost verify <guest.elf> <proof.bin>
//!
//! The input file is passed to the guest as one stdin record. `prove` writes a
//! VADCOP proof and `verify` checks one; both print `vk=<hex> pv=<hex>`, where
//! `vk` is the program verifying key derived from the ELF (four little-endian u64
//! limbs) and `pv` the 256 public output bytes. A guest that fails exits 3 and an
//! invalid proof exits 4, so the caller can tell them from a host error (exit 1).
//!
//! Execution without proving stays on ziskemu.
//!
//! A ZisK proof embeds both the program key and the recursion setup key, and
//! the SDK verifies against the embedded ones unless told otherwise. `verify`
//! pins both: the program key is derived from the ELF, and the setup key is the
//! `vadcop_final` verifying key read from `ZISK_SETUP_VK` (a JSON array of four
//! u64 limbs) or from the proving key at `ZISK_PROVING_KEY` (default
//! `~/.zisk/provingKey`).

use std::path::PathBuf;
use std::process::ExitCode;

use zisk_sdk::{GuestProgram, ProgramVK, Proof, ProverClient, ZiskStdin};

const GUEST_FAILED: u8 = 3;
const INVALID_PROOF: u8 = 4;

/// ZisK's public output: 64 u32 slots.
const PUBLICS_LEN: usize = 64 * 4;

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    match args.as_slice() {
        ["prove", elf, input, proof] => prove(elf, input, proof),
        ["verify", elf, proof] => verify(elf, proof),
        _ => {
            eprintln!("usage: ziskhost prove <elf> <input> <proof> | verify <elf> <proof>");
            ExitCode::from(1)
        }
    }
}

fn prove(elf: &str, input: &str, out: &str) -> ExitCode {
    let program = program(elf);
    let mut builder = ProverClient::embedded();
    if let Some(pk) = proving_key_env() {
        builder = builder.proving_key(pk);
    }
    let client = builder
        .build()
        .unwrap_or_else(|e| fatal(&format!("client: {e}")));
    client
        .setup(&program)
        .run_sync()
        .unwrap_or_else(|e| fatal(&format!("setup: {e}")));
    // A failed guest has no proof: proving re-executes it and errors.
    let result = match client
        .prove(&program, ZiskStdin::from_bytes(read(input)))
        .run_sync()
    {
        Ok(result) => result,
        Err(e) => {
            eprintln!("guest failed: {e}");
            return ExitCode::from(GUEST_FAILED);
        }
    };
    let proof = result.get_proof();
    let vk = program.vk().unwrap_or_else(|e| fatal(&format!("vk: {e}")));
    if let Err(code) = check(proof, &vk) {
        return code;
    }
    proof
        .save(out)
        .unwrap_or_else(|e| fatal(&format!("save {out}: {e}")));
    println!("vk={} pv={}", vk_hex(&vk), publics_hex(proof));
    ExitCode::SUCCESS
}

fn verify(elf: &str, path: &str) -> ExitCode {
    let proof = match Proof::load(path) {
        Ok(proof) => proof,
        Err(e) => {
            eprintln!("invalid proof: {e}");
            return ExitCode::from(INVALID_PROOF);
        }
    };
    // The hash family only selects how the key is derived from our ELF; the
    // pinned setup key belongs to one family, so a wrong one cannot verify.
    let vk = program(elf)
        .vk_with_mode(proof.get_program_vk().hash_mode)
        .unwrap_or_else(|e| fatal(&format!("vk: {e}")));
    if let Err(code) = check(&proof, &vk) {
        return code;
    }
    println!("vk={} pv={}", vk_hex(&vk), publics_hex(&proof));
    ExitCode::SUCCESS
}

/// check verifies proof against the program key vk and the trusted setup key.
fn check(proof: &Proof, vk: &ProgramVK) -> Result<(), ExitCode> {
    let setup_vk = setup_vk();
    match proof.with_program_vk(vk).with_setup_vk(&setup_vk).verify() {
        Ok(()) => Ok(()),
        Err(e) => {
            eprintln!("invalid proof: {e}");
            Err(ExitCode::from(INVALID_PROOF))
        }
    }
}

fn setup_vk() -> Vec<u64> {
    let path = match std::env::var_os("ZISK_SETUP_VK") {
        Some(p) => PathBuf::from(p),
        None => {
            let pk = proving_key_env().unwrap_or_else(|| home().join(".zisk").join("provingKey"));
            let info = read_json(&pk.join("pilout.globalInfo.json"));
            let name = info
                .get("name")
                .and_then(|v| v.as_str())
                .unwrap_or_else(|| fatal("pilout.globalInfo.json has no name"));
            pk.join(name)
                .join("vadcop_final")
                .join("vadcop_final.verkey.json")
        }
    };
    let limbs = match read_json(&path) {
        serde_json::Value::Array(limbs) if limbs.len() == 4 => limbs,
        _ => fatal(&format!(
            "{}: want a JSON array of four u64 limbs",
            path.display()
        )),
    };
    limbs
        .iter()
        .map(|l| match l {
            serde_json::Value::Number(n) => n.as_u64(),
            serde_json::Value::String(s) => s.parse().ok(),
            _ => None,
        })
        .map(|l| l.unwrap_or_else(|| fatal(&format!("{}: limb is not a u64", path.display()))))
        .collect()
}

fn proving_key_env() -> Option<PathBuf> {
    std::env::var_os("ZISK_PROVING_KEY").map(PathBuf::from)
}

fn home() -> PathBuf {
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| fatal("HOME is not set"))
}

fn program(elf: &str) -> GuestProgram {
    GuestProgram::from_bytes("stf", read(elf))
}

fn publics_hex(proof: &Proof) -> String {
    let mut pv = [0u8; PUBLICS_LEN];
    proof.get_publics().read_slice(&mut pv);
    hex(&pv)
}

fn vk_hex(vk: &ProgramVK) -> String {
    let b: Vec<u8> = vk.vk.iter().flat_map(|l| l.to_le_bytes()).collect();
    hex(&b)
}

fn read_json(path: &std::path::Path) -> serde_json::Value {
    let s = std::fs::read_to_string(path)
        .unwrap_or_else(|e| fatal(&format!("read {}: {e}", path.display())));
    serde_json::from_str(&s).unwrap_or_else(|e| fatal(&format!("parse {}: {e}", path.display())))
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
