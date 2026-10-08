package vars

import (
	"reflect"
	"regexp"
	"testing"
	"time"
)

func TestSubstitute(t *testing.T) {
	got, missing := Substitute("{{base}}/users/{{ id }}?x={{nope}}&y={{nope}}", map[string]string{"base": "http://h", "id": "7"})
	if got != "http://h/users/7?x={{nope}}&y={{nope}}" {
		t.Fatalf("got %q", got)
	}
	if !reflect.DeepEqual(missing, []string{"nope"}) {
		t.Fatalf("missing = %v", missing)
	}
}

func TestDynamic(t *testing.T) {
	Now = func() time.Time { return time.Unix(1700000000, 0) }
	defer func() { Now = time.Now }()

	got, missing := Substitute("{{$timestamp}} {{$isoTimestamp}}", nil)
	if got != "1700000000 2023-11-14T22:13:20Z" || missing != nil {
		t.Fatalf("got %q missing %v", got, missing)
	}
	got, _ = Substitute("{{$uuid}}", nil)
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(got) {
		t.Fatalf("bad uuid %q", got)
	}
	got, _ = Substitute("{{$randomInt}}", nil)
	if !regexp.MustCompile(`^\d{1,4}$`).MatchString(got) {
		t.Fatalf("bad int %q", got)
	}
}

func TestUserVariableShadowsDynamic(t *testing.T) {
	got, _ := Substitute("{{$uuid}}", map[string]string{"$uuid": "fixed"})
	if got != "fixed" {
		t.Fatalf("got %q", got)
	}
}
