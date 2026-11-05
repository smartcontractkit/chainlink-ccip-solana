//! A minimal native Solana program that installs and exercises the custom heap.

use solana_account_info::AccountInfo;
use solana_cpi::set_return_data;
use solana_msg::msg;
use solana_program_entrypoint::ProgramResult;
use solana_program_error::ProgramError;
use solana_pubkey::Pubkey;

#[cfg(not(feature = "no-entrypoint"))]
use solana_program_entrypoint::entrypoint;

solana_pubkey::declare_id!("DemoProgram111111111111111111111111111111111");

#[cfg(all(
    feature = "custom-heap",
    not(feature = "no-entrypoint"),
    target_os = "solana"
))]
#[global_allocator]
static ALLOCATOR: suffix_eater_allocator::Allocator = unsafe {
    suffix_eater_allocator::Allocator::with_fixed_address_range(
        solana_program_entrypoint::HEAP_START_ADDRESS as usize,
        suffix_eater_allocator::SOLANA_MAX_HEAP_LENGTH,
    )
};

#[cfg(not(feature = "no-entrypoint"))]
entrypoint!(process_instruction);

/// Number of independently allocated phases run by one instruction.
///
/// Each phase grows a vector from empty and drops it before the next phase.
/// Without deallocation or frontier-aware reallocation, the accumulated
/// backing buffers exceed even the requested heap for the integration input.
pub const ALLOCATION_PHASES: u32 = 8;

/// Largest per-phase workload accepted by the demo.
///
/// Its final 32 KiB vector fits in a requested 64 KiB heap, but the allocator's
/// cursor and footer deliberately make it too large for the default 32 KiB
/// heap. The integration test uses that boundary to prove that the allocator
/// consumes dynamically requested heap space.
pub const MAX_ITEMS: u32 = 4_096;

/// The result returned by the allocation workload.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct AllocationReport {
    pub items: u32,
    pub checksum: u64,
}

impl AllocationReport {
    pub fn to_bytes(self) -> [u8; 12] {
        let mut bytes = [0; 12];
        bytes[..4].copy_from_slice(&self.items.to_le_bytes());
        bytes[4..].copy_from_slice(&self.checksum.to_le_bytes());
        bytes
    }
}

#[inline(never)]
fn run_allocation_phase(items: u32) -> u64 {
    // Starting empty deliberately exercises `GlobalAlloc::realloc` while the
    // vector grows. The vector is the phase's frontier allocation, and it is
    // dropped when this non-inlined function returns.
    let mut values = Vec::new();
    for index in 0..items as u64 {
        values.push(index.wrapping_mul(3).wrapping_add(1));
    }

    // Keep the heap-backed slice opaque to optimization so the SBF integration
    // workload continues to exercise real allocation and memory accesses.
    core::hint::black_box(values.as_slice())
        .iter()
        .copied()
        .fold(0u64, u64::wrapping_add)
}

/// Run repeated allocation phases, forcing reuse of the installed global heap
/// on Solana. Each phase grows and drops a vector before the next phase starts.
pub fn run_allocation_workload(items: u32) -> Result<AllocationReport, ProgramError> {
    if items > MAX_ITEMS {
        return Err(ProgramError::InvalidArgument);
    }

    let mut checksum = 0u64;
    for _ in 0..ALLOCATION_PHASES {
        checksum = checksum.wrapping_add(run_allocation_phase(items));
    }

    Ok(AllocationReport { items, checksum })
}

/// Instruction data is one little-endian `u32` specifying the vector length.
/// The program returns `items || checksum` through Solana return data.
pub fn process_instruction(
    _program_id: &Pubkey,
    _accounts: &[AccountInfo],
    instruction_data: &[u8],
) -> ProgramResult {
    let bytes: [u8; 4] = instruction_data
        .try_into()
        .map_err(|_| ProgramError::InvalidInstructionData)?;
    let items = u32::from_le_bytes(bytes);
    let report = run_allocation_workload(items)?;

    msg!(
        "allocator-demo: completed {} phases of {} u64 values; checksum {}",
        ALLOCATION_PHASES,
        report.items,
        report.checksum
    );
    set_return_data(&report.to_bytes());
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn workload_allocates_and_computes_the_expected_checksum() {
        let report = run_allocation_workload(1_024).unwrap();

        assert_eq!(report.items, 1_024);
        assert_eq!(report.checksum, 12_578_816);
        assert_eq!(report.to_bytes(), [0, 4, 0, 0, 0, 240, 191, 0, 0, 0, 0, 0]);
    }

    #[test]
    fn workload_rejects_a_request_that_could_crowd_the_program_heap() {
        assert_eq!(
            run_allocation_workload(MAX_ITEMS + 1),
            Err(ProgramError::InvalidArgument)
        );
    }

    #[test]
    fn empty_workload_is_valid() {
        assert_eq!(
            run_allocation_workload(0),
            Ok(AllocationReport {
                items: 0,
                checksum: 0,
            })
        );
    }
}
