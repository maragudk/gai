package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"maragu.dev/gai"
)

type ReadFileArgs struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

func NewReadFile(root *os.Root) gai.Tool {
	return gai.Tool{
		Name:        "read_file",
		Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
		Schema:      gai.GenerateToolSchema[ReadFileArgs](),
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ReadFileArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "error parsing arguments", nil
			}
			return fmt.Sprintf(`path="%s"`, args.Path), nil
		},
		Execute: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ReadFileArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling read_file args from JSON: %w", err)
			}

			f, err := fs.ReadFile(root.FS(), args.Path)
			if err != nil {
				return "", err
			}

			return string(f), nil
		},
	}
}

type ListDirArgs struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files and directories from. Defaults to current directory if not provided."`
}

func NewListDir(root *os.Root) gai.Tool {
	return gai.Tool{
		Name:        "list_dir",
		Description: "List files and directories at a given path recursively. If no path is provided, lists files and directories in the current directory.",
		Schema:      gai.GenerateToolSchema[ListDirArgs](),
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ListDirArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "error parsing arguments", nil
			}
			if args.Path == "" || args.Path == "." {
				return "", nil
			}
			return fmt.Sprintf(`path="%s"`, args.Path), nil
		},
		Execute: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ListDirArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling list_dir args from JSON: %w", err)
			}

			if args.Path == "" {
				args.Path = "."
			}

			var files []string
			err := fs.WalkDir(root.FS(), args.Path, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				relPath, err := filepath.Rel(args.Path, path)
				if err != nil {
					return err
				}

				if relPath != "." {
					if d.IsDir() {
						files = append(files, relPath+"/")
					} else {
						files = append(files, relPath)
					}
				}

				return nil
			})
			if err != nil {
				return "", err
			}

			slices.Sort(files)

			result, err := json.Marshal(files)
			if err != nil {
				return "", err
			}

			return string(result), nil
		},
	}
}

type EditFileArgs struct {
	Path       string `json:"path" jsonschema_description:"The path to the file."`
	SearchStr  string `json:"search_str" jsonschema_description:"Text to search for. Must match exactly and must have one match exactly."`
	ReplaceStr string `json:"replace_str" jsonschema_description:"Text to replace search_str with."`
}

func NewEditFile(root *os.Root) gai.Tool {
	return gai.Tool{
		Name: "edit_file",
		Description: `Make edits to a text file.

Replaces 'search_str' with 'replace_str' in the given file. 'search_str' and 'replace_str' MUST be different from each other.

If the file specified with 'path' doesn't exist, it will be created.
`,
		Schema: gai.GenerateToolSchema[EditFileArgs](),
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args EditFileArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "error parsing arguments", nil
			}

			// Truncate search and replace strings
			searchStr := args.SearchStr
			if len(searchStr) > 20 {
				searchStr = searchStr[:20] + "..."
			}
			replaceStr := args.ReplaceStr
			if len(replaceStr) > 20 {
				replaceStr = replaceStr[:20] + "..."
			}

			return fmt.Sprintf(`path="%s" search="%s" replace="%s"`, args.Path, searchStr, replaceStr), nil
		},
		Execute: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args EditFileArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling edit_file args from JSON: %w", err)
			}

			if args.Path == "" {
				return "", errors.New("path cannot be empty")
			}

			if args.SearchStr == args.ReplaceStr {
				return "", errors.New("search_str and replace_str cannot be the same")
			}

			// Check if the file exists, writing new_str if it doesn't
			if _, err := root.Stat(args.Path); err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					return "", fmt.Errorf("error getting file info: %w", err)
				}

				dir := path.Dir(args.Path)
				if dir != "." {
					// Create all directories in the path
					parts := strings.Split(dir, "/")
					currentPath := ""
					for _, part := range parts {
						if part == "" {
							continue
						}
						if currentPath != "" {
							currentPath += "/"
						}
						currentPath += part
						_, err := root.Stat(currentPath)
						if err != nil && errors.Is(err, fs.ErrNotExist) {
							if err := root.Mkdir(currentPath, 0755); err != nil {
								return "", fmt.Errorf("error creating directory: %w", err)
							}
						} else if err != nil {
							return "", fmt.Errorf("error checking directory: %w", err)
						}
					}
				}
				f, err := root.Create(args.Path)
				if err != nil {
					return "", fmt.Errorf("error creating file: %w", err)
				}
				defer func() {
					_ = f.Close()
				}()

				if _, err := f.WriteString(args.ReplaceStr); err != nil {
					return "", fmt.Errorf("error writing to file: %w", err)
				}
				return "Created new file at " + args.Path, nil
			}

			// File exists, open it for reading and writing
			f, err := root.Open(args.Path)
			if err != nil {
				return "", fmt.Errorf("error opening file: %w", err)
			}
			defer func() {
				_ = f.Close()
			}()

			content, err := io.ReadAll(f)
			if err != nil {
				return "", fmt.Errorf("error reading file: %w", err)
			}

			if err := f.Close(); err != nil {
				return "", fmt.Errorf("error closing file: %w", err)
			}

			beforeContent := string(content)
			afterContent := strings.Replace(beforeContent, args.SearchStr, args.ReplaceStr, 1)

			if beforeContent == afterContent && args.SearchStr != "" {
				return "", fmt.Errorf("search_str not found in file")
			}

			f, err = root.Create(args.Path)
			if err != nil {
				return "", fmt.Errorf("error creating file: %w", err)
			}
			defer func() {
				_ = f.Close()
			}()

			if _, err := f.WriteString(afterContent); err != nil {
				return "", fmt.Errorf("error writing to file: %w", err)
			}

			return "Edited file at " + args.Path, nil
		},
	}
}
