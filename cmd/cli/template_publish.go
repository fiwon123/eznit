package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/types"
)

// The limits of a template, the same ones the API enforces. The CLI checks
// them too, so a file that the server would refuse is refused here first,
// with the name of the file in the message.
const (
	maxTemplateFileBytes    = 256 * 1024
	maxTemplateTotalBytes   = 4 * 1024 * 1024
	maxTemplateFiles        = 200
	maxTemplateVariableName = 64
)

// skipFolders are the folders a template never carries: the version control
// metadata and the dependency trees are not part of a project structure.
var skipFolders = map[string]bool{
	".git":          true,
	".svn":          true,
	".hg":           true,
	"node_modules":  true,
	"vendor":        true,
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".next":         true,
	".turbo":        true,
}

// placeholderPattern matches a variable placeholder: {{PROJECT_NAME}}. The
// name is the same shape the API accepts, so a placeholder that the CLI finds
// is a placeholder the server would accept.
var placeholderPattern = regexp.MustCompile(`\{\{([A-Z][A-Z0-9_]{0,63})\}\}`)

// Template Publish Cmd publishes a folder as a template.
type TemplatePublishCmd struct {
	Folder string `arg:"" optional:"" help:"the folder to publish" default:"."`
	Public bool   `help:"publish to the shared catalogue"`
}

// templateFile is a file of a template on the client side: the path it takes
// inside the project, and its content.
type templateFile struct {
	Path         string
	Content      string
	IsExecutable bool
}

// Run publishes the folder as a template.
func (cmd *TemplatePublishCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	root := cmd.Folder
	if root == "" {
		root = "."
	}

	info, err := os.Stat(root)
	if err != nil {
		g.logger.Warn("failed to read the folder. ", slog.String("error", err.Error()))
		return nil
	}

	if !info.IsDir() {
		g.logger.Warn("the path is not a folder. ", slog.String("path", root))
		return nil
	}

	files, skipped, err := collectTemplateFiles(root)
	if err != nil {
		g.logger.Warn("failed to read the folder. ", slog.String("error", err.Error()))
		return nil
	}

	if len(files) == 0 {
		g.logger.Warn("the folder holds no file to publish. ", slog.String("path", root))
		return nil
	}

	for _, name := range skipped {
		fmt.Printf("skipped %s (not a text file, or too big)\n", name)
	}

	fmt.Println()

	name := promptLine("name: ")
	if name == "" {
		g.logger.Warn("a template needs a name. ")
		return nil
	}

	fmt.Printf("kind (%s): ", strings.Join(templates.Kinds, "/"))
	kind := promptLine("")
	if kind == "" {
		kind = templates.KindGeneric
	}

	if !templates.IsKind(kind) {
		g.logger.Warn("that is not a terminal eznit knows. ", slog.String("kind", kind))
		return nil
	}

	description := promptLine("description: ")

	isPublic := cmd.Public
	if !isPublic {
		answer := promptLine("publish to the shared catalogue? (y/N) ")
		isPublic = strings.EqualFold(strings.TrimSpace(answer), "y")
	}

	// The placeholders in the files are the variables the template asks
	// for: the author declares them by writing them.
	keys := findPlaceholders(files)

	variables := make([]templates.PublishVariable, 0, len(keys))

	for _, key := range keys {
		value := promptLine(fmt.Sprintf("default value for {{%s}}: ", key))

		variables = append(variables, templates.PublishVariable{
			Key:    key,
			Prompt: key,
			// A variable without a default has to be answered when the
			// template is applied.
			DefaultValue: value,
			Required:     value == "",
		})
	}

	request := templates.PublishRequest{
		Name:        name,
		Description: description,
		Kind:        kind,
		IsPublic:    isPublic,
	}

	for _, file := range files {
		request.Files = append(request.Files, templates.PublishFile{
			Path:         file.Path,
			Content:      file.Content,
			IsExecutable: file.IsExecutable,
		})
	}

	request.Variables = variables

	detail, ok := sendPublishRequest(g.api.baseURL, request, token, g)
	if !ok {
		return nil
	}

	fmt.Println()
	fmt.Printf("published: %s (%s)\n", detail.Name, detail.ID)
	fmt.Printf("apply it with: eznit template apply %s\n", detail.ID)
	fmt.Println()

	return nil
}

