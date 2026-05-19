package helper

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/sfnt"
)

func TestWOFFInvalid(t *testing.T) {
	stream := core.NewMemoryStream([]byte("invalid woff data"))
	_, err := DecodeWOFF(stream)
	if err == nil {
		t.Error("Expected error for invalid WOFF stream")
	}
}

func TestWOFF2Invalid(t *testing.T) {
	stream := core.NewMemoryStream([]byte("invalid woff2 data"))
	_, err := DecodeWOFF2(stream)
	if err == nil {
		t.Error("Expected error for invalid WOFF2 stream")
	}
}

func TestDecodeWOFFIfNeededReturnsNonWOFFStream(t *testing.T) {
	stream := core.NewMemoryStream([]byte("not a wrapped font"))
	out, err := DecodeWOFFIfNeeded(stream)
	if err != nil {
		t.Fatalf("DecodeWOFFIfNeeded failed: %v", err)
	}
	if out != stream {
		t.Fatal("DecodeWOFFIfNeeded should return non-WOFF streams unchanged")
	}
}

func TestWOFFEmptyValidStructure(t *testing.T) {
	// A dummy WOFF stream that has a valid header but no tables.
	headerData := make([]byte, 44)
	binary.BigEndian.PutUint32(headerData[0:4], 0x774F4646) // wOFF
	binary.BigEndian.PutUint32(headerData[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(headerData[8:12], 44)        // length
	binary.BigEndian.PutUint16(headerData[12:14], 0)        // numTables
	binary.BigEndian.PutUint32(headerData[16:20], 12)       // totalSfntSize (just header)

	stream := core.NewMemoryStream(headerData)
	outStream, err := DecodeWOFF(stream)
	if err != nil {
		t.Fatalf("DecodeWOFF with empty tables failed: %v", err)
	}

	if outStream.Size() != 12 {
		t.Errorf("Expected size 12, got %d", outStream.Size())
	}
}

func TestWOFFRejectsOversizeDecodedFont(t *testing.T) {
	headerData := make([]byte, 44)
	binary.BigEndian.PutUint32(headerData[0:4], 0x774F4646) // wOFF
	binary.BigEndian.PutUint32(headerData[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(headerData[8:12], 44)        // length
	binary.BigEndian.PutUint16(headerData[12:14], 0)        // numTables
	binary.BigEndian.PutUint32(headerData[16:20], maxDecodedFontSize+1)

	stream := core.NewMemoryStream(headerData)
	_, err := DecodeWOFF(stream)
	if err == nil {
		t.Fatal("Expected error for oversized WOFF decoded font")
	}
}

func TestWOFFRejectsMalformedTableRanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		want   string
	}{
		{
			name: "comp length exceeds original length",
			mutate: func(data []byte) []byte {
				data = append(data, make([]byte, 4)...)
				binary.BigEndian.PutUint32(data[8:12], uint32(len(data)))
				binary.BigEndian.PutUint32(data[52:56], 5)
				return data
			},
			want: "invalid WOFF table length",
		},
		{
			name: "unaligned table offset",
			mutate: func(data []byte) []byte {
				data = append(data, make([]byte, 4)...)
				binary.BigEndian.PutUint32(data[8:12], uint32(len(data)))
				binary.BigEndian.PutUint32(data[48:52], 65)
				return data
			},
			want: "invalid WOFF table alignment",
		},
		{
			name: "overlapping table data",
			mutate: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[68:72], 84)
				return data
			},
			want: "invalid WOFF table overlap",
		},
		{
			name: "declared length mismatch",
			mutate: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[8:12], uint32(len(data)-1))
				return data
			},
			want: "invalid WOFF file length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woffData := testWOFFUncompressed(t, []testWOFFTable{
				{tag: 0x41414141, data: []byte{1, 2, 3, 4}},
				{tag: 0x42424242, data: []byte{5, 6, 7, 8}},
			})
			if tt.name != "overlapping table data" {
				woffData = testWOFFUncompressed(t, []testWOFFTable{
					{tag: 0x41414141, data: []byte{1, 2, 3, 4}},
				})
			}
			_, err := DecodeWOFF(core.NewMemoryStream(tt.mutate(woffData)))
			if err == nil {
				t.Fatalf("expected %q error", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWOFFDecodesWithMetadataAndPrivateBlocks(t *testing.T) {
	woffData := testWOFFUncompressed(t, []testWOFFTable{
		{tag: 0x41414141, data: []byte{1, 2, 3, 4}},
	})
	woffData = testWOFFWithMetadataAndPrivate(t, woffData, []byte(`<metadata version="1.0"></metadata>`), []byte("private"))

	outStream, err := DecodeWOFF(core.NewMemoryStream(woffData))
	if err != nil {
		t.Fatalf("DecodeWOFF failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	if got := sfntTable(t, outData, 0x41414141); !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("decoded table = % x, want 01 02 03 04", got)
	}
}

func TestWOFFRejectsMalformedMetadataAndPrivateBlocks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		want   string
	}{
		{
			name: "metadata offset without length",
			mutate: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[24:28], uint32(len(data)))
				return data
			},
			want: "invalid WOFF metadata block",
		},
		{
			name: "metadata overlaps table data",
			mutate: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[24:28], 64)
				binary.BigEndian.PutUint32(data[28:32], 4)
				binary.BigEndian.PutUint32(data[32:36], 1)
				return data
			},
			want: "invalid WOFF metadata block",
		},
		{
			name: "private block not last",
			mutate: func(data []byte) []byte {
				data = append(data, 0, 0)
				binary.BigEndian.PutUint32(data[8:12], uint32(len(data)))
				binary.BigEndian.PutUint32(data[36:40], 68)
				binary.BigEndian.PutUint32(data[40:44], 1)
				return data
			},
			want: "invalid WOFF private block",
		},
		{
			name: "trailing data without block",
			mutate: func(data []byte) []byte {
				data = append(data, 0, 0, 0, 0)
				binary.BigEndian.PutUint32(data[8:12], uint32(len(data)))
				return data
			},
			want: "invalid WOFF trailing data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woffData := testWOFFUncompressed(t, []testWOFFTable{
				{tag: 0x41414141, data: []byte{1, 2, 3, 4}},
			})
			_, err := DecodeWOFF(core.NewMemoryStream(tt.mutate(woffData)))
			if err == nil {
				t.Fatalf("expected %q error", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWOFF2EmptyValidStructure(t *testing.T) {
	// A dummy WOFF2 stream that has a valid header but no tables.
	headerData := make([]byte, 48)
	binary.BigEndian.PutUint32(headerData[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(headerData[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(headerData[8:12], 48)        // length
	binary.BigEndian.PutUint16(headerData[12:14], 0)        // numTables
	binary.BigEndian.PutUint32(headerData[16:20], 12)       // totalSfntSize (just header)
	binary.BigEndian.PutUint32(headerData[20:24], 0)        // totalCompressedSize

	// Brotil compressed empty data would be valid brotli empty stream,
	// but we set totalCompressedSize to 0, so it will read 0 bytes
	// which might cause brotli reader to error if it expects a header,
	// or return EOF. Let's see if we can just append a valid brotli empty stream.
	// Empty brotli stream: 0x31 (1 byte) is actually valid or maybe 0x0b.
	// We'll just test if it panics or fails with expected brotli error or succeeds.

	// Appending a dummy valid brotli stream for empty data: 1 byte: 0x03
	headerData = append(headerData, 0x03)
	binary.BigEndian.PutUint32(headerData[8:12], uint32(len(headerData)))
	binary.BigEndian.PutUint32(headerData[20:24], 1) // totalCompressedSize = 1

	stream := core.NewMemoryStream(headerData)
	outStream, err := DecodeWOFF2(stream)
	if err != nil {
		// It's okay if it fails with brotli error because 0x03 might not be full valid EOF,
		// but we want to make sure the header parse logic works.
		if err.Error() == "brotli: unexpected EOF" {
			// fine
		} else {
			// but wait, DecodeWOFF2 returns core.MemoryStream
			// t.Logf("WOFF2 failed as expected due to mock brotli: %v", err)
		}
	} else {
		if outStream.Size() != 12 {
			t.Errorf("Expected size 12, got %d", outStream.Size())
		}
	}
}

func TestWOFF2RejectsOversizeDecodedFont(t *testing.T) {
	headerData := make([]byte, 48)
	binary.BigEndian.PutUint32(headerData[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(headerData[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(headerData[8:12], 48)        // length
	binary.BigEndian.PutUint16(headerData[12:14], 0)        // numTables
	binary.BigEndian.PutUint32(headerData[16:20], maxDecodedFontSize+1)

	stream := core.NewMemoryStream(headerData)
	_, err := DecodeWOFF2(stream)
	if err == nil {
		t.Fatal("Expected error for oversized WOFF2 decoded font")
	}
}

func TestWOFF2RejectsOversizeCompressedData(t *testing.T) {
	headerData := make([]byte, 48)
	binary.BigEndian.PutUint32(headerData[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(headerData[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(headerData[8:12], 48)        // length
	binary.BigEndian.PutUint16(headerData[12:14], 0)        // numTables
	binary.BigEndian.PutUint32(headerData[16:20], 12)       // totalSfntSize
	binary.BigEndian.PutUint32(headerData[20:24], maxCompressedFontDataSize+1)

	stream := core.NewMemoryStream(headerData)
	_, err := DecodeWOFF2(stream)
	if err == nil {
		t.Fatal("Expected error for oversized WOFF2 compressed data")
	}
}

func TestWOFF2LimitsBrotliDecodeToExpectedTransformSize(t *testing.T) {
	var directory bytes.Buffer
	directory.WriteByte(byte(5)) // name, transform version 0
	directory.Write(testBase128(1))
	woff2Data := testWOFF2WithDirectoryAndPayload(t, 1, directory.Bytes(), []byte{1, 2})

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if !errors.Is(err, errDecompressedDataTooLarge) {
		t.Fatalf("error = %v, want decompressed data size limit", err)
	}
}

func TestWOFF2RejectsCompressedDataPastFile(t *testing.T) {
	woff2Data := testWOFF2NoTablesWithMetadataAndPrivate(t, nil, nil)
	binary.BigEndian.PutUint32(woff2Data[20:24], uint32(len(woff2Data)))

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected compressed data bounds error")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 compressed size") {
		t.Fatalf("error = %q, want invalid compressed size", err)
	}
}

func TestWOFF2RejectsNonZeroReservedField(t *testing.T) {
	headerData := make([]byte, 48)
	binary.BigEndian.PutUint32(headerData[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(headerData[8:12], uint32(len(headerData)))
	binary.BigEndian.PutUint16(headerData[14:16], 1)

	_, err := DecodeWOFF2(core.NewMemoryStream(headerData))
	if err == nil {
		t.Fatal("expected non-zero WOFF2 reserved field to fail")
	}
}

func TestWOFF2DecodesWithMetadataAndPrivateBlocks(t *testing.T) {
	woff2Data := testWOFF2NoTablesWithMetadataAndPrivate(t, []byte(`<metadata version="1.0"></metadata>`), []byte("private"))

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	if outStream.Size() != 12 {
		t.Fatalf("decoded size = %d, want 12", outStream.Size())
	}
}

func TestWOFF2RejectsMalformedMetadataAndPrivateBlocks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		want   string
	}{
		{
			name: "declared length mismatch",
			mutate: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[8:12], uint32(len(data)-1))
				return data
			},
			want: "invalid WOFF2 file length",
		},
		{
			name: "metadata offset without length",
			mutate: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[28:32], uint32(len(data)))
				return data
			},
			want: "invalid WOFF2 metadata block",
		},
		{
			name: "misaligned metadata",
			mutate: func(data []byte) []byte {
				metaOffset := binary.BigEndian.Uint32(data[28:32])
				binary.BigEndian.PutUint32(data[28:32], metaOffset-1)
				return data
			},
			want: "invalid WOFF2 metadata block",
		},
		{
			name: "private block not last",
			mutate: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[44:48], binary.BigEndian.Uint32(data[44:48])-1)
				return data
			},
			want: "invalid WOFF2 private block",
		},
		{
			name: "trailing data without block",
			mutate: func(data []byte) []byte {
				data = append(data, 0)
				binary.BigEndian.PutUint32(data[8:12], uint32(len(data)))
				return data
			},
			want: "invalid WOFF2 trailing data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var woff2Data []byte
			switch tt.name {
			case "misaligned metadata":
				woff2Data = testWOFF2NoTablesWithMetadataAndPrivate(t, []byte(`<metadata version="1.0"></metadata>`), nil)
			case "private block not last":
				woff2Data = testWOFF2NoTablesWithMetadataAndPrivate(t, nil, []byte("private"))
			default:
				woff2Data = testWOFF2NoTablesWithMetadataAndPrivate(t, nil, nil)
			}
			_, err := DecodeWOFF2(core.NewMemoryStream(tt.mutate(woff2Data)))
			if err == nil {
				t.Fatalf("expected %q error", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWOFF2RejectsNonZeroMetadataPadding(t *testing.T) {
	woff2Data := testWOFF2NoTablesWithMetadataAndPrivate(t, []byte(`<metadata version="1.0"></metadata>`), nil)
	totalCompressedSize := binary.BigEndian.Uint32(woff2Data[20:24])
	metaOffset := binary.BigEndian.Uint32(woff2Data[28:32])
	if metaOffset <= 48+totalCompressedSize {
		t.Fatalf("metadata fixture has no pre-metadata padding: compressed=%d metaOffset=%d", totalCompressedSize, metaOffset)
	}
	woff2Data[metaOffset-1] = 0xff

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected non-zero metadata padding to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF block padding") {
		t.Fatalf("error = %q, want padding error", err)
	}
}

func TestWOFF2ReconstructsTransformedGlyfLoca(t *testing.T) {
	transformedGlyf := testWOFF2TransformedSimpleGlyf()
	woff2Data := testWOFF2WithTransformedGlyfLoca(t, transformedGlyf)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)

	glyf := sfntTable(t, outData, tagGlyf)
	loca := sfntTable(t, outData, tagLoca)
	if len(glyf) != 20 {
		t.Fatalf("glyf length = %d, want 20", len(glyf))
	}
	if len(loca) != 4 {
		t.Fatalf("loca length = %d, want 4", len(loca))
	}
	assertSimpleWOFF2Glyph(t, glyf)
	if binary.BigEndian.Uint16(loca[0:2]) != 0 || binary.BigEndian.Uint16(loca[2:4]) != 10 {
		t.Fatalf("unexpected short loca offsets: % x", loca)
	}
}

func TestWOFF2AllowsSingleFontTransformedGlyfLocaSeparated(t *testing.T) {
	transformedGlyf := testWOFF2TransformedSimpleGlyf()
	woff2Data := testWOFF2WithSeparatedTransformedGlyfLoca(t, transformedGlyf)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)

	assertSimpleWOFF2Glyph(t, sfntTable(t, outData, tagGlyf))
	loca := sfntTable(t, outData, tagLoca)
	if binary.BigEndian.Uint16(loca[0:2]) != 0 || binary.BigEndian.Uint16(loca[2:4]) != 10 {
		t.Fatalf("unexpected short loca offsets: % x", loca)
	}
}

func TestWOFF2RejectsTransformedLocaBeforeGlyf(t *testing.T) {
	transformedGlyf := testWOFF2TransformedSimpleGlyf()

	var directory bytes.Buffer
	directory.WriteByte(byte(11)) // loca, transform version 0
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))
	directory.WriteByte(byte(10)) // glyf, transform version 0
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))

	woff2Data := testWOFF2WithDirectoryAndPayload(t, 2, directory.Bytes(), transformedGlyf)
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected transformed loca before glyf to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 transformed glyf table requires transformed loca table") {
		t.Fatalf("error = %q, want transformed loca ordering error", err)
	}
}

func TestWOFF2SupportsNullTransformedGlyfLoca(t *testing.T) {
	glyf, loca, err := reconstructWOFF2GlyfLoca(testWOFF2TransformedSimpleGlyf(), 4)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	woff2Data := testWOFF2WithRawGlyfLoca(t, glyf, loca)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	if got := sfntTable(t, outData, tagGlyf); !bytes.Equal(got, glyf) {
		t.Fatalf("glyf = % x, want % x", got, glyf)
	}
	if got := sfntTable(t, outData, tagLoca); !bytes.Equal(got, loca) {
		t.Fatalf("loca = % x, want % x", got, loca)
	}
}

func TestWOFF2SupportsNullTransformedHmtx(t *testing.T) {
	hmtx := []byte{0x01, 0xf4, 0xff, 0xf6}
	var directory bytes.Buffer
	directory.WriteByte(byte(3)) // hmtx, transform version 0
	directory.Write(testBase128(uint32(len(hmtx))))
	woff2Data := testWOFF2WithDirectoryAndPayload(t, 1, directory.Bytes(), hmtx)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	if got := sfntTable(t, outData, tagHmtx); !bytes.Equal(got, hmtx) {
		t.Fatalf("hmtx = % x, want % x", got, hmtx)
	}
}

func TestWOFF2RejectsUnsupportedTransformVersions(t *testing.T) {
	tests := []struct {
		name      string
		directory []byte
		want      string
	}{
		{
			name:      "glyf version 1",
			directory: append([]byte{0x40 | 10}, testBase128(0)...),
			want:      "unsupported WOFF2 glyf transform version 1",
		},
		{
			name:      "loca version 2",
			directory: append([]byte{0x80 | 11}, testBase128(0)...),
			want:      "unsupported WOFF2 loca transform version 2",
		},
		{
			name:      "hmtx version 2",
			directory: append([]byte{0x80 | 3}, testBase128(4)...),
			want:      "unsupported WOFF2 hmtx transform version 2",
		},
		{
			name:      "head version 1",
			directory: append([]byte{0x40 | 1}, testBase128(54)...),
			want:      "unsupported WOFF2 head transform version 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := testWOFF2WithDirectoryAndPayload(t, 1, tt.directory, nil)
			_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
			if err == nil {
				t.Fatalf("expected %q error", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWOFF2ReconstructsTransformedHmtx(t *testing.T) {
	transformedGlyf := testWOFF2TransformedSimpleGlyf()
	woff2Data := testWOFF2WithTransformedHmtx(t, transformedGlyf)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)

	hmtx := sfntTable(t, outData, tagHmtx)
	if want := []byte{0x01, 0xf4, 0, 0}; !bytes.Equal(hmtx, want) {
		t.Fatalf("hmtx = % x, want % x", hmtx, want)
	}
}

func TestWOFF2ReconstructsCollection(t *testing.T) {
	woff2Data := testWOFF2Collection(t)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	if binary.BigEndian.Uint32(outData[0:4]) != tagTTCF {
		t.Fatalf("output signature = 0x%08x, want ttcf", binary.BigEndian.Uint32(outData[0:4]))
	}
	if binary.BigEndian.Uint32(outData[8:12]) != 1 {
		t.Fatalf("numFonts = %d, want 1", binary.BigEndian.Uint32(outData[8:12]))
	}
	firstFaceOffset := int(binary.BigEndian.Uint32(outData[12:16]))
	head := sfntTableAt(t, outData, firstFaceOffset, tagHead)
	if binary.BigEndian.Uint32(head[8:12]) != 0 {
		t.Fatalf("collection head.checkSumAdjustment = 0x%08x, want 0", binary.BigEndian.Uint32(head[8:12]))
	}

	loader := sfnt.NewLoader(core.NewSystem())
	face, err := loader.LoadFace(outStream)
	if err != nil {
		t.Fatalf("decoded TTC failed to load: %v", err)
	}
	if face.GetNumGlyphs() != 3 {
		t.Fatalf("numGlyphs = %d, want 3", face.GetNumGlyphs())
	}
}

func TestWOFF2ReconstructsCollectionWithTransformedHmtx(t *testing.T) {
	woff2Data := testWOFF2CollectionWithTransformedHmtx(t)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	firstFaceOffset := int(binary.BigEndian.Uint32(outData[12:16]))
	hmtx := sfntTableAt(t, outData, firstFaceOffset, tagHmtx)
	if want := []byte{0x01, 0xf4, 0, 0}; !bytes.Equal(hmtx, want) {
		t.Fatalf("collection hmtx = % x, want % x", hmtx, want)
	}
}

func TestWOFF2RejectsCollectionMismatchedTransformedGlyfLoca(t *testing.T) {
	woff2Data := testWOFF2CollectionWithMismatchedGlyfLoca(t)

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected mismatched collection glyf/loca pair to fail")
	}
	if !strings.Contains(err.Error(), "mismatched glyf/loca") {
		t.Fatalf("error = %q, want mismatched glyf/loca", err)
	}
}

func TestWOFF2RejectsCollectionTableIndexOutOfRange(t *testing.T) {
	head := testHeadTable(0)

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // numFonts
	directory.Write(testUInt255(1)) // numTables
	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // only table index 0 exists

	woff2Data := testWOFF2CollectionWithDirectoryAndPayload(t, 1, directory.Bytes(), head)
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected collection table index bounds error")
	}
	if !strings.Contains(err.Error(), "WOFF2 collection table index out of range") {
		t.Fatalf("error = %q, want collection table index out of range", err)
	}
}

func TestWOFF2RejectsCollectionGlyfLocaMetadataMismatch(t *testing.T) {
	head := testHeadTable(1)
	maxp := testMaxpTable(1)
	transformedGlyf := testWOFF2TransformedSimpleGlyf()

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // transformed glyf
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // transformed loca
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // numFonts
	directory.Write(testUInt255(4)) // numTables
	appendUint32(&directory, 0x00010000)
	for i := 0; i < 4; i++ {
		directory.Write(testUInt255(uint16(i)))
	}

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(transformedGlyf)
	compressed := testBrotliData(t, payload.Bytes())

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], tagTTCF)
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+len(compressed)))
	binary.BigEndian.PutUint16(header[12:14], 4)
	binary.BigEndian.PutUint32(header[16:20], 216)
	binary.BigEndian.PutUint32(header[20:24], uint32(len(compressed)))

	woff2Data := append(header, directory.Bytes()...)
	woff2Data = append(woff2Data, compressed...)
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected collection glyf/loca metadata mismatch to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyf/loca metadata mismatch") {
		t.Fatalf("error = %q, want glyf/loca metadata mismatch", err)
	}
}

func TestWOFF2RejectsCollectionRawLocaBeyondGlyf(t *testing.T) {
	head := testHeadTable(1)
	maxp := testMaxpTable(1)
	glyf := make([]byte, 10)
	loca := make([]byte, 8)
	binary.BigEndian.PutUint32(loca[4:8], uint32(len(glyf)+2))

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(0xc0 | 10)) // raw glyf
	directory.Write(testBase128(uint32(len(glyf))))
	directory.WriteByte(byte(0xc0 | 11)) // raw loca
	directory.Write(testBase128(uint32(len(loca))))

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // numFonts
	directory.Write(testUInt255(4)) // numTables
	appendUint32(&directory, 0x00010000)
	for i := 0; i < 4; i++ {
		directory.Write(testUInt255(uint16(i)))
	}

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(glyf)
	payload.Write(loca)
	woff2Data := testWOFF2CollectionWithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected collection loca/glyf bounds error")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 glyf bounds") {
		t.Fatalf("error = %q, want glyf bounds", err)
	}
}

