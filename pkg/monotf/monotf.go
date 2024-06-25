package monotf

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

var (
	M *Monotf
)

type Monotf struct {
	BinDir         string    `json:"bin_dir" yaml:"bin_dir"`
	Versions       []string  `json:"versions" yaml:"versions"`
	DefaultVersion string    `json:"default_version" yaml:"default_version"`
	Org            string    `json:"org" yaml:"org"`
	ServerAddr     string    `json:"server_addr" yaml:"server_addr"`
	PathTemplate   string    `json:"path_template" yaml:"path_template"`
	PathVars       []PathVar `json:"-" yaml:"-"`
	VaultEnv       *VaultEnv `json:"vault_env" yaml:"vault_env"`
	VarScript      string    `json:"var_script" yaml:"var_script"`

	RepoDir string `json:"dir" yaml:"dir"`
}

type PathVar struct {
	Key   string `json:"key" yaml:"key"`
	Index int    `json:"index" yaml:"index"`
	Value string `json:"value" yaml:"value"`
}

type Workspace struct {
	gorm.Model
	Org           string          `json:"org" gorm:"uniqueIndex:idx_org_name"`
	Name          string          `json:"name" gorm:"uniqueIndex:idx_org_name"`
	WorkspaceName string          `json:"workspace_name" gorm:"uniqueIndex:idx_org_name"`
	Path          string          `json:"path" yaml:"path" gorm:"-"`
	Version       string          `json:"version" yaml:"version"`
	Status        WorkspaceStatus `json:"status"`
	Output        string          `json:"output"`
	Running       *bool           `json:"running"`
	LockId        *string         `json:"lock_id"`
	Force         bool            `json:"force" yaml:"force" gorm:"-"`
	PathVars      []PathVar       `json:"-" yaml:"-" gorm:"-"`
	EnvVars       []string        `json:"-" yaml:"-" gorm:"-"`

	Init   bool `json:"init" yaml:"init" gorm:"-"`
	IsInit bool `json:"is_init" yaml:"is_init" gorm:"-"`
}

func LoadConfig(f string) error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "LoadConfig",
	})
	l.Debugf("loading config from %s", f)
	M = &Monotf{}
	fd, err := os.ReadFile(f)
	if err != nil {
		l.Errorf("error reading config file %s: %v", f, err)
		return err
	}
	err = yaml.Unmarshal(fd, M)
	if err != nil {
		// try as json
		err = json.Unmarshal(fd, M)
		if err != nil {
			l.Errorf("error parsing config file %s: %v", f, err)
			return err
		}
	}
	return nil
}

func (m *Monotf) ParsePathVarKeys() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "ParsePathVarKeys",
	})
	l.Debugf("parsing path var keys")
	if m.PathTemplate == "" {
		l.Debug("no path vars")
		return nil
	}
	l.Debugf("parsing path vars %s", m.PathTemplate)
	splitPath := strings.Split(m.PathTemplate, "/")
	for i, v := range splitPath {
		pv := PathVar{
			Index: i,
		}
		if strings.HasPrefix(v, "{{") {
			// use mustache to parse the path var key name
			pv.Key = strings.ReplaceAll(v, "{{", "")
			pv.Key = strings.ReplaceAll(pv.Key, "}}", "")
			l.Debugf("parsed path var %s at index %d", pv.Key, pv.Index)
		} else {
			l.Debugf("path var NULL at index %d is not a mustache var", i)
			pv.Key = ""
		}
		m.PathVars = append(m.PathVars, pv)
	}
	return nil
}

func (m *Monotf) ParsePathVars(p string) ([]PathVar, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "ParsePathVars",
	})
	l.Debugf("parsing path vars for %s", p)
	// remove the repo dir from the path
	p = strings.Replace(p, m.RepoDir+"/", "", 1)
	p = strings.Replace(p, m.RepoDir, "", 1)
	var pvs []PathVar
	splitPath := strings.Split(p, "/")
	// if the length of the splitPath doesn't match the length of
	// the path vars, then return an error
	if len(splitPath) != len(m.PathVars) {
		l.Errorf("path %s does not match path vars", p)
		return pvs, fmt.Errorf("path %s does not match path vars", p)
	}
	for i, v := range splitPath {
		// get the corresponding path var for index
		pv := m.PathVars[i]
		pv.Value = v
		l.Debugf("parsed path var %s at index %d with value %s", pv.Key, pv.Index, pv.Value)
		pvs = append(pvs, pv)
	}
	return pvs, nil
}

