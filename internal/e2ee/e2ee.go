package e2ee

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"golang.org/x/crypto/pbkdf2"
)

// Joplin encryption methods.
const (
	MethodSJCL1  = 1 // OCB2, AES-128, 1000 iter (deprecated)
	MethodSJCL2  = 2 // OCB2, AES-256, 10000 iter
	MethodSJCL3  = 3 // CCM, AES-128, 1000 iter (legacy)
	MethodSJCL4  = 4 // CCM, AES-256, 10000 iter — common for master keys
	MethodSJCL1a = 5 // CCM, AES-128, 101 iter
	MethodSJCL1b = 7 // CCM, AES-256, 101 iter

	MethodKeyV1    = 8  // Master key decryption: PBKDF2(220000) + AES-256-GCM
	MethodFileV1   = 9  // Resource blob decryption: PBKDF2(3) + AES-256-GCM
	MethodStringV1 = 10 // Note/item string decryption: PBKDF2(3) + AES-256-GCM
)

// cipherPacket is the JSON structure for modern (methods 8-10) encrypted chunks.
type cipherPacket struct {
	Salt string `json:"salt"` // base64-encoded salt
	IV   string `json:"iv"`   // base64-encoded nonce (12 bytes for GCM)
	CT   string `json:"ct"`   // base64-encoded ciphertext+tag
}

// sjclPacket is the JSON structure produced by the sjcl library (methods 1-7).
type sjclPacket struct {
	IV     string `json:"iv"`     // base64-encoded IV (16 bytes)
	V      int    `json:"v"`      // version (always 1)
	Iter   int    `json:"iter"`   // PBKDF2 iteration count
	KS     int    `json:"ks"`     // key size in bits (128 or 256)
	TS     int    `json:"ts"`     // auth tag size in bits (64)
	Mode   string `json:"mode"`   // "ccm" or "ocb2"
	Cipher string `json:"cipher"` // "aes"
	Salt   string `json:"salt"`   // base64-encoded salt
	CT     string `json:"ct"`     // base64-encoded ciphertext+tag
	AData  string `json:"adata"`  // associated data (usually empty)
}

// Service handles Joplin E2EE decryption.
type Service struct {
	masterKeys map[string][]byte // masterKeyID → decrypted key bytes
}

// NewService creates a new E2EE service.
func NewService() *Service {
	return &Service{
		masterKeys: make(map[string][]byte),
	}
}

// LoadMasterKey decrypts a master key using the user's master password and caches it.
// encryptionMethod is the encryption_method field from the MasterKey entity.
func (s *Service) LoadMasterKey(id string, encryptedContent string, password string, encryptionMethod int) error {
	var plaintext []byte
	var err error

	switch encryptionMethod {
	case MethodSJCL1, MethodSJCL2, MethodSJCL3, MethodSJCL4, MethodSJCL1a, MethodSJCL1b:
		plaintext, err = sjclDecrypt(password, encryptedContent)
	case MethodKeyV1:
		plaintext, err = modernDecryptMasterKey(password, encryptedContent)
	default:
		return fmt.Errorf("e2ee: unsupported master key encryption method %d", encryptionMethod)
	}
	if err != nil {
		return fmt.Errorf("e2ee: cannot decrypt master key %s: %w", id, err)
	}

	// Joplin stores the master key as a hex string and passes it directly
	// to PBKDF2 as UTF-8 bytes (512 bytes for a 256-byte key). We must
	// store the hex string bytes, NOT hex-decode them to binary.
	s.masterKeys[id] = plaintext
	return nil
}