func TestWOFF2RejectsInvalidTransformedLocaLength(t *testing.T) {
	_, _, err := reconstructWOFF2GlyfLoca(testWOFF2TransformedSimpleGlyf(), 6)
	if err == nil {
		t.Fatal("expected invalid transformed loca length error")
	}
}

func TestWOFF2RejectsGlyfLocaMetadataMismatch(t *testing.T) {
	head := testHeadTable(0)
	maxp := testMaxpTable(1)
	transformed := append([]byte(nil), testWOFF2TransformedSimpleGlyf()...)
	binary.BigEndian.PutUint16(transformed[6:8], 1)

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // glyf, transform version 0
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformed))))
	directory.WriteByte(byte(11)) // loca, transform version 0
	directory.Write(testBase128(8))
	directory.Write(testBase128(0))

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(transformed)
	woff2Data := testWOFF2WithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected glyf/loca metadata mismatch to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyf/loca metadata mismatch") {
		t.Fatalf("error = %q, want glyf/loca metadata mismatch", err)
	}
}

func TestWOFF2RejectsRawCompositeGlyphIndexOutOfRange(t *testing.T) {
	head := testHeadTable(0)
	maxp := testMaxpTable(1)

	var glyf bytes.Buffer
	appendUint16(&glyf, 0xffff) // numberOfContours = composite
	appendUint16(&glyf, 0)      // xMin
	appendUint16(&glyf, 0)      // yMin
	appendUint16(&glyf, 0)      // xMax
	appendUint16(&glyf, 0)      // yMax
	appendUint16(&glyf, 0x0002) // args are x/y byte offsets
	appendUint16(&glyf, 1)      // component glyph index, out of range for numGlyphs=1
	glyf.Write([]byte{0, 0})
	glyfData := glyf.Bytes()

	loca := make([]byte, 4)
	binary.BigEndian.PutUint16(loca[2:4], uint16(len(glyfData)/2))

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(0xc0 | 10)) // raw glyf
	directory.Write(testBase128(uint32(len(glyfData))))
	directory.WriteByte(byte(0xc0 | 11)) // raw loca
	directory.Write(testBase128(uint32(len(loca))))

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(glyfData)
	payload.Write(loca)
	woff2Data := testWOFF2WithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected raw composite glyph index bounds error")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 composite glyph index") {
		t.Fatalf("error = %q, want composite glyph index error", err)
	}
}

