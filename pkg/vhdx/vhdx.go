package vhdx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"math"
	"math/bits"
	"os"

	"github.com/masahiro331/go-vhdx-parser/pkg/utils"
	"golang.org/x/xerrors"
)

var _ io.ReaderAt = &VHDX{}

type VHDX struct {
	HeaderSection         HeaderSection // 1MB Align
	MetadataTable         MetadataTable
	BlockAllocationTables []BAT

	file *os.File
	off  int64

	state VHDXState
	sinfo VHDXStateInfo
}

type VHDXStateInfo struct {
	batIndex       int
	blockOffset    int64
	fileOffset     int64
	bytesAvailable int64
}

type VHDXState struct {
	chunkRatio            uint32
	chunkRatioBits        int
	sectorPerBlock        uint32
	sectorPerBlockBits    int
	logicalSectorSizeBits int
}

func (v *VHDX) ReadAt(p []byte, off int64) (n int, err error) {
	if len(p) != SupportSectorSize {
		return 0, xerrors.Errorf("invalid byte length %d, required %d bytes length", len(p), SupportSectorSize)
	}
	sinfo, err := v.TranslateOffset(off)
	if err != nil {
		return 0, xerrors.Errorf("failed to translate offset: %w", err)
	}
	v.sinfo = *sinfo

	bat, err := v.bat()
	if err != nil {
		return 0, xerrors.Errorf("failed to get BAT: %w", err)
	}

	switch bat.State {
	case PAYLOAD_BLOCK_NOT_PRESENT, PAYLOAD_BLOCK_UNDEFINED, PAYLOAD_BLOCK_UNMAPPED, PAYLOAD_BLOCK_PARTIALLY_PRESENT:
		fmt.Printf("%+v\n", v.sinfo)
		return 0, xerrors.Errorf("unsupported bat state: %d, bat index: %d", bat.State, v.sinfo.batIndex)
	case PAYLOAD_BLOCK_ZERO:
		buf := bytes.NewBuffer(make([]byte, SupportSectorSize))
		if int64(len(p)) > v.sinfo.bytesAvailable {
			return buf.Write(p[:v.sinfo.bytesAvailable])
		}
		return buf.Write(p)
	case PAYLOAD_BLOCK_FULLY_PRESENT:
		n, err := v.file.Seek(v.sinfo.fileOffset, 0)
		if err != nil {
			return 0, xerrors.Errorf("failed to seek(%d): %w", v.sinfo.fileOffset, err)
		}
		if n != sinfo.fileOffset {
			return 0, xerrors.Errorf("invalid file offset: %d, actual: %d", sinfo.fileOffset, n)
		}
		return v.file.Read(p)
	default:
		return 0, xerrors.Errorf("unknown bat state: %d", bat.State)
	}
}

func (v *VHDX) TranslateOffset(physicalOffset int64) (*VHDXStateInfo, error) {
	if physicalOffset%int64(v.MetadataTable.SystemData.LogicalSectorSize) != 0 {
		return nil, xerrors.New("offset size error")
	}
	secNumber := physicalOffset / int64(v.MetadataTable.SystemData.LogicalSectorSize)

	batIndex := int(secNumber) >> v.state.sectorPerBlockBits
	blockOffset := secNumber - int64(batIndex<<v.state.sectorPerBlockBits)
	batIndex += batIndex >> v.state.chunkRatioBits

	sectorsAvail := int64(v.state.sectorPerBlock) - blockOffset
	blockOffset = blockOffset << v.state.logicalSectorSizeBits

	fileOffset := int64(v.BlockAllocationTables[batIndex].FileOffset<<20) & VHDX_BAT_FILE_OFF_MASK
	fileOffset += blockOffset

	bytesAvailable := sectorsAvail << v.state.logicalSectorSizeBits

	return &VHDXStateInfo{
		batIndex:       batIndex,
		blockOffset:    blockOffset,
		fileOffset:     fileOffset,
		bytesAvailable: bytesAvailable,
	}, nil
}

func (v *VHDX) Reset() error {
	if len(v.BlockAllocationTables) == 0 {
		return xerrors.New("invalid BAT length, empty error")
	}
	startBat := BAT{FileOffset: math.MaxInt64}
	for _, b := range v.BlockAllocationTables {
		if b.FileOffset != 0 && b.FileOffset < startBat.FileOffset {
			startBat = b
		}
	}
	_, err := v.file.Seek(int64(startBat.FileOffset*uint64(_1MB)), 0)
	if err != nil {
		return xerrors.Errorf("failed to seek to %d error: %w", startBat.State, err)
	}
	return nil
}

func (v *VHDX) blockSize() int {
	return int(v.MetadataTable.SystemData.FileParameter.BlockSize)
}

func (v *VHDX) currentPhysicalOffset() int64 {
	off, _ := v.file.Seek(0, io.SeekCurrent)
	return off
}

