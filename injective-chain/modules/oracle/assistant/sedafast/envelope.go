package sedafast

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// Envelope is the chain-internal representation of the top-level "result"
// field from a SEDA Fast WebSocket feed.result or REST /execute response.
// JSON tags match the SEDA wire format exactly.
type Envelope struct {
	Tag  string   `json:"_tag"`
	Data RespData `json:"data"`
}

// RespData maps "result.data" from the SEDA Fast response.
// Only signed fields are captured here; the unsigned top-level "result" mirror
// is intentionally omitted — price parsing uses DataResult.Result (the field
// that is covered by the dataResultId signature).
type RespData struct {
	ID          string      `json:"id"`
	RequestID   string      `json:"requestId"`
	DataRequest DataRequest `json:"dataRequest"`
	DataResult  DataResult  `json:"dataResult"`
	// Signature is a 65-byte hex string (no "0x" prefix, lowercase).
	Signature string `json:"signature"`
}

// DataRequest maps "result.data.dataRequest".
// The Go field FeedID corresponds to the SEDA wire field "execInputs" and is
// the stable on-chain identity of the feed.
type DataRequest struct {
	Version       string `json:"version"`
	ExecProgramID string `json:"execProgramId"`
	// FeedID is the chain-internal name for "execInputs". It is a hex string
	// identifying which feed this result belongs to.
	FeedID            string `json:"execInputs"`
	ExecGasLimit      string `json:"execGasLimit"`
	TallyProgramID    string `json:"tallyProgramId"`
	TallyInputs       string `json:"tallyInputs"`
	TallyGasLimit     string `json:"tallyGasLimit"`
	ReplicationFactor uint16 `json:"replicationFactor"`
	ConsensusFilter   string `json:"consensusFilter"`
	// GasPrice is a string-encoded BigInt.
	GasPrice string `json:"gasPrice"`
	Memo     string `json:"memo"`
}

// DataResult maps "result.data.dataResult".
type DataResult struct {
	Version   string `json:"version"`
	DrID      string `json:"drId"`
	Consensus bool   `json:"consensus"`
	ExitCode  uint32 `json:"exitCode"`
	// Result is hex-encoded bytes (the Oracle Program output), used only to
	// compute the dataResultId keccak256 hash. Not used for price parsing.
	Result string `json:"result"`
	// BlockHeight is always "0" for SEDA Fast (off-chain execution).
	BlockHeight string `json:"blockHeight"`
	// BlockTimestamp is milliseconds from epoch as a string-encoded uint64.
	BlockTimestamp string `json:"blockTimestamp"`
	// GasUsed is a string-encoded BigInt (up to 128-bit).
	GasUsed        string `json:"gasUsed"`
	PaybackAddress string `json:"paybackAddress"`
	SedaPayload    string `json:"sedaPayload"`
}

// ParseEnvelope parses a raw SEDA Fast result JSON envelope. It enforces the
// MaxSedaFastUpdateSize limit and rejects malformed JSON.
func ParseEnvelope(raw []byte) (*Envelope, error) {
	if len(raw) > types.MaxSedaFastUpdateSize {
		return nil, fmt.Errorf("seda fast update too large: %d > %d bytes", len(raw), types.MaxSedaFastUpdateSize)
	}
	if !utf8.Valid(raw) {
		return nil, errors.New("seda fast update is not valid UTF-8")
	}
	var env Envelope
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&env); err != nil {
		return nil, fmt.Errorf("seda fast envelope: %w", err)
	}
	return &env, nil
}

