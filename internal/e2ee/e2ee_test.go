package e2ee

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/crypto/pbkdf2"
)

// buildKeyV1Packet creates a KeyV1-encrypted master key packet the same way
// Joplin does: PBKDF2-HMAC-SHA512(220000) + AES-256-GCM over the raw bytes
// (not the hex string) of the master key.
func buildKeyV1Packet(t *testing.T, password string, rawMasterKey []byte, salt, iv []byte) string {
	t.Helper()
	derived := pbkdf2.Key([]byte(password), salt, 220000, 32, sha512.New)
	block, err := aes.NewCipher(derived)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	ct := gcm.Seal(nil, iv, rawMasterKey, nil)
	pkt := cipherPacket{
		Salt: base64.StdEncoding.EncodeToString(salt),
		IV:   base64.StdEncoding.EncodeToString(iv),
		CT:   base64.StdEncoding.EncodeToString(ct),
	}
	b, err := json.Marshal(pkt)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestModernDecryptMasterKey_ReturnsHexString(t *testing.T) {
	// Known 32-byte raw master key (Joplin generates 256 bytes; we use 32 for brevity since the logic is the same).
	rawKey, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	expectedHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	password := "test-password"
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i)
	}
	iv := make([]byte, 12)
	for i := range iv {
		iv[i] = byte(i + 100)
	}

	packet := buildKeyV1Packet(t, password, rawKey, salt, iv)

	got, err := modernDecryptMasterKey(password, packet)
	if err != nil {
		t.Fatalf("modernDecryptMasterKey failed: %v", err)
	}

	if string(got) != expectedHex {
		t.Errorf("expected hex string %q (len %d), got %q (len %d)",
			expectedHex, len(expectedHex), string(got), len(got))
	}
	if len(got) != len(rawKey)*2 {
		t.Errorf("expected decoded master key length %d (2x raw), got %d", len(rawKey)*2, len(got))
	}
}

func TestModernDecryptMasterKey_256ByteKey(t *testing.T) {
	// Simulate a real 256-byte Joplin master key.
	rawKey := make([]byte, 256)
	for i := range rawKey {
		rawKey[i] = byte(i)
	}
	expectedHex := hex.EncodeToString(rawKey) // 512-char hex string

	password := "my-strong-master-password!"
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i + 50)
	}
	iv := make([]byte, 12)
	for i := range iv {
		iv[i] = byte(i + 200)
	}

	packet := buildKeyV1Packet(t, password, rawKey, salt, iv)

	got, err := modernDecryptMasterKey(password, packet)
	if err != nil {
		t.Fatalf("modernDecryptMasterKey failed: %v", err)
	}

	if string(got) != expectedHex {
		t.Fatalf("hex mismatch: expected len %d, got len %d", len(expectedHex), len(got))
	}
}

func TestLoadMasterKey_KeyV1_ProducesHexBytes(t *testing.T) {
	rawKey := make([]byte, 256)
	for i := range rawKey {
		rawKey[i] = byte(i)
	}
	expectedHex := hex.EncodeToString(rawKey)

	password := "test-password"
	salt := make([]byte, 32)
	iv := make([]byte, 12)
	for i := range salt {
		salt[i] = byte(i)
	}
	for i := range iv {
		iv[i] = byte(i + 10)
	}

	packet := buildKeyV1Packet(t, password, rawKey, salt, iv)

	svc := NewService()
	err := svc.LoadMasterKey("abcdef01234567890123456789abcdef", packet, password, MethodKeyV1)
	if err != nil {
		t.Fatalf("LoadMasterKey failed: %v", err)
	}

	mk := svc.masterKeys["abcdef01234567890123456789abcdef"]
	if string(mk) != expectedHex {
		t.Errorf("loaded master key should be hex string (len %d), got len %d", len(expectedHex), len(mk))
	}
}

