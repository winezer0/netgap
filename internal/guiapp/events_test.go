package guiapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	logging "github.com/winezer0/zaplogs"
	"netgap/internal/guiapp/configstore"
	"netgap/internal/guiapp/logbuffer"
)

func TestGUIErrorsWrittenToFile(t *testing.T) {
	dir := t.TempDir()
	s := &Service{
		store: configstore.NewStoreWithPath(appName, filepath.Join(dir, "settings.yaml")),
		logs:  logbuffer.New(10),
	}
	s.initLogger()
	t.Cleanup(func() { _ = logging.CloseAll() })
	s.writeLog("ERROR", "backend operation failed")
	s.LogUIError("操作失败: permission denied (100%)")
	if err := logging.Sync(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, appName+".log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"backend operation failed", "UI: 操作失败: permission denied (100%)"} {
		if !strings.Contains(string(data), message) {
			t.Errorf("file log missing %q: %s", message, data)
		}
		if !strings.Contains(strings.Join(s.logs.List(), "\n"), message) {
			t.Errorf("UI log missing %q", message)
		}
	}
}
