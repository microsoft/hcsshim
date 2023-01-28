# sign1util

`sign1util` exists as a tool to make it possible to sign policy fragments and check such
signed fragments. It is intended for developers working functionality related to policy
fragments in this repository. It is not intended to be used by "end users".

Usage is of the form `sign1util <cmd> flag1 value1 flag2 value2...`

The output is generally a COSE Sign1 wrapped payload. COSE Sign1 is a signed binary blob that can contain arbitary binary data.
For a fragment the COSE Sign1 document must have been signed by a trusted party (aka "issuer") and use the did matching the cert chain leading to the private signing key as the issuer. Below that chain is `chain.pem` and the private key `leaf.private.pem`. When creating a fragment the issuer can be set using this tool or via the corportate signing authority's COSE Sign1 generating service. It is very important that these private keys and associated signing services are properly controlled. The signing offered by sign1util is by way of an example and useful for testing. It does not have facilities to use a secure key store.

Security policy fragments are checked for having the correct issuer did:x509 and feed as allowed by user security policy. The did must match the chain and key used to sign the document.

## Commands

`sign1utils --help` gives an overview of the commands and flags. Here is a description of the purpose of the commands.

### create

Creates a COSE Sign1 document containing a payload "claims" signed with the supplied key and containing the matching public cert chain.
For the purposes of making fragments documents supply an issuer (typically the did:x509 of the chain) and feed to identity what the fragment represents.

`sign1util create -algo ES384 -chain chain.pem -claims infra.rego -key leaf.private.pem -out infra.rego.cose -feed myregistry.azurecr.io/infra -issuer did:x509:0:sha256:I5ni_nuWegx4NiLaeGabiz36bDUhDDiHEFl8HXMA_4o::subject:CN:Test%20Leaf%20%28DO%20NOT%20TRUST%29`

A zero salt option is available is to facilitate unit tests such that the generated file is deterministic.

### check

Validate such a COSE Sign1 document is signed by the chain it contains and that that chain is a valid chain

`sign1util check -in infra.rego.cose`

Also checking a did matches.
`sign1util check -in infra.rego.cose -did did:x509:0:sha256:I5ni_nuWegx4NiLaeGabiz36bDUhDDiHEFl8HXMA_4o::subject:CN:Test%20Leaf%20%28DO%20NOT%20TRUST%29`

### print

Dumps out various parts of the document to help developers understand what information is contained in the wrapping part of the COSE Sign1 document vs the payload.

### leaf

In some cases (eg UVM reference info document) the check for a good document is by the leaf public key being as expected. This command allows extracting that key from a given COSE Sign1 document.

### did:x509

Print the did:x509 that matches the chain for a given subject. This can be used as an issuer.

`sign1util.exe did:x509 -chain chain.pem -policy CN` might produce `did:x509:0:sha256:I5ni_nuWegx4NiLaeGabiz36bDUhDDiHEFl8HXMA_4o::subject:CN:Test%20Leaf%20%28DO%20NOT%20TRUST%29`

### chain

Dumps the PEM formatted cert chain found in the COSE Sign1 document.
