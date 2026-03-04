package restruct

import (
	"context"
	"net/http"
	"testing"
)

func TestGetSetValues(t *testing.T) {
	r := &http.Request{}
	r2 := SetValue(r, "a", "1")
	a := GetValue(r, "a")
	if a == "1" {
		t.Errorf("Want a to blank got 1 %v", a)
	}
	a = GetValue(r2, "a")
	if a != "1" {
		t.Errorf("Want a to be 1 got %v", a)
	}
	r = r2
	r = SetValue(r, "a", "2")
	r = SetValue(r, "b", "c")
	vals := GetValues(r)
	if vals["a"] != "2" {
		t.Errorf("Want a to be 2 got %v", vals["a"])
	}
	if vals["b"] != "c" {
		t.Errorf("Want b to be c got %v", vals["b"])
	}

	if x, ok := GetValue(r, "hello").(string); ok {
		t.Errorf("X %v %v", x, ok)
	}
}

func TestGetSetVals(t *testing.T) {
	ctx := context.Background()
	c2 := SetVal(ctx, "a", "1")
	a := GetVal(ctx, "a")
	if a == "1" {
		t.Errorf("Want a to blank got 1 %v", a)
	}
	a = GetVal(c2, "a")
	if a != "1" {
		t.Errorf("Want a to be 1 got %v", a)
	}
	ctx = c2
	ctx = SetVal(ctx, "a", "2")
	ctx = SetVal(ctx, "b", "c")
	vals := GetVals(ctx)
	if vals["a"] != "2" {
		t.Errorf("Want a to be 2 got %v", vals["a"])
	}
	if vals["b"] != "c" {
		t.Errorf("Want b to be c got %v", vals["b"])
	}

	if x, ok := GetVal(ctx, "hello").(string); ok {
		t.Errorf("X %v %v", x, ok)
	}
}

func TestSetVars(t *testing.T) {
	ctx := context.Background()
	params := map[string]string{"id": "42", "slug": "hello-world"}
	ctx = SetVars(ctx, params)
	got := Vars(ctx)
	if got["id"] != "42" {
		t.Errorf("Want id=42, got %v", got["id"])
	}
	if got["slug"] != "hello-world" {
		t.Errorf("Want slug=hello-world, got %v", got["slug"])
	}
	// empty context should return empty map
	empty := Vars(context.Background())
	if len(empty) != 0 {
		t.Errorf("Want empty map, got %v", empty)
	}
}

func TestSetParams(t *testing.T) {
	r, _ := http.NewRequest("GET", "/test/42", nil)
	params := map[string]string{"id": "42"}
	r = SetParams(r, params)
	got := Params(r)
	if got["id"] != "42" {
		t.Errorf("Want id=42, got %v", got["id"])
	}
}
