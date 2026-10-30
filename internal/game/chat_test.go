package game

import "testing"

func TestSay(t *testing.T) {
	s := lobby(t)
	if err := s.Say("nobody", "привет"); err != ErrNotInGame {
		t.Fatal(err)
	}
	if err := s.Say("p1", "  \n "); err == nil {
		t.Fatal("blank chat")
	}
	if err := s.Say("p1", "  привет  "); err != nil {
		t.Fatal(err)
	}
	if len(s.Chat) != 1 || s.Chat[0].Text != "привет" || s.Chat[0].Name != "Аня" {
		t.Fatalf("%#v", s.Chat)
	}
	long := make([]rune, 250)
	for i := range long {
		long[i] = 'а'
	}
	if err := s.Say("p1", string(long)); err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(s.Chat[1].Text)); got != 200 {
		t.Fatalf("len %d", got)
	}
	pub := Project(s)
	if len(pub.Chat) != 2 {
		t.Fatalf("public %#v", pub.Chat)
	}
}
