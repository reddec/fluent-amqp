package fluent_flags

// adapter for go-flags
type Writer struct {
	Exchange     string `long:"exchange" env:"EXCHANGE" `
	RoutingKey   string
	Sign         string
}
