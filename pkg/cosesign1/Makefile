# some naming info to explain the confusion...
# pointers to the registry etc so as to simplify typing and scripts
#
# we have images, repositories and repositories.
#
# ACR_NAME is the registry, eg parmawesteuroperegistry, which is a place many containers can live
# REPO, eg payload, infra, kens_container, is the name the repository 
# TAG, eg 2.15, latest, is the version number of an image
# IMAGE which is $REGISTRY/${REPO}:$TAG is a particular image, which is sort of a version of a container.

all: infra.rego 


cose: infra.rego.cose

# from Maksim:
#     fragment atrifact type = application/unknown+rego
#     fragment media type = application/microsoft.security.policy+rego

infra.rego.cose : infra.rego.base64 ec384-key.pem
	/mnt/c/ContainerPlat/sign1util.exe create -algo ES384 -cert ec384-cert.crt -claims infra.rego.base64 -key ec384-key.pem -out infra.rego.cose -issuer TestIssuer -feed TestFeed -salt zero
	/mnt/c/ContainerPlat/sign1util.exe leaf -in infra.rego.cose -keyout leafkey.ec384-public.pem -certout leafcert.ec384-public.pem

oras: infra.rego.cose
	oras push ${REGISTRY}/${INFRA_REPO}:latest \
	--artifact-type application/unknown+rego \
	--manifest-config /dev/null:application/vnd.unknown.config.v1+json \
	--subject ${INFRA_IMAGE} \
	./infra.rego.cose:application/microsoft.security.policy+rego

ec384-key.pem: ec384-private.body
	echo "-----BEGIN PRIVATE KEY-----" > ec384-key.pem
	cat ec384-private.body >> ec384-key.pem
	echo "-----END PRIVATE KEY-----" >> ec384-key.pem


# from Matthew - /securitypolicy -c fragment.toml -n parmafragment        -v 1.0.0 -r                      -t fragment
#                                   input            namespace (in the rego) SVN   output rego to stdout   type is a fragment
#infra.rego.base64: infra-fragment.toml
#	/mnt/c/ContainerPlat/securitypolicy.exe -c infra-fragment.toml -n infra -v 1.0.0 -t fragment > infra.rego.base64

infra.rego.base64: infra.rego
	base64 infra.rego > infra.rego.base64

clean:
	rm -f infra.rego.base64 infra.rego.cose leafkey.ec384-public.pem leafcert.ec384-public.pem

	
