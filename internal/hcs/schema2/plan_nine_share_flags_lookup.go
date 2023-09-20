package hcsschema

func (x Plan9ShareFlags) Int64() (int64, error) {
	return enumLookup(map[Plan9ShareFlags]int64{
		Plan9ShareFlags_NONE:                    0x00000000,
		Plan9ShareFlags_READ_ONLY:               0x00000001,
		Plan9ShareFlags_LINUX_METADATA:          0x00000004,
		Plan9ShareFlags_CASE_SENSITIVE:          0x00000008,
		Plan9ShareFlags_USE_SHARE_ROOT_IDENTITY: 0x00000010,
		Plan9ShareFlags_ALLOW_OPTIONS:           0x00000020,
		Plan9ShareFlags_ALLOW_SUB_PATHS:         0x00000040,
		Plan9ShareFlags_RESTRICT_FILE_ACCESS:    0x00000080,
		Plan9ShareFlags_UNLIMITED_CONNECTIONS:   0x00000100,
	}, x)
}
