// Command gorename-global is like gorename, but replaces multiple identifiers at once.
//
// The tradeoff is that gorename-global is less careful than gorename. It does not scan
// packages other than the ones you name on the command line. It does not check that
// the rename is safe.
//
// It is still safer than using sed, though. It will only replace Go identifiers that
// exactly match the --from argument.
//
// You can use the --auto flag to fix any identifier that 'go lint' would flag.
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
	"strings"
	"sync"
	"unicode"

	"go4.org/syncutil"

	"github.com/kisielk/gotool"
)

var (
	from = flag.String("from", "", "the current name")
	to   = flag.String("to", "", "the new name")
	auto = flag.Bool("auto", false, "automatically change any identifier flagged by 'go lint'")
)

var changeLog = struct {
	sync.Mutex
	m map[[2]string]bool
}{
	m: make(map[[2]string]bool),
}

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
	if len(changeLog.m) > 0 {
		fmt.Println("Changed:")
		for k := range changeLog.m {
			fmt.Printf("\t%s -> %s\n", k[0], k[1])
		}
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

	files := append(pkg.GoFiles, pkg.TestGoFiles...)
	files = append(files, pkg.XTestGoFiles...)

	for _, path := range files {
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
						n := lintName(i.Name)
						if n != i.Name {
							changeLog.Lock()
							changeLog.m[[2]string{i.Name, n}] = true
							changeLog.Unlock()
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

// Copied from go lint.
// lintName returns a different name if it should be different.
func lintName(name string) (should string) {
	// Fast path for simple cases: "_" and all lowercase.
	if name == "_" {
		return name
	}
	allLower := true
	for _, r := range name {
		if !unicode.IsLower(r) {
			allLower = false
			break
		}
	}
	if allLower {
		return name
	}

	// Split camelCase at any lower->upper transition, and split on underscores.
	// Check each word for common initialisms.
	runes := []rune(name)
	w, i := 0, 0 // index of start of word, scan
	for i+1 <= len(runes) {
		eow := false // whether we hit the end of a word
		if i+1 == len(runes) {
			eow = true
		} else if runes[i+1] == '_' {
			// underscore; shift the remainder forward over any run of underscores
			eow = true
			n := 1
			for i+n+1 < len(runes) && runes[i+n+1] == '_' {
				n++
			}

			// Leave at most one underscore if the underscore is between two digits
			if i+n+1 < len(runes) && unicode.IsDigit(runes[i]) && unicode.IsDigit(runes[i+n+1]) {
				n--
			}

			copy(runes[i+1:], runes[i+n+1:])
			runes = runes[:len(runes)-n]
		} else if unicode.IsLower(runes[i]) && !unicode.IsLower(runes[i+1]) {
			// lower->non-lower
			eow = true
		}
		i++
		if !eow {
			continue
		}

		// [w,i) is a word.
		word := string(runes[w:i])
		if u := strings.ToUpper(word); commonInitialisms[u] {
			// Keep consistent case, which is lowercase only at the start.
			if w == 0 && unicode.IsLower(runes[w]) {
				u = strings.ToLower(u)
			}
			// All the common initialisms are ASCII,
			// so we can replace the bytes exactly.
			copy(runes[w:], []rune(u))
		} else if w > 0 && strings.ToLower(word) == word {
			// already all lowercase, and not the first word, so uppercase the first character.
			runes[w] = unicode.ToUpper(runes[w])
		}
		w = i
	}
	return string(runes)
}

// Copied from go lint.
var commonInitialisms = map[string]bool{
	"API":   true,
	"ASCII": true,
	"CPU":   true,
	"CSS":   true,
	"DNS":   true,
	"EOF":   true,
	"GUID":  true,
	"HTML":  true,
	"HTTP":  true,
	"HTTPS": true,
	"ID":    true,
	"IP":    true,
	"JSON":  true,
	"LHS":   true,
	"QPS":   true,
	"RAM":   true,
	"RHS":   true,
	"RPC":   true,
	"SLA":   true,
	"SMTP":  true,
	"SQL":   true,
	"SSH":   true,
	"TCP":   true,
	"TLS":   true,
	"TTL":   true,
	"UDP":   true,
	"UI":    true,
	"UID":   true,
	"UUID":  true,
	"URI":   true,
	"URL":   true,
	"UTF8":  true,
	"VM":    true,
	"XML":   true,
	"XSRF":  true,
	"XSS":   true,
}
