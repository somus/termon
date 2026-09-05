// Command checkgocognit rejects source directives that bypass the repository's
// centralized cognitive-complexity budget.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var errSuppressionFound = errors.New("gocognit suppression directives are forbidden; use a score-frozen .golangci.yml exception")

type suppression struct {
	position  token.Position
	directive string
}

type indexedGoFile struct {
	path     string
	objectID string
	source   []byte
}

func main() {
	if err := run(".", os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(repoDir string, output io.Writer) error {
	files, err := indexedGoFiles(repoDir)
	if err != nil {
		return err
	}

	found := false
	for _, file := range files {
		suppressions, parseErr := findSuppressions(file.path, file.source)
		if parseErr != nil {
			return parseErr
		}
		for _, item := range suppressions {
			found = true
			if _, writeErr := fmt.Fprintf(output, "%s:%d:%d: forbidden gocognit suppression %q\n", file.path, item.position.Line, item.position.Column, item.directive); writeErr != nil {
				return fmt.Errorf("write suppression finding: %w", writeErr)
			}
		}
	}
	if found {
		return errSuppressionFound
	}

	return nil
}

func indexedGoFiles(repoDir string) ([]indexedGoFile, error) {
	command := exec.Command("git", "ls-files", "--stage", "-z", "--", "*.go")
	command.Dir = repoDir
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list indexed Go files: %w", err)
	}

	var files []indexedGoFile
	for record := range bytes.SplitSeq(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		metadata, path, ok := bytes.Cut(record, []byte{'\t'})
		fields := bytes.Fields(metadata)
		if !ok || len(fields) != 3 || !bytes.Equal(fields[2], []byte("0")) {
			return nil, fmt.Errorf("parse indexed Go file entry %q", record)
		}
		files = append(files, indexedGoFile{path: string(path), objectID: string(fields[1])})
	}
	if err := readIndexedBlobs(repoDir, files); err != nil {
		return nil, err
	}

	return files, nil
}

func readIndexedBlobs(repoDir string, files []indexedGoFile) error {
	if len(files) == 0 {
		return nil
	}

	var input strings.Builder
	for _, file := range files {
		input.WriteString(file.objectID)
		input.WriteByte('\n')
	}

	command := exec.Command("git", "cat-file", "--batch")
	command.Dir = repoDir
	command.Stdin = strings.NewReader(input.String())
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("read indexed Go blobs: %w", err)
	}

	reader := bufio.NewReader(bytes.NewReader(output))
	for index := range files {
		header, readErr := reader.ReadString('\n')
		if readErr != nil {
			return fmt.Errorf("read indexed blob header for %q: %w", files[index].path, readErr)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != files[index].objectID || fields[1] != "blob" {
			return fmt.Errorf("unexpected indexed blob header for %q: %q", files[index].path, strings.TrimSpace(header))
		}
		size, sizeErr := strconv.Atoi(fields[2])
		if sizeErr != nil {
			return fmt.Errorf("parse indexed blob size for %q: %w", files[index].path, sizeErr)
		}
		files[index].source = make([]byte, size)
		if _, readErr := io.ReadFull(reader, files[index].source); readErr != nil {
			return fmt.Errorf("read indexed blob for %q: %w", files[index].path, readErr)
		}
		if separator, readErr := reader.ReadByte(); readErr != nil || separator != '\n' {
			return fmt.Errorf("read indexed blob separator for %q", files[index].path)
		}
	}

	return nil
}

func findSuppressions(filename string, source []byte) ([]suppression, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, filename, source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse indexed Go file %q: %w", filename, err)
	}

	nativeDirectives := make(map[token.Pos]struct{})
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Doc == nil {
			continue
		}
		for _, comment := range function.Doc.List {
			if comment.Text == "//gocognit:ignore" {
				nativeDirectives[comment.Pos()] = struct{}{}
			}
		}
	}

	var suppressions []suppression
	for _, group := range file.Comments {
		for _, comment := range group.List {
			_, native := nativeDirectives[comment.Pos()]
			if native || nolintSuppressesGocognit(comment.Text) {
				suppressions = append(suppressions, suppression{
					position:  fileSet.Position(comment.Pos()),
					directive: comment.Text,
				})
			}
		}
	}

	return suppressions, nil
}

func nolintSuppressesGocognit(comment string) bool {
	text := strings.TrimLeft(comment, "/ ")
	if text == "nolint" || strings.HasPrefix(text, "nolint ") {
		return true
	}
	if strings.HasPrefix(text, "nolint:all") {
		return true
	}
	if !strings.HasPrefix(text, "nolint:") {
		return false
	}

	text, _, _ = strings.Cut(text, "//")
	for name := range strings.SplitSeq(strings.TrimPrefix(text, "nolint:"), ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "all", "gocognit":
			return true
		}
	}

	return false
}
