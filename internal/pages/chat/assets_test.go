package chat

import (
	"os"
	"testing"

	asseticon "github.com/ggdream/microlight/internal/icon"
)

func TestMain(m *testing.M) {
	var err error
	icons, err = asseticon.Load(os.DirFS("../../.."))
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
