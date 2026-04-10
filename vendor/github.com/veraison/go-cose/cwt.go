package cose

// https://www.iana.org/assignments/cwt/cwt.xhtml#claims-registry
const (
	CWTClaimIssuer         int64 = 1
	CWTClaimSubject        int64 = 2
	CWTClaimAudience       int64 = 3
	CWTClaimExpirationTime int64 = 4
	CWTClaimNotBefore      int64 = 5
	CWTClaimIssuedAt       int64 = 6
	CWTClaimCWTID          int64 = 7
	CWTClaimConfirmation   int64 = 8
	CWTClaimScope          int64 = 9

	// TODO: the rest upon request
)

// CWTClaims contains parameters that are to be cryptographically
// protected.
type CWTClaims map[any]any