func (m *Monotf) LatestVersion() (string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "LatestVersion",
	})
	l.Debugf("getting latest version")
	if len(m.Versions) == 0 {
		return "", nil
	}
	lv := m.Versions[len(m.Versions)-1]
	l.Debugf("latest version is %s", lv)
	return lv, nil
}

func (m *Monotf) Init() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "Init",
	})
	l.Debugf("initializing monotf")
	// reorder versions by semver
	semver.Sort(m.Versions)
	// check for default version
	if m.DefaultVersion == "" {
		lv := m.Versions[len(m.Versions)-1]
		l.Debugf("setting default version to %s", lv)
		m.DefaultVersion = lv
	} else {
		l.Debugf("default version is %s", m.DefaultVersion)
	}
	if err := m.InstallBinaries(); err != nil {
		return err
	}
	// parse path var keys
	if err := m.ParsePathVarKeys(); err != nil {
		return err
	}
	return nil
}

func installTerraformVersion(bindir, version, osname, arch string) error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "installTerraformVersion",
	})
	l.Debugf("installing terraform version %s for %s %s", version, osname, arch)

	if arch == "x86_64" {
		arch = "amd64"
	}

	url := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.zip", version, version, osname, arch)
	l.Debugf("downloading %s", url)
	zipFile := fmt.Sprintf("terraform_%s_%s_%s.zip", version, osname, arch)

	// Download the zip file
	resp, err := http.Get(url)
	if err != nil {
		l.Errorf("Error downloading %s: %v\n", url, err)
		return err
	}
	defer resp.Body.Close()

	// Create the zip file
	out, err := os.Create(zipFile)
	if err != nil {
		l.Errorf("Error creating %s: %v\n", zipFile, err)
		return err
	}
	defer out.Close()

	// Copy the contents of the downloaded file to the zip file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		l.Errorf("Error copying contents of %s to %s: %v\n", url, zipFile, err)
		return err
	}

	// Unzip the downloaded file
	cmd := exec.Command("unzip", zipFile)
	err = cmd.Run()
	if err != nil {
		l.Errorf("Error unzipping %s: %v\n", zipFile, err)
		return err
	}

	binPath := fmt.Sprintf("%s/terraform_%s", bindir, version)
	cmd = exec.Command("mv", "terraform", binPath)
	err = cmd.Run()
	if err != nil {
		l.Errorf("Error moving terraform binary to /usr/local/bin: %v\n", err)
		return err
	}

	// Remove the zip file
	err = os.Remove(zipFile)
	if err != nil {
		l.Errorf("Error removing %s: %v\n", zipFile, err)
		return err
	}

	// Set executable permissions
	cmd = exec.Command("chmod", "+x", binPath)
	err = cmd.Run()
	if err != nil {
		l.Errorf("Error setting executable permissions on %s: %v\n", binPath, err)
		return err
	}

	l.Debugf("terraform version %s installed", version)
	return nil
}

func (b *Monotf) InstallBinIfNotExist(v string) error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "InstallBinIfNotExist",
	})
	l.Debugf("installing binary for version %s", v)
	// check for bin dir
	if _, err := os.Stat(b.BinDir); os.IsNotExist(err) {
		l.Debugf("creating bin dir %s", b.BinDir)
		if err := os.MkdirAll(b.BinDir, 0755); err != nil {
			l.Errorf("error creating bin dir %s: %v", b.BinDir, err)
			return err
		}
	}
	// check for binary
	binName := "terraform_" + v
	binPath := b.BinDir + "/" + binName
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		l.Debugf("downloading binary %s", binName)
		// download binary
		osName := os.Getenv("OS")
		if osName == "" {
			osName = "linux"
		}
		arch := os.Getenv("ARCH")
		if arch == "" {
			arch = "amd64"
		}
		if err := installTerraformVersion(b.BinDir, v, osName, arch); err != nil {
			return err
		}
	} else {
		l.Debugf("binary %s already exists", binName)
	}
	return nil
}

