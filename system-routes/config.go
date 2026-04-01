package system

type Config struct {
	UI_EndPoint                string `env:"SYSTEM_URI_UI" envDefault:"https://openwifi.wlan.local"`
	Server_certificate_path    string `env:"INTERNAL_RESTAPI_HOST_CERT"`
	Websocket_certificate_path string `env:"WEBSOCKET_HOST_CERT"`
}
