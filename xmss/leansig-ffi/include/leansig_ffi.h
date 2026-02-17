/*
 * leansig_ffi.h - C header for the leansig FFI library.
 *
 * This header provides a C-compatible interface to the leansig XMSS
 * post-quantum signature scheme (devnet-1 instantiation).
 *
 * Memory management: Every allocated object has a corresponding _free function.
 * Byte buffers returned by serialize/sign must be freed with
 * leansig_bytes_free.
 */

#ifndef LEANSIG_FFI_H
#define LEANSIG_FFI_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Message length expected by sign/verify (XMSS signs 32-byte messages). */
#define LEANSIG_MESSAGE_LENGTH 32

/* Result codes. */
typedef enum {
  LEANSIG_OK = 0,
  LEANSIG_NULL_POINTER = 1,
  LEANSIG_INVALID_LENGTH = 2,
  LEANSIG_SIGNING_FAILED = 3,
  LEANSIG_DESERIALIZATION_FAILED = 4,
  LEANSIG_VERIFICATION_FAILED = 5,
  LEANSIG_EPOCH_NOT_PREPARED = 6,
} LeansigResult;

/* Opaque keypair handle. */
typedef struct LeansigKeypair LeansigKeypair;

/* ---------- Key Generation ---------- */

/*
 * Generate a new XMSS keypair.
 *
 * seed:              Random seed for the RNG.
 * activation_epoch:  Starting epoch for which the key is active.
 * num_active_epochs: Number of consecutive active epochs.
 * out_keypair:       Receives the opaque keypair handle. Must be freed
 *                    with leansig_keypair_free().
 */
LeansigResult leansig_keypair_generate(uint64_t seed, uint64_t activation_epoch,
                                       uint64_t num_active_epochs,
                                       LeansigKeypair **out_keypair);

/* Restore a keypair from serialized public and secret key bytes. */
LeansigResult leansig_keypair_restore(const uint8_t *pk_bytes, size_t pk_len,
                                      const uint8_t *sk_bytes, size_t sk_len,
                                      LeansigKeypair **out_keypair);

/* Free a keypair allocated by leansig_keypair_generate or
 * leansig_keypair_restore. */
void leansig_keypair_free(LeansigKeypair *keypair);

/* ---------- Key Serialization ---------- */

/*
 * Serialize the public key to SSZ bytes.
 * out_data/out_len receive the buffer. Free with leansig_bytes_free().
 */
LeansigResult leansig_pubkey_serialize(const LeansigKeypair *keypair,
                                       uint8_t **out_data, size_t *out_len);

/*
 * Serialize the secret key to SSZ bytes.
 * out_data/out_len receive the buffer. Free with leansig_bytes_free().
 */
LeansigResult leansig_seckey_serialize(const LeansigKeypair *keypair,
                                       uint8_t **out_data, size_t *out_len);

/* Free a byte buffer returned by any serialize or sign function. */
void leansig_bytes_free(uint8_t *data, size_t len);

/* ---------- Secret Key Management ---------- */

/* Get the start of the activation interval. */
uint64_t leansig_sk_activation_start(const LeansigKeypair *keypair);

/* Get the end (exclusive) of the activation interval. */
uint64_t leansig_sk_activation_end(const LeansigKeypair *keypair);

/* Get the start of the currently prepared signing window. */
uint64_t leansig_sk_prepared_start(const LeansigKeypair *keypair);

/* Get the end (exclusive) of the currently prepared signing window. */
uint64_t leansig_sk_prepared_end(const LeansigKeypair *keypair);

/* Advance the prepared interval to the next window. */
LeansigResult leansig_sk_advance_preparation(LeansigKeypair *keypair);

/* ---------- Signing ---------- */

/*
 * Sign a 32-byte message at a given epoch.
 *
 * keypair:     Opaque keypair handle (secret key is used).
 * epoch:       The epoch to sign at (must be in prepared interval).
 * message:     Pointer to 32-byte message buffer.
 * out_sig_data: Receives SSZ-serialized signature. Free with
 * leansig_bytes_free(). out_sig_len:  Receives signature length.
 */
LeansigResult leansig_sign(const LeansigKeypair *keypair, uint32_t epoch,
                           const uint8_t *message, uint8_t **out_sig_data,
                           size_t *out_sig_len);

/* ---------- Verification ---------- */

/*
 * Verify a signature against a serialized public key, epoch, and message.
 *
 * Returns LEANSIG_OK on success, LEANSIG_VERIFICATION_FAILED on failure.
 */
LeansigResult leansig_verify(const uint8_t *pk_data, size_t pk_len,
                             uint32_t epoch, const uint8_t *message,
                             const uint8_t *sig_data, size_t sig_len);

/*
 * Verify using the public key from a keypair handle.
 * Convenience wrapper; avoids public key serialization/deserialization.
 */
LeansigResult leansig_verify_with_keypair(const LeansigKeypair *keypair,
                                          uint32_t epoch,
                                          const uint8_t *message,
                                          const uint8_t *sig_data,
                                          size_t sig_len);

#ifdef __cplusplus
}
#endif

#endif /* LEANSIG_FFI_H */