func (b *Monotf) InstallBinaries() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "InstallBinaries",
	})
	l.Debugf("installing binaries")
	// check for bin dir
	if _, err := os.Stat(b.BinDir); os.IsNotExist(err) {
		l.Debugf("creating bin dir %s", b.BinDir)
		if err := os.MkdirAll(b.BinDir, 0755); err != nil {
			l.Errorf("error creating bin dir %s: %v", b.BinDir, err)
			return err
		}
	}
	for _, v := range b.Versions {
		if err := b.InstallBinIfNotExist(v); err != nil {
			return err
		}
	}
	return nil
}

func (w *Workspace) SetVersion() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "SetVersion",
	})
	l.Debugf("getting version for workspace %s", w.Name)
	// if there is a .terraform-version file, use that
	// otherwise just set the default
	tfVersionFile := w.Path + "/.terraform-version"
	if _, err := os.Stat(tfVersionFile); os.IsNotExist(err) {
		l.Debugf("no .terraform-version file found, using default version %s", M.DefaultVersion)
		w.Version = M.DefaultVersion
		return nil
	}
	l.Debugf("reading .terraform-version file %s", tfVersionFile)
	fd, err := os.ReadFile(tfVersionFile)
	if err != nil {
		l.Errorf("error reading .terraform-version file %s: %v", tfVersionFile, err)
		return err
	}
	w.Version = string(fd)
	l.Debugf("workspace %s version is %s", w.Name, w.Version)
	return nil
}

func (w *Workspace) SetName(parentDir string) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "SetName",
	})
	l.Debugf("getting name for workspace %s", w.Path)
	// get relative path
	relPath := strings.ReplaceAll(w.Path, parentDir+"/", "")
	l.Debugf("relative path is %s", relPath)
	w.WorkspaceName = fmt.Sprintf("%s-%s", w.Org, relPath)
	// replace / wih - in path
	w.Name = strings.ReplaceAll(relPath, "/", "-")
	w.WorkspaceName = strings.ReplaceAll(w.WorkspaceName, "/", "-")
	l.Debugf("workspace %s name is %s", w.Path, w.WorkspaceName)
}

func (m *Monotf) SupportsVersion(v string) bool {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "SupportsVersion",
	})
	l.Debugf("checking if monotf supports version %s", v)
	for _, sv := range m.Versions {
		if sv == v {
			l.Debugf("monotf supports version %s", v)
			return true
		}
	}
	l.Debugf("monotf does not support version %s", v)
	return false
}

func (m *Monotf) BinForVersion(v string) (string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "BinForVersion",
	})
	l.Debugf("getting binary for version %s", v)
	if !m.SupportsVersion(v) {
		l.Errorf("monotf does not support version %s", v)
		return "", fmt.Errorf("monotf does not support version %s", v)
	}
	binName := "terraform_" + v
	binPath := m.BinDir + "/" + binName
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		l.Errorf("binary %s does not exist", binPath)
		return "", fmt.Errorf("binary %s does not exist", binPath)
	}
	l.Debugf("binary for version %s is %s", v, binPath)
	return binPath, nil
}

func (b *Monotf) GetWorkspaceLocal(w string) (*Workspace, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "GetWorkspaceLocal",
	})
	l.Debugf("getting local workspace for %s", w)
	// ensure workspace path exists
	ws := &Workspace{}
	workspacePath := b.RepoDir + "/" + w
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		l.WithError(err).Errorf("workspace path %s does not exist", workspacePath)
		return ws, err
	}
	// ensure workspace path is a directory
	fi, err := os.Stat(workspacePath)
	if err != nil {
		l.WithError(err).Errorf("error getting workspace path %s info", workspacePath)
		return ws, err
	}
	if !fi.IsDir() {
		l.Errorf("workspace path %s is not a directory", workspacePath)
		return ws, fmt.Errorf("workspace path %s is not a directory", workspacePath)
	}
	l.Debugf("workspace path %s exists", workspacePath)
	ws.Path = workspacePath
	ws.Org = b.Org
	ws.SetName(b.RepoDir)
	if err := ws.SetVersion(); err != nil {
		return ws, err
	}
	if !b.SupportsVersion(ws.Version) {
		l.Errorf("monotf does not support version %s", ws.Version)
		return ws, fmt.Errorf("monotf does not support version %s", ws.Version)
	}
	return ws, nil
}

