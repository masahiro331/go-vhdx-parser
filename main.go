package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/xerrors"
)

const (
	BitmapAllocationGroup = "2DC27766-F623-4200-9D64-115E9BFD4A08"
	MetadataRegion        = "8B7CA206-4790-4B9A-B8FE-575F050F886E"
	Code                  = 0x1EDC6F41
)

func main() {
	f, err := os.Open("demo-1.0.20220210.0229.vhdx")
	if err != nil {
		log.Fatal(err)
	}

	v := VHDX{}
	v.HeaderSection, err = parseHeaderSection(f)
	if err != nil {
		log.Fatalf("%+v\n", err)
	}

}

type VHDX struct {
	HeaderSection HeaderSection // 1MB Align
}

const (
	_64KB = int(64 << 10)
	_1MB  = int(1 << 20)
)

func parseHeaderSection(rs io.ReadSeeker) (HeaderSection, error) {
	var hs HeaderSection
	var err error
	if err := binaryRead(rs, binary.LittleEndian, &hs.FileIdentifer, _64KB); err != nil {
		return HeaderSection{}, xerrors.Errorf("failed to parse file identifier: %w", err)
	}
	if err := binaryRead(rs, binary.LittleEndian, &hs.Header1, _64KB); err != nil {
		return HeaderSection{}, xerrors.Errorf("failed to parse header 1: %w", err)
	}
	if err := binaryRead(rs, binary.LittleEndian, &hs.Header2, _64KB); err != nil {
		return HeaderSection{}, xerrors.Errorf("failed to parse header 2: %w", err)
	}
	hs.RegionTable1, err = parseRegionTable(rs)
	if err != nil {
		return HeaderSection{}, xerrors.Errorf("failed to parse region table 1: %w", err)
	}
	hs.RegionTable2, err = parseRegionTable(rs)
	if err != nil {
		return HeaderSection{}, xerrors.Errorf("failed to parse region table 2: %w", err)
	}

	return hs, nil
}

func parseRegionTable(rs io.ReadSeeker) (RegionTable, error) {
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

func binaryRead(r io.Reader, o binary.ByteOrder, v interface{}, align int) error {
	buf := make([]byte, align)
	n, err := r.Read(buf)
	if err != nil {
		return xerrors.Errorf("failed to read buf: %w", err)
	}
	if n != align {
		return xerrors.Errorf("read length error: %d", n)
	}

	if err := binary.Read(bytes.NewReader(buf), o, v); err != nil {
		return xerrors.Errorf("failed to read binary: %w", err)
	}

	return nil
}

type HeaderSection struct {
	FileIdentifer FileIdentifer // 64KB Align
	Header1       Header        // 64KB Align
	Header2       Header        // 64KB Align
	RegionTable1  RegionTable   // 64KB Align
	RegionTable2  RegionTable   // 64KB Align
}

type GUID [16]byte

func (g GUID) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		g[0:4], g[4:6], g[6:8], g[8:10], g[10:16])
}

type FileIdentifer struct {
	Signature [8]byte
	Creator   [512]byte
}

type Header struct {
	Signature      [4]byte
	Checksum       [4]byte
	SequenceNumber uint64
	FileWriteGUID  GUID
	DataWriteGUID  GUID
	LogGUID        GUID
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
	GUID       GUID
	FileOffset uint64
	Length     uint32
	Required   [4]byte
}
