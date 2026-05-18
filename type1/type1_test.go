package type1

import (
	"bytes"
	"testing"
)

func TestDecodePFB(t *testing.T) {
	// Dummy PFB data:
	// Header: 128 (0x80), 1 (ASCII), length: 4 (0x04 0x00 0x00 0x00), data: "test"
	// EOF: 128 (0x80), 3 (EOF)
	pfb := []byte{
		0x80, 1, 0x04, 0x00, 0x00, 0x00, 't', 'e', 's', 't',
		0x80, 3,
	}

	decoded, err := DecodePFB(pfb)
	if err != nil {
		t.Fatalf("DecodePFB failed: %v", err)
	}

	if string(decoded) != "test" {
		t.Errorf("expected 'test', got '%s'", string(decoded))
	}

	// PFA test
	pfa := []byte("just some pfa data")
	decodedPFA, err := DecodePFB(pfa)
	if err != nil {
		t.Fatalf("DecodePFB failed on PFA: %v", err)
	}
	if string(decodedPFA) != "just some pfa data" {
		t.Errorf("expected 'just some pfa data', got '%s'", string(decodedPFA))
	}
}

func TestDecodePFBRejectsMalformedBlocks(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "truncated marker", data: []byte{0x80}},
		{name: "unsupported block type", data: []byte{0x80, 4, 0, 0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodePFB(tt.data); err == nil {
				t.Fatalf("DecodePFB succeeded for %s", tt.name)
			}
		})
	}
}

func TestEexecDecryption(t *testing.T) {
	// Standard eexec cipher: r = 55665, c1 = 52845, c2 = 22719
	// Let's create some plaintext and encrypt it.
	plaintext := []byte("padding-and-then-some-data")

	// Encrypt
	encrypted := make([]byte, len(plaintext))
	r := uint16(55665)
	c1 := uint16(52845)
	c2 := uint16(22719)

	for i := 0; i < len(plaintext); i++ {
		plain := plaintext[i]
		cipher := plain ^ byte(r>>8)
		encrypted[i] = cipher
		r = (uint16(cipher)+r)*c1 + c2
	}

	// Decrypt
	decrypted := DecryptEexec(encrypted, 55665)

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decryption failed, expected %v, got %v", plaintext, decrypted)
	}
}

func TestExtractEexec(t *testing.T) {
	// We'll mock a small file:
	// "some plain text eexec " + encrypted hex
	plaintext := []byte("pad!hello world") // length 15 (4 bytes padding + 11 bytes data)

	encrypted := make([]byte, len(plaintext))
	r := uint16(55665)
	c1 := uint16(52845)
	c2 := uint16(22719)
	for i := 0; i < len(plaintext); i++ {
		plain := plaintext[i]
		cipher := plain ^ byte(r>>8)
		encrypted[i] = cipher
		r = (uint16(cipher)+r)*c1 + c2
	}

	// Hex encode it
	var hexStr string
	for _, b := range encrypted {
		hex := "0123456789ABCDEF"
		hexStr += string(hex[b>>4]) + string(hex[b&0x0F])
	}

	data := []byte("some header eexec " + hexStr + " \n000000000000000000000000")

	extracted, err := ExtractEexec(data)
	if err != nil {
		t.Fatalf("ExtractEexec failed: %v", err)
	}

	// first 4 bytes are dropped
	if string(extracted) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(extracted))
	}
}

func TestLexerAndDicts(t *testing.T) {
	data := []byte(`
		/FontBBox [-50 -150 1000 900] readonly def
		/Private 10 dict def
		/CharStrings 2 dict dup begin
			/.notdef 123 def
			/A 456 def
		end def
	`)

	tokens := Lexer(data)
	dicts := ParseDicts(tokens)

	if val, ok := dicts["FontBBox"]; !ok || len(val) != 4 || val[0].Value != "-50" {
		t.Errorf("failed to parse FontBBox, got %v", val)
	}
	if val, ok := dicts["Private"]; !ok || len(val) != 1 || val[0].Value != "10" {
		t.Errorf("failed to parse Private, got %v", val)
	}
	if val, ok := dicts["CharStrings"]; !ok || len(val) != 1 || val[0].Value != "2" {
		t.Errorf("failed to parse CharStrings, got %v", val)
	}
}

func TestLexerStringTrailingEscapeDoesNotPanic(t *testing.T) {
	tokens := Lexer([]byte(`/Notice (ends with escape\`))
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %#v", len(tokens), tokens)
	}
	if tokens[1].Type != "String" || tokens[1].Value != `(ends with escape\` {
		t.Fatalf("unexpected string token: %#v", tokens[1])
	}
}

func TestLexerStandaloneDelimitersAdvance(t *testing.T) {
	tokens := Lexer([]byte(`(abc) > /A + - . 1.2.3`))
	if len(tokens) != 7 {
		t.Fatalf("expected 7 tokens, got %d: %#v", len(tokens), tokens)
	}
	if tokens[1] != (Token{Type: "Delim", Value: ">"}) {
		t.Fatalf("standalone > token = %#v", tokens[1])
	}
	for i, tok := range tokens[3:] {
		if tok.Type != "Operator" {
			t.Fatalf("token %d = %#v, want Operator", i+3, tok)
		}
	}
}

func TestParseDictsUnmatchedClosingDelimiterRecovers(t *testing.T) {
	tokens := Lexer([]byte(`/Bad ] /FontBBox [-50 -150 1000 900] readonly def /Private 10 dict def`))
	dicts := ParseDicts(tokens)

	if _, ok := dicts["Bad"]; ok {
		t.Fatalf("unexpected value for malformed Bad key: %v", dicts["Bad"])
	}
	if val, ok := dicts["FontBBox"]; !ok || len(val) != 4 || val[0].Value != "-50" {
		t.Fatalf("failed to recover FontBBox after unmatched delimiter, got %v", val)
	}
	if val, ok := dicts["Private"]; !ok || len(val) != 1 || val[0].Value != "10" {
		t.Fatalf("failed to recover Private after unmatched delimiter, got %v", val)
	}
}