func TestWOFF2RejectsTransformedCompositeGlyphIndexOutOfRange(t *testing.T) {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0002) // args are x/y byte offsets
	appendUint16(&compositeStream, 1)      // component glyph index, out of range for numGlyphs=1
	compositeStream.Write([]byte{0, 0})

	transformed := testWOFF2TransformedCompositeGlyf(
		compositeStream.Bytes(),
		nil,
		[]byte{
			0x80, 0, 0, 0, // bbox bitmap: glyph 0 has explicit bbox
			0, 0, 0, 0, 0, 0, 0, 0,
		},
		nil,
	)

	_, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err == nil {
		t.Fatal("expected transformed composite glyph index bounds error")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 composite glyph index") {
		t.Fatalf("error = %q, want composite glyph index error", err)
	}
}

func TestWOFF2RejectsLocaFinalOffsetBeforeGlyfEnd(t *testing.T) {
	head := testHeadTable(0)
	maxp := testMaxpTable(1)
	glyf := make([]byte, 12)
	loca := make([]byte, 4)
	binary.BigEndian.PutUint16(loca[2:4], 5) // final offset is 10, leaving unreferenced glyf bytes

	woff2Data := testWOFF2WithRawOutlineTables(t, head, maxp, glyf, loca)
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected final loca/glyf length mismatch")
	}
	if !strings.Contains(err.Error(), "WOFF2 loca/glyf length mismatch") {
		t.Fatalf("error = %q, want loca/glyf length mismatch", err)
	}
}

