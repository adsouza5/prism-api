package config

type Client struct {
	ID          string
	Name        string
	APIKey      string
	RateLimit   int // requests per minute
	UpstreamURL string
}

var Clients = map[string]*Client{
	"demo-web": {
		ID:          "demo-web",
		Name:        "Demo Web App",
		APIKey:      "key_demo_web_123",
		RateLimit:   60,
		UpstreamURL: "https://httpbin.org",
	},
	"mobile-app": {
		ID:          "mobile-app",
		Name:        "Mobile App",
		APIKey:      "key_mobile_456",
		RateLimit:   30,
		UpstreamURL: "https://httpbin.org",
	},
	"data-service": {
		ID:          "data-service",
		Name:        "Data Service",
		APIKey:      "key_data_789",
		RateLimit:   120,
		UpstreamURL: "https://httpbin.org",
	},
}

var JWTSecret = []byte("prism-secret-key-change-in-prod")
