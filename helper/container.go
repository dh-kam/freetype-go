package helper

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

// ExtractMacDFONT extracts 'sfnt' resources from a Mac Resource Fork.
func ExtractMacDFONT(in api.Stream) ([]api.Stream, error) {
	if in.Size() < 16 {
		return nil, errors.New("stream too small for Mac Resource Fork")
	}

	header := make([]byte, 16)
	if _, err := in.ReadAt(header, 0); err != nil {
		return nil, err
	}

	dataOffset := binary.BigEndian.Uint32(header[0:4])
	mapOffset := binary.BigEndian.Uint32(header[4:8])

	if mapOffset >= uint32(in.Size()) {
		return nil, errors.New("invalid map offset")
	}

	mapHeader := make([]byte, 30)
	if _, err := in.ReadAt(mapHeader, int64(mapOffset)); err != nil {
		return nil, err
	}

	typeListOffset := mapOffset + uint32(binary.BigEndian.Uint16(mapHeader[24:26]))

	typeListHeader := make([]byte, 2)
	if _, err := in.ReadAt(typeListHeader, int64(typeListOffset)); err != nil {
		return nil, err
	}

	numTypes := int(binary.BigEndian.Uint16(typeListHeader)) + 1

	var streams []api.Stream

	for i := 0; i < numTypes; i++ {
		typeInfo := make([]byte, 8)
		if _, err := in.ReadAt(typeInfo, int64(typeListOffset+2+uint32(i*8))); err != nil {
			return nil, err
		}

		resType := string(typeInfo[0:4])
		if resType == "sfnt" {
			numRes := int(binary.BigEndian.Uint16(typeInfo[4:6])) + 1
			refListOffset := typeListOffset + uint32(binary.BigEndian.Uint16(typeInfo[6:8]))

			for j := 0; j < numRes; j++ {
				refInfo := make([]byte, 12)
				if _, err := in.ReadAt(refInfo, int64(refListOffset+uint32(j*12))); err != nil {
					return nil, err
				}

				// The first byte of the 4-byte offset is attributes, so mask it out.
				resDataOffset := dataOffset + (binary.BigEndian.Uint32(refInfo[4:8]) & 0x00FFFFFF)

				lenBytes := make([]byte, 4)
				if _, err := in.ReadAt(lenBytes, int64(resDataOffset)); err != nil {
					return nil, err
				}

				resDataLen := binary.BigEndian.Uint32(lenBytes)

				fontData := make([]byte, resDataLen)
				if _, err := in.ReadAt(fontData, int64(resDataOffset+4)); err != nil {
					return nil, err
				}

				streams = append(streams, core.NewMemoryStream(fontData))
			}
		}
	}

	if len(streams) == 0 {
		return nil, errors.New("no 'sfnt' resources found")
	}

	return streams, nil
}

// ExtractWindowsFNT extracts '.fnt' resources from Windows NE and PE files.
func ExtractWindowsFNT(in api.Stream) ([]api.Stream, error) {
	if in.Size() < 64 {
		return nil, errors.New("stream too small for DOS header")
	}

	mzHeader := make([]byte, 64)
	if _, err := in.ReadAt(mzHeader, 0); err != nil {
		return nil, err
	}

	if mzHeader[0] != 'M' || mzHeader[1] != 'Z' {
		return nil, errors.New("invalid MZ signature")
	}

	e_lfanew := binary.LittleEndian.Uint32(mzHeader[0x3C:0x40])

	if int64(e_lfanew)+4 > in.Size() {
		return nil, errors.New("invalid e_lfanew offset")
	}

	magic := make([]byte, 4)
	if _, err := in.ReadAt(magic, int64(e_lfanew)); err != nil {
		return nil, err
	}

	if magic[0] == 'N' && magic[1] == 'E' {
		return extractNE(in, e_lfanew)
	} else if magic[0] == 'P' && magic[1] == 'E' && magic[2] == 0 && magic[3] == 0 {
		return extractPE(in, e_lfanew)
	}

	return nil, errors.New("unsupported executable format")
}

