package config

import "os"

type Config struct {
	HTTPAddr    string
	MQTTAddr    string
	DBPath      string
	DataDir     string
	WorkerToken string
}

func Load() Config {
	return Config{
		HTTPAddr:    env("ESPM_HTTP_ADDR", ":8080"),
		MQTTAddr:    env("ESPM_MQTT_ADDR", ":1883"),
		DBPath:      env("ESPM_DB_PATH", "data/espmanager.db"),
		DataDir:     env("ESPM_DATA_DIR", "data"),
		WorkerToken: env("ESPM_WORKER_TOKEN", ""),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
