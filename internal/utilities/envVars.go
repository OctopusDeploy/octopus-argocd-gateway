package utilities

import "os"

func LookupEnvOrDefault(key string, fallback string) string {
	val, ok := os.LookupEnv(key)

	if !ok {
		return fallback
	}

	return val
}