func extractNE(in api.Stream, neOffset uint32) ([]api.Stream, error) {
	neHeader := make([]byte, 64)
	if _, err := in.ReadAt(neHeader, int64(neOffset)); err != nil {
		return nil, err
	}

	rsOffset := binary.LittleEndian.Uint16(neHeader[0x24:0x26])
	rsBase := neOffset + uint32(rsOffset)

	shiftBytes := make([]byte, 2)
	if _, err := in.ReadAt(shiftBytes, int64(rsBase)); err != nil {
		return nil, err
	}
	alignShift := binary.LittleEndian.Uint16(shiftBytes)

	currOffset := rsBase + 2
	var streams []api.Stream

	for {
		typeInfo := make([]byte, 8)
		if _, err := in.ReadAt(typeInfo, int64(currOffset)); err != nil {
			return nil, err
		}

		typeID := binary.LittleEndian.Uint16(typeInfo[0:2])
		if typeID == 0 {
			break // End of resource table
		}

		resCount := binary.LittleEndian.Uint16(typeInfo[2:4])
		currOffset += 8

		// Check if it's RT_FONT (type 8, but integer types have 0x8000 set in NE)
		// Or RT_FONTDIR (type 7)
		isFont := typeID == 0x8008 || typeID == 0x0008 || typeID == 0x8007 || typeID == 0x0007

		for i := 0; i < int(resCount); i++ {
			resInfo := make([]byte, 12)
			if _, err := in.ReadAt(resInfo, int64(currOffset)); err != nil {
				return nil, err
			}
			currOffset += 12

			if isFont {
				dataOffset := uint32(binary.LittleEndian.Uint16(resInfo[0:2])) << alignShift
				dataLen := uint32(binary.LittleEndian.Uint16(resInfo[2:4])) << alignShift

				fontData := make([]byte, dataLen)
				if _, err := in.ReadAt(fontData, int64(dataOffset)); err != nil {
					return nil, err
				}
				streams = append(streams, core.NewMemoryStream(fontData))
			}
		}
	}

	if len(streams) == 0 {
		return nil, errors.New("no RT_FONT resources found in NE")
	}

	return streams, nil
}

type section struct {
	VirtualAddress   uint32
	VirtualSize      uint32
	SizeOfRawData    uint32
	PointerToRawData uint32
}

