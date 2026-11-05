// Test fixture for INV-LINT-EVENT Rule B
// This program has NO event-cpi context anywhere, so emit_cpi! should be flagged.
//
// Expected findings from this file:
//   INV-LINT-EVENT: 1  (Rule B: emit_cpi! without event-cpi annotation in program)

use anchor_lang::prelude::*;

fn emit_without_context() {
    emit_cpi!(MyEvent { value: 1 });
}
