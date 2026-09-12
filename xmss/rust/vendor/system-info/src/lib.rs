// Modified by Gean: startup-configurable thread count; see ../README.md.
use std::sync::OnceLock;

const _: () = assert!(
    usize::BITS == 64,
    "this project requires a 64-bit target (for now)"
);

static NUM_THREADS: OnceLock<usize> = OnceLock::new();

/// Set the process-wide pool size before any backend operation. Zero selects auto.
/// Once resolved, only the same value may be configured again.
pub fn configure_num_threads(requested: usize) -> Option<usize> {
    let available = std::thread::available_parallelism().ok()?.get();
    let n = if requested == 0 { available } else { requested };
    if n > available {
        return None;
    }
    let actual = *NUM_THREADS.get_or_init(|| n);
    (actual == n).then_some(actual)
}

#[must_use]
pub fn num_threads() -> usize {
    *NUM_THREADS.get_or_init(|| {
        std::thread::available_parallelism()
            .expect("failed to detect available parallelism")
            .get()
    })
}

#[must_use]
pub fn l1_cache_size() -> usize {
    static CACHE: OnceLock<usize> = OnceLock::new();
    *CACHE.get_or_init(|| {
        detect_l1_cache_size().unwrap_or_else(|| {
            eprintln!("Warning: failed to detect L1 cache size, defaulting to 32 KB");
            32 * 1024
        })
    })
}

pub fn peak_rss_bytes() -> u64 {
    let mut ru: libc::rusage = unsafe { std::mem::zeroed() };
    unsafe { libc::getrusage(libc::RUSAGE_SELF, &raw mut ru) };
    let max = ru.ru_maxrss as u64;
    // ru_maxrss unit: bytes on macOS, KiB on Linux.
    if cfg!(target_os = "macos") {
        max
    } else {
        max * 1024
    }
}

#[cfg(target_os = "linux")]
fn detect_l1_cache_size() -> Option<usize> {
    // /sys reports e.g. "32K\n", "48K\n", "1M\n".
    let s = std::fs::read_to_string("/sys/devices/system/cpu/cpu0/cache/index0/size").ok()?;
    let s = s.trim();
    let last = s.chars().last()?;
    match last {
        'K' | 'k' => s[..s.len() - 1].parse::<usize>().ok().map(|n| n * 1024),
        'M' | 'm' => s[..s.len() - 1]
            .parse::<usize>()
            .ok()
            .map(|n| n * 1024 * 1024),
        c if c.is_ascii_digit() => s.parse().ok(),
        _ => None,
    }
}

#[cfg(target_os = "macos")]
fn detect_l1_cache_size() -> Option<usize> {
    // `hw.l1dcachesize` returns the E-core value on Apple Silicon; prefer the P-core size.
    let read_sysctl = |key: &str| -> Option<usize> {
        let out = std::process::Command::new("sysctl")
            .args(["-n", key])
            .output()
            .ok()?;
        std::str::from_utf8(&out.stdout).ok()?.trim().parse().ok()
    };
    read_sysctl("hw.perflevel0.l1dcachesize").or_else(|| read_sysctl("hw.l1dcachesize"))
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
fn detect_l1_cache_size() -> Option<usize> {
    None
}
