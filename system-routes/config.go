package system

type Config struct {
	UiEndPoint               string `env:"SYSTEM_URI_UI" envDefault:"https://openwifi.wlan.local"`
	ServerCertificatePath    string `env:"INTERNAL_RESTAPI_HOST_CERT"`
	WebsocketCertificatePath string `env:"WEBSOCKET_HOST_CERT"`
}
