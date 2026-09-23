// format-go applies Go formatting without horizontal column alignment.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"go/scanner"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	write := flag.Bool("w", false, "write formatted files (otherwise check without changing files)")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: format-go [-w] path ...")
		os.Exit(2)
	}
	changed, err := run(flag.Args(), *write, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if changed && !*write {
		os.Exit(1)
	}
}

func run(paths []string, write bool, output io.Writer) (bool, error) {
	changed := false
	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.Type().IsRegular() || filepath.Ext(path) != ".go" {
				return nil
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			formatted, err := formatSource(source)
			if err != nil {
				return fmt.Errorf("format %s: %w", path, err)
			}
			if bytes.Equal(source, formatted) {
				return nil
			}
			changed = true
			if write {
				info, err := entry.Info()
				if err != nil {
					return fmt.Errorf("stat %s: %w", path, err)
				}
				if err := os.WriteFile(path, formatted, info.Mode().Perm()); err != nil {
					return fmt.Errorf("write %s: %w", path, err)
				}
			}
			_, err = fmt.Fprintln(output, path)
			return err
		})
		if err != nil {
			return changed, err
		}
	}
	return changed, nil
}

func formatSource(source []byte) ([]byte, error) {
	formatted, err := format.Source(source)
	if err != nil {
		return nil, err
	}
	file := token.NewFileSet().AddFile("", -1, len(formatted))
	var lexer scanner.Scanner
	lexer.Init(file, formatted, nil, scanner.ScanComments)
	var output bytes.Buffer
	copied := 0
	for {
		position, kind, literal := lexer.Scan()
		if kind == token.EOF {
			break
		}
		if kind == token.SEMICOLON && literal == "\n" {
			continue
		}
		end := file.Offset(position)
		start := end
		for start > 0 && (formatted[start-1] == ' ' || formatted[start-1] == '\t') {
			start--
		}
		// Only collapse a gap before a real token on the same line. The scanner
		// skips literal/comment interiors; a gap at line start is indentation.
		if start == end || start == 0 || formatted[start-1] == '\n' {
			continue
		}
		output.Write(formatted[copied:start])
		output.WriteByte(' ')
		copied = end
	}
	output.Write(formatted[copied:])
	return output.Bytes(), nil
}
