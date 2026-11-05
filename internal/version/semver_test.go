package version

import "testing"

func TestParseAndCompare(t *testing.T) {
	good := map[string]Semver{"1.0.0": {1, 0, 0}, "v2.10.3": {2, 10, 3}, " 0.9.12 ": {0, 9, 12}}
	for in, want := range good {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Fatalf("Parse(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "1.0", "1.0.0-beta", "01.0.0", "1.0.0.0", "a.b.c", "-1.0.0", "1.0.x"} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("Parse(%q) accepted", bad)
		}
	}
	a, _ := Parse("1.9.0")
	b, _ := Parse("1.10.0")
	if a.Compare(b) != -1 || b.Compare(a) != 1 || a.Compare(a) != 0 {
		t.Fatal("numeric comparison failed")
	}
}

func TestIdentityFor(t *testing.T) {
	if IdentityFor("1.2.3") != "WMXID|com.techsavage1989.weightmaxer|1.2.3|END" {
		t.Fatal(IdentityFor("1.2.3"))
	}
	if Identity != IdentityFor(Version) {
		t.Fatal("default identity must match default version")
	}
}
