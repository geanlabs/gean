use std::array;

use multilinear_toolkit::prelude::*;
use tracing::instrument;
use utils::{FSProver, FSVerifier};

use crate::MIN_VARS_FOR_PACKING;

/*
GKR to compute sum of fractions.
*/

#[instrument(skip_all)]
pub fn prove_gkr_quotient<EF: ExtensionField<PF<EF>>, const N_GROUPS: usize>(
    prover_state: &mut FSProver<EF, impl FSChallenger<EF>>,
    numerators_and_denominators: &MleGroupRef<'_, EF>,
) -> (EF, MultilinearPoint<EF>, EF, EF) {
    assert!(N_GROUPS.is_power_of_two() && N_GROUPS >= 2);
    assert_eq!(numerators_and_denominators.n_columns(), 2);
    let mut layers: Vec<MleGroup<'_, EF>> = vec![split_mle_group(numerators_and_denominators, N_GROUPS / 2).into()];

    loop {
        let prev_layer: MleGroup<'_, EF> = layers.last().unwrap().by_ref().into();
        let prev_layer = if prev_layer.is_packed() && prev_layer.n_vars() < MIN_VARS_FOR_PACKING {
            prev_layer.by_ref().unpack().as_owned_or_clone().into()
        } else {
            prev_layer
        };
        if prev_layer.n_vars() == 1 {
            break;
        }
        layers.push(sum_quotients(prev_layer.by_ref(), N_GROUPS).into());
    }

    let last_layer = layers.pop().unwrap();
    let last_layer = last_layer.as_owned_or_clone().as_extension().unwrap();

    assert_eq!(last_layer.len(), N_GROUPS);
    let last_nums_and_dens: [[_; 2]; N_GROUPS] = array::from_fn(|i| last_layer[i].to_vec().try_into().unwrap());
    for nd in &last_nums_and_dens {
        prover_state.add_extension_scalars(nd);
    }
    let quotient = (0..N_GROUPS / 2)
        .map(|i| last_nums_and_dens[i][0] / last_nums_and_dens[i + N_GROUPS / 2][0])
        .sum::<EF>()
        + (0..N_GROUPS / 2)
            .map(|i| last_nums_and_dens[i][1] / last_nums_and_dens[i + N_GROUPS / 2][1])
            .sum::<EF>();

    let mut point = MultilinearPoint(vec![prover_state.sample()]);
    let mut claims = last_nums_and_dens
        .iter()
        .map(|nd| nd.evaluate(&point))
        .collect::<Vec<_>>();

    for layer in layers[1..].iter().rev() {
        (point, claims) = prove_gkr_quotient_step::<_, N_GROUPS>(prover_state, layer.by_ref(), &point, claims, false);
    }
    (point, claims) = prove_gkr_quotient_step::<_, N_GROUPS>(prover_state, layers[0].by_ref(), &point, claims, true);
    assert_eq!(claims.len(), 2);
    (quotient, point, claims[0], claims[1])
}

fn prove_gkr_quotient_step<EF: ExtensionField<PF<EF>>, const N_GROUPS: usize>(
    prover_state: &mut FSProver<EF, impl FSChallenger<EF>>,
    numerators_and_denominators: MleGroupRef<'_, EF>,
    claim_point: &MultilinearPoint<EF>,
    claims: Vec<EF>,
    univariate_skip: bool,
) -> (MultilinearPoint<EF>, Vec<EF>) {
    let prev_numerators_and_denominators_split = match numerators_and_denominators {
        MleGroupRef::Extension(numerators_and_denominators) => {
            MleGroupRef::Extension(split_chunks(&numerators_and_denominators, 2))
        }
        MleGroupRef::ExtensionPacked(numerators_and_denominators) => {
            MleGroupRef::ExtensionPacked(split_chunks(&numerators_and_denominators, 2))
        }
        _ => unreachable!(),
    };

    let alpha = prover_state.sample();

    let (mut next_point, inner_evals, _) = sumcheck_prove::<EF, _, _>(
        1,
        prev_numerators_and_denominators_split,
        None,
        &GKRQuotientComputation::<N_GROUPS> {},
        &alpha.powers().take(N_GROUPS).collect(),
        Some((claim_point.0.clone(), None)),
        false,
        prover_state,
        dot_product(claims.iter().copied(), alpha.powers()),
        N_GROUPS != 2,
    );

    prover_state.add_extension_scalars(&inner_evals);
    let beta = prover_state.sample();

    let next_claims = if univariate_skip {
        let selectors = univariate_selectors(log2_strict_usize(N_GROUPS));
        vec![
            evaluate_univariate_multilinear::<_, _, _, false>(&inner_evals[..N_GROUPS], &[beta], &selectors, None),
            evaluate_univariate_multilinear::<_, _, _, false>(&inner_evals[N_GROUPS..], &[beta], &selectors, None),
        ]
    } else {
        inner_evals
            .chunks_exact(2)
            .map(|chunk| chunk.evaluate(&MultilinearPoint(vec![beta])))
            .collect::<Vec<_>>()
    };

    next_point.0.insert(0, beta);

    (next_point, next_claims)
}

