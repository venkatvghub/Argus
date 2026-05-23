package analysis

const (
	tiktokenEncoding = "cl100k_base"

	tokenBloatThresholdDefault = 50.0

	phantomCouplingChurnThreshold     = 5
	phantomCouplingOwnershipThreshold = 0.5
	ownershipPercentMultiplier        = 100

	dartAsyncGapMaxRunes = 200
	sqlConcatWindowRunes = 100
	sqlCredentialMinLen  = 3

	pageRankDamping     = 0.85
	pageRankTolerance   = 1e-6
	leidenResolution    = 1.0
	leidenIterations    = 10
	floatCompareEpsilon = 1e-9
)
