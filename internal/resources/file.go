package resources

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PrintFileContents prints the contents of the file as [] string.
func PrintFileContents(f ...string) error {
	for _, file := range f {
		content, err := os.ReadFile(file)
		if err != nil {
			return ReturnLogError("failed to read file: %w\n", err)
		}
		fmt.Println(string(content) + "\n")
	}

	return nil
}

// PrintBase64Encoded prints the base64 encoded contents of the file as string.
func PrintBase64Encoded(path string) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return ReturnLogError("failed to encode file %s: %w", file, err)
	}

	encoded := base64.StdEncoding.EncodeToString(file)
	fmt.Println(encoded)

	return nil
}

// ReplaceFileContents reads file from local path and replaces them based on key value pair provided.
func ReplaceFileContents(filePath string, replaceKV map[string]string) error {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return ReturnLogError("File does not exist: %v", filePath)
	}

	for key, value := range replaceKV {
		if strings.Contains(string(contents), key) {
			contents = bytes.ReplaceAll(contents, []byte(key), []byte(value))
		}
	}

	err = os.WriteFile(filePath, contents, 0o666)
	if err != nil {
		return ReturnLogError("Write to File failed: %v", filePath)
	}

	return nil
}

// VerifyFileContent greps for a specific string in a file on the node.
func VerifyFileContent(filePath, content, ip string) error {
	if filePath == "" {
		return ReturnLogError("filePath should not be sent empty")
	}

	if content == "" {
		return ReturnLogError("assert should not be sent empty")
	}

	cmd := fmt.Sprintf("sudo cat %s | grep %q", filePath, content)
	res, err := RunCommandOnNode(cmd, ip)
	if err != nil {
		return ReturnLogError("error running command: %s, error: %w", cmd, err)
	}
	if res == "" || !strings.Contains(res, content) {
		return ReturnLogError("file: %s does not have content: %s, grep result: %s", filePath, content, res)
	}

	LogLevel("debug", "file: %s has content: %s; grep result: %s", filePath, content, res)

	return nil
}

// CreateDir Creates a directory if it does not exist.
// Optional: If chmodValue is not empty, run 'chmod' to change permission of the directory.
func CreateDir(dir, chmodValue, ip string) {
	cmdPart1 := fmt.Sprintf("test -d '%s' && echo 'directory exists: %s'", dir, dir)
	cmdPart2 := "sudo mkdir -p " + dir
	var cmd string
	if chmodValue != "" {
		cmd = fmt.Sprintf("%s || %s; sudo chmod %s %s; sudo ls -lrt %s", cmdPart1, cmdPart2, chmodValue, dir, dir)
	} else {
		cmd = fmt.Sprintf("%s || %s; sudo ls -lrt %s", cmdPart1, cmdPart2, dir)
	}

	output, mkdirErr := RunCommandOnNode(cmd, ip)
	if mkdirErr != nil {
		LogLevel("warn", "error creating %s dir on node ip: %s", dir, ip)
	}
	if output != "" {
		LogLevel("debug", "create and check %s output: %s", dir, output)
	}
}

// fileExists Checks if a file exists in a directory.
func fileExists(files []os.DirEntry, workload string) bool {
	for _, file := range files {
		if file.Name() == workload {
			return true
		}
	}

	return false
}

// CopyFileContents copies a regular file from srcPath to destPath.
// If mode is provided, it sets the destination permissions to that mode.
func CopyFileContents(srcPath, destPath string, mode ...os.FileMode) error {
	if srcPath == "" || destPath == "" {
		return errors.New("src and dest must be non-empty")
	}

	// choose perms.
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat src: %w", err)
	}
	perm := srcInfo.Mode().Perm()
	if len(mode) > 0 {
		perm = mode[0]
	}

	// ensure dest dir exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("mkdir dest dir: %w", err)
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read src: %w", err)
	}

	if err := os.WriteFile(destPath, data, perm); err != nil {
		return fmt.Errorf("write dest: %w", err)
	}

	return nil
}
