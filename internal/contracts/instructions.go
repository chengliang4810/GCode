package contracts

import "context"

const MaxGlobalInstructionsBytes = 256 << 10

// GlobalInstructionsService edits the business host's instructions. Clients
// never interpret the host's configuration directory as a local file path.
type GlobalInstructionsService interface {
	ReadGlobalInstructions(context.Context) (string, error)
	WriteGlobalInstructions(context.Context, string) error
}
