package structtag

import (
	"reflect"
	"testing"
)

func TestGetFieldsByTag(t *testing.T) {
	type hello struct {
		World string `json:"world" marshal:"f1"`
		X     string `json:"X"`
		Y     string `marshal:"f2,noindex=x"`
		Z     string `marshal:"f1,f2,f3=abc"`
	}
	h := &hello{World: "123"}
	for _, f := range GetFieldsByTag(h, "marshal") {
		k := f.Tag
		if f.Index == 0 {
			if k != "f1" {
				t.Errorf("wanted f1 got %s", k)
			}
			if _, ok := f.Value("noindex"); ok {
				t.Errorf("wanted noindex not found but found")
			}
		} else if f.Index == 2 {
			if k != "f2" {
				t.Errorf("wanted f2 got %s", k)
			}
			if _, ok := f.Value("noindex"); !ok {
				t.Errorf("wanted noindex but found")
			}
		} else if f.Index == 3 {
			if k != "f1" {
				t.Errorf("wanted f1 got %s", k)
			}
			if v, ok := f.Value("f2"); !ok || v != "" {
				t.Errorf("wanted f2 but found")
			}
			if v, ok := f.Value("f3"); !ok || v != "abc" {
				t.Errorf("wanted f3 but found or v != abc -> %s", v)
			}
		}
	}
	key := cacheKey{typ: reflect.TypeOf(h).Elem(), tag: "marshal"}
	if _, ok := tagsCache.Load(key); !ok {
		t.Errorf("No cache found")
	}
	z := &hello{World: "1235"}
	for _, f := range GetFieldsByTag(z, "marshal") {
		if f.Index == 0 {
			if _, ok := f.Value("noindex"); ok {
				t.Errorf("wanted noindex not found but found")
			}
		} else if f.Index == 2 {
			if v, ok := f.Value("noindex"); !ok {
				t.Errorf("wanted noindex but found")
			} else if v != "x" {
				t.Errorf("wanted x value found %s", v)
			}
		}
	}
}
