package changesets

import (
	"errors"
	"fmt"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	mcmstypes "github.com/smartcontractkit/mcms/types"
)

// Whether a changeset needs MCMS is decided on chain, not by its input: each operation signs with the
// deployer key when the deployer is the owner or upgrade authority, and returns batch operations when the
// timelock is. So no changeset requires an MCMS input up front (environments without MCMS never set one),
// and the input is only checked when it is given, and when the operations actually need it.

// validateMCMSInput validates an MCMS input the way the proposal builder will see it: a copy with the
// defaults filled in first, so an unset ValidUntil (which defaults to mcms.DefaultValidDuration) or
// TimelockAction is accepted. An empty input is skipped, and logged since it is otherwise silent: what
// happens instead depends on who owns the program, which is only known when the operations run.
func validateMCMSInput(lggr logger.Logger, input mcms.Input) error {
	if input == (mcms.Input{}) {
		if lggr != nil {
			lggr.Debugw("No MCMS input given, skipping its validation: operations owned by the deployer are sent directly, and operations owned by the timelock will fail to build a proposal")
		}
		return nil
	}
	if err := input.PopulateDefaults(); err != nil {
		return fmt.Errorf("populate MCMS defaults: %w", err)
	}
	return input.Validate()
}

// requireMCMSForBatchOps returns an error when the operations produced batch operations, because the
// timelock owns what they change, but no MCMS input was given to build the proposal. It also logs which way
// the changeset went, since an empty MCMS input is otherwise skipped silently: the deployer key signed
// directly, or an MCMS input that was given turned out not to be needed.
func requireMCMSForBatchOps(lggr logger.Logger, input mcms.Input, batchOps []mcmstypes.BatchOperation) error {
	needsTimelock := false
	for _, op := range batchOps {
		if len(op.Transactions) > 0 {
			needsTimelock = true
			break
		}
	}
	switch {
	case needsTimelock && input == (mcms.Input{}):
		return errors.New("the timelock owns what this changeset changes, so an mcms input is required to build the proposal")
	case !needsTimelock && input == (mcms.Input{}):
		if lggr != nil {
			lggr.Debugw("No MCMS input and nothing is owned by the timelock: changes were sent with the deployer key")
		}
	case !needsTimelock:
		if lggr != nil {
			lggr.Infow("MCMS input ignored: nothing is owned by the timelock, so changes were sent with the deployer key", "qualifier", input.Qualifier)
		}
	}
	return nil
}