func TestEncryptDecryptString_RoundTrip(t *testing.T) {
	masterKeyHex := hex.EncodeToString(make([]byte, 256))
	masterKeyID := "aabbccdd11223344aabbccdd11223344"

	svc := NewService()
	svc.masterKeys[masterKeyID] = []byte(masterKeyHex)

	tests := []struct {
		name  string
		input string
	}{
		{"ascii", "Hello, World!"},
		{"unicode", "Héllo Wörld! 日本語テスト 🎉"},
		{"empty", ""},
		{"newlines", "line1\nline2\nline3"},
		{"joplin_note", "Test Note\n\nThis is a note body with some content.\n\nid: abc123\nparent_id: def456\ntype_: 1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := svc.EncryptString(masterKeyID, tc.input)
			if err != nil {
				t.Fatalf("EncryptString failed: %v", err)
			}

			if !strings.HasPrefix(encrypted, "JED01") {
				t.Fatal("encrypted output missing JED01 prefix")
			}

			decrypted, err := svc.DecryptString(encrypted)
			if err != nil {
				t.Fatalf("DecryptString failed: %v", err)
			}

			if decrypted != tc.input {
				t.Errorf("round-trip mismatch:\n  want: %q\n  got:  %q", tc.input, decrypted)
			}
		})
	}
}

func TestEncryptString_JED01Format(t *testing.T) {
	masterKeyHex := hex.EncodeToString(make([]byte, 256))
	masterKeyID := "aabbccdd11223344aabbccdd11223344"

	svc := NewService()
	svc.masterKeys[masterKeyID] = []byte(masterKeyHex)

	encrypted, err := svc.EncryptString(masterKeyID, "test content")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	mkID, method, chunks, err := parseJED01(encrypted)
	if err != nil {
		t.Fatalf("parseJED01 failed: %v", err)
	}
	if mkID != masterKeyID {
		t.Errorf("master key ID: got %q, want %q", mkID, masterKeyID)
	}
	if method != MethodStringV1 {
		t.Errorf("method: got %d, want %d (StringV1)", method, MethodStringV1)
	}
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}

	// Each chunk must be valid JSON with base64 salt, iv, ct.
	for i, chunk := range chunks {
		var pkt cipherPacket
		if err := json.Unmarshal(chunk, &pkt); err != nil {
			t.Errorf("chunk %d: invalid JSON: %v", i, err)
			continue
		}
		saltBytes, err := base64.StdEncoding.DecodeString(pkt.Salt)
		if err != nil {
			t.Errorf("chunk %d: invalid salt base64: %v", i, err)
		}
		if len(saltBytes) != 32 {
			t.Errorf("chunk %d: salt length = %d, want 32", i, len(saltBytes))
		}
		ivBytes, err := base64.StdEncoding.DecodeString(pkt.IV)
		if err != nil {
			t.Errorf("chunk %d: invalid iv base64: %v", i, err)
		}
		if len(ivBytes) != 12 {
			t.Errorf("chunk %d: iv length = %d, want 12", i, len(ivBytes))
		}
	}
}

func TestUTF16LE_RoundTrip(t *testing.T) {
	tests := []string{
		"Hello",
		"日本語",
		"emoji: 🎉🎊",
		"mixed: abc123日本語🎉",
		"",
	}
	for _, s := range tests {
		encoded := utf8ToUTF16LE(s)
		decoded := utf16LEToUTF8(encoded)
		if decoded != s {
			t.Errorf("UTF-16LE round-trip failed for %q: got %q", s, decoded)
		}
	}
}

// TestUTF16LE_MatchesJoplin verifies that our UTF-16LE encoding matches
// what JavaScript's Buffer.from(str, 'utf16le') produces.
func TestUTF16LE_MatchesJoplin(t *testing.T) {
	// "Hello" in UTF-16LE: H(0x48,0x00) e(0x65,0x00) l(0x6c,0x00) l(0x6c,0x00) o(0x6f,0x00)
	input := "Hello"
	expected := []byte{0x48, 0x00, 0x65, 0x00, 0x6c, 0x00, 0x6c, 0x00, 0x6f, 0x00}
	got := utf8ToUTF16LE(input)
	if len(got) != len(expected) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], expected[i])
		}
	}
}

// TestEncryptString_UTF16LE_Encoding verifies that encrypted data, once
// decrypted at the chunk level, contains UTF-16LE encoded text.
func TestEncryptString_UTF16LE_Encoding(t *testing.T) {
	masterKeyHex := hex.EncodeToString(make([]byte, 256))
	masterKeyID := "aabbccdd11223344aabbccdd11223344"

	svc := NewService()
	svc.masterKeys[masterKeyID] = []byte(masterKeyHex)

	plaintext := "Hello"
	encrypted, err := svc.EncryptString(masterKeyID, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	_, _, chunks, err := parseJED01(encrypted)
	if err != nil {
		t.Fatal(err)
	}

	rawDecrypted, err := decryptChunk([]byte(masterKeyHex), chunks[0], 3)
	if err != nil {
		t.Fatal(err)
	}

	// The raw decrypted chunk should be UTF-16LE of "Hello".
	expectedUTF16LE := utf8ToUTF16LE(plaintext)
	if len(rawDecrypted) != len(expectedUTF16LE) {
		t.Fatalf("raw decrypted length %d != expected UTF-16LE length %d", len(rawDecrypted), len(expectedUTF16LE))
	}
	for i := range expectedUTF16LE {
		if rawDecrypted[i] != expectedUTF16LE[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, rawDecrypted[i], expectedUTF16LE[i])
		}
	}
}

