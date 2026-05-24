package config

func defaultPublicBaseURL(env string) string {
	if env == "production" {
		return "https://abhijitmohanty.com/second-brain"
	}
	return "http://localhost:8080"
}
