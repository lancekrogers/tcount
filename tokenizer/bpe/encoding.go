package bpe

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
)

// Special token constants.
const (
	EndOfText   = "<|endoftext|>"
	FIMPrefix   = "<|fim_prefix|>"
	FIMMiddle   = "<|fim_middle|>"
	FIMSuffix   = "<|fim_suffix|>"
	EndOfPrompt = "<|endofprompt|>"
)

// Encoding names.
const (
	EncodingO200kBase  = "o200k_base"
	EncodingCL100kBase = "cl100k_base"
	EncodingP50kBase   = "p50k_base"
	EncodingP50kEdit   = "p50k_edit"
	EncodingR50kBase   = "r50k_base"
)

// Definition holds the specification for a BPE encoding scheme.
type Definition struct {
	Name           string
	PatStr         string
	MergeableRanks map[string]int
	SpecialTokens  map[string]int
	ExplicitNVocab int
}

var (
	definitionCache = make(map[string]*Definition)
	mu              sync.RWMutex
)

func getDefinition(encodingName string) (*Definition, error) {
	mu.RLock()
	def, ok := definitionCache[encodingName]
	mu.RUnlock()
	if ok {
		return def, nil
	}

	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring write lock.
	if def, ok := definitionCache[encodingName]; ok {
		return def, nil
	}

	def, err := initDefinition(encodingName)
	if err != nil {
		return nil, err
	}
	definitionCache[encodingName] = def
	return def, nil
}

func initDefinition(encodingName string) (*Definition, error) {
	switch encodingName {
	case EncodingO200kBase:
		return o200kBase()
	case EncodingCL100kBase:
		return cl100kBase()
	case EncodingP50kBase:
		return p50kBase()
	case EncodingR50kBase:
		return r50kBase()
	case EncodingP50kEdit:
		return p50kEdit()
	default:
		return nil, errors.New("unknown encoding: " + encodingName)
	}
}

func o200kBase() (*Definition, error) {
	ranks, err := loadEmbeddedVocab(EncodingO200kBase)
	if err != nil {
		return nil, err
	}
	pats := []string{
		`[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?`,
		`[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]+[\p{Ll}\p{Lm}\p{Lo}\p{M}]*(?i:'s|'t|'re|'ve|'m|'ll|'d)?`,
		`\p{N}{1,3}`,
		` ?[^\s\p{L}\p{N}]+[\r\n/]*`,
		`\s*[\r\n]+`,
		`\s+(?!\S)`,
		`\s+`,
	}
	return &Definition{
		Name:           EncodingO200kBase,
		PatStr:         strings.Join(pats, "|"),
		MergeableRanks: ranks,
		SpecialTokens:  map[string]int{EndOfText: 199999, EndOfPrompt: 200018},
	}, nil
}

func cl100kBase() (*Definition, error) {
	ranks, err := loadEmbeddedVocab(EncodingCL100kBase)
	if err != nil {
		return nil, err
	}
	return &Definition{
		Name:           EncodingCL100kBase,
		PatStr:         `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+(?!\S)|\s+`,
		MergeableRanks: ranks,
		SpecialTokens: map[string]int{
			EndOfText: 100257, FIMPrefix: 100258,
			FIMMiddle: 100259, FIMSuffix: 100260,
			EndOfPrompt: 100276,
		},
	}, nil
}

func p50kBase() (*Definition, error) {
	ranks, err := loadEmbeddedVocab(EncodingP50kBase)
	if err != nil {
		return nil, err
	}
	return &Definition{
		Name:           EncodingP50kBase,
		PatStr:         `'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+(?!\S)|\s+`,
		MergeableRanks: ranks,
		SpecialTokens:  map[string]int{EndOfText: 50256},
		ExplicitNVocab: 50281,
	}, nil
}

func p50kEdit() (*Definition, error) {
	ranks, err := loadEmbeddedVocab(EncodingP50kBase)
	if err != nil {
		return nil, err
	}
	return &Definition{
		Name:           EncodingP50kEdit,
		PatStr:         `'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+(?!\S)|\s+`,
		MergeableRanks: ranks,
		SpecialTokens:  map[string]int{EndOfText: 50256, FIMPrefix: 50281, FIMMiddle: 50282, FIMSuffix: 50283},
	}, nil
}

