package graph

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

// Reasons a Page cannot be reached. Two failures, told apart.
const (
	// ReasonNotListed means Graph never mentioned the Page.
	ReasonNotListed = "not_listed"
	// ReasonNoToken means Graph listed the Page and withheld its access_token.
	ReasonNoToken = "no_token"
)

// PageAccessError is a Page the current credentials cannot reach. Carries no
// credential material.
type PageAccessError struct {
	PageID string
	Reason string
}

func (e *PageAccessError) Error() string {
	switch e.Reason {
	case ReasonNoToken:
		return fmt.Sprintf(
			"Graph listed Page %s but returned no access token for it. The identity reaches the "+
				"Page and cannot act as it: check that this Page is assigned to the identity with "+
				"a Page role, not merely visible to it.", e.PageID)
	default:
		return fmt.Sprintf(
			"No Page access token for %s. This identity may not administer that Page, or the "+
				"token may lack pages_show_list.", e.PageID)
	}
}

// TokenProvider resolves the access tokens this server needs.
//
// Two scopes exist: the user or system token, and a per-Page token. A comment
// read carrying a USER token returns an EMPTY ARRAY rather than an error, so the
// Page exchange is required rather than an optimisation — skipping it is the
// quiet wrong answer, an empty result that reads as "no comments".
//
// The credential map is held privately and never exposed: no caller can
// enumerate tokens.
type TokenProvider struct {
	userToken string
	client    *PagesClient

	mu    sync.Mutex
	once  *sync.Once
	creds PageCredentials
	err   error
}

func NewTokenProvider(userToken string, client *PagesClient) *TokenProvider {
	return &TokenProvider{userToken: userToken, client: client, once: &sync.Once{}}
}

// TokenForUser returns the configured user or system token.
func (p *TokenProvider) TokenForUser() string { return p.userToken }

// credentials exchanges once and shares the result.
//
// A single guarded exchange rather than a resolved map, so concurrent callers
// share one call instead of racing several — a sweep would otherwise ask for the
// same Page token once per post.
func (p *TokenProvider) credentials(ctx context.Context) (PageCredentials, error) {
	p.mu.Lock()
	once := p.once
	p.mu.Unlock()

	once.Do(func() {
		creds, err := p.client.Credentials(ctx)

		p.mu.Lock()
		defer p.mu.Unlock()
		p.creds, p.err = creds, err
		if err != nil {
			// A failed exchange must not poison the cache: arm a fresh Once so
			// the next caller retries rather than inheriting a transient fault
			// for the life of the process.
			p.once = &sync.Once{}
		}
	})

	p.mu.Lock()
	defer p.mu.Unlock()
	return p.creds, p.err
}

// credentialFor finds one Page's credential, or says which of the two ways it
// could not.
func (p *TokenProvider) credentialFor(ctx context.Context, pageID string) (PageCredential, error) {
	creds, err := p.credentials(ctx)
	if err != nil {
		return PageCredential{}, err
	}

	for _, candidate := range creds.Usable {
		if candidate.PageID == pageID {
			return candidate, nil
		}
	}
	if slices.Contains(creds.Tokenless, pageID) {
		return PageCredential{}, &PageAccessError{PageID: pageID, Reason: ReasonNoToken}
	}
	return PageCredential{}, &PageAccessError{PageID: pageID, Reason: ReasonNotListed}
}

// TokenFor returns the Page access token for one Page.
func (p *TokenProvider) TokenFor(ctx context.Context, pageID string) (string, error) {
	credential, err := p.credentialFor(ctx, pageID)
	if err != nil {
		return "", err
	}
	return credential.AccessToken, nil
}

// TasksFor returns the Page roles the token carries, e.g. MODERATE.
//
// nil when unknown, an empty slice when known to be none. Graph omits the field
// rather than returning it empty when it will not say, and a caller that refuses
// a write on a missing role must act on the empty slice and never on nil —
// refusing on silence produces a message naming a role the identity may well
// hold.
//
// The returned slice is a copy, because the cached credential is shared and a
// caller holding the live slice could append a role into it.
func (p *TokenProvider) TasksFor(ctx context.Context, pageID string) (*[]string, error) {
	credential, err := p.credentialFor(ctx, pageID)
	if err != nil {
		return nil, err
	}
	if credential.Tasks == nil {
		return nil, nil
	}
	copied := slices.Clone(*credential.Tasks)
	return &copied, nil
}
