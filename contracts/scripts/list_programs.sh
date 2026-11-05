#!/usr/bin/env bash
# Explicit release artifact allowlist. Keep this list limited to independently
# deployable programs; helper crates such as base_token_pool and ccip_common are
# implementation libraries, not release artifacts. New programs are not added
# automatically: update this list deliberately when release support is intended.
set -euo pipefail

cat <<'PROGRAMS'
arbitrary_cpi_proxy
burnmint_token_pool
ccip_offramp
ccip_router
cctp_token_pool
committee_verifier
example_ccip_receiver
example_ccip_receiver_v2
example_ccip_sender
example_ccip_sender_v2
executor
fee_quoter
lockrelease_token_pool
lombard_token_pool
rmn_remote
test_ccip_invalid_receiver
test_ccip_receiver
test_program
PROGRAMS