func (v *VHDX) Size() int64 {
	return int64(v.MetadataTable.SystemData.VirtualDiskSize)
}

func (v *VHDX) Close() error {
	return v.file.Close()
}

func Open(name string) (*io.SectionReader, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, xerrors.Errorf("failed to open %s: %w", name, err)
	}

	v := VHDX{file: f}
	v.HeaderSection, err = parseHeaderSection(f)
	if err != nil {
		return nil, err
	}

	for _, entry := range v.HeaderSection.RegionTables[0].RegionTableEntries {
		n, err := f.Seek(int64(entry.FileOffset), 0)
		if err != nil {
			return nil, xerrors.Errorf("failed to seek entry: %w", err)
		}
		if n != int64(entry.FileOffset) {
			return nil, xerrors.Errorf("failed to seek offset: actual(%d), expected(%d)", n, entry.FileOffset)
		}

		buf := bytes.NewBuffer(nil)
		n, err = buf.ReadFrom(io.LimitReader(f, int64(entry.Length)))
		if err != nil {
			return nil, xerrors.Errorf("failed to read entry: %w", err)
		}
		if n != int64(entry.Length) {
			return nil, xerrors.Errorf("failed to read length: actual(%d), expected(%d)", n, entry.Length)
		}
		switch entry.GUID.String() {
		case BitmapAllocationGroup:
			var bats []BAT
			for i := 0; i < int(entry.Length)/8; i++ {
				b := [8]byte{}
				n, err := buf.Read(b[:])
				if err != nil {
					return nil, xerrors.Errorf("failed to read entry %d buf: %w", i, err)
				}
				if n != len(b) {
					return nil, xerrors.Errorf("failed to read length: actual(%d), expected(%d)", n, len(b))
				}
				bats = append(bats, parseBAT(b))
			}
			v.BlockAllocationTables = bats
		case MetadataRegion:
			table, err := parseMetadataTable(buf)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse metadata table: %w", err)
			}
			for _, entry := range table.Entries {
				if entry.Offset == 0 || entry.Length == 0 {
					continue
				}
				if int(entry.Length) > _1MB {
					return nil, xerrors.Errorf("invalid entry length %d", entry.Length)
				}
				if int(entry.Offset) < _64KB {
					return nil, xerrors.Errorf("invalid entry offset %d", entry.Offset)
				}
				entry.Offset = entry.Offset - uint32(_64KB)

				item := entry.item(buf.Bytes())
				switch entry.ItemID.String() {
				case FileParameters:
					table.SystemData.FileParameter, err = entry.fileParameter(item)
				case VirtualDiskSize:
					table.SystemData.VirtualDiskSize, err = entry.virtualDiskSize(item)
				case VirtualDiskID:
					table.SystemData.VirtualDiskID, err = entry.virtualDiskID(item)
				case LogicalSectorSize:
					table.SystemData.LogicalSectorSize, err = entry.logcalSectorSize(item)
				case PhysicalSectorSize:
					table.SystemData.PhysicalSectorSize, err = entry.physicalSectorSize(item)
				case ParentLocator:
				}
				if err != nil {
					return nil, xerrors.Errorf("failed to parse item error: %w", err)
				}
			}
			v.MetadataTable = *table
		default:
			log.Println(entry.GUID.String())
		}
	}
	v.state = v.NewState()

	if err = v.Reset(); err != nil {
		return nil, xerrors.Errorf("failed to seek initial offset: %w", err)
	}

	return io.NewSectionReader(&v, 0, v.Size()), nil
}

func (v *VHDX) NewState() VHDXState {
	chunkRatio := VHDX_MAX_SECTORS_PER_BLOCK * int64(v.MetadataTable.SystemData.LogicalSectorSize) / int64(v.MetadataTable.SystemData.FileParameter.BlockSize)
	sectorPerBlock := v.MetadataTable.SystemData.FileParameter.BlockSize / v.MetadataTable.SystemData.LogicalSectorSize
	return VHDXState{
		chunkRatio:            uint32(chunkRatio),
		chunkRatioBits:        bits.TrailingZeros64(uint64(chunkRatio)),
		sectorPerBlock:        sectorPerBlock,
		sectorPerBlockBits:    bits.TrailingZeros32(sectorPerBlock),
		logicalSectorSizeBits: bits.TrailingZeros32(v.MetadataTable.SystemData.LogicalSectorSize),
	}
}

func (e *MetadataTableEntry) physicalSectorSize(b []byte) (uint32, error) {
	if len(b) != 4 {
		return 0, xerrors.New("invalid physical sector size error")
	}

	return binary.LittleEndian.Uint32(b), nil
}

func (e *MetadataTableEntry) logcalSectorSize(b []byte) (uint32, error) {
	if len(b) != 4 {
		return 0, xerrors.New("invalid logical sector size error")
	}

	return binary.LittleEndian.Uint32(b), nil
}