// collectTemplateFiles walks a folder and returns the files a template
// holds: the text files, with the path they take inside the project.
//
// The names of the files that were left out come back too: a file that is
// skipped has to be named, or the template is quietly incomplete.
func collectTemplateFiles(root string) ([]templateFile, []string, error) {
	var files []templateFile
	var skipped []string

	total := 0

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			// The root itself is never skipped, only the folders
			// inside it.
			if path != root && skipFolders[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		// The path is stored with forward slashes: it is applied on
		// machines that are not this one.
		relative = filepath.ToSlash(relative)

		// A symlink is not followed: it can point outside the folder, and
		// a template is made of the files themselves. It is named, so the
		// author knows the folder holds more than the template does.
		if entry.Type()&fs.ModeSymlink != 0 {
			skipped = append(skipped, relative)
			return nil
		}

		safe, ok := helper.SafeRelPath(relative)
		if !ok {
			return fmt.Errorf("the path %q cannot be used in a template", relative)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		if info.Size() > maxTemplateFileBytes {
			skipped = append(skipped, relative)
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// A template holds text: the contents live in a text column,
		// and a byte that is not text does not survive the trip.
		if isBinary(content) {
			skipped = append(skipped, relative)
			return nil
		}

		total += len(content)
		if total > maxTemplateTotalBytes {
			return fmt.Errorf("the folder holds more than %d bytes of text", maxTemplateTotalBytes)
		}

		files = append(files, templateFile{
			Path:         safe,
			Content:      string(content),
			IsExecutable: isExecutable(info.Mode()),
		})

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	if len(files) > maxTemplateFiles {
		return nil, nil, fmt.Errorf("the folder holds %d files, a template holds %d", len(files), maxTemplateFiles)
	}

	sort.Strings(skipped)

	return files, skipped, nil
}

// isBinary reports whether a content holds a byte that is not text. A NUL
// byte is the sign: it is the one byte no text file holds.
func isBinary(content []byte) bool {
	return bytes.IndexByte(content, 0) != -1
}

// isExecutable reports whether a file mode says the file is executable.
func isExecutable(mode fs.FileMode) bool {
	return mode&0111 != 0
}

// findPlaceholders returns the names of the placeholders the files hold, in
// the order they are first seen.
func findPlaceholders(files []templateFile) []string {
	seen := map[string]bool{}
	var keys []string

	for _, file := range files {
		for _, match := range placeholderPattern.FindAllStringSubmatch(file.Content, -1) {
			key := match[1]
			if seen[key] {
				continue
			}
			seen[key] = true
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	return keys
}

// sendPublishRequest sends the template to the API.
func sendPublishRequest(baseURL string, request templates.PublishRequest, token string, g *Globals) (*templates.TemplateDetail, bool) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		g.logger.Error("failed to convert request to json. ", slog.Any("error", err))
		return nil, false
	}

	url := baseURL + "/v1/templates"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		g.logger.Warn("failed to create the request. ", slog.String("error", err.Error()))
		return nil, false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("failed to send request. ", slog.String("error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[templates.TemplateDetail]

	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return nil, false
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("the template could not be published. ", slog.Any("result", envelope))
		return nil, false
	}

	return &envelope.Data, true
}

// stdinReader reads the answers to the prompts. One reader for the whole
// run: a second reader on the same stdin loses whatever the first one has
// buffered.
var stdinReader *bufio.Reader

// promptReader returns the reader the prompts read from.
func promptReader() *bufio.Reader {
	if stdinReader == nil {
		stdinReader = bufio.NewReader(os.Stdin)
	}

	return stdinReader
}

// promptLine asks a question and returns the answer.
func promptLine(label string) string {
	fmt.Print(label)

	answer, err := promptReader().ReadString('\n')
	if err != nil && answer == "" {
		return ""
	}

	return strings.TrimSpace(answer)
}
