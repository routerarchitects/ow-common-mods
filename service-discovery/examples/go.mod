module github.com/routerarchitects/service-discovery/examples

go 1.25.1

replace github.com/routerarchitects/ra-common-mods/kafka => ../../../ra-common-mods/kafka

replace github.com/routerarchitects/ra-common-mods/logger => ../../../ra-common-mods/logger

replace github.com/routerarchitects/ra-common-mods/logger-routes => ../../../ra-common-mods/logger-routes

replace github.com/routerarchitects/ow-common-mods/servicediscovery => ../

require (
	github.com/caarlos0/env/v11 v11.3.1
	github.com/gofiber/fiber/v3 v3.0.0-rc.3
	github.com/routerarchitects/ow-common-mods/servicediscovery v0.0.0-00010101000000-000000000000
	github.com/routerarchitects/ra-common-mods/kafka v0.0.0-20251226064524-885b2beb6635
	github.com/routerarchitects/ra-common-mods/logger v0.0.0-20251224120222-b438314c6ae1
	github.com/routerarchitects/ra-common-mods/logger-routes v0.0.0-00010101000000-000000000000
)

require (
	github.com/IBM/sarama v1.43.3 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/eapache/go-resiliency v1.7.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/gofiber/schema v1.6.0 // indirect
	github.com/gofiber/utils/v2 v2.0.0-rc.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/klauspost/compress v1.18.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/tinylib/msgp v1.5.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.68.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	golang.org/x/crypto v0.44.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)
