package golang

import (
	"reflect"
	"testing"
)

func TestTypesOfCompositeLiterals(t *testing.T) {
	f, err := New().Parse("a.go", []byte(`package a

func Enqueue() {
	args := jobs.SendArgs{ID: 1}
	c.Insert(ctx, &OtherArgs{}, nil)
	_ = []string{"x"}
	_ = map[string]int{}
	_ = jobs.SendArgs{}
	_ = Box[int]{}
}
`))
	if err != nil || len(f.Funcs) != 1 {
		t.Fatalf("parse: %v %v", err, f.Funcs)
	}
	want := []string{"SendArgs", "OtherArgs", "Box"}
	if !reflect.DeepEqual(f.Funcs[0].Types, want) {
		t.Errorf("Types = %v, want %v", f.Funcs[0].Types, want)
	}
}