func extractPE(in api.Stream, peOffset uint32) ([]api.Stream, error) {
	coffHeader := make([]byte, 20)
	if _, err := in.ReadAt(coffHeader, int64(peOffset+4)); err != nil {
		return nil, err
	}

	numSections := binary.LittleEndian.Uint16(coffHeader[2:4])
	optHeaderSize := binary.LittleEndian.Uint16(coffHeader[16:18])

	optHeader := make([]byte, optHeaderSize)
	if _, err := in.ReadAt(optHeader, int64(peOffset+24)); err != nil {
		return nil, err
	}

	magic := binary.LittleEndian.Uint16(optHeader[0:2])
	var numRvaAndSizes uint32
	var dirOffset int

	if magic == 0x10B { // PE32
		numRvaAndSizes = binary.LittleEndian.Uint32(optHeader[92:96])
		dirOffset = 96
	} else if magic == 0x20B { // PE32+
		numRvaAndSizes = binary.LittleEndian.Uint32(optHeader[108:112])
		dirOffset = 112
	} else {
		return nil, errors.New("unsupported PE optional header magic")
	}

	if numRvaAndSizes < 3 {
		return nil, errors.New("no resource directory in PE")
	}

	resDirRVA := binary.LittleEndian.Uint32(optHeader[dirOffset+16 : dirOffset+20])
	if resDirRVA == 0 {
		return nil, errors.New("resource directory RVA is 0")
	}

	sectionHeadersOffset := peOffset + 24 + uint32(optHeaderSize)
	sections := make([]section, numSections)

	for i := 0; i < int(numSections); i++ {
		secHdr := make([]byte, 40)
		if _, err := in.ReadAt(secHdr, int64(sectionHeadersOffset+uint32(i*40))); err != nil {
			return nil, err
		}
		sections[i] = section{
			VirtualSize:      binary.LittleEndian.Uint32(secHdr[8:12]),
			VirtualAddress:   binary.LittleEndian.Uint32(secHdr[12:16]),
			SizeOfRawData:    binary.LittleEndian.Uint32(secHdr[16:20]),
			PointerToRawData: binary.LittleEndian.Uint32(secHdr[20:24]),
		}
	}

	rvaToOffset := func(rva uint32) (uint32, error) {
		for _, sec := range sections {
			vSize := sec.VirtualSize
			if vSize == 0 {
				vSize = sec.SizeOfRawData
			}
			if rva >= sec.VirtualAddress && rva < sec.VirtualAddress+vSize {
				if rva-sec.VirtualAddress < sec.SizeOfRawData {
					return sec.PointerToRawData + (rva - sec.VirtualAddress), nil
				}
				return 0, fmt.Errorf("RVA points to uninitialized data: %x", rva)
			}
		}
		return 0, fmt.Errorf("RVA not found in sections: %x", rva)
	}

	resDirOffset, err := rvaToOffset(resDirRVA)
	if err != nil {
		return nil, err
	}

	var streams []api.Stream
	rootTable := make([]byte, 16)
	if _, err := in.ReadAt(rootTable, int64(resDirOffset)); err != nil {
		return nil, err
	}
	numNamed := int(binary.LittleEndian.Uint16(rootTable[12:14]))
	numID := int(binary.LittleEndian.Uint16(rootTable[14:16]))

	for i := 0; i < numNamed+numID; i++ {
		entry := make([]byte, 8)
		if _, err := in.ReadAt(entry, int64(resDirOffset+16+uint32(i*8))); err != nil {
			return nil, err
		}

		nameOrID := binary.LittleEndian.Uint32(entry[0:4])
		offsetToData := binary.LittleEndian.Uint32(entry[4:8])

		// RT_FONT is 8, RT_FONTDIR is 7
		if nameOrID == 8 || nameOrID == 7 {
			if offsetToData&0x80000000 == 0 {
				continue
			}

			typeDirOffset := resDirOffset + (offsetToData & 0x7FFFFFFF)
			typeTable := make([]byte, 16)
			if _, err := in.ReadAt(typeTable, int64(typeDirOffset)); err != nil {
				return nil, err
			}
			numTypeNamed := int(binary.LittleEndian.Uint16(typeTable[12:14]))
			numTypeID := int(binary.LittleEndian.Uint16(typeTable[14:16]))

			for j := 0; j < numTypeNamed+numTypeID; j++ {
				typeEntry := make([]byte, 8)
				if _, err := in.ReadAt(typeEntry, int64(typeDirOffset+16+uint32(j*8))); err != nil {
					return nil, err
				}

				typeOffset := binary.LittleEndian.Uint32(typeEntry[4:8])
				if typeOffset&0x80000000 == 0 {
					continue
				}

				langDirOffset := resDirOffset + (typeOffset & 0x7FFFFFFF)
				langTable := make([]byte, 16)
				if _, err := in.ReadAt(langTable, int64(langDirOffset)); err != nil {
					return nil, err
				}
				numLangNamed := int(binary.LittleEndian.Uint16(langTable[12:14]))
				numLangID := int(binary.LittleEndian.Uint16(langTable[14:16]))

				for k := 0; k < numLangNamed+numLangID; k++ {
					langEntry := make([]byte, 8)
					if _, err := in.ReadAt(langEntry, int64(langDirOffset+16+uint32(k*8))); err != nil {
						return nil, err
					}

					dataEntryOffset := binary.LittleEndian.Uint32(langEntry[4:8])
					if dataEntryOffset&0x80000000 != 0 {
						continue
					}

					actualDataEntryOffset := resDirOffset + dataEntryOffset
					dataEntry := make([]byte, 16)
					if _, err := in.ReadAt(dataEntry, int64(actualDataEntryOffset)); err != nil {
						return nil, err
					}

					dataRVA := binary.LittleEndian.Uint32(dataEntry[0:4])
					dataSize := binary.LittleEndian.Uint32(dataEntry[4:8])

					dataFileOffset, err := rvaToOffset(dataRVA)
					if err != nil {
						return nil, err
					}

					fontData := make([]byte, dataSize)
					if _, err := in.ReadAt(fontData, int64(dataFileOffset)); err != nil {
						return nil, err
					}

					streams = append(streams, core.NewMemoryStream(fontData))
				}
			}
		}
	}

	if len(streams) == 0 {
		return nil, errors.New("no RT_FONT resources found in PE")
	}

	return streams, nil
}
