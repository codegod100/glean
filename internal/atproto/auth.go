package atproto

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type DIDDocument = identity.DIDDocument
type Identity = identity.Identity

func ResolveHandle(ctx context.Context, handle string) (string, error) {
	h, err := syntax.ParseHandle(handle)
	if err != nil {
		return "", fmt.Errorf("parsing handle: %w", err)
	}

	dir := identity.DefaultDirectory()
	ident, err := dir.LookupHandle(ctx, h)
	if err != nil {
		return "", fmt.Errorf("resolving handle: %w", err)
	}

	return ident.DID.String(), nil
}

func ResolveDID(ctx context.Context, did string) (*DIDDocument, error) {
	d, err := syntax.ParseDID(did)
	if err != nil {
		return nil, fmt.Errorf("parsing DID: %w", err)
	}

	dir := identity.DefaultDirectory()
	ident, err := dir.LookupDID(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("resolving DID: %w", err)
	}

	doc := ident.DIDDocument()
	return &doc, nil
}

func ResolveIdentity(ctx context.Context, identifier string) (*Identity, error) {
	atid, err := syntax.ParseAtIdentifier(identifier)
	if err != nil {
		return nil, fmt.Errorf("parsing identifier: %w", err)
	}

	dir := identity.DefaultDirectory()
	ident, err := dir.Lookup(ctx, atid)
	if err != nil {
		return nil, fmt.Errorf("resolving identity: %w", err)
	}

	return ident, nil
}

func ResolvePDSEndpoint(ctx context.Context, did string) (string, error) {
	ident, err := ResolveIdentity(ctx, did)
	if err != nil {
		return "", err
	}

	pds := ident.PDSEndpoint()
	if pds == "" {
		return "", fmt.Errorf("no PDS endpoint found for %s", did)
	}

	return pds, nil
}

type OAuthConfig struct {
	ClientID    string
	RedirectURL string
	Scopes      []string
}

type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	DID          string
	Handle       string
}