// modernDecryptMasterKey decrypts a master key encrypted with method 8 (KeyV1).
// Uses PBKDF2-HMAC-SHA512 with 220000 iterations and AES-256-GCM.
func modernDecryptMasterKey(password string, encryptedContent string) ([]byte, error) {
	var pkt cipherPacket
	if err := json.Unmarshal([]byte(encryptedContent), &pkt); err != nil {
		return nil, fmt.Errorf("cannot parse cipher packet: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(pkt.Salt)
	if err != nil {
		return nil, fmt.Errorf("cannot decode salt: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(pkt.IV)
	if err != nil {
		return nil, fmt.Errorf("cannot decode IV: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(pkt.CT)
	if err != nil {
		return nil, fmt.Errorf("cannot decode ciphertext: %w", err)
	}

	derivedKey := pbkdf2.Key([]byte(password), salt, 220000, 32, sha512.New)
	return decryptAESGCM(derivedKey, iv, ct)
}

// sjclDecrypt decrypts content encrypted by the sjcl library (AES-CCM).
// Uses PBKDF2-HMAC-SHA256 with iterations/key size from the JSON packet.
func sjclDecrypt(password string, encryptedContent string) ([]byte, error) {
	var pkt sjclPacket
	if err := json.Unmarshal([]byte(encryptedContent), &pkt); err != nil {
		return nil, fmt.Errorf("cannot parse sjcl packet: %w", err)
	}

	if pkt.Mode != "ccm" {
		return nil, fmt.Errorf("unsupported sjcl mode %q (only ccm supported)", pkt.Mode)
	}

	salt, err := base64.StdEncoding.DecodeString(pkt.Salt)
	if err != nil {
		return nil, fmt.Errorf("cannot decode salt: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(pkt.IV)
	if err != nil {
		return nil, fmt.Errorf("cannot decode IV: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(pkt.CT)
	if err != nil {
		return nil, fmt.Errorf("cannot decode ciphertext: %w", err)
	}

	keyLen := pkt.KS / 8 // key size in bytes (16 or 32)
	if keyLen != 16 && keyLen != 32 {
		return nil, fmt.Errorf("unsupported sjcl key size %d bits", pkt.KS)
	}

	iterations := pkt.Iter
	if iterations <= 0 {
		iterations = 10000
	}

	tagLen := pkt.TS / 8 // tag size in bytes (typically 8)
	if tagLen < 4 || tagLen > 16 {
		return nil, fmt.Errorf("unsupported sjcl tag size %d bits", pkt.TS)
	}

	// sjcl uses PBKDF2-HMAC-SHA256
	derivedKey := pbkdf2.Key([]byte(password), salt, iterations, keyLen, sha256.New)

	return sjclCCMDecrypt(derivedKey, iv, ct, tagLen)
}

// sjclCCMDecrypt implements AES-CCM decryption compatible with sjcl's CCM mode.
//
// sjcl clamps the IV to (15-L) bytes (typically 13), then builds standard
// RFC 3610 CTR and CBC-MAC blocks:
//
//	CTR: [L-1] + nonce(15-L bytes) + counter(L bytes, starting at 0)
//	B0:  [flags] + nonce(15-L bytes) + msgLen(L bytes)
//
// The counter occupies the 4th 32-bit word (bytes 12-15) and is incremented
// as a big-endian uint32 (matching sjcl's ctr[3]++).
func sjclCCMDecrypt(key, iv, ct []byte, tagLen int) ([]byte, error) {
	if len(ct) < tagLen {
		return nil, fmt.Errorf("ccm: ciphertext shorter than tag")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Split ciphertext and encrypted tag
	encData := ct[:len(ct)-tagLen]
	encTag := ct[len(ct)-tagLen:]

	plaintextLen := len(encData)
	L := sjclComputeL(plaintextLen)

	// Clamp IV to (15-L) bytes, matching sjcl's:
	//   c = f.clamp(c, 8*(15-b))
	nonceLen := 15 - L
	nonce := iv
	if len(nonce) > nonceLen {
		nonce = nonce[:nonceLen]
	}

	// Build CTR block: [L-1] + nonce + zeros (counter starts at 0)
	// sjcl: concat([partial(8,L-1)], c).concat([0,0,0]).slice(0,4)
	ctr := make([]byte, 16)
	ctr[0] = byte(L - 1)
	copy(ctr[1:1+nonceLen], nonce)

	// Decrypt tag using S0 (counter at initial value = 0)
	s0 := make([]byte, 16)
	block.Encrypt(s0, ctr)
	tag := make([]byte, tagLen)
	for i := range tag {
		tag[i] = encTag[i] ^ s0[i]
	}

	// Decrypt data using CTR mode (increment counter for each block)
	plaintext := make([]byte, plaintextLen)
	for i := 0; i < plaintextLen; i += 16 {
		incrementWord32(ctr, 12) // sjcl: ctr[3]++
		enc := make([]byte, 16)
		block.Encrypt(enc, ctr)
		end := i + 16
		if end > plaintextLen {
			end = plaintextLen
		}
		for j := i; j < end; j++ {
			plaintext[j] = encData[j] ^ enc[j-i]
		}
	}

	// Compute CBC-MAC over plaintext and verify
	computedTag := sjclCBCMAC(block, nonce, plaintext, tagLen, L)
	if !hmac.Equal(tag, computedTag) {
		return nil, fmt.Errorf("ccm: authentication failed")
	}

	return plaintext, nil
}

// sjclCBCMAC computes the CBC-MAC tag per RFC 3610 / sjcl.
//
//	B0 = [flags] + nonce(15-L bytes) + msgLen(L bytes)
//	flags = ((tagLen-2)/2)<<3 | (L-1)
//
// The nonce must already be clamped to (15-L) bytes.
func sjclCBCMAC(block cipher.Block, nonce, plaintext []byte, tagLen, L int) []byte {
	nonceLen := 15 - L

	// Build B0
	mac := make([]byte, 16)
	mac[0] = byte(((tagLen-2)/2)<<3 | (L - 1))
	copy(mac[1:1+nonceLen], nonce)

	// Encode plaintext byte-length in the last L bytes.
	// sjcl does d[3]|=e which ORs into the 4th word (bytes 12-15).
	// Since nonce fills bytes 1..(nonceLen) and the remaining bytes are 0,
	// this is equivalent to setting bytes (16-L)..15 to the message length.
	ptLen := uint32(len(plaintext))
	mac[12] |= byte(ptLen >> 24)
	mac[13] |= byte(ptLen >> 16)
	mac[14] |= byte(ptLen >> 8)
	mac[15] |= byte(ptLen)

	// Encrypt B0
	block.Encrypt(mac, mac)

	// No adata processing (sjcl adata is always empty in Joplin)

	// Process plaintext blocks
	var xorBlock [16]byte
	for i := 0; i < len(plaintext); i += 16 {
		// Zero-pad the last block if needed
		for j := range xorBlock {
			xorBlock[j] = 0
		}
		end := i + 16
		if end > len(plaintext) {
			end = len(plaintext)
		}
		copy(xorBlock[:], plaintext[i:end])
		for j := 0; j < 16; j++ {
			mac[j] ^= xorBlock[j]
		}
		block.Encrypt(mac, mac)
	}

	return mac[:tagLen]
}

// sjclComputeL matches sjcl.mode.ccm._computeL.
func sjclComputeL(plaintextLen int) int {
	if plaintextLen < 65536 {
		return 2
	}
	if plaintextLen < 16777216 {
		return 3
	}
	return 4
}

// incrementWord32 increments a big-endian uint32 at the given byte offset.
func incrementWord32(b []byte, off int) {
	v := uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])
	v++
	b[off] = byte(v >> 24)
	b[off+1] = byte(v >> 16)
	b[off+2] = byte(v >> 8)
	b[off+3] = byte(v)
}

// HasMasterKey reports whether a master key has been loaded.
func (s *Service) HasMasterKey(id string) bool {
	_, ok := s.masterKeys[id]
	return ok
}

// StringV1ChunkSize is the chunk size for StringV1 encryption (64k), matching Joplin.
const StringV1ChunkSize = 65536

// EncryptString encrypts plainText using the given master key and returns a JED01 envelope (method 10 = StringV1).
// The master key must already be loaded via LoadMasterKey. Plaintext is encoded as UTF-16LE before encryption.
func (s *Service) EncryptString(masterKeyID string, plainText string) (string, error) {
	masterKey, ok := s.masterKeys[masterKeyID]
	if !ok {
		return "", fmt.Errorf("e2ee: master key %s not loaded", masterKeyID)
	}
	if len(masterKeyID) != 32 {
		return "", fmt.Errorf("e2ee: invalid master key ID length %d", len(masterKeyID))
	}

	plainBytes := utf8ToUTF16LE(plainText)
	var chunks [][]byte
	for off := 0; off < len(plainBytes); {
		end := off + StringV1ChunkSize
		if end > len(plainBytes) {
			end = len(plainBytes)
		}
		chunk, err := encryptChunk(masterKey, plainBytes[off:end], 3)
		if err != nil {
			return "", fmt.Errorf("e2ee: encrypt chunk: %w", err)
		}
		chunks = append(chunks, chunk)
		off = end
	}

	// JED01: "JED01" + 6-char hex metadata length + metadata (2-char hex method + 32-char master key ID) + chunks
	metadata := fmt.Sprintf("%02x%s", MethodStringV1, masterKeyID)
	metaLen := len(metadata)
	var b strings.Builder
	b.WriteString("JED01")
	b.WriteString(fmt.Sprintf("%06x", metaLen))
	b.WriteString(metadata)
	for _, chunk := range chunks {
		b.WriteString(fmt.Sprintf("%06x", len(chunk)))
		b.Write(chunk)
	}
	return b.String(), nil
}

// utf8ToUTF16LE encodes UTF-8 string as UTF-16LE bytes (for StringV1 encryption).
func utf8ToUTF16LE(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	out := make([]byte, len(u16)*2)
	for i, c := range u16 {
		out[i*2] = byte(c)
		out[i*2+1] = byte(c >> 8)
	}
	return out
}

// encryptChunk encrypts a single chunk with PBKDF2(iterations) + AES-256-GCM, returns JSON cipher packet.
func encryptChunk(masterKey, plaintext []byte, iterations int) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	derivedKey := pbkdf2.Key(masterKey, salt, iterations, 32, sha512.New)
	ciphertext, err := encryptAESGCM(derivedKey, nonce, plaintext)
	if err != nil {
		return nil, err
	}
	pkt := cipherPacket{
		Salt: base64.StdEncoding.EncodeToString(salt),
		IV:   base64.StdEncoding.EncodeToString(nonce),
		CT:   base64.StdEncoding.EncodeToString(ciphertext),
	}
	return json.Marshal(pkt)
}

// encryptAESGCM encrypts with AES-256-GCM (no associated data).
func encryptAESGCM(key, nonce, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, nil), nil
}

// DecryptString decrypts a JED01 cipher text (methods 5/7 = SJCL, 10 = StringV1).
func (s *Service) DecryptString(cipherText string) (string, error) {
	masterKeyID, method, chunks, err := parseJED01(cipherText)
	if err != nil {
		return "", err
	}

	masterKey, ok := s.masterKeys[masterKeyID]
	if !ok {
		return "", fmt.Errorf("e2ee: master key %s not loaded", masterKeyID)
	}

	switch method {
	case MethodStringV1:
		var result []byte
		for _, chunk := range chunks {
			decrypted, err := decryptChunk(masterKey, chunk, 3)
			if err != nil {
				return "", fmt.Errorf("e2ee: chunk decryption failed: %w", err)
			}
			result = append(result, decrypted...)
		}
		return utf16LEToUTF8(result), nil

	case MethodSJCL1a, MethodSJCL1b:
		// SJCL string encryption: each chunk is SJCL JSON; key is master key (hex string).
		var result []byte
		for _, chunk := range chunks {
			decrypted, err := sjclDecrypt(string(masterKey), string(chunk))
			if err != nil {
				return "", fmt.Errorf("e2ee: sjcl chunk decryption failed: %w", err)
			}
			result = append(result, decrypted...)
		}
		// Joplin applies unescape() for SJCL1a/SJCL1b (decode %XX sequences only, like JS unescape).
		return unescapePercentXX(string(result)), nil

	default:
		return "", fmt.Errorf("e2ee: unsupported string encryption method %d", method)
	}
}

// DecryptFile decrypts a JED01 cipher text containing raw binary data (method 9).
func (s *Service) DecryptFile(cipherText string) ([]byte, error) {
	masterKeyID, method, chunks, err := parseJED01(cipherText)
	if err != nil {
		return nil, err
	}

	masterKey, ok := s.masterKeys[masterKeyID]
	if !ok {
		return nil, fmt.Errorf("e2ee: master key %s not loaded", masterKeyID)
	}

	if method != MethodFileV1 {
		return nil, fmt.Errorf("e2ee: unsupported file encryption method %d", method)
	}

	var result []byte
	for _, chunk := range chunks {
		decrypted, err := decryptChunk(masterKey, chunk, 3)
		if err != nil {
			return nil, fmt.Errorf("e2ee: chunk decryption failed: %w", err)
		}
		result = append(result, decrypted...)
	}

	return result, nil
}

// parseJED01 parses the JED01 envelope format:
// "JED01" + 6-char hex metadata length + metadata (2-char hex method + 32-char hex master key ID) + chunks
// Each chunk: 6-char hex length + chunk data
func parseJED01(s string) (masterKeyID string, method int, chunks [][]byte, err error) {
	if len(s) < 5 || s[:5] != "JED01" {
		return "", 0, nil, fmt.Errorf("e2ee: not a JED01 encrypted item")
	}

	pos := 5

	// 6-char hex metadata length
	if pos+6 > len(s) {
		return "", 0, nil, fmt.Errorf("e2ee: truncated JED01 header")
	}
	metaLen, err := strconv.ParseInt(s[pos:pos+6], 16, 64)
	if err != nil {
		return "", 0, nil, fmt.Errorf("e2ee: invalid metadata length: %w", err)
	}
	pos += 6

	if pos+int(metaLen) > len(s) {
		return "", 0, nil, fmt.Errorf("e2ee: truncated JED01 metadata")
	}
	metadata := s[pos : pos+int(metaLen)]
	pos += int(metaLen)

	// Metadata: 2-char hex method + 32-char hex master key ID
	if len(metadata) < 34 {
		return "", 0, nil, fmt.Errorf("e2ee: metadata too short")
	}
	methodInt, err := strconv.ParseInt(metadata[:2], 16, 64)
	if err != nil {
		return "", 0, nil, fmt.Errorf("e2ee: invalid encryption method: %w", err)
	}
	method = int(methodInt)
	masterKeyID = metadata[2:34]

	// Parse chunks
	for pos < len(s) {
		if pos+6 > len(s) {
			return "", 0, nil, fmt.Errorf("e2ee: truncated chunk header at pos %d", pos)
		}
		chunkLen, err := strconv.ParseInt(s[pos:pos+6], 16, 64)
		if err != nil {
			return "", 0, nil, fmt.Errorf("e2ee: invalid chunk length: %w", err)
		}
		pos += 6

		if pos+int(chunkLen) > len(s) {
			return "", 0, nil, fmt.Errorf("e2ee: truncated chunk data")
		}
		chunks = append(chunks, []byte(s[pos:pos+int(chunkLen)]))
		pos += int(chunkLen)
	}

	return masterKeyID, method, chunks, nil
}

// decryptChunk decrypts a single chunk using the master key.
// The chunk data is a JSON cipher packet. PBKDF2 iterations vary by method.
func decryptChunk(masterKey []byte, chunkData []byte, iterations int) ([]byte, error) {
	var pkt cipherPacket
	if err := json.Unmarshal(chunkData, &pkt); err != nil {
		return nil, fmt.Errorf("cannot parse chunk JSON: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(pkt.Salt)
	if err != nil {
		return nil, fmt.Errorf("cannot decode salt: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(pkt.IV)
	if err != nil {
		return nil, fmt.Errorf("cannot decode IV: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(pkt.CT)
	if err != nil {
		return nil, fmt.Errorf("cannot decode ciphertext: %w", err)
	}

	// Derive per-chunk encryption key from master key using PBKDF2-HMAC-SHA512
	derivedKey := pbkdf2.Key(masterKey, salt, iterations, 32, sha512.New)

	return decryptAESGCM(derivedKey, iv, ct)
}

// decryptAESGCM decrypts using AES-256-GCM.
func decryptAESGCM(key, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// unescapePercentXX decodes %XX sequences (JavaScript unescape semantics; does not decode +).
func unescapePercentXX(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			out = append(out, hexByte(s[i+1], s[i+2]))
			i += 2
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')
}

func hexByte(a, b byte) byte {
	var n byte
	if a >= 'a' {
		n = (a - 'a' + 10) << 4
	} else if a >= 'A' {
		n = (a - 'A' + 10) << 4
	} else {
		n = (a - '0') << 4
	}
	if b >= 'a' {
		return n | (b - 'a' + 10)
	}
	if b >= 'A' {
		return n | (b - 'A' + 10)
	}
	return n | (b - '0')
}

// utf16LEToUTF8 converts UTF-16LE bytes to a Go string (UTF-8).
func utf16LEToUTF8(data []byte) string {
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	u16 := make([]uint16, len(data)/2)
	for i := range u16 {
		u16[i] = uint16(data[2*i]) | uint16(data[2*i+1])<<8
	}
	runes := utf16.Decode(u16)
	return string(runes)
}

// ChecksumMasterKey computes a checksum to verify the decrypted master key is correct.
// Joplin uses SHA-256 hex digest of the decrypted hex key string.
func ChecksumMasterKey(decryptedHex string) string {
	h := sha256.Sum256([]byte(decryptedHex))
	return hex.EncodeToString(h[:])
}

// MasterKeyIDs returns the IDs of all loaded master keys.
func (s *Service) MasterKeyIDs() []string {
	ids := make([]string, 0, len(s.masterKeys))
	for id := range s.masterKeys {
		ids = append(ids, id)
	}
	return ids
}

// DecryptStringOrFile decrypts a JED01 cipher text, auto-detecting the method.
func (s *Service) DecryptStringOrFile(cipherText string) (string, []byte, error) {
	if !strings.HasPrefix(cipherText, "JED01") {
		return "", nil, fmt.Errorf("e2ee: not a JED01 encrypted item")
	}

	masterKeyID, method, chunks, err := parseJED01(cipherText)
	if err != nil {
		return "", nil, err
	}

	masterKey, ok := s.masterKeys[masterKeyID]
	if !ok {
		return "", nil, fmt.Errorf("e2ee: master key %s not loaded", masterKeyID)
	}

	switch method {
	case MethodStringV1, MethodFileV1:
		var result []byte
		for _, chunk := range chunks {
			decrypted, err := decryptChunk(masterKey, chunk, 3)
			if err != nil {
				return "", nil, fmt.Errorf("e2ee: chunk decryption failed: %w", err)
			}
			result = append(result, decrypted...)
		}
		if method == MethodStringV1 {
			return utf16LEToUTF8(result), nil, nil
		}
		return "", result, nil

	case MethodSJCL1a, MethodSJCL1b:
		var sb []byte
		for _, chunk := range chunks {
			decrypted, err := sjclDecrypt(string(masterKey), string(chunk))
			if err != nil {
				return "", nil, fmt.Errorf("e2ee: sjcl chunk decryption failed: %w", err)
			}
			sb = append(sb, decrypted...)
		}
		return unescapePercentXX(string(sb)), nil, nil

	default:
		return "", nil, fmt.Errorf("e2ee: unsupported method %d for auto-detect", method)
	}
}
