package bot

import "fmt"

const (
	awgCbPrefix     = "awg:"
	awgNameCbPrefix = "awgn:"
)

func makeAwgClientFromEnv() (*wgEasyClient, error) {
	host := envTrim("AWG_HOST")
	port := envTrim("AWG_PORT")
	username := envTrim("AWG_USERNAME")
	pass := envTrim("AWG_PASSWORD")
	client, err := newWgEasyClient(host, port, username, pass, wgEasyTimeoutFromEnv("AWG_TIMEOUT_SECONDS"))
	if err != nil {
		return nil, fmt.Errorf("Не заданы переменные окружения Amnezia WireGuard. Нужно: AWG_HOST, AWG_PORT, AWG_PASSWORD. Для wg-easy v15 обычно также нужен AWG_USERNAME")
	}
	return client, nil
}
