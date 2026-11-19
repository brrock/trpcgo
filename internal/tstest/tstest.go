// Package tstest type-checks and runs generated TypeScript in tests, using the
// compiler and Zod runtime locked in testdata/zodruntime, so no test depends on
// another project's installed versions.
package tstest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// StrictFlags are the type-checking options generated code must satisfy: the
// strictest a consuming project is likely to enable. skipLibCheck does not
// cover generated .ts files, so any option that rejects them breaks the user's
// build. Callers add their own emit and module options.
func StrictFlags() []string {
	return []string{
		"--strict",
		"--exactOptionalPropertyTypes",
		"--noUncheckedIndexedAccess",
		"--noPropertyAccessFromIndexSignature",
		"--noImplicitOverride",
		"--noImplicitReturns",
		"--noFallthroughCasesInSwitch",
		"--verbatimModuleSyntax",
		"--isolatedModules",
		"--erasableSyntaxOnly",
	}
}

// UnusedFlags reject unused declarations, as applications commonly do. They
// apply to generated modules; hand-written test scripts may keep unused
// type-level assertions.
func UnusedFlags() []string {
	return []string{"--noUnusedLocals", "--noUnusedParameters"}
}

// Run writes sources into a fresh directory that resolves the locked packages,
// type-checks every TypeScript file under StrictFlags and UnusedFlags and
// executes main with tsx.
// The test is skipped when the Node dependencies are not installed, except in
// CI, where their absence is a failure.
func Run(t testing.TB, sources map[string]string, main string) {
	t.Helper()
	modules := modulesDir(t)
	dir := t.TempDir()
	if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	var files []string
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, name)
	}
	slices.Sort(files)
	args := slices.Concat([]string{"--noEmit", "--skipLibCheck", "--target", "ES2022", "--module", "ES2022", "--moduleResolution", "bundler"}, StrictFlags(), UnusedFlags(), files)
	compile := exec.CommandContext(t.Context(), filepath.Join(modules, ".bin", "tsc"), args...)
	compile.Dir = dir
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("TypeScript does not compile: %v\n%s", err, output)
	}
	run := exec.CommandContext(t.Context(), filepath.Join(modules, ".bin", "tsx"), main)
	run.Dir = dir
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("TypeScript contract failed: %v\n%s", err, output)
	}
}

func modulesDir(t testing.TB) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	modules := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "zodruntime", "node_modules")
	if _, err := os.Stat(filepath.Join(modules, ".bin", "tsx")); err != nil {
		if os.Getenv("CI") != "" || !os.IsNotExist(err) {
			t.Fatal(err)
		}
		t.Skip("run npm ci --prefix testdata/zodruntime to execute TypeScript contracts")
	}
	return modules
}
