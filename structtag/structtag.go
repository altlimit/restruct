package structtag

import (
	"crypto/sha1"
	"encoding/hex"
	"reflect"
	"strings"
	"sync"
)

var (
	tagsCache sync.Map
)

type StructField struct {
	Tag   string
	Index int
	Tags  map[string]string
}

func (sf *StructField) Value(tag string) (v string, ok bool) {
	v, ok = sf.Tags[tag]
	return
}

func NewStructField(index int, tag string) *StructField {
	tags := strings.Split(tag, ",")
	sf := &StructField{Index: index, Tags: make(map[string]string)}
	for i := 0; i < len(tags); i++ {
		t := strings.Split(tags[i], "=")
		var val string
		if len(t) > 1 {
			val = t[1]
		}
		if i == 0 {
			sf.Tag = t[0]
		}
		sf.Tags[t[0]] = val
	}
	return sf
}

func GetFieldsByTag(i interface{}, tag string) []*StructField {
	t := reflect.TypeOf(i)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	cKey := t.String() + ":" + tag
	if len(cKey) > 50 {
		h := sha1.New()
		h.Write([]byte(cKey))
		cKey = hex.EncodeToString(h.Sum(nil))
	}
	cache, ok := tagsCache.Load(cKey)
	if !ok {
		var fields []*StructField
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			m := f.Tag.Get(tag)
			if m != "" {
				fields = append(fields, NewStructField(i, m))
			}
		}
		tagsCache.Store(cKey, fields)
		cache = fields
	}

	return cache.([]*StructField)
}
