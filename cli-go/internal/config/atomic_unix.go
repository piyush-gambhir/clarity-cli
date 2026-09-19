//go:build !windows

package config

import "github.com/google/renameio/v2"

func atomicWrite(path string, data []byte) error {
	f, err := renameio.NewPendingFile(path, renameio.WithPermissions(0600))
	if err != nil {
		return err
	}
	defer f.Cleanup()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.CloseAtomicallyReplace()
}
