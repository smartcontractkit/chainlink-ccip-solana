use std::path::PathBuf;

use solana_compute_budget_interface::ComputeBudgetInstruction;
use solana_instruction::Instruction;
use solana_message::Message;
use solana_transaction::Transaction;
use surfpool_sdk::{Pubkey, Signer, Surfnet, cheatcodes::builders::DeployProgram};

const PROGRAM_ID: &str = "DemoProgram111111111111111111111111111111111";
const ITEMS: u32 = 4_096;
const EXPECTED_LOG: &str =
    "allocator-demo: completed 8 phases of 4096 u64 values; checksum 201310208";
const REQUESTED_HEAP_BYTES: u32 = 64 * 1024;
const REQUESTED_COMPUTE_UNITS: u32 = 1_400_000;

fn program_artifact() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("target/deploy/allocator_demo.so")
}

#[tokio::test(flavor = "multi_thread")]
async fn embedded_surfnet_starts_with_a_funded_payer() {
    let surfnet = Surfnet::start().await.expect("start embedded Surfnet");
    let balance = surfnet
        .rpc_client()
        .get_balance(&surfnet.payer().pubkey())
        .expect("query payer balance");

    assert!(balance > 0);
}

#[tokio::test(flavor = "multi_thread")]
async fn custom_allocator_uses_the_transaction_requested_heap_in_surfpool() {
    let artifact = program_artifact();
    assert!(
        artifact.is_file(),
        "missing {}; run `scripts/test-surfpool.sh` (or `make \
         test-allocator-surfpool` from the repository root) first",
        artifact.display()
    );

    let surfnet = Surfnet::start().await.expect("start embedded Surfnet");
    let program_id: Pubkey = PROGRAM_ID.parse().expect("valid demo program id");
    surfnet
        .cheatcodes()
        .deploy(DeployProgram::new(program_id).so_path(&artifact))
        .expect("deploy allocator demo");

    let rpc = surfnet.rpc_client();
    let payer = surfnet.payer();
    let instruction = Instruction::new_with_bytes(program_id, &ITEMS.to_le_bytes(), vec![]);
    let blockhash = rpc.get_latest_blockhash().expect("latest blockhash");
    let request_compute = ComputeBudgetInstruction::set_compute_unit_limit(REQUESTED_COMPUTE_UNITS);

    // The final Vec allocation plus its allocator metadata is intentionally a
    // few bytes larger than the default 32 KiB heap. The footer write should
    // therefore hit the runtime mapping boundary before `alloc` can return an
    // invalid pointer.
    let default_message = Message::new(
        &[request_compute.clone(), instruction.clone()],
        Some(&payer.pubkey()),
    );
    let default_transaction = Transaction::new(&[payer], default_message, blockhash);
    let default_simulation = rpc
        .simulate_transaction(&default_transaction)
        .expect("simulate allocator transaction with default heap");
    assert!(default_simulation.value.err.is_some());
    let default_logs = default_simulation.value.logs.unwrap_or_default();
    assert!(
        default_logs.iter().any(|line| {
            let line = line.to_ascii_lowercase();
            line.contains("access violation") && line.contains("heap")
        }),
        "expected a heap access violation, got {default_logs:#?}"
    );

    let request_heap = ComputeBudgetInstruction::request_heap_frame(REQUESTED_HEAP_BYTES);
    let message = Message::new(
        &[request_compute, request_heap, instruction],
        Some(&payer.pubkey()),
    );
    let transaction = Transaction::new(&[payer], message, blockhash);

    let simulation = rpc
        .simulate_transaction(&transaction)
        .expect("simulate allocator transaction");
    assert_eq!(simulation.value.err, None, "{:?}", simulation.value.logs);
    let logs = simulation.value.logs.unwrap_or_default();
    assert!(
        logs.iter().any(|line| line.contains(EXPECTED_LOG)),
        "expected workload log, got {logs:#?}"
    );

    rpc.send_and_confirm_transaction(&transaction)
        .expect("submit allocator transaction");
}
