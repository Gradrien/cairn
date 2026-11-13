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

// Claude Code wraps a long prompt typed in one go in a pasted_content tag:
// cairn's own report of a failed manual step ("… Prêt à pousser …") came
// back that way and read as the human asking for a push.
func TestSentThroughPasteWrapper(t *testing.T) {
	dir := t.TempDir()
	text := "Test à la main de YAK-1 : l'étape 4 ne marche pas.\nRemets l'attente (`attente: Prêt à pousser — …` si c'est prêt)."
	Record(dir, text)
	if !Sent(dir, "<pasted_content id=\"1909\">\n"+text+"\n</pasted_content id=\"1909\">") {
		t.Fatal("the same prompt, wrapped as a paste, is cairn's")
	}
}
