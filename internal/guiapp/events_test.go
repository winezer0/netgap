package guiapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	slogs "github.com/winezer0/slogs"
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
	t.Cleanup(func() { _ = slogs.CloseAll() })
	s.writeLog("ERROR", "backend operation failed")
	s.LogUIError("操作失败: permission denied (100%)")
	if _, count := s.logs.Snapshot(); count != 2 {
		t.Fatalf("backend and UI errors were not both counted: %d", count)
	}
	if err := slogs.CloseAll(); err != nil {
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
