package cache

import (
	"errors"
	"testing"
)

func TestPlanInvalidationMutationMatrix(t *testing.T) {
	textContract := ContractKey{Method: "bpe", Encoding: "o200k_base", Implementation: "bpe-v1"}
	otherContract := ContractKey{Method: "bpe", Encoding: "cl100k_base", Implementation: "bpe-v1"}
	base := testSnapshot("/workspace/project", FileResult{
		Size:           5,
		ModTimeNS:      10,
		ContentDigest:  [32]byte{1},
		Classification: ClassificationText,
		Characters:     5,
		Words:          1,
		Lines:          1,
		Methods:        map[ContractKey]int{textContract: 2},
	})

	tests := []struct {
		name      string
		mode      ValidationMode
		current   FileIdentity
		snapshot  *Snapshot
		contracts []ContractKey
		kind      DecisionKind
		reason    InvalidationReason
		reusable  int
		missing   int
	}{
		{name: "added file", mode: Metadata, current: identity("new.txt", 5, 10, [32]byte{1}, ClassificationText), snapshot: &base, contracts: []ContractKey{textContract}, kind: DecisionMiss, reason: ReasonEntryMissing, missing: 1},
		{name: "metadata hit", mode: Metadata, current: identity("main.txt", 5, 10, [32]byte{1}, ClassificationText), snapshot: &base, contracts: []ContractKey{textContract}, kind: DecisionHit, reason: ReasonMetadataAssumed, reusable: 1},
		{name: "preserved metadata rewrite metadata mode", mode: Metadata, current: identity("main.txt", 5, 10, [32]byte{2}, ClassificationText), snapshot: &base, contracts: []ContractKey{textContract}, kind: DecisionHit, reason: ReasonMetadataAssumed, reusable: 1},
		{name: "preserved metadata rewrite verified mode", mode: Verified, current: identity("main.txt", 5, 10, [32]byte{2}, ClassificationText), snapshot: &base, contracts: []ContractKey{textContract}, kind: DecisionMiss, reason: ReasonContentChanged, missing: 1},
		{name: "size edit", mode: Verified, current: identity("main.txt", 6, 10, [32]byte{1}, ClassificationText), snapshot: &base, contracts: []ContractKey{textContract}, kind: DecisionMiss, reason: ReasonSizeChanged, missing: 1},
		{name: "mtime edit", mode: Verified, current: identity("main.txt", 5, 11, [32]byte{1}, ClassificationText), snapshot: &base, contracts: []ContractKey{textContract}, kind: DecisionMiss, reason: ReasonModTimeChanged, missing: 1},
		{name: "classification edit", mode: Verified, current: identity("main.txt", 5, 10, [32]byte{1}, ClassificationBinary), snapshot: &base, contracts: []ContractKey{textContract}, kind: DecisionMiss, reason: ReasonClassificationChanged, missing: 1},
		{name: "additional encoding partial hit", mode: Verified, current: identity("main.txt", 5, 10, [32]byte{1}, ClassificationText), snapshot: &base, contracts: []ContractKey{textContract, otherContract}, kind: DecisionPartialHit, reason: ReasonContractMissing, reusable: 1, missing: 1},
		{name: "ratio change reuses primitives", mode: Verified, current: identity("main.txt", 5, 10, [32]byte{1}, ClassificationText), snapshot: &base, contracts: nil, kind: DecisionHit, reason: ReasonVerifiedMatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			currentPath := "main.txt"
			if test.name == "added file" {
				currentPath = "new.txt"
			}
			snapshot := test.snapshot
			if test.name == "added file" {
				empty := Snapshot{SchemaVersion: CurrentSchemaVersion, Root: "/workspace/project", Generation: 1, Entries: map[string]FileResult{}}
				snapshot = &empty
			}
			plan, err := PlanInvalidation(InvalidationRequest{Root: "/workspace/project", SchemaVersion: CurrentSchemaVersion, Mode: test.mode, Contracts: test.contracts}, map[string]FileIdentity{currentPath: test.current}, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Decisions) != 1 {
				t.Fatalf("decisions = %+v", plan.Decisions)
			}
			decision := plan.Decisions[0]
			if decision.Kind != test.kind || decision.Reason != test.reason || len(decision.ReusableMethods) != test.reusable || len(decision.MissingMethods) != test.missing {
				t.Fatalf("decision = %+v, want kind=%s reason=%s reusable=%d missing=%d", decision, test.kind, test.reason, test.reusable, test.missing)
			}
		})
	}
}

func TestPlanInvalidationMembershipHandlesDeleteAndRename(t *testing.T) {
	contract := ContractKey{Method: "bpe"}
	snapshot := Snapshot{SchemaVersion: CurrentSchemaVersion, Root: "/workspace/project", Generation: 1, Entries: map[string]FileResult{
		"old.txt": {Size: 1, ModTimeNS: 1, Classification: ClassificationText, Methods: map[ContractKey]int{contract: 1}},
	}}
	current := map[string]FileIdentity{
		"new.txt": identity("new.txt", 1, 1, [32]byte{1}, ClassificationText),
	}
	plan, err := PlanInvalidation(InvalidationRequest{Root: snapshot.Root, SchemaVersion: CurrentSchemaVersion, Mode: Verified, Contracts: []ContractKey{contract}}, current, &snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Decisions) != 2 || plan.Decisions[0].Path != "new.txt" || plan.Decisions[0].Kind != DecisionMiss || plan.Decisions[1].Path != "old.txt" || plan.Decisions[1].Kind != DecisionStale {
		t.Fatalf("rename plan = %+v", plan.Decisions)
	}
	if plan.Decisions[1].Reason != ReasonStaleEntry {
		t.Fatalf("stale reason = %s", plan.Decisions[1].Reason)
	}
}