pub fn verify_gkr_quotient<EF: ExtensionField<PF<EF>>, const N_GROUPS: usize>(
    verifier_state: &mut FSVerifier<EF, impl FSChallenger<EF>>,
    n_vars: usize,
) -> Result<(EF, MultilinearPoint<EF>, EF, EF), ProofError> {
    let last_nums_and_dens: [[_; 2]; N_GROUPS] = (0..N_GROUPS)
        .map(|_| verifier_state.next_extension_scalars_const())
        .collect::<Result<Vec<_>, _>>()?
        .try_into()
        .unwrap();

    let quotient = (0..N_GROUPS / 2)
        .map(|i| last_nums_and_dens[i][0] / last_nums_and_dens[i + N_GROUPS / 2][0])
        .sum::<EF>()
        + (0..N_GROUPS / 2)
            .map(|i| last_nums_and_dens[i][1] / last_nums_and_dens[i + N_GROUPS / 2][1])
            .sum::<EF>();

    let mut point = MultilinearPoint(vec![verifier_state.sample()]);
    let mut claims = last_nums_and_dens
        .iter()
        .map(|nd| nd.evaluate(&point))
        .collect::<Vec<_>>();

    for i in 1..n_vars - log2_strict_usize(N_GROUPS) {
        (point, claims) = verify_gkr_quotient_step::<_, N_GROUPS>(verifier_state, i, &point, claims, false)?;
    }
    (point, claims) = verify_gkr_quotient_step::<_, N_GROUPS>(
        verifier_state,
        n_vars - log2_strict_usize(N_GROUPS),
        &point,
        claims,
        true,
    )?;
    assert_eq!(claims.len(), 2);

    Ok((quotient, point, claims[0], claims[1]))
}

fn verify_gkr_quotient_step<EF: ExtensionField<PF<EF>>, const N_GROUPS: usize>(
    verifier_state: &mut FSVerifier<EF, impl FSChallenger<EF>>,
    n_vars: usize,
    point: &MultilinearPoint<EF>,
    claims: Vec<EF>,
    univariate_skip: bool,
) -> Result<(MultilinearPoint<EF>, Vec<EF>), ProofError> {
    assert_eq!(claims.len(), N_GROUPS);
    let alpha = verifier_state.sample();

    let (retrieved_quotient, postponed) = sumcheck_verify(verifier_state, n_vars, 3)?;

    if retrieved_quotient != dot_product(claims.iter().copied(), alpha.powers()) {
        return Err(ProofError::InvalidProof);
    }

    let inner_evals = verifier_state.next_extension_scalars_vec(N_GROUPS * 2)?;

    if postponed.value
        != point.eq_poly_outside(&postponed.point)
            * <GKRQuotientComputation<N_GROUPS> as SumcheckComputation<EF>>::eval_extension(
                &Default::default(),
                &inner_evals,
                &[],
                &alpha.powers().take(N_GROUPS).collect(),
            )
    {
        return Err(ProofError::InvalidProof);
    }

    let beta = verifier_state.sample();
    let next_claims = if univariate_skip {
        let selectors = univariate_selectors(log2_strict_usize(N_GROUPS));
        vec![
            evaluate_univariate_multilinear::<_, _, _, false>(&inner_evals[..N_GROUPS], &[beta], &selectors, None),
            evaluate_univariate_multilinear::<_, _, _, false>(&inner_evals[N_GROUPS..], &[beta], &selectors, None),
        ]
    } else {
        inner_evals
            .chunks_exact(2)
            .map(|chunk| chunk.evaluate(&MultilinearPoint(vec![beta])))
            .collect::<Vec<_>>()
    };
    let mut next_point = postponed.point.clone();
    next_point.0.insert(0, beta);

    Ok((next_point, next_claims))
}

fn sum_quotients<EF: ExtensionField<PF<EF>>>(
    numerators_and_denominators: MleGroupRef<'_, EF>,
    n_groups: usize,
) -> MleGroupOwned<EF> {
    match numerators_and_denominators {
        MleGroupRef::ExtensionPacked(numerators_and_denominators) => {
            MleGroupOwned::ExtensionPacked(sum_quotients_helper(&numerators_and_denominators, n_groups))
        }
        MleGroupRef::Extension(numerators_and_denominators) => {
            MleGroupOwned::Extension(sum_quotients_helper(&numerators_and_denominators, n_groups))
        }
        _ => unreachable!(),
    }
}

