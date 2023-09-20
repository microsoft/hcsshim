package hcsschema

func (x ResolutionType) Int64() (int64, error) {
	return enumLookup(map[ResolutionType]int64{
		ResolutionType_UNSPECIFIED: 0,
		ResolutionType_MAXIMUM:     2,
		ResolutionType_SINGLE:      3,
		ResolutionType_DEFAULT_:    4,
	}, x)
}
