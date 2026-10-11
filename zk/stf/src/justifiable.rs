//! justifiable.go

/// slot_is_justifiable_after reports whether slot may be justified given the
/// finalized slot, following leanSpec's slot.is_justifiable_after with integer
/// arithmetic only.
pub fn slot_is_justifiable_after(slot: u64, finalized_slot: u64) -> bool {
    if slot < finalized_slot {
        return false;
    }
    let delta = slot - finalized_slot;

    if delta <= 5 {
        return true;
    }

    let root = isqrt(delta);
    if root * root == delta {
        return true;
    }

    // Pronic distances n(n+1): for delta = n(n+1), isqrt(delta) = n, and
    // root*(root+1) cannot overflow (root < 2^32).
    root * (root + 1) == delta
}

/// isqrt returns floor(sqrt(n)) using Newton's method from an upper bound.
fn isqrt(n: u64) -> u64 {
    if n < 2 {
        return n;
    }
    let mut x = 1u64 << (64 - n.leading_zeros()).div_ceil(2);
    loop {
        let y = (x + n / x) / 2;
        if y >= x {
            return x;
        }
        x = y;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn isqrt_is_floor_sqrt() {
        for n in [0, 1, 2, 3, 4, 15, 16, 17, (1 << 32) - 1, 1 << 32, u64::MAX] {
            let r = isqrt(n) as u128;
            assert!(
                r * r <= n as u128 && (r + 1) * (r + 1) > n as u128,
                "isqrt({n}) = {r}"
            );
        }
    }

    #[test]
    fn justifiable_distances() {
        let justifiable: Vec<u64> = (0..31)
            .filter(|&d| slot_is_justifiable_after(100 + d, 100))
            .collect();
        assert_eq!(justifiable, [0, 1, 2, 3, 4, 5, 6, 9, 12, 16, 20, 25, 30]);
        assert!(!slot_is_justifiable_after(99, 100));
    }
}
