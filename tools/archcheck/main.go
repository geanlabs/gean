// Command archcheck checks Gean's source-level package boundaries without
// loading dependencies or linking native crypto.
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const module = "github.com/geanlabs/gean"

type source struct {
	file    string
	pkg     string
	imports []string
}

type boundary struct {
	name      string
	allowed   []string
	forbidden []string
	noC       bool
}

// shared is the root internal/ tree: mechanics that serve both protocols and
// import neither.
var shared = []string{"internal"}

func main() {
	problems, err := check(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	if len(problems) != 0 {
		os.Exit(1)
	}
	fmt.Println("Architecture boundaries passed")
}

func rules(pkg string) []boundary {
	var result []boundary
	switch {
	case within(pkg, []string{"internal/storage"}):
		result = append(result, boundary{name: "KV backend", allowed: []string{"internal/storage"}, noC: true})
	case within(pkg, []string{"lean/internal/blockbuilder"}):
		result = append(result, boundary{name: "block builder", allowed: []string{
			"lean/internal/blockbuilder", "lean/consensus/types", "lean/consensus/statetransition",
			"internal/metrics", "lean/crypto/xmss",
		}})
	case within(pkg, []string{"lean/internal/pools"}):
		result = append(result, boundary{name: "live pools", allowed: []string{"lean/internal/pools", "lean/consensus/types"}, noC: true})
	case within(pkg, []string{"lean/internal/store"}):
		result = append(result, boundary{name: "store ownership", forbidden: []string{"lean/internal/pools", "lean/crypto"}, noC: true})
	case within(pkg, []string{"lean/internal/aggregation"}):
		result = append(result, boundary{name: "aggregation inputs", forbidden: []string{"lean/internal/store"}})
	}

	// The protocol-first layout; see docs/architecture.md.
	if within(pkg, []string{"lean"}) {
		result = append(result, boundary{name: "Lean isolation", forbidden: []string{"beacon"}})
	}
	if within(pkg, []string{"beacon"}) {
		// Beacon reaches only its own tree and the shared mechanics.
		result = append(result, boundary{name: "Beacon isolation", allowed: slices.Concat([]string{"beacon"}, shared)})
	}
	if within(pkg, shared) {
		result = append(result, boundary{name: "shared mechanics", allowed: shared, noC: true})
	}
	switch {
	case within(pkg, []string{"lean/consensus"}):
		result = append(result, boundary{name: "Lean consensus", allowed: []string{"lean/consensus"}, noC: true})
	case within(pkg, []string{"beacon/consensus"}):
		result = append(result, boundary{name: "Beacon consensus", allowed: []string{"beacon/consensus", "beacon/crypto"}})
	case within(pkg, []string{"lean/crypto"}):
		result = append(result, boundary{name: "Lean crypto", allowed: []string{"lean/crypto", "lean/consensus/types"}})
	case within(pkg, []string{"beacon/crypto"}):
		result = append(result, boundary{name: "Beacon crypto", allowed: []string{"beacon/crypto"}})
	case within(pkg, []string{"beacon/validator"}):
		result = append(result, boundary{name: "validator client", forbidden: []string{"beacon/internal", "beacon/node"}})
	}
	return result
}

func within(pkg string, roots []string) bool {
	for _, root := range roots {
		if pkg == root || strings.HasPrefix(pkg, root+"/") {
			return true
		}
	}
	return false
}

func check(root string) ([]string, error) {
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	if !slices.Contains(strings.Split(string(mod), "\n"), "module "+module) {
		return nil, fmt.Errorf("run archcheck from the Gean module root")
	}

	var files []source
	production := make(map[string][]string)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root {
				if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" {
					return filepath.SkipDir
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				if within(filepath.ToSlash(rel), []string{"lean/crypto/xmss/rust/target"}) {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file := source{file: filepath.ToSlash(rel), pkg: filepath.ToSlash(filepath.Dir(rel))}
		for _, spec := range parsed.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			file.imports = append(file.imports, name)
		}
		files = append(files, file)
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			production[file.pkg] = append(production[file.pkg], file.imports...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	problems := make(map[string]string)
	for _, file := range files {
		for _, rule := range rules(file.pkg) {
			seen := make(map[string]bool)
			var visit func(string, []string)
			visit = func(name string, chain []string) {
				if seen[name] {
					return
				}
				seen[name] = true
				local := name == module || strings.HasPrefix(name, module+"/")
				pkg := strings.TrimPrefix(name, module+"/")
				if name == module {
					pkg = "."
				}
				if local && ((rule.allowed != nil && !within(pkg, rule.allowed)) || within(pkg, rule.forbidden)) || name == "C" && rule.noC {
					key := file.pkg + ":" + rule.name + ":" + pkg
					problems[key] = fmt.Sprintf("%s: %s: %s", file.file, rule.name, strings.Join(append(chain, pkg), " -> "))
					return
				}
				if local {
					for _, dep := range production[pkg] {
						visit(dep, append(slices.Clone(chain), pkg))
					}
				}
			}
			for _, dep := range file.imports {
				visit(dep, []string{file.pkg})
			}
		}
	}
	var result []string
	for _, problem := range problems {
		result = append(result, problem)
	}
	slices.Sort(result)
	return result, nil
}
