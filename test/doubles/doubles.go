// Package doubles provides hand-rolled test doubles (stubs and in-memory fakes)
// for the ccswitch domain repository ports. No mocking library is used.
package doubles

import "github.com/rios0rios0/ccswitch/internal/domain/entities"

// InMemoryAccountsRepository is an in-memory AccountsRepository double. It holds
// the store by reference so commands that mutate and save it can be inspected.
type InMemoryAccountsRepository struct {
	Store     *entities.Store
	LoadErr   error
	SaveErr   error
	SaveCalls int
}

// Load returns the held store, or an empty store when none was set.
func (r *InMemoryAccountsRepository) Load() (*entities.Store, error) {
	if r.LoadErr != nil {
		return nil, r.LoadErr
	}
	if r.Store == nil {
		return &entities.Store{}, nil
	}
	return r.Store, nil
}

// Save records the store and counts the call.
func (r *InMemoryAccountsRepository) Save(store *entities.Store) error {
	r.SaveCalls++
	if r.SaveErr != nil {
		return r.SaveErr
	}
	r.Store = store
	return nil
}

// StubCredentialsRepository is a CredentialsRepository double that records writes
// and reflects them back on subsequent reads.
type StubCredentialsRepository struct {
	Creds      *entities.OAuthCredentials
	Identity   *entities.AccountIdentity
	ReadErr    error
	WriteErr   error
	Written    *entities.OAuthCredentials
	WrittenID  *entities.AccountIdentity
	WriteCalls int
	ExistsVal  bool
}

// Read returns the currently held credentials and identity.
func (r *StubCredentialsRepository) Read() (*entities.OAuthCredentials, *entities.AccountIdentity, error) {
	if r.ReadErr != nil {
		return nil, nil, r.ReadErr
	}
	return r.Creds, r.Identity, nil
}

// Write records the credentials and identity and reflects them on later reads.
func (r *StubCredentialsRepository) Write(
	creds *entities.OAuthCredentials,
	identity *entities.AccountIdentity,
) error {
	r.WriteCalls++
	if r.WriteErr != nil {
		return r.WriteErr
	}
	r.Written = creds
	r.WrittenID = identity
	r.Creds = creds
	r.Identity = identity
	return nil
}

// Exists reports the configured existence value.
func (r *StubCredentialsRepository) Exists() bool {
	return r.ExistsVal
}

// StubUsageRepository returns canned usage, optionally keyed by access token.
type StubUsageRepository struct {
	Usage      *entities.Usage
	ByToken    map[string]*entities.Usage
	Err        error
	FetchCalls int
}

// Fetch returns the usage configured for the token, the default usage, or an
// empty usage.
func (r *StubUsageRepository) Fetch(accessToken string) (*entities.Usage, error) {
	r.FetchCalls++
	if r.Err != nil {
		return nil, r.Err
	}
	if usage, ok := r.ByToken[accessToken]; ok {
		return usage, nil
	}
	if r.Usage != nil {
		return r.Usage, nil
	}
	return &entities.Usage{}, nil
}

// StubTokensRepository returns a canned refreshed credential set.
type StubTokensRepository struct {
	Refreshed    *entities.OAuthCredentials
	Err          error
	RefreshCalls int
}

// Refresh returns the configured refreshed credentials.
func (r *StubTokensRepository) Refresh(_ string) (*entities.OAuthCredentials, error) {
	r.RefreshCalls++
	if r.Err != nil {
		return nil, r.Err
	}
	return r.Refreshed, nil
}

// StubSessionsRepository reports a fixed running state.
type StubSessionsRepository struct {
	Running bool
}

// ClaudeRunning reports the configured running state.
func (r *StubSessionsRepository) ClaudeRunning() bool {
	return r.Running
}
