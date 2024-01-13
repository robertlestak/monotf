package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/robertlestak/monotf/pkg/monotf"
	log "github.com/sirupsen/logrus"
)

var (
	monotfflags = flag.NewFlagSet("monotf", flag.ExitOnError)
	Version     = "dev"
)

func init() {
	ll, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		ll = log.InfoLevel
	}
	log.SetLevel(ll)
}

func usage() {
	fmt.Println("usage: monotf [flags] <command> [args]")
	monotfflags.PrintDefaults()
	fmt.Println("commands:")
	fmt.Println("  sys-init")
	fmt.Println("  server")
	fmt.Println("  terraform")
	fmt.Println("  terraform-speculative-plan")
	fmt.Println("  terraform-plan-apply")
	os.Exit(1)
}

func printVersion() {
	fmt.Printf("monotf %s\n", Version)
	os.Exit(0)
}

func main() {
	l := log.WithFields(log.Fields{
		"app": "monotf",
	})
	l.Debug("starting monotf")
	monotfflags.Usage = usage
	configFile := monotfflags.String("config", "monotf.yaml", "path to config file")
	logLevel := monotfflags.String("log-level", log.GetLevel().String(), "log level")
	workspace := monotfflags.String("w", "", "workspace to use")
	repoDir := monotfflags.String("dir", "", "path to repo directory")
	init := monotfflags.Bool("init", true, "initialize repo")
	serverPort := monotfflags.Int("port", 8080, "port to run server on")
	serverAddr := monotfflags.String("addr", "", "monotf server to use")
	waitTimeout := monotfflags.String("wait", "0s", "timeout for waiting for workspace to be ready. 0 means no timeout")
	vaultEnvAddr := monotfflags.String("vault-addr", "", "vault address")
	vaultEnvNamespace := monotfflags.String("vault-namespace", "", "vault namespace")
	vaultEnvPath := monotfflags.String("vault-path", "", "vault path")
	monotfflags.Parse(os.Args[1:])
	ll, err := log.ParseLevel(*logLevel)
	if err != nil {
		ll = log.InfoLevel
	}
	log.SetLevel(ll)
	var ws *monotf.Workspace
	if len(monotfflags.Args()) == 0 {
		usage()
		os.Exit(1)
	}
	cmd := monotfflags.Args()[0]
	if len(monotfflags.Args()) == 0 {
		l.Errorf("no command provided")
		os.Exit(1)
	}
	if cmd != "server" && cmd != "version" {
		if err := monotf.LoadConfig(*configFile); err != nil {
			l.Errorf("error loading config file %s: %v", *configFile, err)
			os.Exit(1)
		}
		if err := monotf.M.Init(); err != nil {
			l.Errorf("error initializing monotf: %v", err)
			os.Exit(1)
		}
		if repoDir != nil && *repoDir != "" {
			monotf.M.RepoDir = *repoDir
		}
		// if there is no dir, set to current dir
		if monotf.M.RepoDir == "" {
			pwd, err := os.Getwd()
			if err != nil {
				l.Errorf("error getting current dir: %v", err)
				os.Exit(1)
			}
			monotf.M.RepoDir = pwd
		}
		if serverAddr != nil && *serverAddr != "" {
			monotf.M.ServerAddr = *serverAddr
		}
		if vaultEnvAddr != nil && *vaultEnvAddr != "" {
			if monotf.M.VaultEnv == nil {
				monotf.M.VaultEnv = &monotf.VaultEnv{}
			}
			monotf.M.VaultEnv.Addr = *vaultEnvAddr
		}
		if vaultEnvNamespace != nil && *vaultEnvNamespace != "" {
			if monotf.M.VaultEnv == nil {
				monotf.M.VaultEnv = &monotf.VaultEnv{}
			}
			monotf.M.VaultEnv.Namespace = *vaultEnvNamespace
		}
		if vaultEnvPath != nil && *vaultEnvPath != "" {
			if monotf.M.VaultEnv == nil {
				monotf.M.VaultEnv = &monotf.VaultEnv{}
			}
			monotf.M.VaultEnv.Path = *vaultEnvPath
		}
		if *workspace != "" {
			var err error
			ws, err = monotf.M.GetWorkspaceLocal(*workspace)
			if err != nil {
				l.Errorf("error switching workspace: %v", err)
				os.Exit(1)
			}
			pv, err := monotf.M.ParsePathVars(ws.Path)
			if err != nil {
				l.Errorf("error parsing path vars: %v", err)
				os.Exit(1)
			}
			ws.PathVars = pv
			l.Debugf("switched to workspace %s", ws.Name)
			if *init {
				ws.Init = true
			}
		} else {
			l.Errorf("no workspace provided")
			os.Exit(1)
		}
		if monotf.M.VaultEnv != nil && monotf.M.VaultEnv.Path != "" {
			envVars, err := monotf.M.VaultEnv.Get()
			if err != nil {
				l.Errorf("error getting vault env: %v", err)
				os.Exit(1)
			}
			ws.EnvVars = append(ws.EnvVars, envVars...)
		}
		if monotf.M.VarScript != "" {
			if !filepath.IsAbs(monotf.M.VarScript) {
				cwd, err := os.Getwd()
				if err != nil {
					l.Errorf("error getting current dir: %v", err)
					os.Exit(1)
				}
				monotf.M.VarScript = filepath.Join(cwd, monotf.M.VarScript)
			}
			envVars, err := ws.VarsFromScript()
			if err != nil {
				l.Errorf("error getting vars from script: %v", err)
				os.Exit(1)
			}
			ws.EnvVars = append(ws.EnvVars, envVars...)
		}
	}
	switch cmd {
	case "sys-init":
		if err := monotf.SysInit(); err != nil {
			l.Errorf("error running sysinit: %v", err)
			os.Exit(1)
		}
	case "server":
		if err := monotf.Server(*serverPort); err != nil {
			l.Errorf("error running server: %v", err)
			os.Exit(1)
		}
	case "terraform":
		args := monotfflags.Args()[1:]
		_, _, err := ws.LockedTerraform(waitTimeout, args)
		if err != nil {
			l.Errorf("error running terraform: %v", err)
			os.Exit(1)
		}
	case "terraform-speculative-plan":
		_, _, err := ws.LockedTerraformSpeculativePlan(waitTimeout, []string{"plan"})
		if err != nil {
			l.Errorf("error running terraform: %v", err)
			os.Exit(1)
		}
	case "terraform-plan-apply":
		_, _, err := ws.LockedTerraformPlanApply(waitTimeout)
		if err != nil {
			l.Errorf("error running terraform: %v", err)
			os.Exit(1)
		}
	case "version":
		printVersion()
		os.Exit(0)
	default:
		l.Errorf("unknown command %s", cmd)
	}

}
