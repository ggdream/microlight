package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf16"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/pkg/sftp"
	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"
)

const keyringService = "dev.local.microlight.ssh"

func SaveCredential(id, value string) error { return keyring.Set(keyringService, id, value) }
func DeleteCredential(id string) error      { return keyring.Delete(keyringService, id) }

type UnknownHostKey struct{ Fingerprint string }

func (e *UnknownHostKey) Error() string { return "SSH 主机指纹 " + e.Fingerprint }

type sshRuntime struct {
	client       *ssh.Client
	target       Target
	env          Environment
	journalID    string
	directoryErr error
}

func openSSH(ctx context.Context, target Target, secret string) (*sshRuntime, error) {
	if target.Host == "" || target.User == "" {
		return nil, errors.New("请输入 SSH 主机和用户名")
	}
	port := target.Port
	if port == "" {
		port = "22"
	}
	var auth ssh.AuthMethod
	switch target.Auth {
	case "password":
		auth = ssh.PasswordCallback(func() (string, error) {
			if secret == "" && target.Credential != "" {
				return keyring.Get(keyringService, target.Credential)
			}
			if secret == "" {
				return "", errors.New("请输入 SSH 密码")
			}
			return secret, nil
		})
	case "key":
		data, err := os.ReadFile(target.KeyFile)
		if err != nil {
			return nil, err
		}
		signer, err := ssh.ParsePrivateKey(data)
		var encrypted *ssh.PassphraseMissingError
		if errors.As(err, &encrypted) {
			if secret == "" && target.Credential != "" {
				secret, err = keyring.Get(keyringService, target.Credential)
				if err != nil {
					return nil, err
				}
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(secret))
		}
		if err != nil {
			return nil, err
		}
		auth = ssh.PublicKeys(signer)
	default:
		return nil, errors.New("请选择 SSH 认证方式")
	}
	config := &ssh.ClientConfig{User: target.User, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)
		if target.HostKey == "" {
			return &UnknownHostKey{Fingerprint: fingerprint}
		}
		if target.HostKey != fingerprint {
			return errors.New("SSH 主机指纹已变化")
		}
		return nil
	}}
	address := net.JoinHostPort(strings.Trim(target.Host, "[]"), port)
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))
	c, ch, requests, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		conn.Close()
		return nil, err
	}
	r := &sshRuntime{client: ssh.NewClient(c, ch, requests), target: target}
	files, closeFiles, err := r.startFiles(ctx)
	if err == nil {
		out, _, probeErr := r.run(ctx, "uname -sm")
		fields := strings.Fields(out)
		if probeErr == nil && len(fields) >= 2 {
			r.env = Environment{OS: strings.ToLower(fields[0]), Arch: fields[1], Shell: "/bin/sh"}
		} else {
			out, _, err = r.run(ctx, powerShell("[Console]::WriteLine([System.Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITECTURE'))"))
			if err == nil && strings.TrimSpace(out) != "" {
				r.env = Environment{OS: "windows", Arch: strings.TrimSpace(out), Shell: "PowerShell"}
			} else if err == nil {
				err = errors.New("无法探测 SSH Shell")
			}
		}
	}
	if err == nil {
		name := r.normalize(target.Directory)
		if !path.IsAbs(name) && !(r.env.OS == "windows" && len(name) > 2 && name[1:3] == ":/") {
			err = errors.New("请输入远端绝对路径")
		} else {
			directory, resolveErr := files.RealPath(name)
			err = resolveErr
			if err == nil {
				r.target.Directory = directory
				var info os.FileInfo
				info, err = files.Stat(directory)
				if err == nil && !info.IsDir() {
					err = errors.New("SSH 工作目录不可用")
				}
			}
			if r.env.OS == "linux" && errors.Is(err, os.ErrNotExist) {
				// Preserve the existing canonical command scope if SFTP can
				// resolve the path despite the missing directory.
				if directory == "" {
					r.target.Directory = path.Clean(name)
				}
				r.directoryErr = err
				err = nil
			}
		}
	}
	if closeFiles != nil {
		closeFiles()
	}
	if err != nil {
		r.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	if r.env.OS == "linux" {
		r.journalID = probeRemoteCommands(ctx, r.runCommandControl)
	}
	if err := ctx.Err(); err != nil {
		r.Close()
		return nil, err
	}
	if r.directoryErr != nil && r.journalID == "" {
		r.Close()
		return nil, r.directoryErr
	}
	return r, nil
}

