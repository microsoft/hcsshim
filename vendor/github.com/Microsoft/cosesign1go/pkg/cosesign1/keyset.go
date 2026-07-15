package cosesign1

import (
	"crypto"

	"github.com/fxamacker/cbor/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	cose "github.com/veraison/go-cose"
)

// Parses a COSE_KeySet, which is a CBOR array of raw COSE_Key objects, into a
// map from key IDs to public keys, to be used for receipt validation.
//
// Reference: https://www.rfc-editor.org/rfc/rfc9052.html#name-cose-keys
func ParseKeySetAsMap(data []byte) (map[string]crypto.PublicKey, error) {
	var rawKeys []cbor.RawMessage
	if err := cbor.Unmarshal(data, &rawKeys); err != nil {
		return nil, errors.Wrap(err, "Failed to parse the COSE_KeySet")
	}
	if len(rawKeys) == 0 {
		return nil, errors.New("empty COSE Key Set")
	}
	var lastKeyError error
	keys := make(map[string]crypto.PublicKey)
	for i, raw := range rawKeys {
		// From RFC: Each element in a COSE Key Set MUST be processed
		// independently. If one element in a COSE Key Set is either malformed
		// or uses a key that is not understood by an application, that key is
		// ignored, and the other keys are processed normally.
		var k cose.Key
		if err := k.UnmarshalCBOR(raw); err != nil {
			logrus.Warnf("Failed to parse element %d of the COSE Key Set: %v", i, err)
			lastKeyError = errors.Wrapf(err, "UnmarshalCBOR element %d", i)
			continue
		}
		kid := string(k.ID)
		if kid == "" {
			logrus.Warnf("Failed to parse element %d of the COSE Key Set: missing key ID, ignoring this key", i)
			lastKeyError = errors.Errorf("missing key ID in element %d", i)
			continue
		}
		pk, err := k.PublicKey()
		if err != nil {
			logrus.Warnf("Failed to construct public key from element %d of the COSE Key Set (kid=%q): %v", i, kid, err)
			lastKeyError = errors.Wrapf(err, "construct PublicKey from element %d", i)
			continue
		}
		if existingKey, exists := keys[kid]; exists {
			// Equal is implemented for all crypto.PublicKey types in std
			eq, ok := existingKey.(interface{ Equal(crypto.PublicKey) bool })
			if !ok || !eq.Equal(pk) {
				logrus.Warnf("Parsing element %d of the COSE Key Set: Key with ID %q already seen earlier but got another conflicting key with same ID, ignoring this one", i, kid)
				continue
			}
		}
		keys[kid] = pk
	}
	if len(keys) == 0 {
		logrus.Errorf("Failed to parse any element of the provided COSE Key Set")
		return nil, lastKeyError
	}
	return keys, nil
}

// ParseTTLPayload parses an unsigned body of a Transparency Trust List (TTL),
// which is a CBOR map from issuer strings to LedgerEntry maps. Each LedgerEntry
// is a CBOR map keyed by integer attributes; the TTL_LedgerEntry_Keys (1)
// attribute holds that issuer's COSE_KeySet. The result is a map from issuer to
// that issuer's map of key IDs to public keys.
//
// Reference: https://github.com/achamayou/scitt-ccf-ledger/blob/ttl/docs/transparent_trust_lists.md
func ParseTTLPayload(data []byte) (map[string]map[string]crypto.PublicKey, error) {
	var rawIssuers map[string]cbor.RawMessage
	if err := cbor.Unmarshal(data, &rawIssuers); err != nil {
		return nil, errors.Wrap(err, "Failed to parse the TTL payload")
	}
	if len(rawIssuers) == 0 {
		return nil, errors.New("empty TTL payload")
	}
	out := make(map[string]map[string]crypto.PublicKey, len(rawIssuers))
	for issuer, rawEntry := range rawIssuers {
		var entry map[int64]cbor.RawMessage
		if err := cbor.Unmarshal(rawEntry, &entry); err != nil {
			return nil, errors.Wrapf(err, "parsing LedgerEntry for issuer %q", issuer)
		}
		rawKeySet, ok := entry[TTL_LedgerEntry_Keys]
		if !ok {
			return nil, errors.Errorf("LedgerEntry for issuer %q is missing the keys attribute (%d)", issuer, TTL_LedgerEntry_Keys)
		}
		keys, err := ParseKeySetAsMap(rawKeySet)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing COSE_KeySet for issuer %q", issuer)
		}
		out[issuer] = keys
	}
	return out, nil
}
