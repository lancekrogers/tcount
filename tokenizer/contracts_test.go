package tokenizer

import "testing"

func TestBuiltInContractsExposeCompleteIdentities(t *testing.T) {
	providers := []Tokenizer{
		mustBPETokenizer(t, EncodingO200kBase),
		NewClaudeApproximator(),
		NewGeminiApproximator(),
		&SPMTokenizerWrapper{vocabDigest: vocabularyDigest([]byte("fixture-vocab"))},
	}
	for _, tokenizer := range providers {
		identity, ok := ContractOf(tokenizer)
		if !ok {
			t.Fatalf("%T does not expose a contract identity", tokenizer)
		}
		if identity.Method == "" || identity.Encoding == "" || identity.Implementation == "" || identity.NormalizationPolicy == "" || identity.SpecialTokenPolicy == "" {
			t.Fatalf("incomplete identity for %T: %+v", tokenizer, identity)
		}
	}
}

func TestModelAliasesShareEncodingContract(t *testing.T) {
	gpt4o, err := NewBPETokenizer("gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	gpt5, err := NewBPETokenizer("gpt-5")
	if err != nil {
		t.Fatal(err)
	}
	left, _ := ContractOf(gpt4o)
	right, _ := ContractOf(gpt5)
	if left != right {
		t.Fatalf("model aliases produced different contracts:\nleft=%+v\nright=%+v", left, right)
	}
	if left.Encoding != EncodingO200kBase {
		t.Fatalf("alias encoding = %q, want %q", left.Encoding, EncodingO200kBase)
	}
}

func TestDifferentEncodingsHaveDifferentContracts(t *testing.T) {
	o200k := mustBPETokenizer(t, EncodingO200kBase)
	cl100k := mustBPETokenizer(t, EncodingCL100kBase)
	left, _ := ContractOf(o200k)
	right, _ := ContractOf(cl100k)
	if left == right {
		t.Fatalf("different encodings shared contract: %+v", left)
	}
}

func TestImplementationRevisionChangesContract(t *testing.T) {
	identity, _ := ContractOf(mustBPETokenizer(t, EncodingO200kBase))
	changed := identity
	changed.Implementation = "bpe-v2"
	if identity == changed {
		t.Fatal("implementation revision change did not change contract")
	}
}

func TestVocabularyBytesChangeContract(t *testing.T) {
	left := sentencePieceContract(vocabularyDigest([]byte("vocab-a")))
	right := sentencePieceContract(vocabularyDigest([]byte("vocab-b")))
	if left == right {
		t.Fatal("vocabulary byte change did not change contract")
	}
	if left.VocabularyDigest == [32]byte{} || right.VocabularyDigest == [32]byte{} {
		t.Fatal("vocabulary digest was empty")
	}
}

func TestContractOfRejectsUnidentifiedTokenizer(t *testing.T) {
	identity, ok := ContractOf(unidentifiedTokenizer{})
	if ok || identity != (ContractIdentity{}) {
		t.Fatalf("unidentified tokenizer contract = %+v, ok=%t", identity, ok)
	}
}

func TestContractOfRejectsIncompleteIdentity(t *testing.T) {
	identity, ok := ContractOf(incompleteTokenizer{})
	if ok || identity != (ContractIdentity{}) {
		t.Fatalf("incomplete tokenizer contract = %+v, ok=%t", identity, ok)
	}
}

type unidentifiedTokenizer struct{}

func (unidentifiedTokenizer) CountTokens(string) (int, error) { return 0, nil }
func (unidentifiedTokenizer) Name() string                    { return "custom" }
func (unidentifiedTokenizer) DisplayName() string             { return "custom" }
func (unidentifiedTokenizer) IsExact() bool                   { return true }

type incompleteTokenizer struct{}

func (incompleteTokenizer) CountTokens(string) (int, error) { return 0, nil }
func (incompleteTokenizer) Name() string                    { return "incomplete" }
func (incompleteTokenizer) DisplayName() string             { return "incomplete" }
func (incompleteTokenizer) IsExact() bool                   { return true }
func (incompleteTokenizer) Contract() ContractIdentity      { return ContractIdentity{Method: "bpe"} }

func mustBPETokenizer(t *testing.T, encoding string) Tokenizer {
	t.Helper()
	tokenizer, err := NewBPETokenizerByEncoding(encoding)
	if err != nil {
		t.Fatal(err)
	}
	return tokenizer
}