func (r *sshRuntime) Commands() backgroundshell.RecoverableShell {
	if r.journalID == "" {
		return nil
	}
	return remoteCommands{journalID: r.journalID, run: r.runCommandControl, directory: r.target.Directory, scope: r.target.Identity(), launchErr: r.directoryErr}
}

func (r *sshRuntime) GetCapabilities(ctx context.Context) (Capabilities, error) {
	return Capabilities{Files: r.directoryErr == nil, Shell: true, RecoverableShell: r.journalID != ""}, ctx.Err()
}

func (r *sshRuntime) CommandSessionID() string {
	if r.journalID == "" {
		return ""
	}
	return r.journalID + "/" + r.target.Identity()
}

func (r *sshRuntime) runCommandControl(ctx context.Context, script string) (string, error) {
	out, truncated, err := r.run(ctx, "/bin/sh -c "+quotePOSIX(script))
	if err == nil && truncated {
		err = errors.New("remote command control response is too large")
	}
	return out, err
}

func powerShell(script string) string {
	var data []byte
	for _, unit := range utf16.Encode([]rune(script)) {
		data = append(data, byte(unit), byte(unit>>8))
	}
	return "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand " + base64.StdEncoding.EncodeToString(data)
}

func (r *sshRuntime) Environment() Environment { return r.env }
func (r *sshRuntime) Close() error             { return r.client.Close() }
func (r *sshRuntime) normalize(name string) string {
	if r.env.OS == "windows" {
		return strings.ReplaceAll(name, "\\", "/")
	}
	return name
}

func (r *sshRuntime) run(ctx context.Context, command string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	type result struct {
		output    string
		truncated bool
		err       error
	}
	done := make(chan result, 1)
	go func() {
		output, truncated, err := r.runSession(ctx, command)
		done <- result{output, truncated, err}
	}()
	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case response := <-done:
		return response.output, response.truncated, response.err
	}
}

func (r *sshRuntime) runSession(ctx context.Context, command string) (string, bool, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return "", false, err
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stop()
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	out := &boundedOutput{limit: remoteResponseLimit}
	session.Stdout, session.Stderr = out, out
	err = session.Run(command)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.data.String(), out.truncated, err
}

func (r *sshRuntime) Execute(ctx context.Context, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	if r.directoryErr != nil {
		return nil, r.directoryErr
	}
	command := "cd " + quotePOSIX(r.target.Directory) + " && exec /bin/sh -c " + quotePOSIX(req.Command)
	if r.env.OS == "windows" {
		dir := strings.TrimPrefix(r.target.Directory, "/")
		command = powerShell("$ErrorActionPreference = 'Stop'; Set-Location -LiteralPath '" + strings.ReplaceAll(dir, "'", "''") + "'; & {\n" + req.Command + "\n}; if ($null -ne $LASTEXITCODE) { exit $LASTEXITCODE }")
	}
	deadline, cancel := commandContext(ctx, req)
	defer cancel()
	out, truncated, err := r.run(deadline, command)
	if err != nil {
		if _, ok := err.(*ssh.ExitError); !ok {
			return nil, fmt.Errorf("SSH 命令结果未知: %w", err)
		}
	}
	return shellResult(out, truncated, err)
}

// Each file operation owns its SFTP channel. A canceled operation can leave
// requests in flight, so the channel is discarded instead of reused.
func (r *sshRuntime) openFiles(ctx context.Context) (*sftp.Client, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	type result struct {
		files *sftp.Client
		close func()
		err   error
	}
	done := make(chan result)
	go func() {
		files, close, err := r.startFiles(ctx)
		select {
		case done <- result{files, close, err}:
		case <-ctx.Done():
			if close != nil {
				close()
			}
		}
	}()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case response := <-done:
		return response.files, response.close, response.err
	}
}

func (r *sshRuntime) startFiles(ctx context.Context) (*sftp.Client, func(), error) {
	session, err := r.client.NewSession()
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	closeSession := func() {
		stop()
		_ = session.Close()
	}
	if err := ctx.Err(); err != nil {
		closeSession()
		return nil, nil, err
	}
	files, err := openSFTP(session)
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	return files, func() {
		closeSession()
		_ = files.Close()
	}, nil
}

