package analytics

type TimepointsData struct {
	ID         string     `json:"id"`
	BoardID    string     `json:"boardId"`
	Timestamp  int64      `json:"timestamp"`
	Serial     string     `json:"serialNumber"`
	DeviceInfo DeviceInfo `json:"device_info"`
	SSIDData   []SSIDData `json:"ssid_data"`
}

type DeviceInfo struct {
	Associations2G int     `json:"associations_2g"`
	Associations5G int     `json:"associations_5g"`
	Associations6G int     `json:"associations_6g"`
	BoardID        string  `json:"boardId"`
	Connected      bool    `json:"connected"`
	ConnectionIP   string  `json:"connectionIp"`
	DeviceType     string  `json:"deviceType"`
	Health         int     `json:"health"`
	LastConnection int64   `json:"lastConnection"`
	LastContact    int64   `json:"lastContact"`
	LastDisconn    int64   `json:"lastDisconnection"`
	LastFirmware   string  `json:"lastFirmware"`
	LastFWUpdate   int64   `json:"lastFirmwareUpdate"`
	LastHealth     int64   `json:"lastHealth"`
	LastPing       int64   `json:"lastPing"`
	LastState      int64   `json:"lastState"`
	Locale         string  `json:"locale"`
	Memory         float64 `json:"memory"`
	Pings          int     `json:"pings"`
	SerialNumber   string  `json:"serialNumber"`
	States         int     `json:"states"`
	Type           string  `json:"type"`
	Uptime         int64   `json:"uptime"`
}

type SSIDData struct {
	Associations  []SSIDAssociation `json:"associations"`
	Band          int               `json:"band"`
	BSSID         string            `json:"bssid"`
	Channel       int               `json:"channel"`
	Mode          string            `json:"mode"`
	SSID          string            `json:"ssid"`
	RxBytesBW     *SSIDMetric       `json:"rx_bytes_bw,omitempty"`
	RxPacketsBW   *SSIDMetric       `json:"rx_packets_bw,omitempty"`
	TxBytesBW     *SSIDMetric       `json:"tx_bytes_bw,omitempty"`
	TxDurationPct *SSIDMetric       `json:"tx_duration_pct,omitempty"`
	TxFailedPct   *SSIDMetric       `json:"tx_failed_pct,omitempty"`
	TxPacketsBW   *SSIDMetric       `json:"tx_packets_bw,omitempty"`
	TxRetriesPct  *SSIDMetric       `json:"tx_retries_pct,omitempty"`
}

type SSIDMetric struct {
	Avg float64 `json:"avg"`
	Max float64 `json:"max"`
	Min float64 `json:"min"`
}

type SSIDAssociation struct {
	Connected   int             `json:"connected"`
	Inactive    int             `json:"inactive"`
	RSSI        int             `json:"rssi"`
	Station     string          `json:"station"`
	RxRate      AssociationRate `json:"rx_rate"`
	TxRate      AssociationRate `json:"tx_rate"`
	Fingerprint map[string]any  `json:"fingerprint,omitempty"`
}

type AssociationRate struct {
	Bitrate int `json:"bitrate"`
	Chwidth int `json:"chwidth"`
}

type TimepointRequest struct {
	BoardID    string `json:"boardId"`
	FromDate   uint64 `json:"fromDate,omitempty"`
	EndDate    uint64 `json:"endDate,omitempty"`
	MaxRecords int    `json:"maxRecords,omitempty"`
}