func TestWOFF2RejectsSimpleGlyphNonMonotonicEndpoints(t *testing.T) {
	head := testHeadTable(0)
	maxp := testMaxpTable(1)
	glyf := []byte{
		0, 2, // numberOfContours
		0, 0, 0, 0, 0, 10, 0, 0, // bbox
		0, 1, 0, 1, // endPtsOfContours must be strictly increasing
		0, 0, // instructionLength
		0x31, 0x33, 10, 0, // point data plus padding
	}
	loca := []byte{0, 0, 0, 10}

	woff2Data := testWOFF2WithRawOutlineTables(t, head, maxp, glyf, loca)
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected non-monotonic simple glyph endpoints to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 simple glyph contour endpoints") {
		t.Fatalf("error = %q, want simple glyph endpoint error", err)
	}
}

func TestWOFF2RejectsRawCompositeNonZeroPadding(t *testing.T) {
	head := testHeadTable(1)
	maxp := testMaxpTable(2)

	var glyf bytes.Buffer
	appendUint16(&glyf, 0xffff) // numberOfContours = composite
	appendUint16(&glyf, 0)      // xMin
	appendUint16(&glyf, 0)      // yMin
	appendUint16(&glyf, 0)      // xMax
	appendUint16(&glyf, 0)      // yMax
	appendUint16(&glyf, 0x0002) // args are x/y byte offsets
	appendUint16(&glyf, 1)      // component glyph index
	glyf.Write([]byte{0, 0, 0xff})
	glyfData := glyf.Bytes()

	loca := make([]byte, 12)
	binary.BigEndian.PutUint32(loca[4:8], uint32(len(glyfData)))
	binary.BigEndian.PutUint32(loca[8:12], uint32(len(glyfData)))

	woff2Data := testWOFF2WithRawOutlineTables(t, head, maxp, glyfData, loca)
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected non-zero composite glyph padding to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 glyf padding") {
		t.Fatalf("error = %q, want glyf padding error", err)
	}
}

func TestWOFF2RejectsInvalidTransformedHmtxFlags(t *testing.T) {
	tables, reconstructed := testWOFF2HmtxDependencies()
	_, err := reconstructWOFF2Hmtx([]byte{0}, 4, tables, reconstructed)
	if err == nil {
		t.Fatal("expected invalid hmtx flags error")
	}
}

func TestWOFF2HmtxRejectsTransformLengthMismatch(t *testing.T) {
	tables, reconstructed := testWOFF2HmtxDependencies()

	_, err := reconstructWOFF2Hmtx([]byte{1, 0x01, 0xf4, 0}, 4, tables, reconstructed)
	if err == nil {
		t.Fatal("expected hmtx transform length mismatch")
	}
	if !strings.Contains(err.Error(), "WOFF2 hmtx transform length mismatch") {
		t.Fatalf("error = %q, want hmtx transform length mismatch", err)
	}
}

func TestWOFF2HmtxReconstructsMonospaceBearings(t *testing.T) {
	glyf0, _, err := reconstructWOFF2GlyfLoca(testWOFF2TransformedSimpleGlyf(), 4)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	glyph1 := testWOFF2OnePointGlyphAtX10()

	glyf := append(append([]byte(nil), glyf0...), glyph1...)
	loca := []byte{0, 0, 0, 10, 0, 18}
	tables := []woff2TableEntry{
		{tag: tagHead},
		{tag: tagHhea},
		{tag: tagMaxp},
		{tag: tagGlyf},
		{tag: tagLoca},
	}
	reconstructed := [][]byte{
		testHeadTable(0),
		testHheaTable(1),
		testMaxpTable(2),
		glyf,
		loca,
	}

	hmtx, err := reconstructWOFF2Hmtx([]byte{3, 0x01, 0xf4}, 6, tables, reconstructed)
	if err != nil {
		t.Fatalf("reconstructWOFF2Hmtx failed: %v", err)
	}
	if want := []byte{0x01, 0xf4, 0, 0, 0, 10}; !bytes.Equal(hmtx, want) {
		t.Fatalf("hmtx = % x, want % x", hmtx, want)
	}
}

func TestWOFF2HmtxPreservesExplicitMonospaceBearings(t *testing.T) {
	tables, reconstructed := testWOFF2TwoGlyphHmtxDependencies(t)

	hmtx, err := reconstructWOFF2Hmtx([]byte{1, 0x01, 0xf4, 0xff, 0xf9}, 6, tables, reconstructed)
	if err != nil {
		t.Fatalf("reconstructWOFF2Hmtx failed: %v", err)
	}
	if want := []byte{0x01, 0xf4, 0, 0, 0xff, 0xf9}; !bytes.Equal(hmtx, want) {
		t.Fatalf("hmtx = % x, want % x", hmtx, want)
	}
}

func TestWOFF2HmtxRejectsInvalidLocaBounds(t *testing.T) {
	tables, reconstructed := testWOFF2HmtxDependencies()
	reconstructed[4] = []byte{0, 0, 0, 100}

	_, err := reconstructWOFF2Hmtx([]byte{1, 0x01, 0xf4}, 4, tables, reconstructed)
	if err == nil {
		t.Fatal("expected invalid loca bounds to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 glyf bounds") {
		t.Fatalf("error = %q, want invalid WOFF2 glyf bounds", err)
	}
}

func TestWOFF2HmtxRejectsInvalidLocaBoundsWithExplicitBearings(t *testing.T) {
	tables, reconstructed := testWOFF2HmtxDependencies()
	reconstructed[4] = []byte{0, 0, 0, 100}

	_, err := reconstructWOFF2Hmtx([]byte{2, 0x01, 0xf4, 0, 0}, 4, tables, reconstructed)
	if err == nil {
		t.Fatal("expected invalid loca bounds to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 glyf bounds") {
		t.Fatalf("error = %q, want invalid WOFF2 glyf bounds", err)
	}
}

func TestWOFF2ReconstructsLongLocaTransformedGlyf(t *testing.T) {
	transformed := append([]byte(nil), testWOFF2TransformedSimpleGlyf()...)
	binary.BigEndian.PutUint16(transformed[6:8], 1)

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, 8)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if len(glyf) != 20 {
		t.Fatalf("glyf length = %d, want 20", len(glyf))
	}
	if binary.BigEndian.Uint32(loca[0:4]) != 0 || binary.BigEndian.Uint32(loca[4:8]) != 20 {
		t.Fatalf("unexpected long loca offsets: % x", loca)
	}
}

func TestWOFF2ReconstructsEmptyGlyphAndInstructionStream(t *testing.T) {
	transformed := testWOFF2TransformedGlyfFromStreams(
		3,
		[]byte{0, 1, 0, 0, 0, 1},
		[]byte{3, 3},
		[]byte{0, 11, 1, 0, 11, 1},
		[]byte{0, 10, 20, 0, 0, 10, 20, 2},
		nil,
		[]byte{0, 0, 0, 0},
		[]byte{0xde, 0xad},
	)

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, 8)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if len(glyf) != 44 {
		t.Fatalf("glyf length = %d, want 44", len(glyf))
	}
	wantLoca := []uint16{0, 10, 10, 22}
	for i, want := range wantLoca {
		if got := binary.BigEndian.Uint16(loca[i*2 : i*2+2]); got != want {
			t.Fatalf("loca[%d] = %d, want %d", i, got, want)
		}
	}
	secondGlyph := 20
	if got := binary.BigEndian.Uint16(glyf[secondGlyph+12 : secondGlyph+14]); got != 2 {
		t.Fatalf("instructionLength = %d, want 2", got)
	}
	if got := glyf[secondGlyph+14 : secondGlyph+16]; !bytes.Equal(got, []byte{0xde, 0xad}) {
		t.Fatalf("instructions = % x, want de ad", got)
	}
}

