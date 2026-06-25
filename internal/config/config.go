package config

import "os"

type Config struct {
	HTTPAddr      string
	MQTTAddr      string
	DBPath        string
	DataDir       string
	ArtifactsDir  string
	WorkerToken   string
	AdminUser     string
	AdminPassword string
	SignerURL     string
	SignerToken   string
}

func Load() Config {
	return Config{
		HTTPAddr:      env("ESPM_HTTP_ADDR", ":8080"),
		MQTTAddr:      env("ESPM_MQTT_ADDR", ":1883"),
		DBPath:        env("ESPM_DB_PATH", "data/espmanager.db"),
		DataDir:       env("ESPM_DATA_DIR", "data"),
		ArtifactsDir:  env("ESPM_ARTIFACTS_DIR", "data/artifacts"),
		WorkerToken:   env("ESPM_WORKER_TOKEN", ""),
		AdminUser:     env("ESPM_ADMIN_USER", "admin"),
		AdminPassword: env("ESPM_ADMIN_PASSWORD", ""),
		SignerURL:     env("ESPM_SIGNER_URL", "http://localhost:8090"),
		SignerToken:   env("ESPM_SIGNER_TOKEN", ""),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
