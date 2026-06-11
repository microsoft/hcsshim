package cosesign1

const COSE_Sign1_Tag = 18

// COSE Header Parameters
// https://www.iana.org/assignments/cose/cose.xhtml
const (
	COSE_Header_kid                 = int64(4)
	COSE_Header_CWTClaims           = int64(15)
	COSE_Header_x5chain             = int64(33)
	COSE_Header_x5t                 = int64(34)
	COSE_Header_PayloadHashAlg      = int64(258)
	COSE_Header_PreimageContentType = int64(259)
	COSE_Header_PayloadLocation     = int64(260)
	COSE_Header_Receipts            = int64(394)
	COSE_Header_vds                 = int64(395)
	COSE_Header_vdp                 = int64(396)
)

// COSE Verifiable Data Structure Algorithms
// (Values for COSE_HeaderLabelvds)
const (
	COSE_vds_RFC9162_SHA256    = int64(1)

	// TBD_1 in https://www.ietf.org/archive/id/draft-birkholz-cose-receipts-ccf-profile-05.html
	COSE_vds_CCF_LEDGER_SHA256 = int64(2)
)

// COSE Verifiable Data Structure Proofs
// (These are the map keys inside a COSE_HeaderLabelReceipts header).
const (
	COSE_ProofInclusion   = int64(-1)
	COSE_ProofConsistency = int64(-2)
)

// CWT Claims
// https://www.iana.org/assignments/cwt/cwt.xhtml
const (
	CWT_Issuer  = int64(1)
	CWT_Subject = int64(2)
)
