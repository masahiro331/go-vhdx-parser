package vhdk

import (
	"fmt"
	"github.com/google/uuid"
)

type PayloadState int

type HeaderSection struct {
	FileIdentifer FileIdentifer  // 64KB Align
	Headers       [2]Header      // 64KB Align
	RegionTables  [2]RegionTable // 64KB Align
}

type FileIdentifer struct {
	Signature [8]byte
	Creator   [512]byte
}

type Header struct {
	Signature      [4]byte
	Checksum       [4]byte
	SequenceNumber uint64
	FileWriteGUID  uuid.UUID
	DataWriteGUID  uuid.UUID
	LogGUID        uuid.UUID
	LogVersion     uint16
	Version        uint16
	LogLength      uint32
	LogOffset      uint64
	_              [4016]byte
}

type RegionTable struct {
	RegionTableHeader  RegionTableHeader
	RegionTableEntries []RegionTableEntry
}

type RegionTableHeader struct {
	Signature  [4]byte
	Checksum   [4]byte
	EntryCount uint32
	Required   [4]byte
}

type RegionTableEntry struct {
	GUID       uuid.UUID
	FileOffset uint64
	Length     uint32
	Required   [4]byte
}

type BAT struct {
	State      PayloadState
	FileOffset uint64
}

// MetadataTable is required 64 KB
type MetadataTable struct {
	Header     MetadataTableHeader
	Entries    []MetadataTableEntry
	SystemData SystemMetadata
}

type MetadataTableHeader struct {
	Signature  [8]byte
	_          [2]byte
	EntryCount uint16
	_          [20]byte
}

type MetadataTableEntry struct {
	ItemID     uuid.UUID
	Offset     uint32
	Length     uint32
	Permission Permission
	// Permission structure
	// isUser        bool // 1 bit
	// IsVirtualDisk bool // 1 bit
	// IsRequired    bool // 1 bit
	_Reserved [7]byte
}

type SystemMetadata struct {
	FileParameter      FileParameter
	VirtualDiskSize    uint64
	VirtualDiskID      uuid.UUID
	LogicalSectorSize  uint32
	PhysicalSectorSize uint32
}

type FileParameter struct {
	BlockSize           uint32
	LeaveBlockAllocated bool
	HasParent           bool
}

type Permission uint8

// IsUser required
func (p Permission) IsUser() bool {
	return p&1 == 1
}

// IsVirtualDisk required
func (p Permission) IsVirtualDisk() bool {
	return p&2 == 2
}

// IsRequired required
func (p Permission) IsRequired() bool {
	return p&4 == 4
}

func (p Permission) String() string {
	return fmt.Sprintf("%d%d%d", p&1/1, p&2/2, p&4/4)
}
