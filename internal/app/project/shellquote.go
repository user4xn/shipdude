package project

import "strings"

// shellQuote membungkus satu nilai menjadi satu argumen POSIX shell yang utuh.
// Isi selalu diapit single quote, sehingga "$", backtick, ";", spasi, dan "&&"
// tidak pernah diinterpretasi shell. Single quote di dalam nilai di-escape
// dengan pola '\” (tutup kutip, kutip-lepas, buka kutip lagi).
//
// Catatan: fmt.Sprintf("%q", v) TIDAK aman untuk ini — %q menghasilkan
// double quote ala Go, dan "$(...)" atau backtick di dalam double quote tetap
// dieksekusi shell.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
