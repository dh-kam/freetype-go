package helper

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionMalformedDependencyRejectsSharedHmtxWithMismatchedOutlineMetadata(t *testing.T) {
	hmtx := []byte{3, 0x01, 0xf4} // omit all lsb values; derive them from glyf xMin
	entries := []woff2CollectionMalformedDependencyEntry{
		woff2CollectionMalformedDependencyRawEntry(1, testHeadTable(0)),
		woff2CollectionMalformedDependencyRawEntry(1, testHeadTable(0)),
		woff2CollectionMalformedDependencyRawEntry(2, testHheaTable(1)),
		woff2CollectionMalformedDependencyTransformedEntry(0x40|3, 6, hmtx),
		woff2CollectionMalformedDependencyRawEntry(4, testMaxpTable(2)),
		woff2CollectionMalformedDependencyTransformedEntry(10, 20, woff2CollectionMalformedDependencySimpleAndEmptyGlyf()),
		woff2CollectionMalformedDependencyTransformedEntry(11, 6, nil),
		woff2CollectionMalformedDependencyTransformedEntry(10, 20, testWOFF2TransformedSimpleGlyf()),
		woff2CollectionMalformedDependencyTransformedEntry(11, 4, nil),
	}
	woff2Data := woff2CollectionMalformedDependencyWOFF2(t, entries, [][]uint16{
		{0, 2, 3, 4, 5, 6},
		{1, 2, 3, 4, 7, 8},
	})

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected malformed shared hmtx outline metadata to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyf/loca metadata mismatch") {
		t.Fatalf("error = %q, want glyf/loca metadata mismatch", err)
	}
}

func TestWOFF2CollectionMalformedDependencyRejectsHmtxWithMixedTransformedGlyfLocaPairs(t *testing.T) {
	hmtx := []byte{1, 0x01, 0xf4} // omit proportional lsb[0]; derive it from glyf xMin
	entries := []woff2CollectionMalformedDependencyEntry{
		woff2CollectionMalformedDependencyRawEntry(1, testHeadTable(0)),
		woff2CollectionMalformedDependencyRawEntry(2, testHheaTable(1)),
		woff2CollectionMalformedDependencyTransformedEntry(0x40|3, 4, hmtx),
		woff2CollectionMalformedDependencyRawEntry(4, testMaxpTable(1)),
		woff2CollectionMalformedDependencyTransformedEntry(10, 20, testWOFF2TransformedSimpleGlyf()),
		woff2CollectionMalformedDependencyTransformedEntry(11, 4, nil),
		woff2CollectionMalformedDependencyTransformedEntry(10, 20, testWOFF2TransformedSimpleGlyf()),
		woff2CollectionMalformedDependencyTransformedEntry(11, 4, nil),
	}
	woff2Data := woff2CollectionMalformedDependencyWOFF2(t, entries, [][]uint16{
		{0, 1, 2, 3, 4, 5},
		{0, 1, 2, 3, 4, 7},
	})

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected mixed transformed glyf/loca references to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 collection font has mismatched glyf/loca tables") {
		t.Fatalf("error = %q, want collection glyf/loca mismatch", err)
	}
}

type woff2CollectionMalformedDependencyEntry struct {
	flag               byte
	origLength         uint32
	transformLength    uint32
	hasTransformLength bool
	payload            []byte
}

func woff2CollectionMalformedDependencyWOFF2(t *testing.T, entries []woff2CollectionMalformedDependencyEntry, faceRefs [][]uint16) []byte {
	t.Helper()

	var directory bytes.Buffer
	var payload bytes.Buffer
	for _, entry := range entries {
		directory.WriteByte(entry.flag)
		directory.Write(testBase128(entry.origLength))
		if entry.hasTransformLength {
			directory.Write(testBase128(entry.transformLength))
		}
		payload.Write(entry.payload)
	}

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(uint16(len(faceRefs))))
	for _, refs := range faceRefs {
		directory.Write(testUInt255(uint16(len(refs))))
		appendUint32(&directory, 0x00010000)
		for _, ref := range refs {
			directory.Write(testUInt255(ref))
		}
	}

	return testWOFF2CollectionWithDirectoryAndPayload(t, uint16(len(entries)), directory.Bytes(), payload.Bytes())
}

func woff2CollectionMalformedDependencyRawEntry(flag byte, data []byte) woff2CollectionMalformedDependencyEntry {
	return woff2CollectionMalformedDependencyEntry{
		flag:       flag,
		origLength: uint32(len(data)),
		payload:    data,
	}
}

func woff2CollectionMalformedDependencyTransformedEntry(flag byte, origLength uint32, payload []byte) woff2CollectionMalformedDependencyEntry {
	return woff2CollectionMalformedDependencyEntry{
		flag:               flag,
		origLength:         origLength,
		transformLength:    uint32(len(payload)),
		hasTransformLength: true,
		payload:            payload,
	}
}

func woff2CollectionMalformedDependencySimpleAndEmptyGlyf() []byte {
	return testWOFF2TransformedGlyfFromStreams(
		2,
		[]byte{0, 1, 0, 0},
		[]byte{3},
		[]byte{0, 11, 1},
		[]byte{0, 10, 20, 0},
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)
}
