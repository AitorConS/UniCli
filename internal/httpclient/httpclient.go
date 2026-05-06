package httpclient

import "net/http"

import "time"

var Default = &http.Client{Timeout: 30 * time.Second}