func openSFTP(session *ssh.Session) (*sftp.Client, error) {
	in, err := session.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := session.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return nil, err
	}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	if err := session.RequestSubsystem("sftp"); err != nil {
		return nil, err
	}
	return sftp.NewClientPipe(out, in)
}

type sshFiles struct {
	*sshRuntime
	files *sftp.Client
}

func (r *sshRuntime) Resolve(ctx context.Context, name string, write bool) (string, string, error) {
	files, close, err := r.openFiles(ctx)
	if err != nil {
		return "", "", err
	}
	defer close()
	return (sshFiles{r, files}).Resolve(ctx, name, write)
}

func (r *sshRuntime) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	files, close, err := r.openFiles(ctx)
	if err != nil {
		return nil, err
	}
	defer close()
	return (sshFiles{r, files}).LsInfo(ctx, req)
}

func (r *sshRuntime) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	files, close, err := r.openFiles(ctx)
	if err != nil {
		return nil, err
	}
	defer close()
	return (sshFiles{r, files}).Read(ctx, req)
}

func (r *sshRuntime) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	files, close, err := r.openFiles(ctx)
	if err != nil {
		return err
	}
	defer close()
	return (sshFiles{r, files}).Write(ctx, req)
}

func (r sshFiles) Resolve(ctx context.Context, name string, write bool) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	name = r.normalize(name)
	if !path.IsAbs(name) && !(r.env.OS == "windows" && len(name) > 2 && name[1:3] == ":/") {
		name = path.Join(r.target.Directory, name)
	}
	if write {
		info, err := r.files.Lstat(name)
		if os.IsNotExist(err) {
			parent, err := r.files.RealPath(path.Dir(name))
			if err != nil {
				return "", "", err
			}
			return path.Join(parent, path.Base(name)), "missing", nil
		}
		if err != nil {
			return "", "", err
		}
		if !info.Mode().IsRegular() {
			return "", "", errors.New("仅支持写入普通文件")
		}
	}
	name, err := r.files.RealPath(name)
	if err != nil {
		return "", "", err
	}
	info, err := r.files.Stat(name)
	if err != nil {
		return "", "", err
	}
	version := fmt.Sprintf("%d:%d:%d", info.Size(), info.ModTime().UnixNano(), info.Mode())
	if write && info.Size() <= fileLimit {
		file, err := r.files.Open(name)
		if err != nil {
			return "", "", err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, io.LimitReader(file, fileLimit+1))
		if err = errors.Join(err, file.Close()); err != nil {
			return "", "", err
		}
		version += fmt.Sprintf(":%x", hash.Sum(nil))
	}
	return name, version, nil
}

func (r sshFiles) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list, err := r.files.ReadDirContext(ctx, req.Path)
	if err != nil {
		return nil, err
	}
	var result []filesystem.FileInfo
	for _, item := range list[:min(len(list), 200)] {
		result = append(result, filesystem.FileInfo{Path: item.Name(), IsDir: item.IsDir(), Size: item.Size(), ModifiedAt: item.ModTime().Format(time.RFC3339)})
	}
	return result, nil
}
func (r sshFiles) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := r.files.Stat(req.FilePath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("目标不是普通文件")
	}
	file, err := r.files.Open(req.FilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readText(file, req)
}
func (r sshFiles) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	version, err := writeVersion(ctx, r, req.FilePath)
	if err != nil {
		return err
	}
	temp := path.Join(path.Dir(req.FilePath), ".microlight-"+fmt.Sprint(time.Now().UnixNano()))
	file, err := r.files.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	defer r.files.Remove(temp)
	_, err = file.Write([]byte(req.Content))
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	if version == "missing" {
		return r.files.Rename(temp, req.FilePath)
	}
	if _, ok := r.files.HasExtension("posix-rename@openssh.com"); !ok {
		return errors.New("SSH 服务器不支持原子覆盖")
	}
	info, err := r.files.Lstat(req.FilePath)
	if err != nil {
		return err
	}
	if err = r.files.Chmod(temp, info.Mode().Perm()); err != nil {
		return err
	}
	if _, err = writeVersion(ctx, r, req.FilePath); err != nil {
		return err
	}
	return r.files.PosixRename(temp, req.FilePath)
}