func r50kBase() (*Definition, error) {
	ranks, err := loadEmbeddedVocab(EncodingR50kBase)
	if err != nil {
		return nil, err
	}
	return &Definition{
		Name:           EncodingR50kBase,
		PatStr:         `'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+(?!\S)|\s+`,
		MergeableRanks: ranks,
		SpecialTokens:  map[string]int{EndOfText: 50256},
		ExplicitNVocab: 50257,
	}, nil
}

// NewEncoderByName returns a BPETokenizer for the named encoding.
func NewEncoderByName(encodingName string) (*BPETokenizer, error) {
	def, err := getDefinition(encodingName)
	if err != nil {
		return nil, err
	}
	enc, err := NewEncoder(def.MergeableRanks, def.SpecialTokens, def.PatStr)
	if err != nil {
		return nil, err
	}
	specialTokensSet := make(map[string]any, len(def.SpecialTokens))
	for k := range def.SpecialTokens {
		specialTokensSet[k] = true
	}
	return newBPETokenizer(enc, def, specialTokensSet), nil
}

// BPETokenizer is the main tokenizer that wraps a BPE Encoder.
type BPETokenizer struct {
	encoder          *Encoder
	definition       *Definition
	specialTokensSet map[string]any
}

func newBPETokenizer(encoder *Encoder, definition *Definition, specialTokensSet map[string]any) *BPETokenizer {
	return &BPETokenizer{
		encoder:          encoder,
		definition:       definition,
		specialTokensSet: specialTokensSet,
	}
}

// Encode tokenizes text with optional special token handling.
// Returns an error if text contains a disallowed special token.
func (tok *BPETokenizer) Encode(text string, allowedSpecial []string, disallowedSpecial []string) ([]int, error) {
	var allowedSpecialSet map[string]any
	if len(allowedSpecial) == 0 {
		allowedSpecialSet = map[string]any{}
	} else if len(allowedSpecial) == 1 && allowedSpecial[0] == "all" {
		allowedSpecialSet = tok.specialTokensSet
	} else {
		allowedSpecialSet = make(map[string]any, len(allowedSpecial))
		for _, v := range allowedSpecial {
			allowedSpecialSet[v] = nil
		}
	}

	disallowedSpecialSet := make(map[string]any, len(disallowedSpecial))
	for _, v := range disallowedSpecial {
		disallowedSpecialSet[v] = nil
	}
	if len(disallowedSpecial) == 1 && disallowedSpecial[0] == "all" {
		disallowedSpecialSet = difference(tok.specialTokensSet, allowedSpecialSet)
	}

	if len(disallowedSpecialSet) > 0 {
		specialRegex := tok.specialTokenRegex(disallowedSpecialSet)
		m := findMatch(text, specialRegex)
		if m != "" {
			return nil, fmt.Errorf("text contains disallowed special token %q", m)
		}
	}

	tokens, _ := tok.encoder.encode(text, allowedSpecialSet)
	return tokens, nil
}

// EncodeOrdinary tokenizes text without special token handling.
func (tok *BPETokenizer) EncodeOrdinary(text string) []int {
	return tok.encoder.encodeOrdinary(text)
}

// Decode converts token IDs back to text.
func (tok *BPETokenizer) Decode(tokens []int) string {
	return string(tok.encoder.decode(tokens))
}

func (tok *BPETokenizer) specialTokenRegex(disallowedSpecialSet map[string]any) *regexp2.Regexp {
	strs := make([]string, 0, len(disallowedSpecialSet))
	for k := range disallowedSpecialSet {
		strs = append(strs, regexp.QuoteMeta(k))
	}
	return regexp2.MustCompile(strings.Join(strs, "|"), regexp2.None)
}

func findMatch(text string, reg *regexp2.Regexp) string {
	m, _ := reg.FindStringMatch(text)
	if m == nil {
		return ""
	}
	return m.String()
}

func difference(setA, setB map[string]any) map[string]any {
	result := make(map[string]any)
	for k := range setA {
		if _, ok := setB[k]; !ok {
			result[k] = true
		}
	}
	return result
}
