package blocks

import "context"


// AuthorityProvider defines an interface for providing the active authority set for a given network state.
// This allows the BlockValidator to remain decoupled from the specifics of how authorities are managed,
// whether they are loaded from a static configuration, fetched from genesis, or updated over time.
type AuthorityProvider interface {
	// GetAuthorities returns the set of trusted public key hashes and the required voting threshold.
	// TODO: The exact signature of this method will be refined as the implementation progresses.
	GetAuthorities(ctx context.Context) (map[[32]byte]bool, uint64, error)
}

// StaticAuthorityProvider is a simple implementation of AuthorityProvider that uses a fixed set of authorities.
// This serves as a direct replacement for the previous hardcoded map in the BlockValidator and as a
// foundational piece for building a more dynamic provider.
type StaticAuthorityProvider struct {
	Authorities map[[32]byte]bool
	Threshold   uint64
}

// NewStaticAuthorityProvider creates a new StaticAuthorityProvider with the given set of authorities and threshold.
func NewStaticAuthorityProvider(authorities [][32]byte, threshold uint64) *StaticAuthorityProvider {
	p := &StaticAuthorityProvider{
		Authorities: make(map[[32]byte]bool),
		Threshold:   threshold,
	}
	for _, auth := range authorities {
		p.Authorities[auth] = true
	}
	return p
}

// GetAuthorities returns the authorities and threshold from the static provider.
func (p *StaticAuthorityProvider) GetAuthorities(_ context.Context) (map[[32]byte]bool, uint64, error) {
	return p.Authorities, p.Threshold, nil
}

// Ensure StaticAuthorityProvider implements the AuthorityProvider interface.
var _ AuthorityProvider = (*StaticAuthorityProvider)(nil)
