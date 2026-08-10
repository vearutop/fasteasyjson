// Command fasteasyjson is a CLI-compatible drop-in for github.com/mailru/easyjson's
// easyjson command: same flags, same GOFILE/-pkg go:generate behavior, same
// generated output. It differs in how it gets there.
//
// bootstrap.Generator.Run (the original) writes a randomly-named temp
// launcher file next to the target on every invocation, to `go run` the
// actual generator. The compiler embeds a source file's on-disk path into
// the debug info of the binary it produces, so a randomly-suffixed
// launcher name makes that one compile+link action permanently
// un-cacheable: GOCACHE grows by one fresh, never-reused entry per file,
// on every single run, forever, even when nothing changed. Multiplied
// across every annotated file in a repo, on every CI run, that is what
// makes easyjson slow to run repeatedly against a persisted build cache.
// (Confirmed by elimination: an overlay-based fix that stopped writing to
// the target file at all still showed the same unbounded growth, until the
// launcher's name was also made deterministic - only then did it stop.)
// The original also writes the target *_easyjson.go file itself twice per
// invocation - a compile stub, then the final output - which is a separate
// correctness concern (a crashed run can leave a bare stub committed in
// place of working code) more than the performance one above.
//
// fasteasyjson gives the launcher a deterministic name (a hash of its
// group's target paths) instead of a random one, and serves each file's
// compile stub to the generator process through `go run -overlay=...`
// instead of writing it to the real path, and batches as many files as
// possible into as few overlays/`go run` invocations as possible, instead
// of one per file - all without changing the generated output. Every file
// with no `internal/` import constraint is batched into a single group
// regardless of how many distinct packages it spans (a launcher can import
// any number of non-`internal/` packages from anywhere on disk); files that
// do need an `internal/` package are grouped by their specific `internal/`
// ancestor directory, since a launcher can only reach such a package from
// somewhere under that package's parent-of-`internal` directory. The real
// file is written at most once, and only if its content actually changed
// (add -check to skip writing entirely and just report staleness, exit 1 if
// anything is out of date - a fast, non-destructive check for CI, see the
// mailru/easyjson issue this was built to avoid: repeated full-repo runs of
// the original generator with a persisted GOCACHE across CI jobs).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bool64/dev/version"
	"github.com/mailru/easyjson/bootstrap"
	_ "github.com/mailru/easyjson/jlexer"
	_ "github.com/mailru/easyjson/jwriter"
	"github.com/mailru/easyjson/parser"
)

const (
	genPackage = "github.com/mailru/easyjson/gen"
	pkgWriter  = "github.com/mailru/easyjson/jwriter"
	pkgLexer   = "github.com/mailru/easyjson/jlexer"
)

// splitMarker separates one target's generated output from the next in the
// launcher's combined stdout. It is written as a bare line by the launcher,
// before any content is formatted, so it does not need to be valid Go.
const splitMarker = "===FASTEASYJSON-SPLIT==="

var buildFlagsRegexp = regexp.MustCompile(`'.+'|".+"|\S+`)