// TestEncryptChunk_SaltSize verifies that encryptChunk produces a 32-byte salt.
func TestEncryptChunk_SaltSize(t *testing.T) {
	masterKey := []byte("0000000000000000000000000000000000000000000000000000000000000000")
	data := []byte("test data")

	chunkJSON, err := encryptChunk(masterKey, data, 3)
	if err != nil {
		t.Fatal(err)
	}

	var pkt cipherPacket
	if err := json.Unmarshal(chunkJSON, &pkt); err != nil {
		t.Fatal(err)
	}

	saltBytes, err := base64.StdEncoding.DecodeString(pkt.Salt)
	if err != nil {
		t.Fatal(err)
	}
	if len(saltBytes) != 32 {
		t.Errorf("salt size = %d bytes, want 32", len(saltBytes))
	}
}

// TestDecryptString_CrossCompatFormat creates an encrypted JED01 envelope
// using the exact same steps Joplin would use (PBKDF2-SHA512(3) + AES-256-GCM,
// UTF-16LE encoding), and verifies go-joplin can decrypt it.
func TestDecryptString_CrossCompatFormat(t *testing.T) {
	// Simulate a master key as Joplin stores it: hex string bytes.
	rawMasterKeyBytes := make([]byte, 32)
	for i := range rawMasterKeyBytes {
		rawMasterKeyBytes[i] = byte(i + 1)
	}
	masterKeyHex := hex.EncodeToString(rawMasterKeyBytes)
	masterKeyID := "aabbccdd11223344aabbccdd11223344"

	plaintext := "This is a Joplin note"
	utf16Bytes := joplinUTF16LE(plaintext)

	// Encrypt with known salt and IV (simulating Joplin's crypto.encrypt).
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i + 42)
	}
	iv := make([]byte, 12)
	for i := range iv {
		iv[i] = byte(i + 99)
	}

	derived := pbkdf2.Key([]byte(masterKeyHex), salt, 3, 32, sha512.New)
	block, err := aes.NewCipher(derived)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	ct := gcm.Seal(nil, iv, utf16Bytes, nil)

	pkt := cipherPacket{
		Salt: base64.StdEncoding.EncodeToString(salt),
		IV:   base64.StdEncoding.EncodeToString(iv),
		CT:   base64.StdEncoding.EncodeToString(ct),
	}
	chunkJSON, _ := json.Marshal(pkt)

	// Build JED01 envelope.
	metadata := fmt.Sprintf("%02x%s", MethodStringV1, masterKeyID)
	var envelope strings.Builder
	envelope.WriteString("JED01")
	envelope.WriteString(fmt.Sprintf("%06x", len(metadata)))
	envelope.WriteString(metadata)
	envelope.WriteString(fmt.Sprintf("%06x", len(chunkJSON)))
	envelope.Write(chunkJSON)

	// Now decrypt with go-joplin.
	svc := NewService()
	svc.masterKeys[masterKeyID] = []byte(masterKeyHex)

	result, err := svc.DecryptString(envelope.String())
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}
	if result != plaintext {
		t.Errorf("decrypted text = %q, want %q", result, plaintext)
	}
}

// joplinUTF16LE mimics JavaScript's Buffer.from(str, 'utf16le').
func joplinUTF16LE(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	out := make([]byte, len(u16)*2)
	for i, c := range u16 {
		out[i*2] = byte(c)
		out[i*2+1] = byte(c >> 8)
	}
	return out
}

func TestChecksumMasterKey(t *testing.T) {
	// The checksum is SHA-256 of the hex string, matching Joplin's EncryptionService.sha256().
	hexKey := "0123456789abcdef"
	got := ChecksumMasterKey(hexKey)
	if len(got) != 64 {
		t.Errorf("checksum length = %d, want 64 hex chars", len(got))
	}
}
