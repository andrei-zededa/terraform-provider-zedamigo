// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

func TestEmbeddedOVMFVersionIsStable(t *testing.T) {
	a, err := embeddedOVMFVersion()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := embeddedOVMFVersion()
	if a != b || len(a) != 12 {
		t.Fatalf("expected a stable 12-char version, got %q and %q", a, b)
	}
}

func TestResolveOVMFExtractsToVersionedDir(t *testing.T) {
	ctx := context.Background()
	lib := t.TempDir()
	ver, err := embeddedOVMFVersion()
	if err != nil {
		t.Fatal(err)
	}

	// A firmware left behind by an older release at the old, unversioned path
	// must neither be used nor overwritten.
	old := filepath.Join(lib, "embedded_ovmf", "OVMF_CODE.fd")
	if err := os.MkdirAll(filepath.Dir(old), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old firmware"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, vars, err := resolveOVMF(ctx, exec.NewLocal(false), lib, "", "")
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(lib, "embedded_ovmf", ver)
	if filepath.Dir(code) != wantDir || filepath.Dir(vars) != wantDir {
		t.Fatalf("expected both files under %s, got %s and %s", wantDir, code, vars)
	}
	for i, p := range []string{code, vars} {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := embeddedOVMF.ReadFile(embeddedOVMFFiles[i])
		if len(got) != len(want) {
			t.Fatalf("%s: extracted %d bytes, embedded %d", p, len(got), len(want))
		}
	}
	if b, _ := os.ReadFile(old); string(b) != "old firmware" {
		t.Fatalf("old unversioned firmware was modified")
	}
}

func TestResolveOVMFOverrides(t *testing.T) {
	ctx := context.Background()
	lib := t.TempDir()
	code := filepath.Join(t.TempDir(), "MY_CODE.fd")
	vars := filepath.Join(t.TempDir(), "MY_VARS.fd")
	for _, p := range []string{code, vars} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gotCode, gotVars, err := resolveOVMF(ctx, exec.NewLocal(false), lib, code, vars)
	if err != nil {
		t.Fatal(err)
	}
	if gotCode != code || gotVars != vars {
		t.Fatalf("overrides not used: got %s, %s", gotCode, gotVars)
	}
	if _, err := os.Stat(filepath.Join(lib, "embedded_ovmf")); !os.IsNotExist(err) {
		t.Fatalf("nothing should be extracted when both overrides are set")
	}

	// Only the code overridden: vars still come from the embedded copy.
	gotCode, gotVars, err = resolveOVMF(ctx, exec.NewLocal(false), lib, code, "")
	if err != nil {
		t.Fatal(err)
	}
	if gotCode != code || filepath.Base(gotVars) != "OVMF_VARS.fd" {
		t.Fatalf("partial override: got %s, %s", gotCode, gotVars)
	}

	// A missing override is an error, not a silent fallback.
	if _, _, err := resolveOVMF(ctx, exec.NewLocal(false), lib, filepath.Join(lib, "missing.fd"), ""); err == nil {
		t.Fatalf("expected an error for a missing ovmf_code")
	}
}