fn sum_quotients_helper<F: PrimeCharacteristicRing + Sync + Send + Copy>(
    numerators_and_denominators: &[&[F]],
    n_groups: usize,
) -> Vec<Vec<F>> {
    assert_eq!(numerators_and_denominators.len(), n_groups);
    let n = numerators_and_denominators[0].len();
    assert!(n.is_power_of_two() && n >= 2, "n = {n}");
    let mut new_numerators = Vec::new();
    let mut new_denominators = Vec::new();
    let (prev_numerators, prev_denominators) = numerators_and_denominators.split_at(n_groups / 2);
    for i in 0..n_groups / 2 {
        let (new_num, new_den) = sum_quotients_2_by_2::<F>(prev_numerators[i], prev_denominators[i]);
        new_numerators.push(new_num);
        new_denominators.push(new_den);
    }
    new_numerators.extend(new_denominators);
    new_numerators
}
fn sum_quotients_2_by_2<F: PrimeCharacteristicRing + Sync + Send + Copy>(
    numerators: &[F],
    denominators: &[F],
) -> (Vec<F>, Vec<F>) {
    let n = numerators.len();
    let new_n = n / 2;
    let mut new_numerators = unsafe { uninitialized_vec(new_n) };
    let mut new_denominators = unsafe { uninitialized_vec(new_n) };
    new_numerators
        .par_iter_mut()
        .zip(new_denominators.par_iter_mut())
        .enumerate()
        .for_each(|(i, (num, den))| {
            let my_numerators: [_; 2] = [numerators[i], numerators[i + new_n]];
            let my_denominators: [_; 2] = [denominators[i], denominators[i + new_n]];
            *num = my_numerators[0] * my_denominators[1] + my_numerators[1] * my_denominators[0];
            *den = my_denominators[0] * my_denominators[1];
        });
    (new_numerators, new_denominators)
}
fn split_mle_group<'a, EF: ExtensionField<PF<EF>>>(
    polys: &'a MleGroupRef<'a, EF>,
    n_groups: usize,
) -> MleGroupRef<'a, EF> {
    match polys {
        MleGroupRef::Extension(polys) => MleGroupRef::Extension(split_chunks(polys, n_groups)),
        MleGroupRef::ExtensionPacked(polys) => MleGroupRef::ExtensionPacked(split_chunks(polys, n_groups)),
        _ => unreachable!(),
    }
}

fn split_chunks<'a, A>(numerators_and_denominators: &[&'a [A]], num_groups: usize) -> Vec<&'a [A]> {
    let n = numerators_and_denominators[0].len();
    assert!(n.is_power_of_two() && n >= num_groups);
    assert!(num_groups.is_power_of_two());

    let mut res = Vec::new();
    for slice in numerators_and_denominators {
        assert_eq!(slice.len(), n);
        res.extend(split_at_many(
            slice,
            &(1..num_groups).map(|i| i * n / num_groups).collect::<Vec<_>>(),
        ));
    }
    res
}

#[cfg(test)]
mod tests {
    use std::time::Instant;

    use super::*;
    use p3_koala_bear::QuinticExtensionFieldKB;
    use rand::{Rng, SeedableRng, rngs::StdRng};
    use utils::{build_prover_state, build_verifier_state, init_tracing};

    type EF = QuinticExtensionFieldKB;

    fn sum_all_quotients(nums: &[EF], den: &[EF]) -> EF {
        nums.iter().zip(den.iter()).map(|(&n, &d)| n / d).sum()
    }

    const N_GROUPS: usize = 8;

    #[test]
    fn test_gkr_quotient() {
        let log_n = 22;
        let n = 1 << log_n;
        init_tracing();

        let mut rng = StdRng::seed_from_u64(0);

        let numerators = (0..n).map(|_| rng.random()).collect::<Vec<EF>>();
        let c: EF = rng.random();
        let denominators_indexes = (0..n)
            .map(|_| PF::<EF>::from_usize(rng.random_range(..n)))
            .collect::<Vec<_>>();
        let denominators = denominators_indexes.iter().map(|&i| c - i).collect::<Vec<EF>>();
        let real_quotient = sum_all_quotients(&numerators, &denominators);
        let mut prover_state = build_prover_state(false);

        let time = Instant::now();
        let prover_statements = prove_gkr_quotient::<EF, N_GROUPS>(
            &mut prover_state,
            &MleGroupRef::ExtensionPacked(vec![&pack_extension(&numerators), &pack_extension(&denominators)]),
        );
        println!("Proving time: {:?}", time.elapsed());

        let mut verifier_state = build_verifier_state(prover_state);

        let verifier_statements = verify_gkr_quotient::<EF, N_GROUPS>(&mut verifier_state, log_n).unwrap();
        assert_eq!(&verifier_statements, &prover_statements);
        let (retrieved_quotient, claim_point, claim_num, claim_den) = verifier_statements;

        assert_eq!(retrieved_quotient, real_quotient);
        let selectors = univariate_selectors::<PF<EF>>(log2_strict_usize(N_GROUPS));
        assert_eq!(
            evaluate_univariate_multilinear::<_, _, _, true>(&numerators, &claim_point, &selectors, None),
            claim_num
        );
        assert_eq!(
            evaluate_univariate_multilinear::<_, _, _, true>(&denominators, &claim_point, &selectors, None),
            claim_den
        );
    }
}