func (w *Workspace) CreateWorkspaceIfNotExist() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "CreateWorkspaceIfNotExist",
	})
	l.Debugf("creating workspace %s", w.WorkspaceName)
	// run terraform workspace new $name
	out, stderr, err := w.Terraform([]string{"workspace", "new", w.WorkspaceName})
	if err != nil {
		// if stderr contains "already exists", ignore
		if strings.Contains(stderr, "already exists") {
			l.Debugf("workspace %s already exists", w.WorkspaceName)
			return nil
		}
		l.Errorf("error creating workspace %s: %v", w.WorkspaceName, err)
		l.Errorf("stderr: %s", stderr)
		return err
	}
	l.Debugf("workspace %s created", w.WorkspaceName)
	l.Debugf("output: %s", out)
	return nil
}

func (w *Workspace) Terraform(args []string) (string, string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "Terraform",
		"ws":  w.Name,
		"ver": w.Version,
	})
	var out []byte
	var outStr string
	var errOut []byte
	var errOutStr string
	binPath, err := M.BinForVersion(w.Version)
	if err != nil {
		l.Errorf("error getting binary for version %s: %v", w.Version, err)
		return outStr, errOutStr, err
	}
	argStr := strings.Join(args, " ")
	l.Debugf("running %s %s", binPath, argStr)
	cmd := exec.Command(binPath, args...)
	cmd.Env = os.Environ()
	// and env vars:
	cmd.Env = append(cmd.Env, "TF_IN_AUTOMATION=true")
	if w.IsInit {
		l.Debugf("setting TF_WORKSPACE=%s", w.WorkspaceName)
		cmd.Env = append(cmd.Env, "TF_WORKSPACE="+w.WorkspaceName)
	}
	// for each of the path vars, export them
	for _, pv := range w.PathVars {
		if pv.Key != "" {
			l.Debugf("setting %s=%s", pv.Key, pv.Value)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", pv.Key, pv.Value))
		}
	}
	// for each of the env vars, export them
	cmd.Env = append(cmd.Env, w.EnvVars...)
	cmd.Dir = w.Path
	// tee the out to both the stdout and out var
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		l.Errorf("error getting stdout pipe: %v", err)
		return outStr, errOutStr, err
	}
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					l.Errorf("error reading stdout: %v", err)
				}
				break
			}
			out = append(out, buf[:n]...)
			outStr = string(out)
			// write to stdout
			fmt.Print(string(buf[:n]))
		}
	}()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		l.Errorf("error getting stderr pipe: %v", err)
		return outStr, errOutStr, err
	}
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := stderr.Read(buf)
			if err != nil {
				if err != io.EOF {
					l.Errorf("error reading stderr: %v", err)
				}
				break
			}
			errOut = append(errOut, buf[:n]...)
			errOutStr = string(errOut)
			// write to stderr
			fmt.Fprint(os.Stderr, string(buf[:n]))
		}
	}()
	err = cmd.Run()
	if err != nil {
		l.Errorf("error running %s %s: %v", binPath, argStr, err)
		return outStr, errOutStr, err
	}
	l.Debugf("ran %s %s", binPath, argStr)
	// combine stdout and stderr, base64 encode, and set to w.Output
	w.Output = base64.StdEncoding.EncodeToString(append(out, errOut...))
	return outStr, errOutStr, err
}

func (w *Workspace) TerraformInit() (string, string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "TerraformInit",
		"ws":  w.Name,
		"ver": w.Version,
	})
	l.Debugf("running terraform init")
	return w.Terraform([]string{"init", "-reconfigure", "-upgrade", "-input=false"})
}

func (w *Workspace) TerraformWorkspacePreflight() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "TerraformWorkspacePreflight",
		"ws":  w.WorkspaceName,
		"ver": w.Version,
	})
	l.Debugf("running terraform preflight")
	if w.Init {
		if _, _, err := w.TerraformInit(); err != nil {
			return err
		}
	}
	if err := w.CreateWorkspaceIfNotExist(); err != nil {
		return err
	}
	w.IsInit = true
	return nil
}

