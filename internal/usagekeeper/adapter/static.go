package adapter

import (
	"io/fs"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/assets"
)

func EmbeddedStaticFileSystem() (http.FileSystem, error) {
	dist, err := fs.Sub(assets.FS, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(dist), nil
}
