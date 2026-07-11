package knowledge

import (
	"reflect"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func TestGraphSearchTermsNormalizesDeduplicatesAndDropsNoise(t *testing.T) {
	terms := graphSearchTerms("Authentication authentication and safe authorization boundaries")
	seen := map[string]bool{}
	for _, term := range terms {
		if seen[term] {
			t.Fatalf("duplicate term %q in %v", term, terms)
		}
		seen[term] = true
	}
	if !seen["authentication"] || !seen["authorization"] || seen["and"] {
		t.Fatalf("unexpected search terms: %v", terms)
	}
}

func TestGraphRecordConvertersHandleDriverValueShapes(t *testing.T) {
	record := &neo4j.Record{
		Keys:   []string{"string", "number", "list", "int64", "float32", "nil"},
		Values: []any{" value ", 42, []any{"first", " ", 3}, int64(7), float32(2.5), nil},
	}
	if got := stringRecordValue(record, "string"); got != " value " {
		t.Fatalf("string value = %q", got)
	}
	if got := stringRecordValue(record, "number"); got != "42" {
		t.Fatalf("formatted value = %q", got)
	}
	if got := stringSliceRecordValue(record, "list"); !reflect.DeepEqual(got, []string{"first", "3"}) {
		t.Fatalf("list value = %#v", got)
	}
	if got := floatRecordValue(record, "int64"); got != 7 {
		t.Fatalf("int64 value = %v", got)
	}
	if got := floatRecordValue(record, "float32"); got != 2.5 {
		t.Fatalf("float32 value = %v", got)
	}
	if stringRecordValue(record, "missing") != "" || stringSliceRecordValue(record, "nil") != nil || floatRecordValue(record, "string") != 0 {
		t.Fatal("expected safe zero values for missing or incompatible records")
	}
}

func TestNonEmptyStringsTrimsAndPreservesOrder(t *testing.T) {
	got := nonEmptyStrings([]string{" first ", "", "  ", "second"})
	if !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("nonEmptyStrings = %#v", got)
	}
}
