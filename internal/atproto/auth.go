package atproto

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type (
	DIDDocument = identity.DIDDocument
	Identity    = identity.Identity
)

var directory identity.Directory

func InitIdentity(plcURL string) {
	if plcURL == "" {
		plcURL = identity.DefaultPLCURL
	}
	base := identity.BaseDirectory{
		PLCURL: plcURL,
		HTTPClient: http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				Proxy:             http.ProxyFromEnvironment,
				IdleConnTimeout:   1000 * time.Millisecond,
				MaxIdleConns:      100,
			},
		},
		Resolver: net.Resolver{
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, network, address)
			},
		},
		TryAuthoritativeDNS:  true,
		SkipDNSDomainSuffixes: []string{".bsky.social"},
		UserAgent:             "glean/1.0",
	}
	directory = identity.NewCacheDirectory(&base, 250_000, 24*time.Hour, 2*time.Minute, 5*time.Minute)
}

func dir() identity.Directory {
	if directory == nil {
		plcURL := os.Getenv("GLEAN_PLC_URL")
		if plcURL == "" {
			plcURL = identity.DefaultPLCURL
		}
		InitIdentity(plcURL)
	}
	return directory
}

func ResolveHandle(ctx context.Context, handle string) (string, error) {
	h, err := syntax.ParseHandle(handle)
	if err != nil {
		return "", fmt.Errorf("parsing handle: %w", err)
	}

	ident, err := dir().LookupHandle(ctx, h)
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

	ident, err := dir().LookupDID(ctx, d)
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

	ident, err := dir().Lookup(ctx, atid)
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