// Flags mirror github.com/mailru/easyjson/easyjson's exactly, so scripts and
// //go:generate directives written for the original work unchanged, plus
// one addition: -check.
var (
	buildTags                = flag.String("build_tags", "", "build tags to add to generated file")
	genBuildFlags            = flag.String("gen_build_flags", "", "build flags when running the generator while bootstrapping")
	snakeCase                = flag.Bool("snake_case", false, "use snake_case names instead of CamelCase by default")
	lowerCamelCase           = flag.Bool("lower_camel_case", false, "use lowerCamelCase names instead of CamelCase by default")
	noStdMarshalers          = flag.Bool("no_std_marshalers", false, "don't generate MarshalJSON/UnmarshalJSON funcs")
	omitEmpty                = flag.Bool("omit_empty", false, "omit empty fields by default")
	omitZero                 = flag.Bool("omit_zero", false, "omit zero value fields by default")
	allStructs               = flag.Bool("all", false, "generate marshaler/unmarshalers for all structs in a file")
	simpleBytes              = flag.Bool("byte", false, "use simple bytes instead of Base64Bytes for slice of bytes")
	leaveTemps               = flag.Bool("leave_temps", false, "do not delete temporary files")
	stubsOnly                = flag.Bool("stubs", false, "only generate stubs for marshaler/unmarshaler funcs")
	noformat                 = flag.Bool("noformat", false, "do not run 'gofmt -w' on output file")
	specifiedName            = flag.String("output_filename", "", "specify the filename of the output")
	processPkg               = flag.Bool("pkg", false, "process the whole package instead of just the given file")
	disallowUnknownFields    = flag.Bool("disallow_unknown_fields", false, "return error if any unknown field in json appeared")
	skipMemberNameUnescaping = flag.Bool("disable_members_unescape", false, "don't perform unescaping of member names to improve performance")
	showVersion              = flag.Bool("version", false, "print version and exit")

	checkOnly = flag.Bool("check", false, "check that generated output matches what would be generated, without writing any file; "+
		"exit 1 and print each stale path if anything is out of date")
)

// target is one file's easyjson generation request: its own struct set and
// output path, within a package shared with other targets in its group.
type target struct {
	fname   string
	outName string
	pkgPath string
	pkgName string
	types   []string
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("fasteasyjson\nversion: %s\n", version.Info().Version)
		os.Exit(0)
	}

	files := flag.Args()
	gofile := os.Getenv("GOFILE")
	if *processPkg {
		gofile = filepath.Dir(gofile)
	}
	if len(files) == 0 && gofile != "" {
		files = []string{gofile}
	} else if len(files) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	targets := make([]*target, 0, len(files))
	for _, fname := range files {
		t, err := parseTarget(fname)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", fname, err)
			os.Exit(1)
		}
		targets = append(targets, t)
	}

	if *stubsOnly {
		for _, t := range targets {
			g := &bootstrap.Generator{
				BuildTags:       strings.TrimSpace(*buildTags),
				PkgName:         t.pkgName,
				Types:           t.types,
				NoStdMarshalers: *noStdMarshalers,
			}
			if err := os.WriteFile(t.outName, stubSource(g), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", t.fname, err)
				os.Exit(1)
			}
		}
		return
	}

	var publicGroup []*target
	var internalOrder []string
	internalGroups := map[string][]*target{}
	for _, t := range targets {
		if dir, ok := internalAncestorDir(t.pkgPath, t.outName); ok {
			if _, seen := internalGroups[dir]; !seen {
				internalOrder = append(internalOrder, dir)
			}
			internalGroups[dir] = append(internalGroups[dir], t)
		} else {
			publicGroup = append(publicGroup, t)
		}
	}

	var groups [][]*target
	var launcherDirs []string // "" for the public group: no internal/ constraint, launcher can live in a scratch dir
	if len(publicGroup) > 0 {
		groups = append(groups, publicGroup)
		launcherDirs = append(launcherDirs, "")
	}
	for _, dir := range internalOrder {
		groups = append(groups, internalGroups[dir])
		launcherDirs = append(launcherDirs, dir)
	}

	stale := false
	for i, group := range groups {
		generated, err := generateGroup(group, launcherDirs[i])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		for _, t := range group {
			out := generated[t.outName]
			current, err := os.ReadFile(t.outName)
			if err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "%s: %v\n", t.fname, err)
				os.Exit(1)
			}
			if bytes.Equal(current, out) {
				continue
			}
			if *checkOnly {
				stale = true
				fmt.Printf("stale: %s\n", t.outName)
				continue
			}
			if err := os.WriteFile(t.outName, out, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", t.fname, err)
				os.Exit(1)
			}
		}
	}
	if *checkOnly && stale {
		os.Exit(1)
	}
}

