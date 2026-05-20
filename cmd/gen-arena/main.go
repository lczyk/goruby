// gen-arena regenerates ast/arena_gen.go from ast/ast.go and ast/arena.go.
//
// The generator emits per-type Arena.NewX() constructors that bump-allocate
// out of the arena's slab[T] and explicitly clear every field on the
// returned slot. Slice fields are truncated with the `:0` pattern so
// backing arrays survive Arena.Reset and the next parse re-fills them in
// place -- the arena-reuse path's primary win.
//
// Target types come from the Arena struct: any `slab[T]` field declared
// there gets a corresponding `NewT() *T`. To enrol a new AST type in the
// arena, add the matching `slab[T]` line to Arena and rerun the generator.
//
//	go generate ./ast/...
//
// Flags:
//
//	-in <dir>   directory containing ast.go + arena.go (default ".")
//	-out <path> output file path (default "arena_gen.go", relative to -in)
//
// The output file starts with a "DO NOT EDIT" header. Hand-editing it is
// fine for one-off experiments but the next regen will clobber the
// changes.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/goruby/internal/version"
	ver "github.com/lczyk/version/go"
)

type Options struct {
	In      string `long:"in" description:"directory containing ast.go + arena.go" default:"." value-name:"DIR"`
	Out     string `long:"out" description:"output file path (relative to --in)" default:"arena_gen.go" value-name:"PATH"`
	Version bool   `short:"v" long:"version" description:"print version and exit"`
}

func main() {
	var opts Options
	p := flags.NewParser(&opts, flags.Default)
	if _, err := p.Parse(); err != nil {
		var fe *flags.Error
		if errors.As(err, &fe) && fe.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if opts.Version {
		fmt.Println(ver.FormatVersion(version.Version, version.CommitSHA, version.BuildDate, version.BuildInfo))
		return
	}
	inDir := &opts.In
	outPath := &opts.Out

	astPath := filepath.Join(*inDir, "ast.go")
	arenaPath := filepath.Join(*inDir, "arena.go")

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, astPath, nil, parser.ParseComments)
	if err != nil {
		fatalf("parse %s: %v", astPath, err)
	}
	arenaFile, err := parser.ParseFile(fset, arenaPath, nil, parser.ParseComments)
	if err != nil {
		fatalf("parse %s: %v", arenaPath, err)
	}

	// Index ast.go's struct types.
	structs := collectStructs(astFile)

	// Index ast.go's named non-primitive types that expose a Reset() method.
	// Used to dispatch field clears for embedded value types like
	// OrderedExprMap.
	hasResetMethod := collectResetMethods(astFile)

	// Walk Arena's slab[T] fields to derive the target list. Field name +
	// type parameter come from the source so the generator never drifts.
	targets := collectArenaSlabs(arenaFile)
	if len(targets) == 0 {
		fatalf("no slab[T] fields found on Arena in %s", arenaPath)
	}

	// Generate into a buffer, then gofmt + write.
	var buf bytes.Buffer
	emitHeader(&buf, *inDir)
	for _, t := range targets {
		st, ok := structs[t.typeName]
		if !ok {
			fatalf("Arena.%s references unknown type %s (not declared in %s)",
				t.slabField, t.typeName, astPath)
		}
		emitNewX(&buf, t, st, hasResetMethod)
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		// Emit unformatted output for debugging when gofmt rejects.
		_, _ = os.Stderr.Write(buf.Bytes())
		fatalf("gofmt: %v", err)
	}

	outFinal := filepath.Join(*inDir, *outPath)
	if err := os.WriteFile(outFinal, src, 0o644); err != nil {
		fatalf("write %s: %v", outFinal, err)
	}
}

// arenaTarget links an Arena slab field to its element type.
type arenaTarget struct {
	slabField string // e.g. "identifierSlab"
	typeName  string // e.g. "Identifier"
}

func collectStructs(f *ast.File) map[string]*ast.StructType {
	out := map[string]*ast.StructType{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			out[ts.Name.Name] = st
		}
	}
	return out
}

// collectResetMethods returns the set of named types in f that declare
// a `Reset()` method on a pointer receiver. Used so the generator can
// emit `.Reset()` for embedded value types (e.g. OrderedExprMap) that
// know how to truncate themselves in-place.
func collectResetMethods(f *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || fd.Name.Name != "Reset" {
			continue
		}
		if len(fd.Recv.List) != 1 {
			continue
		}
		var typeName string
		switch rt := fd.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if id, ok := rt.X.(*ast.Ident); ok {
				typeName = id.Name
			}
		case *ast.Ident:
			typeName = rt.Name
		}
		if typeName != "" {
			out[typeName] = true
		}
	}
	return out
}

// collectArenaSlabs scans arenaFile for `type Arena struct { ... }` and
// returns each `<fieldName> slab[T]` entry as an arenaTarget. Skips
// non-slab fields (LitPool etc.) so the generator only emits NewX for
// arena-managed types.
func collectArenaSlabs(arenaFile *ast.File) []arenaTarget {
	var out []arenaTarget
	for _, decl := range arenaFile.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			if ts.Name.Name != "Arena" {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, fld := range st.Fields.List {
				idx, ok := fld.Type.(*ast.IndexExpr)
				if !ok {
					continue
				}
				slabIdent, ok := idx.X.(*ast.Ident)
				if !ok || slabIdent.Name != "slab" {
					continue
				}
				typeIdent, ok := idx.Index.(*ast.Ident)
				if !ok {
					continue
				}
				for _, name := range fld.Names {
					out = append(out, arenaTarget{
						slabField: name.Name,
						typeName:  typeIdent.Name,
					})
				}
			}
		}
	}
	// Deterministic order: as declared in the Arena struct.
	return out
}

