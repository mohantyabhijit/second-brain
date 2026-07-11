package main

import "testing"

func TestAsInt64AcceptsDriverIntegerShapes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
	}{
		{"int64", int64(42), 42},
		{"int", int(7), 7},
		{"float is rejected", 3.5, 0},
		{"nil is rejected", nil, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := asInt64(test.value); got != test.want {
				t.Fatalf("asInt64(%#v) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}
