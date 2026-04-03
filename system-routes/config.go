package system

type Config struct {
	uiEndPoint               string `env:"SYSTEM_URI_UI" envDefault:"https://openwifi.wlan.local"`
	serverCertificatePath    string `env:"INTERNAL_RESTAPI_HOST_CERT"`
	websocketCertificatePath string `env:"WEBSOCKET_HOST_CERT"`
}
