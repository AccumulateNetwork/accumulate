package security

// KeyState
// Required to provide the signatures for managing chains within an ADI
// A KeyState always has at
type KeyState struct {
	SigGroups map[string]SigGroup
}