func emitHeader(buf *bytes.Buffer, inDir string) {
	fmt.Fprintln(buf, "// Code generated by cmd/gen-arena. DO NOT EDIT.")
	fmt.Fprintln(buf, "//")
	fmt.Fprintln(buf, "// Regenerate via `go generate ./ast/...` (or run cmd/gen-arena directly).")
	fmt.Fprintln(buf, "// The generator reads ast.go for struct field lists and arena.go for the")
	fmt.Fprintln(buf, "// Arena.slab[T] field list -- add a slab field there to enrol a new type.")
	fmt.Fprintln(buf)
	// Package name lifted from the Arena file's directory.
	pkgName := filepath.Base(absOrSelf(inDir))
	fmt.Fprintf(buf, "package %s\n\n", pkgName)
	fmt.Fprintln(buf, `import "github.com/lczyk/goruby/token"`)
	fmt.Fprintln(buf)
}

func absOrSelf(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func emitNewX(buf *bytes.Buffer, t arenaTarget, st *ast.StructType, hasReset map[string]bool) {
	const recv = "p" // uniform; avoids clash with Arena's `a`
	fmt.Fprintf(buf, "func (a *Arena) New%s() *%s {\n", t.typeName, t.typeName)
	fmt.Fprintf(buf, "\tif a == nil {\n\t\treturn new(%s)\n\t}\n", t.typeName)
	fmt.Fprintf(buf, "\t%s := arenaAlloc(&a.%s)\n", recv, t.slabField)
	for _, fld := range st.Fields.List {
		for _, name := range fld.Names {
			emitClear(buf, recv, name.Name, fld.Type, hasReset)
		}
		if len(fld.Names) == 0 {
			fmt.Fprintf(buf, "\t// TODO(arena-gen): embedded field %s\n", exprString(fld.Type))
		}
	}
	fmt.Fprintf(buf, "\treturn %s\n}\n\n", recv)
}

// emitClear writes a single zero-out statement for one field of T.
//
//   - slice:                p.F = p.F[:0]   (preserve backing cap)
//   - pointer / interface:  p.F = nil
//   - string:               p.F = ""
//   - bool:                 p.F = false
//   - int / float family:   p.F = 0
//   - token.Type (int32):   p.F = 0
//   - struct from another pkg (token.Token, big.Int, ...): p.F = pkg.Type{}
//   - same-package named type with Reset method:          p.F.Reset()
//   - same-package named type without Reset (struct):     p.F = Type{}
func emitClear(buf *bytes.Buffer, recv, field string, t ast.Expr, hasReset map[string]bool) {
	switch ty := t.(type) {
	case *ast.ArrayType:
		fmt.Fprintf(buf, "\t%s.%s = %s.%s[:0]\n", recv, field, recv, field)
	case *ast.StarExpr, *ast.MapType, *ast.InterfaceType:
		fmt.Fprintf(buf, "\t%s.%s = nil\n", recv, field)
	case *ast.Ident:
		switch ty.Name {
		case "string":
			fmt.Fprintf(buf, "\t%s.%s = \"\"\n", recv, field)
		case "bool":
			fmt.Fprintf(buf, "\t%s.%s = false\n", recv, field)
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"byte", "rune", "uintptr":
			fmt.Fprintf(buf, "\t%s.%s = 0\n", recv, field)
		case "float32", "float64":
			fmt.Fprintf(buf, "\t%s.%s = 0\n", recv, field)
		case "Expression", "Statement", "Node":
			fmt.Fprintf(buf, "\t%s.%s = nil\n", recv, field)
		default:
			if hasReset[ty.Name] {
				fmt.Fprintf(buf, "\t%s.%s.Reset()\n", recv, field)
			} else {
				// Same-package struct or named type; assume struct, zero
				// it via composite literal.
				fmt.Fprintf(buf, "\t%s.%s = %s{}\n", recv, field, ty.Name)
			}
		}
	case *ast.SelectorExpr:
		// Cross-package type: pkg.Name. Special-case token.Type since
		// it's a named int32, not a struct -- composite-literal zero
		// would fail to compile.
		x, _ := ty.X.(*ast.Ident)
		if x == nil {
			fmt.Fprintf(buf, "\t// TODO(arena-gen): clear %s: unsupported selector %s\n",
				field, exprString(t))
			return
		}
		if x.Name == "token" && ty.Sel.Name == "Type" {
			fmt.Fprintf(buf, "\t%s.%s = 0\n", recv, field)
		} else {
			fmt.Fprintf(buf, "\t%s.%s = %s.%s{}\n", recv, field, x.Name, ty.Sel.Name)
		}
	default:
		fmt.Fprintf(buf, "\t// TODO(arena-gen): clear %s: unsupported field type %s\n",
			field, exprString(t))
	}
}

func exprString(e ast.Expr) string {
	var buf bytes.Buffer
	_ = format.Node(&buf, token.NewFileSet(), e)
	return buf.String()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen-arena: "+format+"\n", args...)
	os.Exit(1)
}
