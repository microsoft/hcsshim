#
# This makefile lets us exercise the library via the sign1util tool and shows various examples of
# creating and checking COSE Sign1 documents as well as getting DID:x509 documents out of them
# Note that today the PEM files that match are not checked in as the github/ADO credential checker will trigger.
# I may fix that with some head/tail magic. Meanwhile, you need to build fresh certs with make -f Makefile.certs
# and these checks below (for infra.rego.cose, not the EPRS one) will fail. You will need to update the certificate
# fingerprints to match the fresh ones.
# 
# eg change the following
#
#did-check: chain.pem infra.rego.cose sign1util
#	./sign1util check -in infra.rego.cose -did did:x509:0:sha256:RgpNsHOK5hPlCAfTtiGY_BcDhFRxQbJnhlxNDhxps6U::subject:CN:Test%20Leaf%20%28DO%20NOT%20TRUST%29
#
# to
#
#did-check: chain.pem infra.rego.cose sign1util
#	./sign1util check -in infra.rego.cose -did did:x509:0:sha256:my-new-cert-fingerprint::subject:CN:Test%20Leaf%20%28DO%20NOT%20TRUST%29
#
# where 
# 
#	./sign1util did-x509 -chain chain.pem -i 1 -policy "subject:CN:Test Leaf (DO NOT TRUST)"
#
# will print the new fingerprint of the intermediate cert as part of the did:x509 generated
#

cose: infra.rego.cose

# from these media types have to match containerd. The also need to change and the security policy one ought to be x-ms-ccepolicy-frag
#     fragment atrifact type = application/x-ms-ccepolicy-frag
#     fragment media type = application/cose_x509+rego

# Use a local linux build of the tool for the purposes of this Makefile - ie assume using in wsl.
# Usually sign1util.exe is a windows exe in /mnt/c/ContainerPlat aka c:\ContainerPlat but that is not certain.

sign1util: ../../cmd/sign1util/main.go *.go
	go build ../../cmd/sign1util

infra.rego.cose: infra.rego.base64 chain.pem leaf.private.pem sign1util
	./sign1util create -algo ES384 -chain chain.pem -claims infra.rego.base64 -key leaf.private.pem -out $@ -issuer TestIssuer -feed TestFeed -salt zero
	./sign1util check -in $@

print: infra.rego.cose sign1util
	./sign1util chain -in $< > tmp.chain.pem
	./sign1util did-x509 -chain tmp.chain.pem --policy cn

show: sign1util
	./sign1util chain -in esrp.test.cose > tmp.chain.pem
	./sign1util did-x509 -chain tmp.chain.pem -policy cn


didx509: chain.pem sign1util
	./sign1util did-x509 -chain chain.pem -i 1 -policy "subject:CN:Test Leaf (DO NOT TRUST)" -verbose

# for this to pass the did:x509 fingerprint (RgpNsHOK5hPlCAfTtiGY_BcDhFRxQbJnhlxNDhxps6U here) needs to be the one output from make print
did-check: chain.pem infra.rego.cose sign1util
	./sign1util check -in infra.rego.cose -did did:x509:0:sha256:RgpNsHOK5hPlCAfTtiGY_BcDhFRxQbJnhlxNDhxps6U::subject:CN:Test%20Leaf%20%28DO%20NOT%20TRUST%29

# For normal workflow start from the chain.pem, here we'd take the chain from inside the cose sign1 doc, eg to manually confirm it is
# as otherwise expected (ie that the issuer DID matches the chain) or to shortcut getting a DID from a cose document.

did-from-cose: sign1util infra.rego.cose
	./sign1util did-x509 -in infra.rego.cose -policy cn

did-fail-fingerprint: chain.pem sign1util
	./sign1util check -chain chain.pem -in infra.rego.cose -did did:x509:0:sha256:XXXi_nuWegx4NiLaeGabiz36bDUhDDiHEFl8HXMA_4o::subject:CN:Test+Leaf+%28DO+NOT+TRUST%29

did-fail-subject: chain.pem sign1util
	./sign1util check -chain chain.pem -in infra.rego.cose -did did:x509:0:sha256:RgpNsHOK5hPlCAfTtiGY_BcDhFRxQbJnhlxNDhxps6U::subject:CN:Test+XXXX+%28DO+NOT+TRUST%29

did-fail: did-fail-subject did-fail-fingerprint


# beyond the scope of this repo
#infra.rego.base64: infra-fragment.toml
#	/mnt/c/ContainerPlat/securitypolicy.exe -c infra-fragment.toml -n infra -v 1.0.0 -t fragment > infra.rego.base64

# this would push the COSE Sign1 fragment given the appropriate env variables
#oras: infra.rego.cose
#	oras push ${REGISTRY}/${INFRA_REPO}:latest \
#	--artifact-type application/x-ms-ccepolicy-frag \
#	--manifest-config /dev/null:application/vnd.unknown.config.v1+json \
#	--subject ${INFRA_IMAGE} \
#	./infra.rego.cose:application/cose_x509+rego

%.pem:
	 $(MAKE) -f Makefile.certs chain.pem


infra.rego.base64: infra.rego
	base64 infra.rego > infra.rego.base64

test-all: print show didx509 did-check did-from-cose did-fail

clean:
	$(MAKE) -f Makefile.certs $@
	rm -f infra.rego.base64 infra.rego.cose sign1util
