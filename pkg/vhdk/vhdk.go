package vhdk

import (
	"bytes"
	"encoding/binary"
	"github.com/google/uuid"
	"io"
	"log"
	"math"
	"os"

	"github.com/masahiro331/go-vhdx-parser/pkg/utils"
	"golang.org/x/xerrors"
)

var _ io.ReaderAt = VHDX{}
var _ io.ReadSeekCloser = VHDX{}

type VHDX struct {
	HeaderSection          HeaderSection // 1MB Align
	MetadataTable          MetadataTable
	BitmapAllocationGroups []BAT

	file *os.File
	off  int64
}

func (v VHDX) Read(p []byte) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (v VHDX) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
	case io.SeekEnd:
	}

	//TODO implement me
	panic("implement me")
}

func (v VHDX) Reset() error {
	if len(v.BitmapAllocationGroups) == 0 {
		return xerrors.New("invalid bit map allocation groups length,  empty error")
	}
	startBat := BAT{FileOffset: math.MaxInt64}
	for _, b := range v.BitmapAllocationGroups {
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

func (v VHDX) blockSize() int {
	return int(v.MetadataTable.SystemData.FileParameter.BlockSize)
}

func (v VHDX) currentPhysicalOffset() int64 {
	off, _ := v.file.Seek(0, io.SeekCurrent)
	return off
}

func (v VHDX) ReadAt(p []byte, off int64) (n int, err error) {
	for _, bat := range v.BitmapAllocationGroups {
		switch bat.State {
		case PAYLOAD_BLOCK_NOT_PRESENT:
		case PAYLOAD_BLOCK_UNDEFINED:
		case PAYLOAD_BLOCK_ZERO:
		case PAYLOAD_BLOCK_UNMAPPED:
		case PAYLOAD_BLOCK_FULLY_PRESENT:
			// _, err := v.file.Seek(int64(bat.FileOffset*uint64(_1MB)), 0)
			// r := io.LimitReader(v.file, int64(v.MetadataTable.SystemData.FileParameter.BlockSize))

		case PAYLOAD_BLOCK_PARTIALLY_PRESENT:
		default:
		}
	}
	return 0, nil
}

func (v VHDX) Close() error {
	return v.file.Close()
}

func Open(name string) (*VHDX, error) {
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
		_, err := f.Seek(int64(entry.FileOffset), 0)
		if err != nil {
			log.Fatal(err)
		}

		buf := bytes.NewBuffer(nil)
		if _, err := buf.ReadFrom(io.LimitReader(f, int64(entry.Length))); err != nil {
			return nil, xerrors.Errorf("failed to read entry: %w", err)
		}
		switch entry.GUID.String() {
		case BitmapAllocationGroup:
			var bats []BAT
			for i := 0; i < int(entry.Length)/8; i++ {
				b := [8]byte{}
				_, err := buf.Read(b[:])
				if err != nil {
					return nil, xerrors.Errorf("failed to read entry %d buf: %w", i, err)
				}
				bats = append(bats, parseBAT(b))
			}
			v.BitmapAllocationGroups = bats
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

	if err = v.Reset(); err != nil {
		return nil, xerrors.Errorf("failed to seek initial offset: %w", err)
	}
	return &v, nil
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

func (e *MetadataTableEntry) item(b []byte) []byte {
	// fmt.Printf("Offset: %d, Length: %d, range[%d ~ %d]: raw[%v]\n", e.Offset, e.Length, e.Offset, e.Length+e.Offset, b[e.Offset:e.Offset+e.Length])
	return b[e.Offset : e.Offset+e.Length]
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
