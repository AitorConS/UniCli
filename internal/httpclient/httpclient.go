package httpclient

import (
	"net/http"
	"time"
)

var Default = &http.Client{Timeout: 30 * time.Second}
