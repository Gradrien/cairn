package prompted

import "testing"

func TestRecordSent(t *testing.T) {
	dir := t.TempDir()
	if Sent(dir, "pousse") {
		t.Fatal("nothing sent yet")
	}
	Record(dir, "Push autorisé :\n  pousse maintenant")
	if !Sent(dir, "Push autorisé : pousse maintenant ") {
		t.Fatal("same text, other spacing")
	}
	if Sent(dir, "push") {
		t.Fatal("other text")
	}
}
