package subsystemmodules

type Config struct {
	UI_EndPoint string   `env:"UI_ENDPOINT" envDefault:"https://openwifi.wlan.local"`
	CERTS       []string `env:"CERTS" envSeparator:","`
}
