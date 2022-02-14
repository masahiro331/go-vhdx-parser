package vhdk

const (
	Code  = 0x1EDC6F41
	_64KB = int(64 << 10)
	_1MB  = int(1 << 20)

	BitmapAllocationGroup = "6677c22d-23f6-0042-9d64-115e9bfd4a08"
	MetadataRegion        = "06a27c8b-9047-9a4b-b8fe-575f050f886e"
	FileParameters        = "3767a1ca-36fa-434d-b3b6-33f0aa44e76b"
	VirtualDiskSize       = "2442a52f-1bcd-7648-b211-5dbed83bf4b8"
	VirtualDiskID         = "ab12cabe-e6b2-2345-93ef-c309e000c746"
	LogicalSectorSize     = "1dbf4181-6fa9-0947-ba47-f233a8faab5f"
	PhysicalSectorSize    = "c748a3cd-5d44-7144-9cc9-e9885251c556"
	ParentLocator         = "2d5fd3a8-0bb3-4d45-abf7-d3d84834ab0c"

	PAYLOAD_BLOCK_NOT_PRESENT       PayloadState = 0
	PAYLOAD_BLOCK_UNDEFINED         PayloadState = 1
	PAYLOAD_BLOCK_ZERO              PayloadState = 2
	PAYLOAD_BLOCK_UNMAPPED          PayloadState = 3
	PAYLOAD_BLOCK_FULLY_PRESENT     PayloadState = 6
	PAYLOAD_BLOCK_PARTIALLY_PRESENT PayloadState = 7
)
