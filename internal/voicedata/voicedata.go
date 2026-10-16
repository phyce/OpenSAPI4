// Package voicedata finds the voices folder the commands speak with: the folder OPENSAPI4_VOICES names, or else
// the voices folder next to the program.
package voicedata

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/phyce/OpenSAPI4"
	truvoicedatapack "github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/whistler"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

const EnvironmentVariable = "OPENSAPI4_VOICES"

// Folder is the voices folder: OPENSAPI4_VOICES when it is set, otherwise the voices folder next to the program,
// links resolved.
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

// Voices are the voices of the voices folder, each nil when the folder has no such pack: the Microsoft voices'
// data, and the TruVoice voices' pack, opened but not loaded. Loading it takes longer than a line takes to speak,
// and a program that speaks one line with a Microsoft voice doesn't need it.
type Voices struct {
	Microsoft *whistler.Data
	TruVoice  *truvoicedatapack.Pack
}

// Load loads the voices of the voices folder: its microsoft folder, its truvoice folder, or both.
func Load() (*Voices, error) {
	folder, err := Folder()
	if err != nil {
		return nil, err
	}
	voices := &Voices{}
	path := filepath.Join(folder, opensapi4.MicrosoftFolder)
	if found, err := present(path); err != nil {
		return nil, err
	} else if found {
		pack, err := datapack.Open(path)
		if err != nil {
			return nil, fmt.Errorf("no Microsoft voices in %s: %w", path, err)
		}
		if voices.Microsoft, err = whistler.Load(pack); err != nil {
			return nil, fmt.Errorf("load the Microsoft voices from %s: %w", path, err)
		}
	}
	path = filepath.Join(folder, opensapi4.TruVoiceFolder)
	if found, err := present(path); err != nil {
		return nil, err
	} else if found {
		if voices.TruVoice, err = truvoicedatapack.Open(path); err != nil {
			return nil, fmt.Errorf("no TruVoice voices in %s: %w", path, err)
		}
	}
	if voices.Microsoft == nil && voices.TruVoice == nil {
		return nil, fmt.Errorf("no voices in %s: it has no %s or %s folder (keep the voices folder next to the program, or set %s to it)",
			folder, opensapi4.MicrosoftFolder, opensapi4.TruVoiceFolder, EnvironmentVariable)
	}
	return voices, nil
}

func present(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
