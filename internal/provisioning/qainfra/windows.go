package qainfra

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

const windowsMarker = "# __WINDOWS_MODULE_ARGS__"

const windowsModuleArgs = `aws_ami_windows         = var.aws_ami_windows
  instance_type_windows   = var.instance_type_windows
  aws_volume_size_windows = var.aws_volume_size_windows
  aws_volume_type_windows = var.aws_volume_type_windows
  aws_windows_ssh_user    = var.aws_windows_ssh_user
  windows_enable_rdp      = var.windows_enable_rdp`

// injectWindowsModuleArgs adds Windows inputs only when requested, keeping Linux runs
// valid on qa-infra refs that predate them.
func injectWindowsModuleArgs(mainTf string, windows bool) string {
	args := ""
	if windows {
		args = windowsModuleArgs
	}

	return strings.ReplaceAll(mainTf, windowsMarker, args)
}

var nodesStartRE = regexp.MustCompile(`(?m)^\s*nodes\s*=\s*\[`)

var (
	nodeEntryRE = regexp.MustCompile(`\{[^{}]*\}`)
	nodeCountRE = regexp.MustCompile(`"?\bcount"?\s*[=:]\s*"?(\d+)`)
)

var windowsNodeRE = regexp.MustCompile(`(?i)"?\bos"?\s*[=:]\s*"windows"`)

// windowsRequested checks the effective topology: the env-built nodes override wins,
// otherwise the vars.tfvars nodes list (e.g. from the Jenkins TFVARS blob).
func windowsRequested(tfvarsPath string) (bool, error) {
	winWorkers, err := envInt("no_of_windows_worker_nodes", 0)
	if err != nil {
		return false, err
	}
	if winWorkers > 0 {
		return true, nil
	}
	// A non-empty env topology replaces the tfvars nodes, so tfvars Windows entries no longer apply.
	if nodesJSON, nodesErr := buildNodesTFVar(); nodesErr == nil && nodesJSON != "" {
		return false, nil
	}

	content, err := os.ReadFile(tfvarsPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return hasWindowsGroup(nodesBlock(string(content))), nil
}

// hasWindowsGroup reports a Windows entry with a positive (or unparseable) count; count = 0 creates no instances.
func hasWindowsGroup(block string) bool {
	for _, entry := range nodeEntryRE.FindAllString(block, -1) {
		if !windowsNodeRE.MatchString(entry) {
			continue
		}
		m := nodeCountRE.FindStringSubmatch(entry)
		if m == nil {
			return true
		}
		if n, err := strconv.Atoi(m[1]); err != nil || n > 0 {
			return true
		}
	}

	return false
}

// nodesBlock returns the bracketed value of the top-level `nodes` variable with HCL
// comments removed; text outside it or inside comments never counts as a node.
func nodesBlock(src string) string {
	text := stripHCLComments(src)
	loc := nodesStartRE.FindStringIndex(text)
	if loc == nil {
		return ""
	}

	return bracketedValue(text, loc[1]-1)
}

// stripHCLComments removes #, // and /* */ comments while leaving quoted strings intact.
func stripHCLComments(src string) string {
	var clean strings.Builder
	inStr := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case inStr:
			clean.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				clean.WriteByte(src[i])
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			clean.WriteByte(c)
		case c == '#' || strings.HasPrefix(src[i:], "//"):
			i += strings.IndexByte(src[i:]+"\n", '\n') - 1
			clean.WriteByte('\n')
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return clean.String()
			}
			i += end + 3
		default:
			clean.WriteByte(c)
		}
	}

	return clean.String()
}

// bracketedValue returns text[start:] up to the bracket matching text[start], ignoring strings.
func bracketedValue(text string, start int) string {
	depth := 0
	inStr := false
	for i := start; i < len(text); i++ {
		c := text[i]
		switch {
		case inStr && c == '\\':
			i++
		case inStr:
			inStr = c != '"'
		case c == '"':
			inStr = true
		case c == '[':
			depth++
		case c == ']':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}

	return text[start:]
}
