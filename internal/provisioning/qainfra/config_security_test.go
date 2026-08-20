package qainfra

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/pkg/logger"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func TestLoadQAInfraTFVarsDebugLogDoesNotExposeSecrets(t *testing.T) {
	const accessKey = "AKIA_GO_LOG_SENTINEL"
	secretKey := strings.Join([]string{"AWS", "SECRET", "GO", "LOG", "SENTINEL"}, "_")

	// DB_PASSWORD is covered by the entrypoint regression; it is not stored in driver.Cluster.
	nodeSource := t.TempDir()
	tfvars := "aws_access_key = \"" + accessKey + "\"\n" +
		"aws_secret_key = \"" + secretKey + "\"\n" +
		"aws_region = \"us-east-2\"\n"
	if err := os.WriteFile(filepath.Join(nodeSource, "vars.tfvars"), []byte(tfvars), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("AWS_ACCESS_KEY_ID", accessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)

	logEntry := logger.AddLogger()
	previousOutput := logEntry.Logger.Out
	var output bytes.Buffer
	logEntry.Logger.SetOutput(&output)
	t.Cleanup(func() { logEntry.Logger.SetOutput(previousOutput) })

	var cluster driver.Cluster
	if err := loadQAInfraTFVars(&cluster, false, false, nodeSource); err != nil {
		t.Fatal(err)
	}

	logged := output.String()
	if !strings.Contains(logged, "Cluster configuration loaded from vars.tfvars") {
		t.Fatalf("expected debug log was not captured: %q", logged)
	}
	for _, secret := range []string{accessKey, secretKey} {
		if strings.Contains(logged, secret) {
			t.Errorf("debug log exposed secret sentinel %q", secret)
		}
	}
}

func TestQAInfraConfigObjectsNotPassedToLogger(t *testing.T) {
	files := token.NewFileSet()
	source, err := parser.ParseFile(files, "config.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	protected := map[string]bool{"infraConfig": true, "clusterConfig": true}
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "LogLevel" {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "resources" {
			return true
		}

		for _, argument := range call.Args {
			ast.Inspect(argument, func(n ast.Node) bool {
				identifier, ok := n.(*ast.Ident)
				if ok && protected[identifier.Name] {
					t.Errorf("%s passes %s to resources.LogLevel", files.Position(identifier.Pos()), identifier.Name)
				}

				return true
			})
		}

		return true
	})
}