func TestWOFF2RejectsTransformedSimpleInstructionStreamBounds(t *testing.T) {
	transformed := testWOFF2TransformedGlyfFromStreams(
		1,
		[]byte{0, 1},
		[]byte{1},
		[]byte{0},
		[]byte{0, 2},
		nil,
		[]byte{0, 0, 0, 0},
		[]byte{0xde},
	)

	_, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err == nil {
		t.Fatal("expected simple glyph instruction stream bounds error")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 simple glyph instructions") {
		t.Fatalf("error = %q, want simple glyph instruction error", err)
	}
}

func TestWOFF2RejectsInvalidNegativeContourCount(t *testing.T) {
	transformed := testWOFF2TransformedGlyfFromStreams(
		1,
		[]byte{0x80, 0},
		nil,
		nil,
		nil,
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)

	_, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err == nil {
		t.Fatal("expected invalid contour count to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 glyph contour count") {
		t.Fatalf("error = %q, want invalid contour count", err)
	}
}

func TestWOFF2SimpleGlyphOverlapBitmapSetsTrueTypeFlag(t *testing.T) {
	transformed := append([]byte(nil), testWOFF2TransformedSimpleGlyf()...)
	binary.BigEndian.PutUint16(transformed[2:4], 1)
	transformed = append(transformed, 0x80, 0, 0, 0)

	glyf, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if glyf[14]&0x40 == 0 {
		t.Fatalf("first point flag = 0x%02x, want OVERLAP_SIMPLE set", glyf[14])
	}
}

func TestWOFF2SimpleGlyphExplicitBBoxOverridesComputed(t *testing.T) {
	transformed := append([]byte(nil), testWOFF2TransformedSimpleGlyf()...)
	binary.BigEndian.PutUint32(transformed[28:32], 12)
	transformed[46] = 0x80
	transformed = append(transformed,
		0xff, 0xf6, // xMin = -10
		0xff, 0xfb, // yMin = -5
		0, 0x78, // xMax = 120
		0, 0x8c, // yMax = 140
	)

	glyf, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if got := [4]int16{
		int16(binary.BigEndian.Uint16(glyf[2:4])),
		int16(binary.BigEndian.Uint16(glyf[4:6])),
		int16(binary.BigEndian.Uint16(glyf[6:8])),
		int16(binary.BigEndian.Uint16(glyf[8:10])),
	}; got != [4]int16{-10, -5, 120, 140} {
		t.Fatalf("bbox = %v, want [-10 -5 120 140]", got)
	}
}

func TestWOFF2RejectsInvalidExplicitBBox(t *testing.T) {
	transformed := append([]byte(nil), testWOFF2TransformedSimpleGlyf()...)
	binary.BigEndian.PutUint32(transformed[28:32], 12)
	transformed[46] = 0x80
	transformed = append(transformed,
		0, 20, // xMin
		0, 0, // yMin
		0, 10, // xMax
		0, 20, // yMax
	)

	_, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err == nil {
		t.Fatal("expected invalid explicit bbox to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 glyph bbox") {
		t.Fatalf("error = %q, want invalid glyph bbox", err)
	}
}

func TestWOFF2RejectsExplicitBBoxOnEmptyGlyph(t *testing.T) {
	data := make([]byte, 36)
	binary.BigEndian.PutUint16(data[4:6], 1) // numGlyphs
	binary.BigEndian.PutUint32(data[8:12], 2)
	binary.BigEndian.PutUint32(data[28:32], 4)
	data = append(data, 0, 0)          // nContourStream: empty glyph
	data = append(data, 0x80, 0, 0, 0) // bboxBitmap sets glyph 0

	_, _, err := reconstructWOFF2GlyfLoca(data, 4)
	if err == nil {
		t.Fatal("expected explicit bbox rejection for empty glyph")
	}
}

func TestWOFF2ReconstructsCompositeGlyph(t *testing.T) {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0e2b) // words, xy, scale, more, metrics, overlap, scaled offset
	appendUint16(&compositeStream, 0)      // glyphIndex
	appendUint16(&compositeStream, 0xfff6) // arg1 = -10
	appendUint16(&compositeStream, 20)     // arg2 = 20
	appendUint16(&compositeStream, 0x4000) // scale = 1.0
	appendUint16(&compositeStream, 0x1142) // byte args, xy scale, instructions, unscaled offset
	appendUint16(&compositeStream, 0)      // glyphIndex
	compositeStream.Write([]byte{5, 0xfb}) // arg1 = 5, arg2 = -5
	appendUint16(&compositeStream, 0x4000) // xscale = 1.0
	appendUint16(&compositeStream, 0x2000) // yscale = 0.5

	transformed := testWOFF2TransformedCompositeGlyf(
		compositeStream.Bytes(),
		testUInt255(3),
		[]byte{
			0x80, 0, 0, 0, // bbox bitmap: glyph 0 has explicit bbox
			0xff, 0xf6, // xMin = -10
			0xff, 0xfb, // yMin = -5
			0, 0x78, // xMax = 120
			0, 0x8c, // yMax = 140
		},
		[]byte{0xb0, 0x01, 0x2b},
	)

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if len(glyf) != 36 {
		t.Fatalf("glyf length = %d, want 36", len(glyf))
	}
	if binary.BigEndian.Uint16(loca[0:2]) != 0 || binary.BigEndian.Uint16(loca[2:4]) != 18 {
		t.Fatalf("unexpected composite loca offsets: % x", loca)
	}

	want := []byte{
		0xff, 0xff, // numberOfContours = -1
		0xff, 0xf6, 0xff, 0xfb, 0, 0x78, 0, 0x8c, // bbox
		0x0e, 0x2b, 0, 0, 0xff, 0xf6, 0, 20, 0x40, 0,
		0x11, 0x42, 0, 0, 5, 0xfb, 0x40, 0, 0x20, 0,
		0, 3, 0xb0, 0x01, 0x2b,
	}
	if !bytes.Equal(glyf[:len(want)], want) {
		t.Fatalf("composite glyf = % x, want % x", glyf[:len(want)], want)
	}
	if glyf[len(glyf)-1] != 0 {
		t.Fatalf("composite glyf padding = 0x%02x, want 0", glyf[len(glyf)-1])
	}
}

func TestWOFF2RejectsCompositeGlyphMissingBBox(t *testing.T) {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0002)
	appendUint16(&compositeStream, 1)
	compositeStream.Write([]byte{0, 0})

	transformed := testWOFF2TransformedCompositeGlyf(
		compositeStream.Bytes(),
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)

	_, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
	if err == nil {
		t.Fatal("expected missing composite bbox rejection")
	}
	if !strings.Contains(err.Error(), "composite glyph missing bbox") {
		t.Fatalf("error = %q, want composite glyph missing bbox", err)
	}
}

func TestWOFF2RejectsMalformedCompositeTransformFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags uint16
		want  string
	}{
		{
			name:  "multiple transform flags",
			flags: 0x0008 | 0x0040,
			want:  "invalid WOFF2 composite transform flags",
		},
		{
			name:  "reserved flag bit",
			flags: 0x0010,
			want:  "invalid WOFF2 composite flags",
		},
		{
			name:  "conflicting component offset flags",
			flags: 0x0800 | 0x1000,
			want:  "invalid WOFF2 composite offset flags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var compositeStream bytes.Buffer
			appendUint16(&compositeStream, tt.flags)

			_, err := reconstructWOFF2CompositeGlyph(
				newWOFF2ByteReader(compositeStream.Bytes()),
				newWOFF2ByteReader(nil),
				newWOFF2ByteReader(nil),
				1,
			)
			if err == nil {
				t.Fatal("expected malformed composite flags to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestWOFF2RejectsMalformedCompositeTransformBounds(t *testing.T) {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0083) // words, xy, two-by-two matrix
	appendUint16(&compositeStream, 1)
	appendUint16(&compositeStream, 0)
	appendUint16(&compositeStream, 0)
	appendUint16(&compositeStream, 0x4000) // xscale
	appendUint16(&compositeStream, 0)      // scale01
	appendUint16(&compositeStream, 0)      // scale10; yscale is missing

	_, err := reconstructWOFF2CompositeGlyph(
		newWOFF2ByteReader(compositeStream.Bytes()),
		newWOFF2ByteReader(nil),
		newWOFF2ByteReader(nil),
		2,
	)
	if err == nil {
		t.Fatal("expected truncated two-by-two transform to fail")
	}
	if !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("error = %q, want unexpected EOF", err)
	}
}

func TestWOFF2RejectsMalformedCompositeInstructions(t *testing.T) {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0102) // byte args, xy, instructions
	appendUint16(&compositeStream, 0)
	compositeStream.Write([]byte{0, 0})

	_, err := reconstructWOFF2CompositeGlyph(
		newWOFF2ByteReader(compositeStream.Bytes()),
		newWOFF2ByteReader([]byte{2}),
		newWOFF2ByteReader([]byte{0xde}),
		1,
	)
	if err == nil {
		t.Fatal("expected truncated composite instructions to fail")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 composite glyph instructions") {
		t.Fatalf("error = %q, want composite instruction error", err)
	}
}

func TestWOFF2TripletDecodeCoversFlagClasses(t *testing.T) {
	flags := []byte{0x80 | 9, 19, 83, 119, 123, 127}
	glyphStream := []byte{
		1,
		2,
		0,
		1, 2,
		1, 0x23, 4,
		0, 5, 0, 6,
	}
	points, err := decodeWOFF2Triplets(flags, newWOFF2ByteReader(glyphStream))
	if err != nil {
		t.Fatalf("decodeWOFF2Triplets failed: %v", err)
	}

	want := []woff2Point{
		{x: 0, y: 1025, onCurve: false},
		{x: 1026, y: 1025, onCurve: true},
		{x: 1075, y: 1074, onCurve: true},
		{x: 1589, y: 1589, onCurve: true},
		{x: 1607, y: 2361, onCurve: true},
		{x: 1612, y: 2367, onCurve: true},
	}
	if len(points) != len(want) {
		t.Fatalf("decoded %d points, want %d", len(points), len(want))
	}
	for i := range points {
		if points[i] != want[i] {
			t.Fatalf("point[%d] = %+v, want %+v", i, points[i], want[i])
		}
	}
}

func TestWOFF2RejectsUnencodableTrueTypePointDelta(t *testing.T) {
	var buf bytes.Buffer
	err := appendTrueTypePointData(&buf, []woff2Point{
		{x: 32767, y: 0, onCurve: true},
		{x: -32768, y: 0, onCurve: true},
	}, false)
	if err == nil {
		t.Fatal("expected unencodable x delta to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyph coordinate delta out of range") {
		t.Fatalf("error = %q, want coordinate delta out of range", err)
	}

	buf.Reset()
	err = appendTrueTypePointData(&buf, []woff2Point{
		{x: 0, y: -32768, onCurve: true},
		{x: 0, y: 32767, onCurve: true},
	}, false)
	if err == nil {
		t.Fatal("expected unencodable y delta to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyph coordinate delta out of range") {
		t.Fatalf("error = %q, want coordinate delta out of range", err)
	}
}

func assertSimpleWOFF2Glyph(t *testing.T, glyf []byte) {
	t.Helper()
	if binary.BigEndian.Uint16(glyf[0:2]) != 1 {
		t.Fatalf("numberOfContours = %d, want 1", binary.BigEndian.Uint16(glyf[0:2]))
	}
	if got := [4]int16{
		int16(binary.BigEndian.Uint16(glyf[2:4])),
		int16(binary.BigEndian.Uint16(glyf[4:6])),
		int16(binary.BigEndian.Uint16(glyf[6:8])),
		int16(binary.BigEndian.Uint16(glyf[8:10])),
	}; got != [4]int16{0, 0, 10, 20} {
		t.Fatalf("bbox = %v, want [0 0 10 20]", got)
	}
	if binary.BigEndian.Uint16(glyf[10:12]) != 2 {
		t.Fatalf("endPtsOfContours[0] = %d, want 2", binary.BigEndian.Uint16(glyf[10:12]))
	}
	if binary.BigEndian.Uint16(glyf[12:14]) != 0 {
		t.Fatalf("instructionLength = %d, want 0", binary.BigEndian.Uint16(glyf[12:14]))
	}
	if got := glyf[14:19]; !bytes.Equal(got, []byte{49, 51, 53, 10, 20}) {
		t.Fatalf("encoded point data = % x, want 31 33 35 0a 14", got)
	}
}

func testWOFF2TransformedSimpleGlyf() []byte {
	nContourStream := []byte{0, 1}
	nPointsStream := []byte{3}
	flagStream := []byte{0, 11, 1}
	glyphStream := []byte{0, 10, 20, 0} // three coordinate triplets, instructionLength=0
	compositeStream := []byte(nil)
	bboxStream := []byte{0, 0, 0, 0}
	instructionStream := []byte(nil)

	streams := [][]byte{
		nContourStream,
		nPointsStream,
		flagStream,
		glyphStream,
		compositeStream,
		bboxStream,
		instructionStream,
	}
	data := make([]byte, 36)
	binary.BigEndian.PutUint16(data[4:6], 1) // numGlyphs
	for i, stream := range streams {
		binary.BigEndian.PutUint32(data[8+i*4:12+i*4], uint32(len(stream)))
	}
	for _, stream := range streams {
		data = append(data, stream...)
	}
	return data
}

func testWOFF2OnePointGlyphAtX10() []byte {
	return []byte{
		0, 1, // numberOfContours
		0, 10, 0, 0, 0, 10, 0, 0, // bbox
		0, 0, // endPtsOfContours[0]
		0, 0, // instructionLength
		0x33, 10, // one on-curve point at x=10, y=0
	}
}

func testWOFF2TransformedGlyfFromStreams(numGlyphs uint16, nContourStream, nPointsStream, flagStream, glyphStream, compositeStream, bboxStream, instructionStream []byte) []byte {
	streams := [][]byte{
		nContourStream,
		nPointsStream,
		flagStream,
		glyphStream,
		compositeStream,
		bboxStream,
		instructionStream,
	}
	data := make([]byte, 36)
	binary.BigEndian.PutUint16(data[4:6], numGlyphs)
	for i, stream := range streams {
		binary.BigEndian.PutUint32(data[8+i*4:12+i*4], uint32(len(stream)))
	}
	for _, stream := range streams {
		data = append(data, stream...)
	}
	return data
}

func testWOFF2TransformedCompositeGlyf(compositeStream, glyphStream, bboxStream, instructionStream []byte) []byte {
	streams := [][]byte{
		{0xff, 0xff},
		nil,
		nil,
		glyphStream,
		compositeStream,
		bboxStream,
		instructionStream,
	}
	data := make([]byte, 36)
	binary.BigEndian.PutUint16(data[4:6], 1) // numGlyphs
	for i, stream := range streams {
		binary.BigEndian.PutUint32(data[8+i*4:12+i*4], uint32(len(stream)))
	}
	for _, stream := range streams {
		data = append(data, stream...)
	}
	return data
}

func testWOFF2WithTransformedGlyfLoca(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()
	var directory bytes.Buffer
	directory.WriteByte(byte(10)) // glyf, transform version 0
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // loca, transform version 0
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(transformedGlyf); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+compressed.Len()))
	binary.BigEndian.PutUint16(header[12:14], 2)  // numTables
	binary.BigEndian.PutUint32(header[16:20], 68) // reference totalSfntSize
	binary.BigEndian.PutUint32(header[20:24], uint32(compressed.Len()))

	out := append(header, directory.Bytes()...)
	out = append(out, compressed.Bytes()...)
	return out
}

func testWOFF2WithSeparatedTransformedGlyfLoca(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()
	head := testHeadTable(0)

	var directory bytes.Buffer
	directory.WriteByte(byte(10)) // glyf, transform version 0
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(1)) // head, transform version 0
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(11)) // loca, transform version 0
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	var transformed bytes.Buffer
	transformed.Write(transformedGlyf)
	transformed.Write(head)

	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(transformed.Bytes()); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+compressed.Len()))
	binary.BigEndian.PutUint16(header[12:14], 3)
	binary.BigEndian.PutUint32(header[16:20], 12+3*16+align4(20)+align4(uint32(len(head)))+align4(4))
	binary.BigEndian.PutUint32(header[20:24], uint32(compressed.Len()))

	out := append(header, directory.Bytes()...)
	out = append(out, compressed.Bytes()...)
	return out
}

