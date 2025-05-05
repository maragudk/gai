package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"

	"maragu.dev/gai"
)

type ReadFileArgs struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

func NewReadFile(fsys fs.FS) gai.Tool {
	return gai.Tool{
		Name:        "read_file",
		Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
		Schema:      gai.GenerateSchema[ReadFileArgs](),
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ReadFileArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling read_file args from JSON: %w", err)
			}

			f, err := fs.ReadFile(fsys, args.Path)
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

func NewListDir(fsys fs.FS) gai.Tool {
	return gai.Tool{
		Name:        "list_dir",
		Description: "List files and directories at a given path recursively. If no path is provided, lists files and directories in the current directory.",
		Schema:      gai.GenerateSchema[ListDirArgs](),
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ListDirArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling list_dir args from JSON: %w", err)
			}

			if args.Path == "" {
				args.Path = "."
			}

			var files []string
			err := fs.WalkDir(fsys, args.Path, func(path string, d fs.DirEntry, err error) error {
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

			result, err := json.Marshal(files)
			if err != nil {
				return "", err
			}

			return string(result), nil
		},
	}
}
