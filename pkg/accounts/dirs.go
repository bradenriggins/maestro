package accounts

import (
	"fmt"
	"os"
	"path/filepath"
)

var RequiredDirs = []string{
	"accounts",
	"bin",
	"tasks",
	"status",
	"results",
	"captures",
	"logs",
	"history",
	"archive",
	"usage",
}

func EnsureConductorDirs() error {
	base, err := ConductorDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(base, 0700); err != nil {
		return fmt.Errorf("failed to create conductor dir: %w", err)
	}

	if err := os.Chmod(base, 0700); err != nil {
		return fmt.Errorf("failed to set permissions on conductor dir: %w", err)
	}

	for _, dir := range RequiredDirs {
		path := filepath.Join(base, dir)
		if err := os.MkdirAll(path, 0700); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	gitignorePath := filepath.Join(base, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(gitignorePath, []byte("*\n"), 0600); err != nil {
			return fmt.Errorf("failed to create .gitignore: %w", err)
		}
	}

	return nil
}

func ConductorConfigExists() bool {
	base, err := ConductorDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(base, ConfigFileName))
	return err == nil
}
