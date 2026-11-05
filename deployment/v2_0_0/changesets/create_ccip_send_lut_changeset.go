package changesets

import (
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
)

type CreateCcipSendLookupTableChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	// The Router's reference addresses are updated through MCMS when the timelock owns the Router.
	MCMS mcms.Input `json:"mcms"`
}

// CreateCcipSendLookupTableChangeset sets up the ccip_send_v2 lookup table on a chain whose Router
// was upgraded in place: it creates the table when the datastore has none, fills it with the static
// accounts and the pool fee vaults, and points the Router's reference addresses at it. The deployer
// creates and extends the table directly.
func CreateCcipSendLookupTableChangeset() cldf.ChangeSetV2[CreateCcipSendLookupTableChangesetInput] {
	return cldf.CreateChangeSet(createCcipSendLookupTableApply, createCcipSendLookupTableVerify)
}

func createCcipSendLookupTableVerify(env cldf.Environment, input CreateCcipSendLookupTableChangesetInput) error {
	return extendCcipSendLookupTableVerify(env, ExtendCcipSendLookupTableChangesetInput(input))
}

func createCcipSendLookupTableApply(e cldf.Environment, input CreateCcipSendLookupTableChangesetInput) (cldf.ChangesetOutput, error) {
	return applyCcipSendLut(e, input.ChainSelector, input.MCMS, true)
}