// parseTarget parses fname's struct set and computes its would-be *_easyjson.go path.
func parseTarget(fname string) (*target, error) {
	fInfo, err := os.Stat(fname)
	if err != nil {
		return nil, err
	}

	p := parser.Parser{AllStructs: *allStructs}
	if err := p.Parse(fname, fInfo.IsDir()); err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	var outName string
	if fInfo.IsDir() {
		outName = filepath.Join(fname, p.PkgName+"_easyjson.go")
	} else {
		outName = strings.TrimSuffix(fname, ".go") + "_easyjson.go"
	}
	if *specifiedName != "" {
		outName = *specifiedName
	}
	outName, err = filepath.Abs(outName)
	if err != nil {
		return nil, err
	}

	return &target{
		fname:   fname,
		outName: outName,
		pkgPath: p.PkgPath,
		pkgName: p.PkgName,
		types:   p.StructNames,
	}, nil
}

// internalAncestorDir reports the on-disk directory a launcher must live
// under to be allowed to import pkgPath, if pkgPath contains an "internal"
// path element - that directory is pkgPath's parent-of-"internal", found on
// disk by trimming the same number of path components off outName's own
// directory (Go import paths mirror on-disk directory structure below the
// module root, so the two walks match).
func internalAncestorDir(pkgPath, outName string) (string, bool) {
	segs := strings.Split(pkgPath, "/")
	idx := -1
	for i, s := range segs {
		if s == "internal" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", false
	}

	dir := filepath.Dir(outName)
	for i := 0; i < len(segs)-idx; i++ {
		dir = filepath.Dir(dir)
	}
	return dir, true
}

