//go:build aud17realupgrade

package cli

import (
	"context"
	"testing"
)

func TestRealUpgradeCommandRunnerReturnsSchemaFingerprint(t *testing.T) {
	output, err := (realUpgradeCommandRunner{}).Output(context.Background(), "", "docker", "compose", "run", "migrate", "schema-fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != realUpgradeSchemaFingerprint+"\n" {
		t.Fatalf("schema fingerprint output = %q", output)
	}

	output, err = (realUpgradeCommandRunner{}).Output(context.Background(), "", "docker", "compose", "ps")
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 0 {
		t.Fatalf("unhandled Docker output = %q, want empty", output)
	}
}