// DrID reconstructs the deterministic Data Request ID. Formula from:
// https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast/advanced-usage
//
//	drId = keccak256(
//	  keccak256(version) ||
//	  execProgramId(32 bytes) ||
//	  keccak256(execInputs) ||
//	  execGasLimit(8 bytes BE) ||
//	  tallyProgramId(32 bytes) ||
//	  keccak256(tallyInputs) ||
//	  tallyGasLimit(8 bytes BE) ||
//	  replicationFactor(2 bytes BE) ||
//	  keccak256(consensusFilter) ||
//	  gasPrice(16 bytes BE) ||
//	  keccak256(memo)
//	)
func (req *DataRequest) DrID() ([32]byte, error) {
	execProgramIDBytes, err := decodeHexField("execProgramId", req.ExecProgramID)
	if err != nil {
		return [32]byte{}, err
	}
	feedIDBytes, err := decodeHexField("execInputs/feedId", req.FeedID)
	if err != nil {
		return [32]byte{}, err
	}
	tallyProgramIDBytes, err := decodeHexField("tallyProgramId", req.TallyProgramID)
	if err != nil {
		return [32]byte{}, err
	}
	tallyInputsBytes, err := decodeHexField("tallyInputs", req.TallyInputs)
	if err != nil {
		return [32]byte{}, err
	}
	consensusFilterBytes, err := decodeHexField("consensusFilter", req.ConsensusFilter)
	if err != nil {
		return [32]byte{}, err
	}

	execGasLimit, err := parseUint64String("execGasLimit", req.ExecGasLimit)
	if err != nil {
		return [32]byte{}, err
	}
	tallyGasLimit, err := parseUint64String("tallyGasLimit", req.TallyGasLimit)
	if err != nil {
		return [32]byte{}, err
	}

	gasPriceBE, err := parseBigIntBE16("gasPrice", req.GasPrice)
	if err != nil {
		return [32]byte{}, err
	}

	var buf [8]byte

	preimage := make([]byte, 0, 256)
	preimage = append(preimage, crypto.Keccak256([]byte(req.Version))...)
	preimage = append(preimage, execProgramIDBytes...)
	preimage = append(preimage, crypto.Keccak256(feedIDBytes)...)
	binary.BigEndian.PutUint64(buf[:], execGasLimit)
	preimage = append(preimage, buf[:]...)
	preimage = append(preimage, tallyProgramIDBytes...)
	preimage = append(preimage, crypto.Keccak256(tallyInputsBytes)...)
	binary.BigEndian.PutUint64(buf[:], tallyGasLimit)
	preimage = append(preimage, buf[:]...)
	binary.BigEndian.PutUint16(buf[:2], req.ReplicationFactor)
	preimage = append(preimage, buf[:2]...)
	preimage = append(preimage, crypto.Keccak256(consensusFilterBytes)...)
	preimage = append(preimage, gasPriceBE...)
	preimage = append(preimage, crypto.Keccak256([]byte(req.Memo))...)

	return [32]byte(crypto.Keccak256(preimage)), nil
}

// DataResultID reconstructs the deterministic Data Result ID. Formula from:
// https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast/advanced-usage
//
//	dataResultId = keccak256(
//	  keccak256(version) ||
//	  drId(32 bytes) ||
//	  consensus(1 byte) ||
//	  exitCode(1 byte) ||
//	  keccak256(result) ||
//	  blockHeight(8 bytes BE) ||
//	  blockTimestamp(8 bytes BE) ||
//	  gasUsed(16 bytes BE) ||
//	  keccak256(paybackAddress) ||
//	  keccak256(sedaPayload)
//	)
func (res *DataResult) DataResultID() ([32]byte, error) {
	drIDBytes, err := decodeHexField("drId", res.DrID)
	if err != nil {
		return [32]byte{}, err
	}
	resultBytes, err := decodeHexField("result", res.Result)
	if err != nil {
		return [32]byte{}, err
	}
	paybackAddrBytes, err := decodeHexField("paybackAddress", res.PaybackAddress)
	if err != nil {
		return [32]byte{}, err
	}
	sedaPayloadBytes, err := decodeHexField("sedaPayload", res.SedaPayload)
	if err != nil {
		return [32]byte{}, err
	}

	blockHeight, err := parseUint64String("blockHeight", res.BlockHeight)
	if err != nil {
		return [32]byte{}, err
	}
	blockTimestamp, err := parseUint64String("blockTimestamp", res.BlockTimestamp)
	if err != nil {
		return [32]byte{}, err
	}
	gasUsedBE, err := parseBigIntBE16("gasUsed", res.GasUsed)
	if err != nil {
		return [32]byte{}, err
	}

	var consensusByte byte
	if res.Consensus {
		consensusByte = 1
	}

	var buf [8]byte

	preimage := make([]byte, 0, 256)
	preimage = append(preimage, crypto.Keccak256([]byte(res.Version))...)
	preimage = append(preimage, drIDBytes...)
	preimage = append(preimage, consensusByte, byte(res.ExitCode))
	preimage = append(preimage, crypto.Keccak256(resultBytes)...)
	binary.BigEndian.PutUint64(buf[:], blockHeight)
	preimage = append(preimage, buf[:]...)
	binary.BigEndian.PutUint64(buf[:], blockTimestamp)
	preimage = append(preimage, buf[:]...)
	preimage = append(preimage, gasUsedBE...)
	preimage = append(preimage, crypto.Keccak256(paybackAddrBytes)...)
	preimage = append(preimage, crypto.Keccak256(sedaPayloadBytes)...)

	return [32]byte(crypto.Keccak256(preimage)), nil
}

