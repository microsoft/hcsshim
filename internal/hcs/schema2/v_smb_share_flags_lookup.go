package hcsschema

func (x VSmbShareFlags) Int64() (int64, error) {
	return enumLookup(map[VSmbShareFlags]int64{
		VSmbShareFlags_NONE:                        0x00000000,
		VSmbShareFlags_READ_ONLY:                   0x00000001,
		VSmbShareFlags_SHARE_READ:                  0x00000002,
		VSmbShareFlags_CACHE_IO:                    0x00000004,
		VSmbShareFlags_NO_OPLOCKS:                  0x00000008,
		VSmbShareFlags_TAKE_BACKUP_PRIVILEGE:       0x00000010,
		VSmbShareFlags_USE_SHARE_ROOT_IDENTITY:     0x00000020,
		VSmbShareFlags_NO_DIRECTMAP:                0x00000040,
		VSmbShareFlags_NO_LOCKS:                    0x00000080,
		VSmbShareFlags_NO_DIRNOTIFY:                0x00000100,
		VSmbShareFlags_TEST:                        0x00000200,
		VSmbShareFlags_VM_SHARED_MEMORY:            0x00000400,
		VSmbShareFlags_RESTRICT_FILE_ACCESS:        0x00000800,
		VSmbShareFlags_FORCE_LEVEL_II_OPLOCKS:      0x00001000,
		VSmbShareFlags_REPARSE_BASE_LAYER:          0x00002000,
		VSmbShareFlags_PSEUDO_OPLOCKS:              0x00004000,
		VSmbShareFlags_NON_CACHE_IO:                0x00008000,
		VSmbShareFlags_PSEUDO_DIRNOTIFY:            0x00010000,
		VSmbShareFlags_DISABLE_INDEXING:            0x00020000,
		VSmbShareFlags_HIDE_ALTERNATE_DATA_STREAMS: 0x00040000,
		VSmbShareFlags_ENABLE_FSCTL_FILTERING:      0x00080000,
		VSmbShareFlags_ALLOW_NEW_CREATES:           0x00100000,
	}, x)
}
