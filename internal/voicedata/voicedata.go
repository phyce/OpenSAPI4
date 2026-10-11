// Package voicedata finds the data packs the commands speak with: in the folder OPENSAPI4_VOICES names, or
// else in the voices folder next to the program.
package voicedata

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/phyce/OpenSAPI4/whistler"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

const (
	EnvironmentVariable = "OPENSAPI4_VOICES"
	microsoftFolder     = "microsoft"
)

// Folder is the folder of the data packs: OPENSAPI4_VOICES when it is set, otherwise the voices folder next to
// the program, links resolved.
func Folder() (string, error) {
	if folder := os.Getenv(EnvironmentVariable); folder != "" {
		return folder, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find the program's folder: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return filepath.Join(filepath.Dir(executable), "voices"), nil
}

// LoadMicrosoft loads the Microsoft voices' pack, the microsoft folder of the data packs' folder.
func LoadMicrosoft() (*whistler.Data, error) {
	folder, err := Folder()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(folder, microsoftFolder)
	pack, err := datapack.Open(path)
	if err != nil {
		return nil, fmt.Errorf("no Microsoft voices in %s (keep the voices folder next to the program, or set %s to it): %w",
			path, EnvironmentVariable, err)
	}
	data, err := whistler.Load(pack)
	if err != nil {
		return nil, fmt.Errorf("load the Microsoft voices from %s: %w", path, err)
	}
	return data, nil
}
