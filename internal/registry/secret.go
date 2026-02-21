package registry

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// dockerConfig represents the structure of a .dockerconfigjson secret.
type dockerConfig struct {
	Auths map[string]dockerAuth `json:"auths"`
}

type dockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"` // base64(username:password)
}

// ExtractCredentials extracts username and password for a registry host from
// dockerconfigjson secret data. Tries exact host match, then https:// and
// http:// prefixed variants.
func ExtractCredentials(secretData []byte, registryHost string) (string, string, error) {
	var cfg dockerConfig
	if err := json.Unmarshal(secretData, &cfg); err != nil {
		return "", "", fmt.Errorf("parsing dockerconfigjson: %w", err)
	}

	auth, ok := cfg.Auths[registryHost]
	if !ok {
		auth, ok = cfg.Auths["https://"+registryHost]
	}
	if !ok {
		auth, ok = cfg.Auths["http://"+registryHost]
	}
	if !ok {
		return "", "", fmt.Errorf("no credentials found for registry %s", registryHost)
	}

	if auth.Username != "" && auth.Password != "" {
		return auth.Username, auth.Password, nil
	}

	if auth.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
		if err != nil {
			return "", "", fmt.Errorf("decoding auth field: %w", err)
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid auth field format")
		}
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("no username/password or auth field for registry %s", registryHost)
}