// VerifySignature verifies a 65-byte secp256k1 ECDSA signature against
// dataResultID using the given SEC1-encoded public key. It enforces low-S to
// prevent signature malleability.
func VerifySignature(pubkey, sig []byte, dataResultID [32]byte) error {
	if len(sig) != 65 {
		return fmt.Errorf("seda fast signature must be 65 bytes, got %d", len(sig))
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := sig[64]

	if v == 27 || v == 28 {
		v -= 27
	}
	if v != 0 && v != 1 {
		return fmt.Errorf("seda fast signature: invalid recovery id %d", v)
	}

	// Enforce low-S to prevent signature malleability (same policy as Stork / Peggy).
	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return errors.New("seda fast signature: values failed validation (high-S or out-of-range)")
	}

	// crypto.VerifySignature expects a 64-byte compact (r||s) and verifies
	// against the raw (unhashed) message.
	if !crypto.VerifySignature(pubkey, dataResultID[:], sig[:64]) {
		return errors.New("seda fast signature: verification failed")
	}
	return nil
}

// decodeHexField hex-decodes a named field that may or may not carry a "0x"
// prefix. An empty string is decoded as an empty byte slice (valid for
// optional fields like memo, paybackAddress, sedaPayload).
func decodeHexField(name, s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return []byte{}, nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("seda fast %s: invalid hex %q: %w", name, s, err)
	}
	return b, nil
}

// parseUint64String parses a string-encoded uint64 (used for blockHeight,
// blockTimestamp, execGasLimit, tallyGasLimit). Returns 0 for empty strings
// (blockHeight is always "0" for SEDA Fast off-chain executions).
func parseUint64String(name, s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		return 0, fmt.Errorf("seda fast %s: invalid uint64 string %q", name, s)
	}
	if !n.IsUint64() {
		return 0, fmt.Errorf("seda fast %s: value %q overflows uint64", name, s)
	}
	return n.Uint64(), nil
}

// parseBigIntBE16 parses a string-encoded non-negative integer into a 16-byte
// big-endian representation, used for gasUsed and gasPrice which can be up to
// 128-bit.
func parseBigIntBE16(name, s string) ([]byte, error) {
	if s == "" || s == "0" {
		return make([]byte, 16), nil
	}
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		return nil, fmt.Errorf("seda fast %s: invalid integer %q", name, s)
	}
	if n.Sign() < 0 {
		return nil, fmt.Errorf("seda fast %s: negative value not allowed", name)
	}
	b := n.Bytes()
	if len(b) > 16 {
		return nil, fmt.Errorf("seda fast %s: value %q exceeds 128 bits", name, s)
	}
	out := make([]byte, 16)
	copy(out[16-len(b):], b)
	return out, nil
}