// generateGroup reproduces bootstrap.Generator.Run's output for every target
// in the group, without ever writing to any target's outName: every target's
// compile stub is served to the generator process through a single build
// overlay instead of being written to the real path, and the launcher runs
// all of the group's generators in one process (one per target, each in its
// own package if the group spans several), returning each target's output
// keyed by its outName.
//
// launcherDir is where the launcher file itself must be created: empty for
// a group with no `internal/`-constrained target (any directory works, so
// the scratch dir is reused), otherwise the on-disk directory computed by
// internalAncestorDir - Go's "internal/" import visibility is decided by
// the importing file's on-disk location, so a launcher outside that tree
// cannot import a target under .../internal/...
//
// The launcher's name is deterministic (a hash of the group's outNames),
// not random: the compiler embeds a source file's path into the debug info
// of the binary it produces, so a randomly suffixed name makes that one
// compile+link action un-cacheable - GOCACHE grows by one fresh,
// never-reused entry per group on every single run, forever, even though
// nothing changed.
func generateGroup(targets []*target, launcherDir string) (map[string][]byte, error) {
	hash := fnv.New32a()
	outNames := make([]string, len(targets))
	for i, t := range targets {
		outNames[i] = t.outName
	}
	sort.Strings(outNames)
	for _, n := range outNames {
		_, _ = hash.Write([]byte(n))
	}
	sum := hash.Sum32()

	scratch := filepath.Join(os.TempDir(), fmt.Sprintf("fasteasyjson-%x", sum))
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		return nil, err
	}
	if !*leaveTemps {
		defer func() { _ = os.RemoveAll(scratch) }()
	}

	base := &bootstrap.Generator{
		BuildTags:                strings.TrimSpace(*buildTags),
		GenBuildFlags:            strings.TrimSpace(*genBuildFlags),
		SnakeCase:                *snakeCase,
		LowerCamelCase:           *lowerCamelCase,
		NoStdMarshalers:          *noStdMarshalers,
		DisallowUnknownFields:    *disallowUnknownFields,
		SkipMemberNameUnescaping: *skipMemberNameUnescaping,
		OmitEmpty:                *omitEmpty,
		OmitZero:                 *omitZero,
		SimpleBytes:              *simpleBytes,
	}

	replace := map[string]string{}
	for i, t := range targets {
		stubPath := filepath.Join(scratch, fmt.Sprintf("stub-%d", i))
		g := *base
		g.PkgName, g.Types = t.pkgName, t.types
		if err := os.WriteFile(stubPath, stubSource(&g), 0o600); err != nil {
			return nil, err
		}
		replace[t.outName] = stubPath
	}

	overlay, err := json.Marshal(struct{ Replace map[string]string }{Replace: replace})
	if err != nil {
		return nil, err
	}
	overlayPath := filepath.Join(scratch, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0o600); err != nil {
		return nil, err
	}

	mainName := fmt.Sprintf("fasteasyjson-bootstrap-%x.go", sum)
	mainDir := launcherDir
	if mainDir == "" {
		mainDir = scratch
	}
	mainPath := filepath.Join(mainDir, mainName)
	if err := os.WriteFile(mainPath, mainSource(base, targets), 0o600); err != nil {
		return nil, err
	}
	if !*leaveTemps {
		defer func() { _ = os.Remove(mainPath) }()
	}

	cmdDir := launcherDir
	if cmdDir == "" {
		cmdDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	execArgs := []string{"run", "-trimpath", "-overlay=" + overlayPath}
	if base.GenBuildFlags != "" {
		execArgs = append(execArgs, buildFlagsRegexp.FindAllString(base.GenBuildFlags, -1)...)
	}
	if base.BuildTags != "" {
		execArgs = append(execArgs, "-tags", base.BuildTags)
	}
	execArgs = append(execArgs, mainPath)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", execArgs...)
	cmd.Dir = cmdDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}

	parts := strings.Split(stdout.String(), splitMarker+"\n")
	if len(parts) != len(targets)+1 {
		return nil, fmt.Errorf("expected %d generated blocks, got %d", len(targets), len(parts)-1)
	}

	result := make(map[string][]byte, len(targets))
	for i, t := range targets {
		raw := []byte(parts[i+1])
		if *noformat {
			result[t.outName] = raw
			continue
		}
		formatted, err := format.Source(raw)
		if err != nil {
			return nil, fmt.Errorf("formatting %s: %w", t.outName, err)
		}
		result[t.outName] = formatted
	}
	return result, nil
}

// stubSource mirrors bootstrap.Generator's private writeStub, producing the
// same content in memory instead of writing it to the real output file.
func stubSource(g *bootstrap.Generator) []byte {
	var buf bytes.Buffer

	if g.BuildTags != "" {
		fmt.Fprintln(&buf, "// +build ", g.BuildTags)
		fmt.Fprintln(&buf)
	}
	fmt.Fprintln(&buf, "// TEMPORARY AUTOGENERATED FILE: easyjson stub code to make the package")
	fmt.Fprintln(&buf, "// compilable during generation.")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "package ", g.PkgName)

	if len(g.Types) > 0 {
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf, "import (")
		fmt.Fprintln(&buf, `  "`+pkgWriter+`"`)
		fmt.Fprintln(&buf, `  "`+pkgLexer+`"`)
		fmt.Fprintln(&buf, ")")
	}

	types := sortedTypes(g.Types)
	for _, t := range types {
		fmt.Fprintln(&buf)
		if !g.NoStdMarshalers {
			fmt.Fprintln(&buf, "func (", t, ") MarshalJSON() ([]byte, error) { return nil, nil }")
			fmt.Fprintln(&buf, "func (*", t, ") UnmarshalJSON([]byte) error { return nil }")
		}

		fmt.Fprintln(&buf, "func (", t, ") MarshalEasyJSON(w *jwriter.Writer) {}")
		fmt.Fprintln(&buf, "func (*", t, ") UnmarshalEasyJSON(l *jlexer.Lexer) {}")
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf, "type EasyJSON_exporter_"+t+" *"+t)
	}

	return buf.Bytes()
}

