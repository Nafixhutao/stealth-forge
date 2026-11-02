package manageddb

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateNameTrimsAndBoundsTheDisplayName(t *testing.T) {
	name, err := ValidateName("  toko  ")
	if err != nil || name != "toko" {
		t.Fatalf("ValidateName() = %q, %v", name, err)
	}
	for _, value := range []string{"", "a", strings.Repeat("x", 121)} {
		if _, err := ValidateName(value); err == nil {
			t.Fatalf("ValidateName(%q) was accepted", value)
		}
	}
}

func TestGeneratedCredentialsStayInsideMySQLLimits(t *testing.T) {
	databaseName, username := generatedNames(uuid.Must(uuid.NewV7()))
	if len(databaseName) > 64 || len(username) > 32 {
		t.Fatalf("generated identifiers exceed MySQL limits: %q %q", databaseName, username)
	}
	if !strings.HasPrefix(databaseName, "mdb_") || !strings.HasPrefix(username, "u_") {
		t.Fatalf("generated identifiers have the wrong prefix: %q %q", databaseName, username)
	}
	password, err := randomPassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(password) < 40 || !safeMySQLString(password) {
		t.Fatalf("generated password is unusable: %q", password)
	}
	if safeMySQLString("has space") || safeMySQLString("has'quote") || safeMySQLString("") {
		t.Fatal("safeMySQLString accepted an unsafe value")
	}
	if got := quoteIdentifier("mdb_abc"); got != "`mdb_abc`" {
		t.Fatalf("quoteIdentifier() = %q", got)
	}
}
