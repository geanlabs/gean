# beacon/crypto/bls

BLS12-381 signatures behind one interface, with a `blst` backend and a fake
backend selected by build tag so most tests run without real cryptography.
