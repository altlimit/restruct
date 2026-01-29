package structtag

import (
	"reflect"
	"strings"
	"sync"
)

var (
	tagsCache sync.Map
)

type cacheKey struct {
	typ reflect.Type
	tag string
}

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
	sf := &StructField{Index: index, Tags: make(map[string]string)}

	for tag != "" {
		// Skip leading accumulation (e.g. if we had "key1=val1,,key2=val2" or spaces)
		// but spec says comma separated.
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}

		// Find end of current tag item (comma)
		idx := strings.IndexByte(tag, ',')
		var item string
		if idx == -1 {
			item = tag
			tag = ""
		} else {
			item = tag[:idx]
			tag = tag[idx+1:]
		}

		// Split key=val
		eqIdx := strings.IndexByte(item, '=')
		var key, val string
		if eqIdx == -1 {
			key = strings.TrimSpace(item)
		} else {
			key = strings.TrimSpace(item[:eqIdx])
			val = strings.TrimSpace(item[eqIdx+1:])
		}

		if key == "" {
			continue
		}

		if sf.Tag == "" {
			sf.Tag = key
		}
		sf.Tags[key] = val
	}
	return sf
}

func GetFieldsByTag(i interface{}, tag string) []*StructField {
	t := reflect.TypeOf(i)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	cKey := cacheKey{
		typ: t,
		tag: tag,
	}

	if cache, ok := tagsCache.Load(cKey); ok {
		return cache.([]*StructField)
	}

	var fields []*StructField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		m := f.Tag.Get(tag)
		if m != "" {
			fields = append(fields, NewStructField(i, m))
		}
	}
	tagsCache.Store(cKey, fields)
	return fields

}