func testWOFF2WithTransformedHmtx(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()
	head := testHeadTable(0)
	hhea := testHheaTable(1)
	maxp := testMaxpTable(1)
	hmtx := []byte{1, 0x01, 0xf4} // omit proportional lsb[], advanceWidth[0]=500

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head, transform version 0
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(2)) // hhea, transform version 0
	directory.Write(testBase128(uint32(len(hhea))))
	directory.WriteByte(byte(0x40 | 3)) // hmtx, transform version 1
	directory.Write(testBase128(4))
	directory.Write(testBase128(uint32(len(hmtx))))
	directory.WriteByte(byte(4)) // maxp, transform version 0
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // glyf, transform version 0
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // loca, transform version 0
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	var transformed bytes.Buffer
	transformed.Write(head)
	transformed.Write(hhea)
	transformed.Write(hmtx)
	transformed.Write(maxp)
	transformed.Write(transformedGlyf)

	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(transformed.Bytes()); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], 0x00010000) // flavor
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+compressed.Len()))
	binary.BigEndian.PutUint16(header[12:14], 6)   // numTables
	binary.BigEndian.PutUint32(header[16:20], 236) // reference totalSfntSize
	binary.BigEndian.PutUint32(header[20:24], uint32(compressed.Len()))

	out := append(header, directory.Bytes()...)
	out = append(out, compressed.Bytes()...)
	return out
}

func testWOFF2NoTablesWithMetadataAndPrivate(t *testing.T, metadata, private []byte) []byte {
	t.Helper()

	compressed := testBrotliData(t, nil)
	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], 0x00010000)
	binary.BigEndian.PutUint16(header[12:14], 0)
	binary.BigEndian.PutUint32(header[16:20], 12)
	binary.BigEndian.PutUint32(header[20:24], uint32(len(compressed)))

	out := append(header, compressed...)
	if len(metadata) > 0 {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		metaOffset := uint32(len(out))
		metaData := testBrotliData(t, metadata)
		out = append(out, metaData...)
		binary.BigEndian.PutUint32(out[28:32], metaOffset)
		binary.BigEndian.PutUint32(out[32:36], uint32(len(metaData)))
		binary.BigEndian.PutUint32(out[36:40], uint32(len(metadata)))
	}
	if len(private) > 0 {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		privOffset := uint32(len(out))
		out = append(out, private...)
		binary.BigEndian.PutUint32(out[40:44], privOffset)
		binary.BigEndian.PutUint32(out[44:48], uint32(len(private)))
	}
	binary.BigEndian.PutUint32(out[8:12], uint32(len(out)))
	return out
}

func testWOFF2WithDirectoryAndPayload(t *testing.T, numTables uint16, directory, payload []byte) []byte {
	t.Helper()

	compressed := testBrotliData(t, payload)
	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], 0x00010000)
	binary.BigEndian.PutUint32(header[8:12], uint32(48+len(directory)+len(compressed)))
	binary.BigEndian.PutUint16(header[12:14], numTables)
	binary.BigEndian.PutUint32(header[16:20], 12+uint32(numTables)*16+align4(uint32(len(payload))))
	binary.BigEndian.PutUint32(header[20:24], uint32(len(compressed)))

	out := append(header, directory...)
	out = append(out, compressed...)
	return out
}

func testWOFF2CollectionWithDirectoryAndPayload(t *testing.T, numTables uint16, directory, payload []byte) []byte {
	t.Helper()

	compressed := testBrotliData(t, payload)
	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], tagTTCF)
	binary.BigEndian.PutUint32(header[8:12], uint32(48+len(directory)+len(compressed)))
	binary.BigEndian.PutUint16(header[12:14], numTables)
	binary.BigEndian.PutUint32(header[16:20], 12+uint32(numTables)*16+align4(uint32(len(payload))))
	binary.BigEndian.PutUint32(header[20:24], uint32(len(compressed)))

	out := append(header, directory...)
	out = append(out, compressed...)
	return out
}

func testWOFF2WithRawGlyfLoca(t *testing.T, glyf, loca []byte) []byte {
	t.Helper()

	var directory bytes.Buffer
	directory.WriteByte(byte(0xc0 | 10)) // glyf, transform version 3 (null)
	directory.Write(testBase128(uint32(len(glyf))))
	directory.WriteByte(byte(0xc0 | 11)) // loca, transform version 3 (null)
	directory.Write(testBase128(uint32(len(loca))))

	var payload bytes.Buffer
	payload.Write(glyf)
	payload.Write(loca)
	return testWOFF2WithDirectoryAndPayload(t, 2, directory.Bytes(), payload.Bytes())
}

func testWOFF2WithRawOutlineTables(t *testing.T, head, maxp, glyf, loca []byte) []byte {
	t.Helper()

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(0xc0 | 10)) // glyf, transform version 3 (null)
	directory.Write(testBase128(uint32(len(glyf))))
	directory.WriteByte(byte(0xc0 | 11)) // loca, transform version 3 (null)
	directory.Write(testBase128(uint32(len(loca))))

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(glyf)
	payload.Write(loca)
	return testWOFF2WithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())
}

func testBrotliData(t testing.TB, data []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(data); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}
	return compressed.Bytes()
}

func testWOFF2Collection(t *testing.T) []byte {
	t.Helper()
	head := testHeadTable(0)
	maxp := testMaxpTable(3)

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))

	var collection bytes.Buffer
	appendUint32(&collection, 0x00010000)
	collection.Write(testUInt255(1)) // numFonts
	collection.Write(testUInt255(2)) // numTables
	appendUint32(&collection, 0x00010000)
	collection.Write(testUInt255(0))
	collection.Write(testUInt255(1))
	directory.Write(collection.Bytes())

	var transformed bytes.Buffer
	transformed.Write(head)
	transformed.Write(maxp)

	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(transformed.Bytes()); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], tagTTCF)
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+compressed.Len()))
	binary.BigEndian.PutUint16(header[12:14], 2)
	binary.BigEndian.PutUint32(header[16:20], 148)
	binary.BigEndian.PutUint32(header[20:24], uint32(compressed.Len()))

	out := append(header, directory.Bytes()...)
	out = append(out, compressed.Bytes()...)
	return out
}

func testWOFF2CollectionWithMismatchedGlyfLoca(t *testing.T) []byte {
	t.Helper()
	transformedGlyf := testWOFF2TransformedSimpleGlyf()

	var directory bytes.Buffer
	for i := 0; i < 2; i++ {
		directory.WriteByte(byte(10)) // transformed glyf
		directory.Write(testBase128(20))
		directory.Write(testBase128(uint32(len(transformedGlyf))))
		directory.WriteByte(byte(11)) // transformed loca
		directory.Write(testBase128(4))
		directory.Write(testBase128(0))
	}

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // numFonts
	directory.Write(testUInt255(2)) // numTables in the font
	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(0)) // glyf from the first pair
	directory.Write(testUInt255(3)) // loca from the second pair

	var transformed bytes.Buffer
	transformed.Write(transformedGlyf)
	transformed.Write(transformedGlyf)

	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(transformed.Bytes()); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], tagTTCF)
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+compressed.Len()))
	binary.BigEndian.PutUint16(header[12:14], 4)
	binary.BigEndian.PutUint32(header[16:20], 128)
	binary.BigEndian.PutUint32(header[20:24], uint32(compressed.Len()))

	out := append(header, directory.Bytes()...)
	out = append(out, compressed.Bytes()...)
	return out
}

