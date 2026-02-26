/*
Logup* (Lev Soukhanov)

https://eprint.iacr.org/2025/946.pdf

*/

mod quotient_gkr;
pub use quotient_gkr::*;

mod logup_star;
pub use logup_star::*;

pub(crate) const MIN_VARS_FOR_PACKING: usize = 8;
