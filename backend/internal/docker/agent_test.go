package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deployfox/internal/platform"
)

func TestSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if _, err := containedPath(root, "escape"); err == nil {
		t.Fatal("symlink escape permitted")
	}
}

func TestChildEnvironmentDoesNotInheritPanelSecrets(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "panel-agent-secret")
	t.Setenv("DATABASE_URL", "panel-database-secret")
	output, err := command(context.Background(), "", []string{"DEPLOYFOX_EXPLICIT=test-value"}, "env")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "panel-agent-secret") || strings.Contains(output, "panel-database-secret") {
		t.Fatal("child inherited panel credentials")
	}
	if !strings.Contains(output, "DEPLOYFOX_EXPLICIT=test-value") {
		t.Fatal("explicit tool environment missing")
	}
}
func TestRejectForeignImage(t *testing.T) {
	a := Agent{}
	_, err := a.deploy(context.Background(), Request{Project: platform.Project{ID: "3e278cad-8c15-4416-aec0-33dc988d8662"}, DeploymentID: "3e278cad-8c15-4416-aec0-33dc988d8662", Image: "postgres:16"})
	if err == nil {
		t.Fatal("foreign image accepted")
	}
}