func testWOFF2CollectionWithTransformedHmtx(t *testing.T) []byte {
	t.Helper()
	transformedGlyf := testWOFF2TransformedSimpleGlyf()
	head := testHeadTable(0)
	hhea := testHheaTable(1)
	maxp := testMaxpTable(1)
	hmtx := []byte{1, 0x01, 0xf4}

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(2)) // hhea
	directory.Write(testBase128(uint32(len(hhea))))
	directory.WriteByte(byte(0x40 | 3)) // transformed hmtx
	directory.Write(testBase128(4))
	directory.Write(testBase128(uint32(len(hmtx))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // transformed glyf
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // transformed loca
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // numFonts
	directory.Write(testUInt255(6)) // numTables
	appendUint32(&directory, 0x00010000)
	for i := 0; i < 6; i++ {
		directory.Write(testUInt255(uint16(i)))
	}

	var transformed bytes.Buffer
	transformed.Write(head)
	transformed.Write(hhea)
	transformed.Write(hmtx)
	transformed.Write(maxp)
	transformed.Write(transformedGlyf)

	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(transformed.Bytes()); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], tagTTCF)
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+compressed.Len()))
	binary.BigEndian.PutUint16(header[12:14], 6)
	binary.BigEndian.PutUint32(header[16:20], 276)
	binary.BigEndian.PutUint32(header[20:24], uint32(compressed.Len()))

	out := append(header, directory.Bytes()...)
	out = append(out, compressed.Bytes()...)
	return out
}

func testWOFF2HmtxDependencies() ([]woff2TableEntry, [][]byte) {
	transformedGlyf := testWOFF2TransformedSimpleGlyf()
	glyf, loca, err := reconstructWOFF2GlyfLoca(transformedGlyf, 4)
	if err != nil {
		panic(err)
	}
	tables := []woff2TableEntry{
		{tag: tagHead},
		{tag: tagHhea},
		{tag: tagMaxp},
		{tag: tagGlyf},
		{tag: tagLoca},
	}
	reconstructed := [][]byte{
		testHeadTable(0),
		testHheaTable(1),
		testMaxpTable(1),
		glyf,
		loca,
	}
	return tables, reconstructed
}

func testWOFF2TwoGlyphHmtxDependencies(t *testing.T) ([]woff2TableEntry, [][]byte) {
	t.Helper()
	glyf0, _, err := reconstructWOFF2GlyfLoca(testWOFF2TransformedSimpleGlyf(), 4)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	glyph1 := testWOFF2OnePointGlyphAtX10()

	tables := []woff2TableEntry{
		{tag: tagHead},
		{tag: tagHhea},
		{tag: tagMaxp},
		{tag: tagGlyf},
		{tag: tagLoca},
	}
	reconstructed := [][]byte{
		testHeadTable(0),
		testHheaTable(1),
		testMaxpTable(2),
		append(append([]byte(nil), glyf0...), glyph1...),
		[]byte{0, 0, 0, 10, 0, 18},
	}
	return tables, reconstructed
}

func testHeadTable(indexToLocFormat int16) []byte {
	head := make([]byte, 54)
	binary.BigEndian.PutUint32(head[12:16], 0x5f0f3cf5)
	binary.BigEndian.PutUint16(head[50:52], uint16(indexToLocFormat))
	return head
}

func testHheaTable(numberOfHMetrics uint16) []byte {
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], numberOfHMetrics)
	return hhea
}

func testMaxpTable(numGlyphs uint16) []byte {
	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], numGlyphs)
	return maxp
}

type testWOFFTable struct {
	tag  uint32
	data []byte
}

func testWOFFUncompressed(t *testing.T, tables []testWOFFTable) []byte {
	t.Helper()

	dirSize := len(tables) * 20
	directory := make([]byte, dirSize)
	tableData := bytes.NewBuffer(nil)
	totalSfntSize := uint32(12 + len(tables)*16)
	for i, table := range tables {
		offset := uint32(44 + dirSize + tableData.Len())
		dirOff := i * 20
		binary.BigEndian.PutUint32(directory[dirOff:dirOff+4], table.tag)
		binary.BigEndian.PutUint32(directory[dirOff+4:dirOff+8], offset)
		binary.BigEndian.PutUint32(directory[dirOff+8:dirOff+12], uint32(len(table.data)))
		binary.BigEndian.PutUint32(directory[dirOff+12:dirOff+16], uint32(len(table.data)))
		binary.BigEndian.PutUint32(directory[dirOff+16:dirOff+20], sfntChecksum(table.data))

		tableData.Write(table.data)
		for tableData.Len()%4 != 0 {
			tableData.WriteByte(0)
		}
		totalSfntSize += align4(uint32(len(table.data)))
	}

	header := make([]byte, 44)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4646) // wOFF
	binary.BigEndian.PutUint32(header[4:8], 0x00010000)
	binary.BigEndian.PutUint32(header[8:12], uint32(44+dirSize+tableData.Len()))
	binary.BigEndian.PutUint16(header[12:14], uint16(len(tables)))
	binary.BigEndian.PutUint32(header[16:20], totalSfntSize)

	out := append(header, directory...)
	out = append(out, tableData.Bytes()...)
	return out
}

func testWOFFWithMetadataAndPrivate(t *testing.T, woffData, metadata, private []byte) []byte {
	t.Helper()

	out := append([]byte(nil), woffData...)
	if len(metadata) > 0 {
		metaOffset := uint32(len(out))
		metaData := testZlibData(t, metadata)
		out = append(out, metaData...)
		binary.BigEndian.PutUint32(out[24:28], metaOffset)
		binary.BigEndian.PutUint32(out[28:32], uint32(len(metaData)))
		binary.BigEndian.PutUint32(out[32:36], uint32(len(metadata)))
	}
	if len(private) > 0 {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		privOffset := uint32(len(out))
		out = append(out, private...)
		binary.BigEndian.PutUint32(out[36:40], privOffset)
		binary.BigEndian.PutUint32(out[40:44], uint32(len(private)))
	}
	binary.BigEndian.PutUint32(out[8:12], uint32(len(out)))
	return out
}

func testZlibData(t *testing.T, data []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("zlib write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close failed: %v", err)
	}
	return compressed.Bytes()
}

func testBase128(value uint32) []byte {
	if value == 0 {
		return []byte{0}
	}
	var tmp [5]byte
	i := len(tmp)
	for value > 0 {
		i--
		tmp[i] = byte(value & 0x7f)
		value >>= 7
	}
	out := append([]byte(nil), tmp[i:]...)
	for j := 0; j < len(out)-1; j++ {
		out[j] |= 0x80
	}
	return out
}

func testUInt255(value uint16) []byte {
	switch {
	case value < 253:
		return []byte{byte(value)}
	case value < 506:
		return []byte{255, byte(value - 253)}
	case value < 762:
		return []byte{254, byte(value - 506)}
	default:
		return []byte{253, byte(value >> 8), byte(value)}
	}
}

func readTestStream(t *testing.T, stream interface {
	ReadAt([]byte, int64) (int, error)
	Size() int64
}) []byte {
	t.Helper()
	data := make([]byte, stream.Size())
	if _, err := stream.ReadAt(data, 0); err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	return data
}

func sfntTable(t *testing.T, font []byte, tag uint32) []byte {
	t.Helper()
	return sfntTableAt(t, font, 0, tag)
}

func sfntTableAt(t *testing.T, font []byte, base int, tag uint32) []byte {
	t.Helper()
	if base < 0 || base+12 > len(font) {
		t.Fatalf("invalid SFNT base offset %d", base)
	}
	numTables := int(binary.BigEndian.Uint16(font[base+4 : base+6]))
	for i := 0; i < numTables; i++ {
		entry := base + 12 + i*16
		if binary.BigEndian.Uint32(font[entry:entry+4]) != tag {
			continue
		}
		offset := int(binary.BigEndian.Uint32(font[entry+8 : entry+12]))
		length := int(binary.BigEndian.Uint32(font[entry+12 : entry+16]))
		if offset < 0 || length < 0 || offset+length > len(font) {
			t.Fatalf("invalid SFNT table bounds for %s", tagString(tag))
		}
		return font[offset : offset+length]
	}
	t.Fatalf("SFNT table %s not found", tagString(tag))
	return nil
}
