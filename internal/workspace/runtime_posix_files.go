package workspace

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

const posixFileProbe = `set -e; for utility in realpath stat head mktemp cat ln mv chmod; do command -v "$utility" >/dev/null || { echo "Missing utility: $utility" >&2; exit 1; }; done; test -d .`

// posixFiles implements the Eino file operations over a Linux execution transport.
type posixFiles struct {
	directory string
	command   func(ctx context.Context, script, input string) (string, error)
}

func (d posixFiles) Resolve(ctx context.Context, name string, write bool) (string, string, error) {
	if !path.IsAbs(name) {
		name = path.Join(d.directory, name)
	}
	name = path.Clean(name)
	if write {
		if _, err := d.command(ctx, "test ! -L "+quotePOSIX(name), ""); err != nil {
			return "", "", errors.New("仅支持写入普通文件")
		}
	}
	target := quotePOSIX(name)
	resolved, err := d.command(ctx, "test -e "+target+" && realpath "+target, "")
	if err != nil && write {
		if _, check := d.command(ctx, "test ! -e "+quotePOSIX(name)+" && test ! -L "+quotePOSIX(name), ""); check != nil {
			return "", "", err
		}
		parentPath := quotePOSIX(path.Dir(name))
		parent, e := d.command(ctx, "test -d "+parentPath+" && realpath "+parentPath, "")
		if e != nil {
			return "", "", e
		}
		return path.Join(strings.TrimSuffix(parent, "\n"), path.Base(name)), "missing", nil
	}
	if err != nil {
		return "", "", err
	}
	name = strings.TrimSuffix(resolved, "\n")
	if write {
		if _, err := d.command(ctx, "test -f "+quotePOSIX(name), ""); err != nil {
			return "", "", errors.New("仅支持写入普通文件")
		}
	}
	version, err := d.command(ctx, "stat -c '%d:%i:%f:%s:%y:%z' -- "+quotePOSIX(name), "")
	return name, strings.TrimSpace(version), err
}
func (d posixFiles) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	out, err := d.command(ctx, "cd "+quotePOSIX(req.Path)+" || exit; n=0; for entry in ./* ./.[!.]* ./..?*; do if [ -e \"$entry\" ] || [ -L \"$entry\" ]; then printf '%s\\0' \"${entry#./}\"; n=$((n+1)); [ \"$n\" -lt 200 ] || break; fi; done", "")
	if err != nil {
		return nil, err
	}
	var entries []filesystem.FileInfo
	for entry := range strings.SplitSeq(out, "\x00") {
		if entry != "" {
			entries = append(entries, filesystem.FileInfo{Path: path.Base(entry)})
		}
	}
	return entries, nil
}
func (d posixFiles) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	target := quotePOSIX(req.FilePath)
	out, err := d.command(ctx, "test -f "+target+" && head -c 65537 -- "+target, "")
	if err != nil {
		return nil, err
	}
	return readText(strings.NewReader(out), req)
}
func (d posixFiles) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	version, err := writeVersion(ctx, d, req.FilePath)
	if err != nil {
		return err
	}
	target := quotePOSIX(req.FilePath)
	script := "set -e; t=$(mktemp " + quotePOSIX(path.Join(path.Dir(req.FilePath), ".microlight-XXXXXXXX")) + "); trap 'rm -f -- \"$t\"' EXIT; cat > \"$t\"; "
	if version == "missing" {
		script += "ln -T -- \"$t\" " + target
	} else {
		script += "test ! -L " + target + "; test -f " + target + "; test \"$(stat -c '%d:%i:%f:%s:%y:%z' -- " + target + ")\" = " + quotePOSIX(version) + "; chmod \"$(stat -c '%a' -- " + target + ")\" \"$t\"; mv -fT -- \"$t\" " + target
	}
	_, err = d.command(ctx, script, req.Content)
	return err
}
