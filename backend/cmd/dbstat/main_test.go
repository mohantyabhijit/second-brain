package main

import "testing"

func TestStrNormalizesNullableDatabaseValues(t *testing.T) {
	if got := str(nil); got != "" {
		t.Fatalf("str(nil) = %q", got)
	}
	value := "active"
	if got := str(&value); got != value {
		t.Fatalf("str(value) = %q", got)
	}
}
