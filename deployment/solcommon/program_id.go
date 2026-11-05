package solcommon

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// SetProgramID updates a gobinding package's ProgramID var and re-registers the
// instruction decoder under the new address. Call this at the top of every
// non-Deploy operation so instruction builders target the freshly deployed
// address rather than the compile-time constant baked into the gobinding.
func SetProgramID[T any](programIDVar *solana.PublicKey, parser func([]byte) (T, error), newID solana.PublicKey) {
	*programIDVar = newID
	_ = solana.RegisterInstructionDecoder(newID, func(_ []*solana.AccountMeta, data []byte) (any, error) {
		return parser(data)
	})
}

// WithProgramID retargets a built instruction at the given program without
// rebinding the binding's package-level ProgramID, which is process-global and
// unsafe to mutate when several chains or runs share the process.
func WithProgramID(ix solana.Instruction, programID solana.PublicKey) (solana.Instruction, error) {
	generic, ok := ix.(*solana.GenericInstruction)
	if !ok {
		return nil, fmt.Errorf("cannot retarget instruction of type %T", ix)
	}
	generic.ProgID = programID
	return generic, nil
}