func SysInit() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "SysInit",
	})
	l.Debugf("running sysinit")
	return nil
}

func (w *Workspace) MonotfToken() string {
	if os.Getenv("MONOTF_TOKEN") != "" {
		return os.Getenv("MONOTF_TOKEN")
	}
	// loop through the workspace env vars, split on =, and find key MONOTF_TOKEN if exists
	for _, e := range w.EnvVars {
		ev := strings.Split(e, "=")
		if ev[0] == "MONOTF_TOKEN" {
			return ev[1]
		}
	}
	return ""
}

func (w *Workspace) GetStatus() (Workspace, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "GetStatus",
		"ws":  w.Name,
	})
	l.Debugf("getting status for workspace %s", w.Name)
	var rw Workspace
	req, err := http.NewRequest("GET", M.ServerAddr+"/ws/"+w.Org+"/"+w.Name, nil)
	if err != nil {
		l.Errorf("error creating request: %v", err)
		return rw, err
	}

	tokenVar := w.MonotfToken()
	req.Header.Set("Content-Type", "application/json")
	if tokenVar != "" {
		req.Header.Set("Authorization", "token "+tokenVar)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		l.Errorf("error getting workspace status: %v", err)
		return rw, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		l.Errorf("error getting workspace status: %v", resp.Status)
		return rw, fmt.Errorf("error getting workspace status: %v", resp.Status)
	}
	err = json.NewDecoder(resp.Body).Decode(&rw)
	if err != nil {
		l.Errorf("error decoding workspace status: %v", err)
		return rw, err
	}
	return rw, nil
}

func (w *Workspace) WaitForReady(timeoutStr string) error {
	l := log.WithFields(log.Fields{
		"app":     "monotf",
		"fn":      "WaitForReady",
		"ws":      w.Name,
		"timeout": timeoutStr,
	})
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		l.Errorf("error parsing timeout %s: %v", timeoutStr, err)
		return err
	}
	l.Debugf("waiting for workspace %s to be ready for %s", w.Name, timeout)
	start := time.Now()
	for {
		if timeout > 0 && time.Since(start) > timeout {
			l.Errorf("timeout waiting for workspace %s to be ready", w.Name)
			return fmt.Errorf("timeout waiting for workspace %s to be ready", w.Name)
		}
		wss, err := w.GetStatus()
		if err != nil {
			l.Errorf("error getting workspace status: %v", err)
			return err
		}
		// wait for running to not be true
		if wss.Running == nil || !*wss.Running {
			l.Debugf("workspace %s is ready", w.Name)
			break
		}
		l.Debugf("workspace %s is not ready", w.Name)
		time.Sleep(10 * time.Second)
	}
	return nil
}

func (w *Workspace) SaveRemote() error {
	l := log.WithFields(log.Fields{
		"app":  "monotf",
		"fn":   "SaveRemote",
		"ws":   w.Name,
		"ver":  w.Version,
		"run":  w.Running,
		"path": w.Path,
	})
	l.Debugf("saving remote workspace %s", w.Name)
	reqBody, err := json.Marshal(w)
	if err != nil {
		l.Errorf("error marshaling workspace %s: %v", w.Name, err)
		return err
	}
	req, err := http.NewRequest("POST", M.ServerAddr+"/ws", strings.NewReader(string(reqBody)))
	if err != nil {
		l.Errorf("error creating request: %v", err)
		return err
	}
	tokenVar := w.MonotfToken()
	req.Header.Set("Content-Type", "application/json")
	if tokenVar != "" {
		req.Header.Set("Authorization", "token "+tokenVar)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		l.Errorf("error saving workspace: %v", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		bd, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to set status: %s", string(bd))
	}
	return nil
}

func (w *Workspace) SetRunning(running bool) error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "SetRunning",
		"ws":  w.Name,
		"run": running,
	})
	l.Debugf("locking workspace %s", w.Name)
	w.Running = &running
	if w.Running == nil || !*w.Running {
		w.LockId = nil
	}
	if err := w.SaveRemote(); err != nil {
		l.Errorf("error saving workspace: %v", err)
		return err
	}
	l.Debugf("workspace %s %t", w.Name, running)
	return nil
}

