package main

import (
	"bytes"
	"go/format"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFormatSourceOnlyChangesPaddingAfterGoFormatting(t *testing.T) {
	source := []byte("package sample\n\n" +
		"type Kind string\nconst (\n\tShort       Kind = \"a  b\"\n\tLongerName  Kind = \"c\\td\"\n)\n" +
		"type Item struct {\n\tName    string `json:\"name\" note:\"two  spaces\"`\n\tCount   int\n}\n" +
		"var item = Item{\n\tName:    \"a  b\", // retain  comment spacing\n\tCount:   2,\n}\n" +
		"var query = `select  id\n\t  from records\n  where name = 'a  b'`\n" +
		"/* keep  this\n   block  comment */\n" +
		"func run() {\n\tif true {\n\t\titem.Count++\n\t}\n}\n")
	got, err := formatSource(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\tShort Kind =",
		"\tLongerName Kind =",
		"\tName string `json:\"name\" note:\"two  spaces\"`",
		"\tName: \"a  b\", // retain  comment spacing",
		"\t\titem.Count++",
		"`select  id\n\t  from records\n  where name = 'a  b'`",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("formatted source missing %q", want)
		}
	}
	baseline, err := format.Source(source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scanTokens(baseline), scanTokens(got)) {
		t.Fatal("formatting changed tokens, comments, or literal contents")
	}
	again, err := formatSource(got)
	if err != nil || !bytes.Equal(got, again) {
		t.Fatalf("formatting is not idempotent: %v", err)
	}
}

func TestFormatSourceRejectsInvalidGo(t *testing.T) {
	if _, err := formatSource([]byte("package broken\nfunc {")); err == nil {
		t.Fatal("invalid Go must not produce a replacement file")
	}
}

func TestRunChecksWithoutWritingAndSkipsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	original := []byte("package sample\ntype Value struct {\n\tShort     string\n\tLongName  int\n}\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "query.sql")
	if err := os.WriteFile(other, []byte("select  id"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	changed, err := run([]string{dir}, false, &output)
	if err != nil || !changed || !strings.Contains(output.String(), path) {
		t.Fatalf("check: changed=%v output=%q error=%v", changed, output.String(), err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("check mode modified the file")
	}
	if _, err := run([]string{dir}, true, &output); err != nil {
		t.Fatal(err)
	}
	changed, err = run([]string{dir}, false, &output)
	if err != nil || changed {
		t.Fatalf("written file still needs formatting: %v", err)
	}
	untouched, err := os.ReadFile(other)
	if err != nil || string(untouched) != "select  id" {
		t.Fatal("formatter changed a non-Go file")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("formatter changed file permissions")
	}
}

func TestRunDoesNotOverwriteInvalidSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.go")
	source := []byte("package broken\nfunc {")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run([]string{path}, true, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a formatting error")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, source) {
		t.Fatal("invalid source was overwritten")
	}
}

// Include comments and inserted semicolons, but omit positions because spacing changes them.
func scanTokens(source []byte) []string {
	file := token.NewFileSet().AddFile("", -1, len(source))
	var lexer scanner.Scanner
	lexer.Init(file, source, nil, scanner.ScanComments)
	var tokens []string
	for {
		_, kind, literal := lexer.Scan()
		if kind == token.EOF {
			return tokens
		}
		tokens = append(tokens, kind.String()+":"+literal)
	}
}
