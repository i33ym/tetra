package status

import "testing"

func TestParse(t *testing.T) {
	for _, name := range []string{"pending", "processing", "done", "failed"} {
		s, err := Parse(name)
		if err != nil {
			t.Fatalf("Parse(%q): %v", name, err)
		}
		if s.String() != name {
			t.Errorf("String() = %q, want %q", s.String(), name)
		}
	}

	if _, err := Parse("bogus"); err == nil {
		t.Error("expected an error for an unknown status")
	}
}

func TestEqual(t *testing.T) {
	if !Pending.Equal(MustParse("pending")) {
		t.Error("Pending should equal parsed pending")
	}
	if Done.Equal(Failed) {
		t.Error("Done should not equal Failed")
	}
}
