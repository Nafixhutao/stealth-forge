package manageddbmaint

import (
	"strings"
	"testing"
)

func TestBackupPathRegexAcceptsOnlyValidRelativePaths(t *testing.T) {
	valid := []string{
		"mdb_0123456789abcdef01234567/2026-11-03T120000Z.sql.gz",
		"mdb_0123456789abcdef01234567/2026-11-03T120000Z.sql",
	}
	for _, path := range valid {
		if !backupPathRe.MatchString(path) {
			t.Fatalf("backupPathRe rejected valid path %q", path)
		}
	}
	invalid := []string{
		"../../etc/passwd",
		"/etc/passwd",
		"mdb_x/../secret",
		"mdb_0123456789abcdef01234567/2026.sql.gz",
		"tmp/plain.sql",
		"mdb_0123456789abcdef01234567/2026-11-03T120000Z.sql.gz\x00",
	}
	for _, path := range invalid {
		if backupPathRe.MatchString(path) {
			t.Fatalf("backupPathRe accepted invalid path %q", path)
		}
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	got := shellQuote("a'b")
	if got != `'a'\''b'` {
		t.Fatalf("shellQuote() = %q", got)
	}
	if shellQuote("plain") != "'plain'" {
		t.Fatalf("shellQuote dropped quotes")
	}
}

func TestQuoteIdentAndUser(t *testing.T) {
	if quoteIdent("mdb_x") != "`mdb_x`" {
		t.Fatalf("quoteIdent() = %q", quoteIdent("mdb_x"))
	}
	if quoteIdent("a`b") != "`a``b`" {
		t.Fatalf("quoteIdent did not escape backtick")
	}
	if quoteUser("u_x") != "'u_x'@'%'" {
		t.Fatalf("quoteUser() = %q", quoteUser("u_x"))
	}
	if quoteUser("u'x") != `'u''x'@'%'` {
		t.Fatalf("quoteUser did not escape quote")
	}
}

func TestTailAndBoundedError(t *testing.T) {
	long := strings.Repeat("x", 500)
	if got := tail(long); len(got) != 300 {
		t.Fatalf("tail() length = %d", len(got))
	}
	if tail("short") != "short" {
		t.Fatalf("tail() truncated a short string")
	}
	err := boundedError(&testError{parts: strings.Repeat("y", 3000)})
	if len(err) != 2000 {
		t.Fatalf("boundedError() length = %d", len(err))
	}
}

type testError struct{ parts string }

func (e *testError) Error() string { return e.parts }