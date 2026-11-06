package strictjson

import (
	"strings"
	"testing"
)

func TestRejectAmbiguousJSON(t *testing.T) {
	for _, data := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `[{"a":1,"\u0061":2}]`, `{} {}`, `null true`, `{"a":`, strings.Repeat("[", 34) + strings.Repeat("]", 34), "{\"a\":\"\xff\"}"} {
		if err := Validate([]byte(data), 262144); err == nil {
			t.Errorf("accepted %q", data)
		}
	}
}

func TestExactFields(t *testing.T) {
	type record struct {
		Count int `json:"count"`
	}
	for _, data := range []string{`{}`, `{"Count":1}`, `{"count":null}`, `{"count":1,"extra":2}`, `{"count":"1"}`, `{"count":1.0}`, `{"count":9223372036854775808}`, `null`, `[]`} {
		var result record
		if err := Decode([]byte(data), &result, "count"); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
	var result record
	if err := Decode([]byte(`{"count":1}`), &result, "count"); err != nil || result.Count != 1 {
		t.Fatal(result, err)
	}
}

func FuzzValidate(fuzzer *testing.F) {
	fuzzer.Add([]byte(`{"a":[1,true,null]}`))
	fuzzer.Fuzz(func(t *testing.T, data []byte) { _ = Validate(data, 4096) })
}