// mainSource mirrors bootstrap.Generator's private writeMain, producing a
// launcher that runs one generator per target in the group - each target
// imported under its own alias, since a group may span several packages -
// and prints each target's result to stdout separated by splitMarker, so
// the results can be captured and told apart without a temp file next to
// each real output. base carries the flags shared by every target in the
// group (all are CLI-global); each target supplies its own package and type
// set.
func mainSource(base *bootstrap.Generator, targets []*target) []byte {
	var buf bytes.Buffer

	alias := map[string]string{}
	var pkgPaths []string
	for _, t := range targets {
		if _, ok := alias[t.pkgPath]; !ok {
			alias[t.pkgPath] = fmt.Sprintf("pkg%d", len(pkgPaths))
			pkgPaths = append(pkgPaths, t.pkgPath)
		}
	}

	fmt.Fprintln(&buf, "// +build ignore")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "// TEMPORARY AUTOGENERATED FILE: easyjson bootstrapping code to launch")
	fmt.Fprintln(&buf, "// the actual generator.")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "package main")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "import (")
	fmt.Fprintln(&buf, `  "fmt"`)
	fmt.Fprintln(&buf, `  "os"`)
	fmt.Fprintln(&buf)
	fmt.Fprintf(&buf, "  %q\n", genPackage)
	fmt.Fprintln(&buf)
	for _, p := range pkgPaths {
		fmt.Fprintf(&buf, "  %s %q\n", alias[p], p)
	}
	fmt.Fprintln(&buf, ")")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "func main() {")
	for _, t := range targets {
		fmt.Fprintf(&buf, "  fmt.Println(%q)\n", splitMarker)
		fmt.Fprintln(&buf, "  {")
		fmt.Fprintf(&buf, "    g := gen.NewGenerator(%q)\n", filepath.Base(t.outName))
		fmt.Fprintf(&buf, "    g.SetPkg(%q, %q)\n", t.pkgName, t.pkgPath)
		if base.BuildTags != "" {
			fmt.Fprintf(&buf, "    g.SetBuildTags(%q)\n", base.BuildTags)
		}
		if base.SnakeCase {
			fmt.Fprintln(&buf, "    g.UseSnakeCase()")
		}
		if base.LowerCamelCase {
			fmt.Fprintln(&buf, "    g.UseLowerCamelCase()")
		}
		if base.OmitEmpty {
			fmt.Fprintln(&buf, "    g.OmitEmpty()")
		}
		if base.OmitZero {
			fmt.Fprintln(&buf, "    g.OmitZero()")
		}
		if base.NoStdMarshalers {
			fmt.Fprintln(&buf, "    g.NoStdMarshalers()")
		}
		if base.DisallowUnknownFields {
			fmt.Fprintln(&buf, "    g.DisallowUnknownFields()")
		}
		if base.SimpleBytes {
			fmt.Fprintln(&buf, "    g.SimpleBytes()")
		}
		if base.SkipMemberNameUnescaping {
			fmt.Fprintln(&buf, "    g.SkipMemberNameUnescaping()")
		}
		for _, v := range sortedTypes(t.types) {
			fmt.Fprintln(&buf, "    g.Add("+alias[t.pkgPath]+".EasyJSON_exporter_"+v+"(nil))")
		}
		fmt.Fprintln(&buf, "    if err := g.Run(os.Stdout); err != nil {")
		fmt.Fprintln(&buf, "      fmt.Fprintln(os.Stderr, err)")
		fmt.Fprintln(&buf, "      os.Exit(1)")
		fmt.Fprintln(&buf, "    }")
		fmt.Fprintln(&buf, "  }")
	}
	fmt.Fprintln(&buf, "}")

	return buf.Bytes()
}

func sortedTypes(types []string) []string {
	out := append([]string(nil), types...)
	sort.Strings(out)
	return out
}