func TestPlanInvalidationRejectsIncompatibleSnapshot(t *testing.T) {
	snapshot := testSnapshot("/workspace/project", FileResult{Size: 1, ModTimeNS: 1, Classification: ClassificationText})
	snapshot.SchemaVersion++
	current := map[string]FileIdentity{"main.txt": identity("main.txt", 1, 1, [32]byte{1}, ClassificationText)}
	plan, err := PlanInvalidation(InvalidationRequest{Root: snapshot.Root, SchemaVersion: CurrentSchemaVersion, Mode: Verified}, current, &snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Decisions) != 1 || plan.Decisions[0].Kind != DecisionMiss || plan.Decisions[0].Reason != ReasonSchemaMismatch {
		t.Fatalf("schema plan = %+v", plan.Decisions)
	}
}

func TestPlanInvalidationRejectsMalformedInputs(t *testing.T) {
	current := map[string]FileIdentity{"main.txt": identity("main.txt", 1, 1, [32]byte{1}, ClassificationText)}
	cases := []InvalidationRequest{
		{Root: "", SchemaVersion: CurrentSchemaVersion, Mode: Verified},
		{Root: "/workspace/project", SchemaVersion: 0, Mode: Verified},
		{Root: "/workspace/project", SchemaVersion: CurrentSchemaVersion, Mode: ValidationMode(99)},
	}
	for _, request := range cases {
		if _, err := PlanInvalidation(request, current, nil); !errors.Is(err, ErrInvalidationRequest) {
			t.Fatalf("request %+v error = %v, want ErrInvalidationRequest", request, err)
		}
	}
	badIdentity := current
	badIdentity["main.txt"] = identity("../outside.txt", 1, 1, [32]byte{1}, ClassificationText)
	if _, err := PlanInvalidation(InvalidationRequest{Root: "/workspace/project", SchemaVersion: CurrentSchemaVersion, Mode: Verified}, badIdentity, nil); !errors.Is(err, ErrInvalidationRequest) {
		t.Fatalf("bad identity error = %v, want ErrInvalidationRequest", err)
	}
}

func TestAggregateFreshAndReusableResultsMatch(t *testing.T) {
	contract := ContractKey{Method: "bpe", Encoding: "o200k_base"}
	fresh := []FileResult{
		{Size: 4, Characters: 4, Words: 1, Lines: 1, Classification: ClassificationText, Methods: map[ContractKey]int{contract: 2}},
		{Size: 6, Characters: 6, Words: 2, Lines: 1, Classification: ClassificationText, Methods: map[ContractKey]int{contract: 3}},
	}
	reusable := []FileResult{fresh[0], fresh[1]}
	freshAggregate, err := AggregateFileResults(fresh)
	if err != nil {
		t.Fatal(err)
	}
	reusableAggregate, err := AggregateFileResults(reusable)
	if err != nil {
		t.Fatal(err)
	}
	if freshAggregate.FileCount != 2 || freshAggregate.FileSize != 10 || freshAggregate.Characters != 10 || freshAggregate.Words != 3 || freshAggregate.Lines != 2 || freshAggregate.Methods[contract] != 5 {
		t.Fatalf("aggregate = %+v", freshAggregate)
	}
	if freshAggregate.FileCount != reusableAggregate.FileCount || freshAggregate.FileSize != reusableAggregate.FileSize || freshAggregate.Characters != reusableAggregate.Characters || freshAggregate.Words != reusableAggregate.Words || freshAggregate.Lines != reusableAggregate.Lines || freshAggregate.Methods[contract] != reusableAggregate.Methods[contract] {
		t.Fatalf("fresh=%+v reusable=%+v", freshAggregate, reusableAggregate)
	}
}

func TestAggregateRejectsOverflowAndNegativeValues(t *testing.T) {
	if _, err := AggregateFileResults([]FileResult{{Size: -1, Classification: ClassificationText}}); !errors.Is(err, ErrInvalidAggregate) {
		t.Fatalf("negative aggregate error = %v", err)
	}
	if _, err := AggregateFileResults([]FileResult{{Size: int64(^uint64(0) >> 1), Classification: ClassificationText}, {Size: 1, Classification: ClassificationText}}); !errors.Is(err, ErrAggregateOverflow) {
		t.Fatalf("overflow aggregate error = %v", err)
	}
	if _, err := AggregateFileResults([]FileResult{{Size: 1}}); !errors.Is(err, ErrInvalidAggregate) {
		t.Fatalf("invalid aggregate error = %v", err)
	}
}

func FuzzPlanInvalidationDoesNotPanic(f *testing.F) {
	f.Add("main.txt", int64(1), int64(2), uint8(Verified))
	f.Add("../outside", int64(-1), int64(0), uint8(99))
	f.Fuzz(func(t *testing.T, path string, size, modTime int64, mode uint8) {
		identity := FileIdentity{RelativePath: path, Size: size, ModTimeNS: modTime, Classification: ClassificationText}
		_, _ = PlanInvalidation(InvalidationRequest{Root: "/workspace/project", SchemaVersion: CurrentSchemaVersion, Mode: ValidationMode(mode)}, map[string]FileIdentity{path: identity}, nil)
	})
}

func testSnapshot(root string, entry FileResult) Snapshot {
	return Snapshot{SchemaVersion: CurrentSchemaVersion, Root: root, Generation: 1, Entries: map[string]FileResult{"main.txt": entry}}
}

func identity(path string, size, modTime int64, digest [32]byte, classification FileClassification) FileIdentity {
	return FileIdentity{RelativePath: path, Size: size, ModTimeNS: modTime, ContentDigest: digest, Classification: classification}
}
