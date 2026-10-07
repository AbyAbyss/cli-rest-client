package script

import (
	"net/http"
	"testing"
	"time"
)

func TestRunPre(t *testing.T) {
	v := map[string]string{"a": "1", "gone": "x"}
	as, errs := RunPre("# comment\nset b = {{a}}-2\nset c = \"quoted value\"\nunset gone\nbogus line\nset = 3", v)
	if len(errs) != 2 {
		t.Fatalf("errs = %v", errs)
	}
	if v["b"] != "1-2" || v["c"] != "quoted value" {
		t.Fatalf("vars = %v", v)
	}
	if _, ok := v["gone"]; ok {
		t.Fatalf("unset failed")
	}
	if len(as) != 3 || !as[2].Unset {
		t.Fatalf("assignments = %+v", as)
	}
}

func TestRunTests(t *testing.T) {
	resp := Response{
		Status:   201,
		Headers:  http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body:     []byte(`{"id": 42, "name": "Aby Abyss", "ok": true, "items": [{"v": 1.50}, {"v": 2}], "nil": null, "obj": {"k": "v"}}`),
		Duration: 120 * time.Millisecond,
	}
	vars := map[string]string{"want": "42"}
	cases := map[string]bool{
		"status == 201":                     true,
		"status != 200":                     true,
		"status >= 200":                     true,
		"status < 300":                      true,
		"status == 200":                     false,
		"time < 500":                        true,
		"time > 500":                        false,
		"size > 10":                         true,
		"header content-type contains json": true,
		"header X-Missing exists":           false,
		"header X-Missing !exists":          true,
		`body contains "Aby Abyss"`:         true,
		"body !contains error":              true,
		"json.id == {{want}}":               true,
		"json.id == 42.0":                   true,
		`json.name == "Aby Abyss"`:          true,
		"json.name matches ^Aby":            true,
		"json.ok == true":                   true,
		"json.items[0].v == 1.5":            true,
		"json.items.1.v == 2":               true,
		"json.items.length == 2":            true,
		"json.items[5] exists":              false,
		"json.nil == null":                  true,
		`json.obj == {"k":"v"}`:             true,
		"json.missing exists":               false,
		"json.missing !exists":              true,
		"status":                            false,
		"status ==":                         false,
		"nonsense == 1":                     false,
		"json.name > 3":                     false,
	}
	for line, want := range cases {
		res := RunTests(line, resp, vars)
		if len(res) != 1 {
			t.Fatalf("%q: %d results", line, len(res))
		}
		if res[0].Passed != want {
			t.Errorf("%q: passed=%v want %v (%s)", line, res[0].Passed, want, res[0].Message)
		}
	}
}

func TestCaptures(t *testing.T) {
	resp := Response{Status: 200, Headers: http.Header{"Location": {"/x"}}, Body: []byte(`{"token": "abc", "n": 3}`)}
	vars := map[string]string{}
	res := RunTests("set tok = json.token\nset loc = header Location\nset st = status\nset bad = json.nope\nset = json.n", resp, vars)
	if len(res) != 5 {
		t.Fatalf("results = %+v", res)
	}
	if vars["tok"] != "abc" || vars["loc"] != "/x" || vars["st"] != "200" {
		t.Fatalf("vars = %v", vars)
	}
	if res[3].Passed || res[4].Passed {
		t.Fatalf("expected failures: %+v", res[3:])
	}
	if res[0].Capture == nil || res[0].Capture.Value != "abc" {
		t.Fatalf("capture missing")
	}
}

func TestNonJSONBody(t *testing.T) {
	res := RunTests("json.a exists", Response{Status: 200, Body: []byte("<html>")}, map[string]string{})
	if res[0].Passed || res[0].Message != "response body is not JSON" {
		t.Fatalf("got %+v", res[0])
	}
}