func (w *Workspace) SetOutput() error {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "SetOutput",
		"ws":  w.Name,
	})
	l.Debugf("setting output for workspace %s", w.Name)
	if err := w.SaveRemote(); err != nil {
		l.Errorf("error saving workspace: %v", err)
		return err
	}
	l.Debugf("output set for workspace %s", w.Name)
	return nil
}

func (ws *Workspace) LockedTerraform(waitTimeout *string, args []string) (string, string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "LockedTerraform",
		"ws":  ws.Name,
		"ver": ws.Version,
	})
	if err := ws.TerraformWorkspacePreflight(); err != nil {
		l.Errorf("error running terraform preflight: %v", err)
		os.Exit(1)
	}
	lid := uuid.New().String()
	ws.LockId = &lid
	var stdoutstr, stderrstr string
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	cleanup := func() {
		if err := ws.SetRunning(false); err != nil {
			l.Errorf("error setting workspace to not running: %v", err)
			return
		}
	}
	go func() {
		sig := <-sigs
		l.Debugf("Received signal: %s, stopping services...", sig)
		cleanup()
		os.Exit(0)
	}()
	defer cleanup()
	if err := ws.WaitForReady(*waitTimeout); err != nil {
		l.Errorf("error waiting for workspace to be ready: %v", err)
		return stdoutstr, stderrstr, err
	}
	if err := ws.SetRunning(true); err != nil {
		l.Errorf("error setting workspace to running: %v", err)
		return stdoutstr, stderrstr, err
	}
	var err error
	stdoutstr, stderrstr, err = ws.Terraform(args)
	if err != nil {
		l.Errorf("error running terraform: %v", err)
		return stdoutstr, stderrstr, err
	}
	if log.GetLevel() == log.DebugLevel {
		l.Debugf("stdout: %s", stdoutstr)
		l.Debugf("stderr: %s", stderrstr)
	}
	if err := ws.SetOutput(); err != nil {
		l.Errorf("error setting workspace output: %v", err)
		return stdoutstr, stderrstr, err
	}
	stat, err := ws.GetStatus()
	if err != nil {
		l.Errorf("error getting workspace status: %v", err)
		return stdoutstr, stderrstr, err
	}
	l.Debugf("workspace status is %s", stat.Status)
	if stat.Status == WorkspaceStatusFailed {
		l.Errorf("workspace status is failed")
		return stdoutstr, stderrstr, fmt.Errorf("workspace status is failed")
	}
	return stdoutstr, stderrstr, nil
}

func (ws *Workspace) LockedTerraformSpeculativePlan(waitTimeout *string, args []string) (string, string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "LockedTerraformSpeculativePlan",
		"ws":  ws.Name,
		"ver": ws.Version,
	})
	l.Debugf("running terraform speculative plan")
	var stdoutstr, stderrstr string
	var err error
	// get the current status
	stat, err := ws.GetStatus()
	if err != nil {
		l.Errorf("error getting workspace status: %v", err)
		return stdoutstr, stderrstr, err
	}
	l.Debugf("workspace status is %s", stat.Status)
	// run terraform plan
	stdoutstr, stderrstr, err = ws.LockedTerraform(waitTimeout, args)
	if err != nil {
		l.Errorf("error running terraform: %v", err)
		return stdoutstr, stderrstr, err
	}
	// set the status back to the original
	ws.Status = stat.Status
	ws.Output = stat.Output
	if err := ws.SaveRemote(); err != nil {
		l.Errorf("error saving workspace: %v", err)
	}
	return stdoutstr, stderrstr, err
}

