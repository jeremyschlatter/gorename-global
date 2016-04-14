// Command gorename-global is like gorename, but replaces multiple identifiers at once.
//
// The tradeoff is that gorename-global is less careful than gorename. It does not scan
// packages other than the ones you name on the command line. It does not check that
// the rename is safe.
//
// It is still safer than using sed, though. It will only replace Go identifiers that
// exactly match the -from argument.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go4.org/syncutil"

	"github.com/kisielk/gotool"
	"github.com/serenize/snaker"
)

var (
	from = flag.String("from", "", "the current name")
	to   = flag.String("to", "", "the new name")
	auto = flag.Bool("auto", false, "automatically change ALL_CAPS to CamelCase")
)

func main() {
	flag.Parse()
	if *auto && (*from != "" || *to != "") ||
		!*auto && (*from == "" || *to == "") {
		fmt.Fprintf(os.Stderr, "Usage: %s [-from <name> -to <name>] [-auto] [pkg...]\n", os.Args[0])
		os.Exit(1)
	}
	paths := gotool.ImportPaths(flag.Args())
	var wg syncutil.Group
	for _, p := range paths {
		p := p
		wg.Go(func() error {
			return renameIn(p)
		})
	}
	if errs := wg.Errs(); errs != nil {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func renameIn(pkgPath string) error {
	printerConf := printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 8,
	}
	pkg, err := build.Import(pkgPath, ".", 0)
	if err != nil {
		return err
	}
	var wg syncutil.Group
	for _, path := range pkg.GoFiles {
		path := filepath.Join(pkg.Dir, path)
		wg.Go(func() error {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			changed := false
			ast.Inspect(f, func(node ast.Node) bool {
				if i, ok := node.(*ast.Ident); ok {
					if *auto {
						n := autoFix(i.Name)
						if n != i.Name {
							changed = true
							i.Name = n
						}
					} else if i.Name == *from {
						changed = true
						i.Name = *to
					}
				}
				return true
			})
			if !changed {
				return nil
			}
			wc, err := os.Create(path)
			if err != nil {
				return err
			}
			defer wc.Close()
			return printerConf.Fprint(wc, fset, f)
		})
	}
	return wg.Err()
}

var (
	allCapsRE   = regexp.MustCompile(`^[A-Z0-9_]+$`)
	allUndersRE = regexp.MustCompile(`^_+$`)
)

func autoFix(name string) string {
	if allCapsRE.MatchString(name) && !allUndersRE.MatchString(name) && strings.Contains(name, "_") {
		return snaker.SnakeToCamel(strings.ToLower(name))
	}
	return name
}
