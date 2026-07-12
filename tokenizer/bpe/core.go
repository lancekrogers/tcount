package bpe

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/dlclark/regexp2"
)

// Encoder is the core BPE encoder/decoder.
type Encoder struct {
	encoder              map[string]int
	decoder              map[int]string
	specialTokensEncoder map[string]int
	specialTokensDecoder map[int]string
	splitRegex           *regexp2.Regexp
	specialRegex         *regexp2.Regexp
}

// NewEncoder creates a new Encoder from encoder maps and a regex pattern.
func NewEncoder(encoder map[string]int, specialTokensEncoder map[string]int, pattern string) (*Encoder, error) {
	regex, err := regexp2.Compile(pattern, regexp2.None)
	if err != nil {
		return nil, fmt.Errorf("compiling BPE split regex: %w", err)
	}

	specialRegexStrs := make([]string, 0, len(specialTokensEncoder))
	for k := range specialTokensEncoder {
		specialRegexStrs = append(specialRegexStrs, regexp.QuoteMeta(k))
	}
	specialRegex, err := regexp2.Compile(strings.Join(specialRegexStrs, "|"), regexp2.None)
	if err != nil {
		return nil, fmt.Errorf("compiling special token regex: %w", err)
	}

	decoder := make(map[int]string, len(encoder))
	for k, v := range encoder {
		decoder[v] = k
	}

	if len(encoder) != len(decoder) {
		return nil, errors.New("encoder and decoder map sizes are different")
	}

	specialTokensDecoder := make(map[int]string, len(specialTokensEncoder))
	for k, v := range specialTokensEncoder {
		specialTokensDecoder[v] = k
	}

	return &Encoder{
		encoder:              encoder,
		specialTokensEncoder: specialTokensEncoder,
		decoder:              decoder,
		specialTokensDecoder: specialTokensDecoder,
		splitRegex:           regex,
		specialRegex:         specialRegex,
	}, nil
}

func (enc *Encoder) encode(text string, allowedSpecial map[string]any) ([]int, int) {
	specialRegex := enc.specialRegex
	regex := enc.splitRegex
	textRunes := []rune(text)
	ret := make([]int, 0, len(textRunes)/3+1)
	lastPieceTokenLen := 0

	start := 0
	for {
		var nextSpecial []int
		startFind := start
		for {
			temp := cutRunes(textRunes, startFind, len(textRunes))
			nextSpecial = findMatchIndex(temp, specialRegex)
			if nextSpecial != nil {
				token := cutRunes(textRunes, startFind+nextSpecial[0], startFind+nextSpecial[1])
				if _, ok := allowedSpecial[token]; ok {
					break
				}
				startFind += nextSpecial[1]
			} else {
				break
			}
		}

		end := len(textRunes)
		if nextSpecial != nil {
			end = start + nextSpecial[0]
		}

		for _, mat := range findAllMatchIndices(cutRunes(textRunes, start, end), regex) {
			piece := cutRunes(textRunes, start+mat[0], start+mat[1])
			if token, ok := enc.encoder[piece]; ok {
				lastPieceTokenLen = 1
				ret = append(ret, token)
				continue
			}
			tokens := bytePairEncode([]byte(piece), enc.encoder)
			lastPieceTokenLen = len(tokens)
			ret = append(ret, tokens...)
		}

		if nextSpecial != nil {
			temp := cutRunes(textRunes, start+nextSpecial[0], start+nextSpecial[1])
			token := enc.specialTokensEncoder[temp]
			ret = append(ret, token)
			start = start + nextSpecial[1]
			lastPieceTokenLen = 0
		} else {
			break
		}
	}

	return ret, lastPieceTokenLen
}

func (enc *Encoder) encodeOrdinary(text string) []int {
	textRunes := []rune(text)
	ret := make([]int, 0, len(textRunes)/3+1)
	for _, mat := range findAllMatchIndices(text, enc.splitRegex) {
		piece := cutRunes(textRunes, mat[0], mat[1])
		if token, ok := enc.encoder[piece]; ok {
			ret = append(ret, token)
			continue
		}
		tokens := bytePairEncode([]byte(piece), enc.encoder)
		ret = append(ret, tokens...)
	}
	return ret
}

func (enc *Encoder) decode(tokens []int) []byte {
	ret := make([]byte, 0, len(tokens)*2)
	for _, token := range tokens {
		tokenBytes, ok := enc.decoder[token]
		if !ok {
			tokenBytes = enc.specialTokensDecoder[token]
		}
		if len(tokenBytes) > 0 {
			ret = append(ret, tokenBytes...)
		}
	}
	return ret
}

// findMatchIndex returns the index of the first match of reg in text.
// The error from FindStringMatch is safe to discard because all regexes
// are compiled and validated in NewEncoder; a successfully compiled
// regexp2 pattern will not error during matching.
func findMatchIndex(text string, reg *regexp2.Regexp) []int {
	m, _ := reg.FindStringMatch(text)
	if m == nil {
		return nil
	}
	return []int{m.Index, m.Index + m.Length}
}

// findAllMatchIndices returns indices of all non-overlapping matches.
// See findMatchIndex for why errors are safe to discard.
func findAllMatchIndices(text string, reg *regexp2.Regexp) [][]int {
	var matches [][]int
	m, _ := reg.FindStringMatch(text)
	for m != nil {
		matches = append(matches, []int{m.Index, m.Index + m.Length})
		m, _ = reg.FindNextMatch(m)
	}
	return matches
}

func cutRunes(runes []rune, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}
