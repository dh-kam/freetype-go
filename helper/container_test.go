package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func buildMockDFONT() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint32(16))
	binary.Write(buf, binary.BigEndian, uint32(36))
	binary.Write(buf, binary.BigEndian, uint32(20))
	binary.Write(buf, binary.BigEndian, uint32(50))

	binary.Write(buf, binary.BigEndian, uint32(16))
	buf.WriteString("mock_sfnt_data__")

	buf.Write(make([]byte, 24))
	binary.Write(buf, binary.BigEndian, uint16(28))
	binary.Write(buf, binary.BigEndian, uint16(50))

	binary.Write(buf, binary.BigEndian, uint16(0))
	buf.WriteString("sfnt")
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint16(10))

	binary.Write(buf, binary.BigEndian, uint16(128))
	binary.Write(buf, binary.BigEndian, uint16(0))
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00})
	binary.Write(buf, binary.BigEndian, uint32(0))

	return buf.Bytes()
}

func TestExtractMacDFONT(t *testing.T) {
	data := buildMockDFONT()
	stream := core.NewMemoryStream(data)

	streams, err := ExtractMacDFONT(stream)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if len(streams) != 1 {
		t.Fatalf("Expected 1 stream, got %d", len(streams))
	}

	resData := make([]byte, 16)
	streams[0].ReadAt(resData, 0)
	if string(resData) != "mock_sfnt_data__" {
		t.Errorf("Unexpected resource data: %s", string(resData))
	}
}

func buildMockNE() []byte {
	buf := make([]byte, 128)

	buf[0] = 'M'
	buf[1] = 'Z'
	binary.LittleEndian.PutUint32(buf[0x3C:0x40], 64)

	buf[64] = 'N'
	buf[65] = 'E'
	binary.LittleEndian.PutUint16(buf[64+0x24:64+0x26], 16)

	binary.LittleEndian.PutUint16(buf[80:82], 0)

	binary.LittleEndian.PutUint16(buf[82:84], 0x8008)
	binary.LittleEndian.PutUint16(buf[84:86], 1)

	binary.LittleEndian.PutUint16(buf[90:92], 110)
	binary.LittleEndian.PutUint16(buf[92:94], 10)

	binary.LittleEndian.PutUint16(buf[102:104], 0)

	copy(buf[110:120], []byte("fontdata__"))

	return buf
}

func TestExtractWindowsFNT_NE(t *testing.T) {
	data := buildMockNE()
	stream := core.NewMemoryStream(data)

	streams, err := ExtractWindowsFNT(stream)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if len(streams) != 1 {
		t.Fatalf("Expected 1 stream, got %d", len(streams))
	}

	resData := make([]byte, 10)
	streams[0].ReadAt(resData, 0)
	if string(resData) != "fontdata__" {
		t.Errorf("Unexpected resource data: %s", string(resData))
	}
}

func buildMockPE() []byte {
	buf := make([]byte, 1024)

	buf[0] = 'M'
	buf[1] = 'Z'
	binary.LittleEndian.PutUint32(buf[0x3C:0x40], 64)

	buf[64] = 'P'
	buf[65] = 'E'
	buf[66] = 0
	buf[67] = 0

	binary.LittleEndian.PutUint16(buf[68:70], 0x014C) // Machine
	binary.LittleEndian.PutUint16(buf[70:72], 1)      // 1 section
	binary.LittleEndian.PutUint16(buf[84:86], 224)

	binary.LittleEndian.PutUint16(buf[88:90], 0x10B)
	binary.LittleEndian.PutUint32(buf[180:184], 16)
	binary.LittleEndian.PutUint32(buf[200:204], 0x1000)

	secOffset := 88 + 224
	binary.LittleEndian.PutUint32(buf[secOffset+8:secOffset+12], 0x1000)
	binary.LittleEndian.PutUint32(buf[secOffset+12:secOffset+16], 0x1000)
	binary.LittleEndian.PutUint32(buf[secOffset+16:secOffset+20], 0x1000)
	binary.LittleEndian.PutUint32(buf[secOffset+20:secOffset+24], 512)

	resOffset := 512
	binary.LittleEndian.PutUint16(buf[resOffset+14:resOffset+16], 1)

	binary.LittleEndian.PutUint32(buf[resOffset+16:resOffset+20], 8)
	binary.LittleEndian.PutUint32(buf[resOffset+20:resOffset+24], 0x80000018)

	typeOffset := resOffset + 24
	binary.LittleEndian.PutUint16(buf[typeOffset+14:typeOffset+16], 1)
	binary.LittleEndian.PutUint32(buf[typeOffset+16:typeOffset+20], 1)
	binary.LittleEndian.PutUint32(buf[typeOffset+20:typeOffset+24], 0x80000030)

	langOffset := resOffset + 48
	binary.LittleEndian.PutUint16(buf[langOffset+14:langOffset+16], 1)
	binary.LittleEndian.PutUint32(buf[langOffset+16:langOffset+20], 0x409)
	binary.LittleEndian.PutUint32(buf[langOffset+20:langOffset+24], 0x00000048)

	dataEntryOffset := resOffset + 72
	binary.LittleEndian.PutUint32(buf[dataEntryOffset:dataEntryOffset+4], 0x1058)
	binary.LittleEndian.PutUint32(buf[dataEntryOffset+4:dataEntryOffset+8], 10)

	copy(buf[600:610], []byte("pefontdata"))

	return buf
}

func TestExtractWindowsFNT_PE(t *testing.T) {
	data := buildMockPE()
	stream := core.NewMemoryStream(data)

	streams, err := ExtractWindowsFNT(stream)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if len(streams) != 1 {
		t.Fatalf("Expected 1 stream, got %d", len(streams))
	}

	resData := make([]byte, 10)
	streams[0].ReadAt(resData, 0)
	if string(resData) != "pefontdata" {
		t.Errorf("Unexpected resource data: %s", string(resData))
	}
}
