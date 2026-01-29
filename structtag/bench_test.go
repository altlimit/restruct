package structtag

import (
	"testing"
)

type benchStruct struct {
	Field1 string `json:"field1" custom:"val1,opt=1"`
	Field2 int    `json:"field2" custom:"val2,opt=2"`
	Field3 bool   `json:"field3" custom:"val3,opt=3"`
	Field4 string `json:"field4" custom:"val4,opt=4"`
	Field5 int    `json:"field5" custom:"val5,opt=5"`
}

func BenchmarkGetFieldsByTag(b *testing.B) {
	s := &benchStruct{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetFieldsByTag(s, "custom")
	}
}

func BenchmarkGetFieldsByTagParallel(b *testing.B) {
	s := &benchStruct{}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			GetFieldsByTag(s, "custom")
		}
	})
}
