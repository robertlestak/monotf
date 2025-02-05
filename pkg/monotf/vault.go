package monotf

import (
	"errors"
	"os"
	"strings"

	"github.com/hashicorp/vault/api"
	log "github.com/sirupsen/logrus"
)

type VaultEnv struct {
	Addr      string `json:"addr" yaml:"addr"`
	Namespace string `json:"namespace" yaml:"namespace"`
	Path      string `json:"path" yaml:"path"`
	Token     string `json:"token" yaml:"token"`
}

func (v *VaultEnv) Get() ([]string, error) {
	l := log.WithFields(log.Fields{
		"app":       "monotf",
		"addr":      v.Addr,
		"namespace": v.Namespace,
		"path":      v.Path,
	})
	l.Debugf("getting vault env")
	config := &api.Config{
		Address: v.Addr,
	}
	client, err := api.NewClient(config)
	if err != nil {
		l.Errorf("error creating vault client: %v", err)
		return nil, err
	}
	if v.Namespace != "" {
		client.SetNamespace(v.Namespace)
	}
	if v.Path == "" {
		l.Errorf("vault path not set")
		return nil, errors.New("vault path not set")
	}
	v.Token = os.ExpandEnv(v.Token)
	if v.Token == "" {
		v.Token = os.Getenv("VAULT_TOKEN")
	}
	if v.Token == "" {
		l.Errorf("vault token not set")
		return nil, errors.New("vault token not set")
	}
	client.SetToken(v.Token)
	var secrets map[string]interface{}
	var envVars []string
	ss := strings.Split(v.Path, "/")
	if len(ss) < 2 {
		return envVars, errors.New("secret path must be in kv/path/to/secret format")
	}
	kv := ss[0]
	kp := strings.Join(ss[1:], "/")
	c := client.Logical()
	secret, err := c.Read(kv + "/data/" + kp)
	if err != nil {
		l.Errorf("error reading secret: %v", err)
		return envVars, err
	}
	if secret == nil || secret.Data == nil {
		return nil, errors.New("secret not found")
	}
	secrets = secret.Data["data"].(map[string]interface{})
	for k, v := range secrets {
		envVars = append(envVars, k+"="+v.(string))
	}
	return envVars, nil
}
