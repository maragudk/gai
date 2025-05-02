package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"

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
		Function: func(ctx context.Context, args json.RawMessage) (string, error) {
			var readFileArgs ReadFileArgs
			if err := json.Unmarshal(args, &readFileArgs); err != nil {
				return "", fmt.Errorf("error unmarshaling read_file args from JSON: %w", err)
			}

			f, err := fs.ReadFile(fsys, readFileArgs.Path)
			if err != nil {
				return "", err
			}

			return string(f), nil
		},
	}
}
