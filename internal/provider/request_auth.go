package provider

type RequestAuth struct {
	APIKey      string
	BearerToken string
	AccountID   string
	Headers     map[string]string
}

