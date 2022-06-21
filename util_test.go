package restruct

import (
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
