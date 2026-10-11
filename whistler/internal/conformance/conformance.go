package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

const EnvironmentVariable = "OPENSAPI4_CONFORMANCE"

func Path(t testing.TB, elements ...string) string {
	t.Helper()
	rootPath := os.Getenv(EnvironmentVariable)
	if rootPath == "" {
		t.Skip(EnvironmentVariable + " is not set")
	}
	return filepath.Join(append([]string{rootPath}, elements...)...)
}

func Pack(t testing.TB) *datapack.Pack {
	t.Helper()
	pack, err := datapack.Open(Path(t, "datapack", "microsoft"))
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func Voice(t testing.TB, pack *datapack.Pack, name string) *datapack.Voice {
	t.Helper()
	voice, err := pack.LoadVoice(name)
	if err != nil {
		t.Fatal(err)
	}
	return voice
}
