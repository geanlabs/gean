#[test]
fn configured_count_reaches_backend() {
    assert_eq!(multisig_glue::xmss_configure_prover_threads(1), 1);
    assert_eq!(backend::parallel::num_threads(), 1);
    backend::parallel::init();
    let sum = std::sync::atomic::AtomicUsize::new(0);
    backend::parallel::for_each_index(32, |i| {
        sum.fetch_add(i, std::sync::atomic::Ordering::Relaxed);
    });
    assert_eq!(sum.load(std::sync::atomic::Ordering::Relaxed), 496);
    assert_eq!(multisig_glue::xmss_configure_prover_threads(2), 0);
    assert_eq!(backend::parallel::num_threads(), 1);
}
