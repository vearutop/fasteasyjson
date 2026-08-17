package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateAndCheck is a round-trip, CLI-level self-check: it builds the
// real binary and drives it as an external caller would, since the whole
// point of this tool is CLI compatibility with the original easyjson plus
// the new -check/write-only-if-changed behavior.
func TestGenerateAndCheck(t *testing.T) {
	bin := buildBinary(t)

	// Created under testdata (inside this module), not t.TempDir(), so the
	// package path resolves the same way a real target file's would.
	dir, err := os.MkdirTemp("testdata", "pkg_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	src := filepath.Join(dir, "sample.go")
	writeSample(t, src, "First string")
	easyjsonPath := filepath.Join(dir, "sample_easyjson.go")

	out, err := runTool(bin, "-check", src)
	if err == nil {
		t.Fatalf("expected -check to fail on an ungenerated file, got: %s", out)
	}
	if !strings.Contains(out, "stale:") {
		t.Fatalf("expected a stale report, got: %s", out)
	}
	if _, err := os.Stat(easyjsonPath); !os.IsNotExist(err) {
		t.Fatal("-check must never write the generated file")
	}

	if out, err := runTool(bin, src); err != nil {
		t.Fatalf("generate failed: %s: %v", out, err)
	}
	before, err := os.ReadFile(easyjsonPath)
	if err != nil {
		t.Fatal(err)
	}

	if out, err := runTool(bin, "-check", src); err != nil {
		t.Fatalf("expected a clean check on freshly generated output, got %q: %v", out, err)
	}

	beforeInfo, err := os.Stat(easyjsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runTool(bin, src); err != nil {
		t.Fatalf("regenerate failed: %s: %v", out, err)
	}
	afterInfo, err := os.Stat(easyjsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("regenerating an already up-to-date file must not rewrite it")
	}

	writeSample(t, src, "First string\n\tSecond string")
	out, err = runTool(bin, "-check", src)
	if err == nil || !strings.Contains(out, "stale:") {
		t.Fatalf("expected stale after a struct change, got %q: %v", out, err)
	}
	after, err := os.ReadFile(easyjsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("-check must never write the generated file")
	}

	if out, err := runTool(bin, src); err != nil {
		t.Fatalf("regenerate after struct change failed: %s: %v", out, err)
	}
	after2, err := os.ReadFile(easyjsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after2) {
		t.Fatal("expected regenerated content to differ after the struct change")
	}
}

// TestGenerateBatchWithZeroStructPackage guards against a package that
// contributes no types to its batch (a file matched by an easyjson tool but
// with no annotated struct) making the launcher's import of that package
// unused, which fails the whole batch's build.
func TestGenerateBatchWithZeroStructPackage(t *testing.T) {
	bin := buildBinary(t)

	emptyDir, err := os.MkdirTemp("testdata", "pkgempty_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(emptyDir) }()
	emptySrc := filepath.Join(emptyDir, "empty.go")
	if err := os.WriteFile(emptySrc, []byte("package pkgempty\n\ntype Unannotated struct {\n\tField string\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fullDir, err := os.MkdirTemp("testdata", "pkgfull_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(fullDir) }()
	fullSrc := filepath.Join(fullDir, "sample.go")
	writeSample(t, fullSrc, "First string")

	if out, err := runTool(bin, emptySrc, fullSrc); err != nil {
		t.Fatalf("batch with a zero-struct package failed: %s: %v", out, err)
	}
}

// TestGenerateIndependentOfCWD guards against resolving the target package
// against the tool's own process working directory instead of the
// launcher's location: running from outside any module must not change
// what gets generated.
func TestGenerateIndependentOfCWD(t *testing.T) {
	bin := buildBinary(t)

	dir, err := os.MkdirTemp("testdata", "pkgcwd_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	src := filepath.Join(dir, "sample.go")
	writeSample(t, src, "First string")
	absSrc, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, absSrc)
	cmd.Dir = t.TempDir() // outside any module
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate from outside the module failed: %s: %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sample_easyjson.go")); err != nil {
		t.Fatalf("expected generated output next to the target: %v", err)
	}
}

func writeSample(t *testing.T, path, field string) {
	t.Helper()

	content := "package pkg\n\n//easyjson:json\ntype Sample struct {\n\t" + field + "\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "fasteasyjson")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s: %v", out, err)
	}
	return bin
}

func runTool(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