func (e *MetadataTableEntry) virtualDiskSize(b []byte) (uint64, error) {
	if len(b) != 8 {
		return 0, xerrors.New("invalid virtual disk size error")
	}

	return binary.LittleEndian.Uint64(b), nil
}

func (e *MetadataTableEntry) virtualDiskID(b []byte) (uuid.UUID, error) {
	if len(b) != 16 {
		return uuid.UUID{}, xerrors.New("invalid virtual disk id error")
	}
	return uuid.FromBytes(b)
}

func (e *MetadataTableEntry) fileParameter(b []byte) (FileParameter, error) {
	if len(b) < 6 {
		return FileParameter{}, xerrors.New("invalid file parameter error")
	}

	var f FileParameter
	f.BlockSize = binary.LittleEndian.Uint32(b[0:4])
	f.LeaveBlockAllocated = b[5]&1 == 1
	f.HasParent = b[5]&2 == 2
	return f, nil
}
func parseMetadataTable(r io.Reader) (*MetadataTable, error) {
	r = io.LimitReader(r, int64(_64KB))
	defer io.ReadAll(r) // alignment

	metadataTable := MetadataTable{}
	if err := binary.Read(r, binary.LittleEndian, &metadataTable.Header); err != nil {
		return nil, xerrors.Errorf("failed to read metadata table header: %w")
	}
	if metadataTable.Header.EntryCount > 2047 {
		return nil, xerrors.Errorf("invalid entry count: %d", metadataTable.Header.EntryCount)
	}
	for i := 0; i < int(metadataTable.Header.EntryCount); i++ {
		entry := MetadataTableEntry{}
		if err := binary.Read(r, binary.LittleEndian, &entry.ItemID); err != nil {
			return nil, xerrors.Errorf("failed to read entry Item ID: %d: %w", i, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &entry.Offset); err != nil {
			return nil, xerrors.Errorf("failed to read entry Offset: %d: %w", i, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &entry.Length); err != nil {
			return nil, xerrors.Errorf("failed to read entry Length: %d: %w", i, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &entry.Permission); err != nil {
			return nil, xerrors.Errorf("failed to read entry Permission: %d: %w", i, err)
		}
		if _, err := r.Read(entry._Reserved[:]); err != nil {
			return nil, xerrors.Errorf("failed to read entry Reserved: %d: %w", i, err)
		}
		metadataTable.Entries = append(metadataTable.Entries, entry)
	}

	return &metadataTable, nil
}

func parseHeaderSection(r io.Reader) (HeaderSection, error) {
	var hs HeaderSection
	var err error

	if err := utils.BinaryRead(r, binary.LittleEndian, &hs.FileIdentifer, _64KB); err != nil {
		return HeaderSection{}, xerrors.Errorf("failed to parse file identifier: %w", err)
	}

	for i, hs := range hs.Headers {
		if err := utils.BinaryRead(r, binary.LittleEndian, &hs, _64KB); err != nil {
			return HeaderSection{}, xerrors.Errorf("failed to parse header %d: %w", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		hs.RegionTables[i], err = parseRegionTable(r)
		if err != nil {
			return HeaderSection{}, xerrors.Errorf("failed to parse region table 1: %w", err)
		}
	}

	return hs, nil
}

func parseRegionTable(rs io.Reader) (RegionTable, error) {
	buf := make([]byte, _64KB)
	n, err := rs.Read(buf)
	if err != nil {
		return RegionTable{}, xerrors.Errorf("failed to read buf: %w", err)
	}
	if n != _64KB {
		return RegionTable{}, xerrors.Errorf("read length error: %d", n)
	}

	r := bytes.NewReader(buf)
	var rt RegionTable
	if err := binary.Read(r, binary.LittleEndian, &rt.RegionTableHeader); err != nil {
		return RegionTable{}, xerrors.Errorf("failed to read region table header: %w", err)
	}
	for n := uint32(0); n < rt.RegionTableHeader.EntryCount; n++ {
		var entry RegionTableEntry
		if err := binary.Read(r, binary.LittleEndian, &entry); err != nil {
			return RegionTable{}, xerrors.Errorf("failed to read region table entry %d: %w", n, err)
		}
		rt.RegionTableEntries = append(rt.RegionTableEntries, entry)
	}

	return rt, nil
}

func parseBAT(b [8]byte) BAT {
	return BAT{
		State:      PayloadState(b[0]),
		FileOffset: binary.LittleEndian.Uint64(append(b[2:], make([]byte, 3)...)) >> 4,
	}
}

func (v *VHDX) bat() (BAT, error) {
	if len(v.BlockAllocationTables) <= v.sinfo.batIndex {
		return BAT{}, xerrors.Errorf("invalid bat index, BAT length: %d, BAT index: %d", len(v.BlockAllocationTables), v.sinfo.batIndex)
	}
	return v.BlockAllocationTables[v.sinfo.batIndex], nil
}

func (e *MetadataTableEntry) item(b []byte) []byte {
	return b[e.Offset : e.Offset+e.Length]
}
