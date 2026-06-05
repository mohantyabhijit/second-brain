package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type lintIssue struct {
	path    string
	line    int
	message string
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}

	issues, err := lintRoots(roots)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-style-lint: %v\n", err)
		os.Exit(2)
	}
	if len(issues) == 0 {
		return
	}

	for _, issue := range issues {
		fmt.Fprintf(os.Stderr, "%s:%d: %s\n", issue.path, issue.line, issue.message)
	}
	os.Exit(1)
}

func lintRoots(roots []string) ([]lintIssue, error) {
	var paths []string
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if strings.HasSuffix(root, ".go") {
				paths = append(paths, root)
			}
			continue
		}

		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".cache", ".git", "node_modules", "vendor":
					if path != root {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if strings.HasSuffix(entry.Name(), ".go") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	sort.Strings(paths)
	fset := token.NewFileSet()
	var issues []lintIssue
	for _, path := range paths {
		fileIssues, err := lintFile(fset, path)
		if err != nil {
			return nil, err
		}
		issues = append(issues, fileIssues...)
	}
	return issues, nil
}

func lintFile(fset *token.FileSet, path string) ([]lintIssue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isGeneratedGo(data) {
		return nil, nil
	}

	file, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var issues []lintIssue
	var previous ast.Decl
	for _, decl := range file.Decls {
		currentStartLine := declarationStartLine(fset, decl)
		requiresGap := previous != nil && (isFuncDecl(previous) || isFuncDecl(decl))
		if requiresGap && !hasBlankLineBetween(lines, fset.Position(previous.End()).Line, currentStartLine) {
			issues = append(issues, lintIssue{
				path:    path,
				line:    currentStartLine,
				message: "missing blank line around function declaration",
			})
		}
		previous = decl
	}
	return issues, nil
}

func isFuncDecl(decl ast.Decl) bool {
	_, ok := decl.(*ast.FuncDecl)
	return ok
}

func declarationStartLine(fset *token.FileSet, decl ast.Decl) int {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if typed.Doc != nil {
			return fset.Position(typed.Doc.Pos()).Line
		}
	case *ast.GenDecl:
		if typed.Doc != nil {
			return fset.Position(typed.Doc.Pos()).Line
		}
	}
	return fset.Position(decl.Pos()).Line
}

func hasBlankLineBetween(lines []string, previousEndLine int, nextStartLine int) bool {
	for line := previousEndLine + 1; line < nextStartLine; line++ {
		if line <= 0 || line > len(lines) {
			continue
		}
		if strings.TrimSpace(lines[line-1]) == "" {
			return true
		}
	}
	return false
}

func isGeneratedGo(data []byte) bool {
	const generatedHeaderLimit = 2048
	if len(data) > generatedHeaderLimit {
		data = data[:generatedHeaderLimit]
	}
	return bytes.Contains(data, []byte("Code generated")) &&
		bytes.Contains(data, []byte("DO NOT EDIT."))
}