func (ws *Workspace) LockedTerraformPlanApply(waitTimeout *string) (string, string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "LockedTerraformPlanApply",
		"ws":  ws.Name,
		"ver": ws.Version,
	})
	l.Debugf("running terraform plan apply")
	var stdoutstr, stderrstr string
	var err error
	// run a plan, writing the plan file to a temp file
	// if the plan is successful, then run an apply using that plan file
	outFile, err := os.CreateTemp("", "monotf-plan-*.tfplan")
	if err != nil {
		l.Errorf("error creating plan file: %v", err)
		return stdoutstr, stderrstr, err
	}
	if err := ws.TerraformWorkspacePreflight(); err != nil {
		l.Errorf("error running terraform preflight: %v", err)
		os.Exit(1)
	}
	lid := uuid.New().String()
	ws.LockId = &lid
	defer func() {
		if err := ws.SetRunning(false); err != nil {
			l.Errorf("error setting workspace to not running: %v", err)
			return
		}
	}()
	if err := ws.WaitForReady(*waitTimeout); err != nil {
		l.Errorf("error waiting for workspace to be ready: %v", err)
		return stdoutstr, stderrstr, err
	}
	if err := ws.SetRunning(true); err != nil {
		l.Errorf("error setting workspace to running: %v", err)
		return stdoutstr, stderrstr, err
	}
	planArgs := []string{"plan", "-out", outFile.Name()}
	stdoutstr, stderrstr, err = ws.Terraform(planArgs)
	if err != nil {
		l.Errorf("error running terraform: %v", err)
		return stdoutstr, stderrstr, err
	}
	if err := ws.SetOutput(); err != nil {
		l.Errorf("error setting workspace output: %v", err)
		return stdoutstr, stderrstr, err
	}
	stat, err := ws.GetStatus()
	if err != nil {
		l.Errorf("error getting workspace status: %v", err)
		return stdoutstr, stderrstr, err
	}
	l.Debugf("workspace status is %s", stat.Status)
	// if the status is pending, apply it
	if stat.Status == WorkspaceStatusPending || stat.Status == WorkspaceStatusUnknown {
		applyArgs := []string{"apply", "-auto-approve", outFile.Name()}
		stdoutstr, stderrstr, err = ws.Terraform(applyArgs)
		if err != nil {
			l.Errorf("error running terraform: %v", err)
			return stdoutstr, stderrstr, err
		}
		if err := ws.SetOutput(); err != nil {
			l.Errorf("error setting workspace output: %v", err)
			return stdoutstr, stderrstr, err
		}
		stat, err = ws.GetStatus()
		if err != nil {
			l.Errorf("error getting workspace status: %v", err)
			return stdoutstr, stderrstr, err
		}
		l.Debugf("workspace status is %s", stat.Status)
	}
	if stat.Status == WorkspaceStatusFailed {
		l.Errorf("workspace status is failed")
		return stdoutstr, stderrstr, fmt.Errorf("workspace status is failed")
	}
	return stdoutstr, stderrstr, nil
}

func (ws *Workspace) VarsFromScript() ([]string, error) {
	l := log.WithFields(log.Fields{
		"app": "monotf",
		"fn":  "VarsFromScript",
	})
	if M.VarScript == "" {
		l.Debugf("no var script")
		return nil, nil
	}
	l.Debugf("getting vars from script %s", M.VarScript)
	// create a tempfile for the vars
	varFile, err := os.CreateTemp("", "monotf-vars-*.vars")
	if err != nil {
		l.Errorf("error creating var file: %v", err)
		return nil, err
	}
	defer os.Remove(varFile.Name())
	cmd := exec.Command(os.ExpandEnv(M.VarScript))
	cmd.Env = os.Environ()
	// add MONOTF_ENV to env
	cmd.Env = append(cmd.Env, fmt.Sprintf("MONOTF_ENV=%s", varFile.Name()))
	cmd.Env = append(cmd.Env, ws.EnvVars...)
	cmd.Dir = ws.Path
	// run the script
	out, err := cmd.CombinedOutput()
	if err != nil {
		l.Errorf("error running var script: %v", err)
		return nil, err
	}
	l.Debugf("var script output: %s", string(out))
	// read the var file and parse out the env vars
	vars := []string{}
	fd, err := os.ReadFile(varFile.Name())
	if err != nil {
		l.Errorf("error reading var file: %v", err)
		return nil, err
	}
	for _, v := range strings.Split(string(fd), "\n") {
		if v != "" {
			vars = append(vars, v)
		}
	}
	l.Debugf("parsed %d variables", len(vars))
	return vars, nil
}
